package v2pipeline

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"localize-agent/workflow/internal/contracts"
	"localize-agent/workflow/pkg/platform"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// Compile-time interface check.
var _ contracts.V2PipelineStore = (*Store)(nil)

//go:embed postgres_v2_schema.sql
var postgresV2Schema string

// sqliteV2Schema is the SQLite-compatible DDL (no JSONB, no TIMESTAMPTZ, no NOW()).
const sqliteV2Schema = `
CREATE TABLE IF NOT EXISTS pipeline_items_v2 (
    id TEXT PRIMARY KEY,
    sort_index INTEGER NOT NULL DEFAULT 0,
    source_file TEXT NOT NULL DEFAULT '',
    knot TEXT NOT NULL DEFAULT '',
    content_type TEXT NOT NULL DEFAULT '',
    speaker TEXT NOT NULL DEFAULT '',
    choice TEXT NOT NULL DEFAULT '',
    gate TEXT NOT NULL DEFAULT '',
    source_raw TEXT NOT NULL,
    source_hash TEXT NOT NULL UNIQUE,
    has_tags INTEGER NOT NULL DEFAULT 0,
    state TEXT NOT NULL,
    ko_raw TEXT,
    ko_formatted TEXT,
    translate_attempts INTEGER NOT NULL DEFAULT 0,
    format_attempts INTEGER NOT NULL DEFAULT 0,
    score_attempts INTEGER NOT NULL DEFAULT 0,
    score_final REAL NOT NULL DEFAULT -1,
    failure_type TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    attempt_log TEXT,
    claimed_by TEXT NOT NULL DEFAULT '',
    claimed_at TEXT,
    lease_until TEXT,
    batch_id TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_pv2_state ON pipeline_items_v2(state);
CREATE INDEX IF NOT EXISTS idx_pv2_state_lease ON pipeline_items_v2(state, lease_until);
CREATE INDEX IF NOT EXISTS idx_pv2_source_hash ON pipeline_items_v2(source_hash);
CREATE INDEX IF NOT EXISTS idx_pv2_batch ON pipeline_items_v2(batch_id);
`

// Store implements contracts.V2PipelineStore for both PostgreSQL and SQLite backends.
type Store struct {
	db      *sql.DB
	backend string
}

// OpenStore opens a v2 pipeline store with the specified backend.
func OpenStore(backend string, dbPath string, dsn string) (*Store, error) {
	normalizedBackend, err := platform.NormalizeDBBackend(backend)
	if err != nil {
		return nil, err
	}
	switch normalizedBackend {
	case platform.DBBackendSQLite:
		return openSQLiteStore(dbPath)
	case platform.DBBackendPostgres:
		return openPostgresStore(dsn)
	default:
		return nil, fmt.Errorf("unsupported db backend: %s", normalizedBackend)
	}
}

// Open opens a SQLite-backed store (convenience for tests).
func Open(dbPath string) (*Store, error) {
	return OpenStore(platform.DBBackendSQLite, dbPath, "")
}

func openSQLiteStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(sqliteV2Schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init sqlite schema: %w", err)
	}
	return &Store{db: db, backend: platform.DBBackendSQLite}, nil
}

func openPostgresStore(dsn string) (*Store, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("postgres dsn required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(16)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	if _, err := db.Exec(postgresV2Schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init postgres schema: %w", err)
	}
	return &Store{db: db, backend: platform.DBBackendPostgres}, nil
}

// Close releases database resources.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Seed inserts items into pipeline_items_v2, deduplicating by source_hash via ON CONFLICT DO NOTHING.
// Design note: items with different IDs but the same source_hash are silently skipped (INFRA-02).
// The pipeline references the first-inserted ID for a given source_hash, which may originate from
// a different knot/gate context. This is intentional — identical source text gets one translation.
func (s *Store) Seed(items []contracts.V2PipelineItem) (int, int, error) {
	if len(items) == 0 {
		return 0, 0, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}

	inserted := 0
	skipped := 0

	for _, item := range items {
		now := s.nowValue()
		hasTags := s.boolValue(item.HasTags)

		var result sql.Result
		if s.backend == platform.DBBackendPostgres {
			result, err = tx.Exec(`
				INSERT INTO pipeline_items_v2 (id, sort_index, source_file, knot, content_type, speaker, choice, gate, source_raw, source_hash, has_tags, state, ko_raw, ko_formatted, translate_attempts, format_attempts, score_attempts, score_final, failure_type, last_error, attempt_log, claimed_by, batch_id, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24)
				ON CONFLICT (source_hash) DO NOTHING`,
				item.ID, item.SortIndex, item.SourceFile, item.Knot, item.ContentType,
				item.Speaker, item.Choice, item.Gate,
				item.SourceRaw, item.SourceHash, hasTags, item.State,
				nullableText(item.KORaw), nullableText(item.KOFormatted),
				item.TranslateAttempts, item.FormatAttempts, item.ScoreAttempts,
				item.ScoreFinal, item.FailureType, item.LastError, nil,
				item.ClaimedBy, item.BatchID, now,
			)
		} else {
			result, err = tx.Exec(`
				INSERT INTO pipeline_items_v2 (id, sort_index, source_file, knot, content_type, speaker, choice, gate, source_raw, source_hash, has_tags, state, ko_raw, ko_formatted, translate_attempts, format_attempts, score_attempts, score_final, failure_type, last_error, attempt_log, claimed_by, batch_id, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT (source_hash) DO NOTHING`,
				item.ID, item.SortIndex, item.SourceFile, item.Knot, item.ContentType,
				item.Speaker, item.Choice, item.Gate,
				item.SourceRaw, item.SourceHash, hasTags, item.State,
				nullableText(item.KORaw), nullableText(item.KOFormatted),
				item.TranslateAttempts, item.FormatAttempts, item.ScoreAttempts,
				item.ScoreFinal, item.FailureType, item.LastError, nil,
				item.ClaimedBy, item.BatchID, now,
			)
		}
		if err != nil {
			tx.Rollback()
			return 0, 0, fmt.Errorf("seed item %s: %w", item.ID, err)
		}
		affected, _ := result.RowsAffected()
		if affected > 0 {
			inserted++
		} else {
			skipped++
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return inserted, skipped, nil
}

// ClaimPending atomically claims items from pendingState to workingState with a lease.
func (s *Store) ClaimPending(pendingState, workingState, workerID string, batchSize int, leaseSec int) ([]contracts.V2PipelineItem, error) {
	if workerID == "" {
		return nil, fmt.Errorf("workerID required")
	}
	if batchSize <= 0 {
		return nil, nil
	}

	now := time.Now().UTC()
	leaseUntil := now.Add(time.Duration(leaseSec) * time.Second)
	nowVal := s.timeValue(now)
	leaseVal := s.timeValue(leaseUntil)

	var rows *sql.Rows
	var err error

	if s.backend == platform.DBBackendPostgres {
		rows, err = s.db.Query(fmt.Sprintf(`
			WITH picked AS (
				SELECT id FROM pipeline_items_v2
				WHERE state = $1
				  AND (claimed_by = '' OR lease_until IS NULL OR lease_until < $2)
				ORDER BY batch_id, sort_index, id
				LIMIT %d
				FOR UPDATE SKIP LOCKED
			)
			UPDATE pipeline_items_v2
			SET state = $3, claimed_by = $4, claimed_at = $5, lease_until = $6, updated_at = $7
			WHERE id IN (SELECT id FROM picked)
			RETURNING id, sort_index, source_file, knot, content_type, speaker, choice, gate, source_raw, source_hash, has_tags, state, ko_raw, ko_formatted, translate_attempts, format_attempts, score_attempts, score_final, failure_type, last_error, attempt_log, claimed_by, batch_id`, batchSize),
			pendingState, nowVal, workingState, workerID, nowVal, leaseVal, nowVal,
		)
	} else {
		// SQLite: no CTE UPDATE RETURNING, use two-step approach
		// First select IDs to claim
		idRows, err2 := s.db.Query(fmt.Sprintf(`
			SELECT id FROM pipeline_items_v2
			WHERE state = ?
			  AND (claimed_by = '' OR lease_until IS NULL OR lease_until < ?)
			ORDER BY batch_id, sort_index, id
			LIMIT %d`, batchSize),
			pendingState, nowVal,
		)
		if err2 != nil {
			return nil, err2
		}
		var ids []string
		for idRows.Next() {
			var id string
			if err := idRows.Scan(&id); err != nil {
				idRows.Close()
				return nil, err
			}
			ids = append(ids, id)
		}
		idRows.Close()
		if len(ids) == 0 {
			return nil, nil
		}

		// Update claimed items
		placeholders := make([]string, len(ids))
		args := make([]interface{}, 0, len(ids)+5)
		args = append(args, workingState, workerID, nowVal, leaseVal, nowVal)
		for i, id := range ids {
			placeholders[i] = "?"
			args = append(args, id)
		}
		_, err2 = s.db.Exec(fmt.Sprintf(`
			UPDATE pipeline_items_v2
			SET state = ?, claimed_by = ?, claimed_at = ?, lease_until = ?, updated_at = ?
			WHERE id IN (%s)`, strings.Join(placeholders, ",")),
			args...,
		)
		if err2 != nil {
			return nil, err2
		}

		// Re-read claimed items
		readPlaceholders := make([]string, len(ids))
		readArgs := make([]interface{}, len(ids))
		for i, id := range ids {
			readPlaceholders[i] = "?"
			readArgs[i] = id
		}
		rows, err = s.db.Query(fmt.Sprintf(`
			SELECT id, sort_index, source_file, knot, content_type, speaker, choice, gate, source_raw, source_hash, has_tags, state, ko_raw, ko_formatted, translate_attempts, format_attempts, score_attempts, score_final, failure_type, last_error, attempt_log, claimed_by, batch_id
			FROM pipeline_items_v2
			WHERE id IN (%s)
			ORDER BY batch_id, sort_index, id`, strings.Join(readPlaceholders, ",")),
			readArgs...,
		)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var claimed []contracts.V2PipelineItem
	for rows.Next() {
		item, err := s.scanItem(rows)
		if err != nil {
			return nil, err
		}
		claimed = append(claimed, item)
	}
	return claimed, rows.Err()
}

// ClaimBatch claims all pending items belonging to the next available batch_id.
// Instead of claiming N arbitrary items, this finds the first unclaimed batch_id
// and claims all its pending items at once. Returns the batch_id and items.
// This ensures 1 claim = 1 batch = 1 LLM call with no re-grouping needed.
func (s *Store) ClaimBatch(pendingState, workingState, workerID string, leaseSec int) (string, []contracts.V2PipelineItem, error) {
	if workerID == "" {
		return "", nil, fmt.Errorf("workerID required")
	}

	now := time.Now().UTC()
	leaseUntil := now.Add(time.Duration(leaseSec) * time.Second)
	nowVal := s.timeValue(now)
	leaseVal := s.timeValue(leaseUntil)

	// Step 1: Find the next unclaimed batch_id.
	var batchID string
	err := s.db.QueryRow(s.rebind(`
		SELECT batch_id FROM pipeline_items_v2
		WHERE state = ? AND (claimed_by = '' OR lease_until IS NULL OR lease_until < ?)
		ORDER BY batch_id, sort_index
		LIMIT 1`),
		pendingState, nowVal,
	).Scan(&batchID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil, nil
		}
		return "", nil, err
	}

	// Step 2: Claim all items with this batch_id.
	var rows *sql.Rows
	if s.backend == platform.DBBackendPostgres {
		rows, err = s.db.Query(`
			WITH picked AS (
				SELECT id FROM pipeline_items_v2
				WHERE state = $1 AND batch_id = $2
				  AND (claimed_by = '' OR lease_until IS NULL OR lease_until < $3)
				FOR UPDATE SKIP LOCKED
			)
			UPDATE pipeline_items_v2
			SET state = $4, claimed_by = $5, claimed_at = $6, lease_until = $7, updated_at = $8
			WHERE id IN (SELECT id FROM picked)
			RETURNING id, sort_index, source_file, knot, content_type, speaker, choice, gate, source_raw, source_hash, has_tags, state, ko_raw, ko_formatted, translate_attempts, format_attempts, score_attempts, score_final, failure_type, last_error, attempt_log, claimed_by, batch_id`,
			pendingState, batchID, nowVal, workingState, workerID, nowVal, leaseVal, nowVal,
		)
	} else {
		// SQLite fallback
		_, err = s.db.Exec(s.rebind(`
			UPDATE pipeline_items_v2
			SET state = ?, claimed_by = ?, claimed_at = ?, lease_until = ?, updated_at = ?
			WHERE state = ? AND batch_id = ?
			  AND (claimed_by = '' OR lease_until IS NULL OR lease_until < ?)`),
			workingState, workerID, nowVal, leaseVal, nowVal, pendingState, batchID, nowVal,
		)
		if err != nil {
			return "", nil, err
		}
		rows, err = s.db.Query(s.rebind(`
			SELECT id, sort_index, source_file, knot, content_type, speaker, choice, gate, source_raw, source_hash, has_tags, state, ko_raw, ko_formatted, translate_attempts, format_attempts, score_attempts, score_final, failure_type, last_error, attempt_log, claimed_by, batch_id
			FROM pipeline_items_v2
			WHERE batch_id = ? AND claimed_by = ?
			ORDER BY sort_index`),
			batchID, workerID,
		)
	}
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()

	var items []contracts.V2PipelineItem
	for rows.Next() {
		item, err := s.scanItem(rows)
		if err != nil {
			return "", nil, err
		}
		items = append(items, item)
	}
	return batchID, items, rows.Err()
}

// MarkState sets an item to a new state, clearing claim fields.
func (s *Store) MarkState(id, newState string) error {
	now := s.nowValue()
	_, err := s.db.Exec(s.rebind(`
		UPDATE pipeline_items_v2
		SET state = ?, claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ?
		WHERE id = ?`),
		newState, now, id,
	)
	return err
}

// MarkTranslated sets ko_raw and routes based on has_tags:
// has_tags=true -> pending_format, has_tags=false -> pending_score.
func (s *Store) MarkTranslated(id, koRaw string) error {
	now := s.nowValue()
	_, err := s.db.Exec(s.rebind(`
		UPDATE pipeline_items_v2
		SET ko_raw = ?,
		    state = CASE WHEN has_tags THEN ? ELSE ? END,
		    translate_attempts = translate_attempts + 1,
		    claimed_by = '', claimed_at = NULL, lease_until = NULL,
		    updated_at = ?
		WHERE id = ?`),
		koRaw, StatePendingFormat, StatePendingScore, now, id,
	)
	return err
}

// MarkFormatted sets ko_formatted and advances to pending_score.
func (s *Store) MarkFormatted(id, koFormatted string) error {
	now := s.nowValue()
	_, err := s.db.Exec(s.rebind(`
		UPDATE pipeline_items_v2
		SET ko_formatted = ?, state = ?, format_attempts = format_attempts + 1,
		    claimed_by = '', claimed_at = NULL, lease_until = NULL,
		    updated_at = ?
		WHERE id = ?`),
		koFormatted, StatePendingScore, now, id,
	)
	return err
}

// MarkScored applies score result and routes by failure_type per D-14:
// "pass"->done, "translation"->pending_translate, "format"->pending_format, "both"->pending_translate.
// If has_tags=false, "format" is rerouted to "pending_translate" since format stage
// cannot process items without tags (score LLM sometimes misjudges).
func (s *Store) MarkScored(id string, scoreFinal float64, failureType, reason string) error {
	// Check has_tags to prevent routing tagless items to format stage.
	if failureType == "format" {
		var hasTags bool
		err := s.db.QueryRow(s.rebind(`SELECT has_tags FROM pipeline_items_v2 WHERE id = ?`), id).Scan(&hasTags)
		if err == nil && !hasTags {
			failureType = "translation" // re-translate instead of format
		}
	}

	var newState string
	switch failureType {
	case "pass":
		newState = StateDone
	case "translation", "both":
		newState = StatePendingTranslate
	case "format":
		newState = StatePendingFormat
	default:
		newState = StateFailed
	}

	now := s.nowValue()
	_, err := s.db.Exec(s.rebind(`
		UPDATE pipeline_items_v2
		SET score_final = ?, failure_type = ?, last_error = ?,
		    state = ?, score_attempts = score_attempts + 1,
		    claimed_by = '', claimed_at = NULL, lease_until = NULL,
		    updated_at = ?
		WHERE id = ?`),
		scoreFinal, failureType, reason, newState, now, id,
	)
	return err
}

// MarkFailed sets state to failed with an error message.
func (s *Store) MarkFailed(id, lastError string) error {
	now := s.nowValue()
	_, err := s.db.Exec(s.rebind(`
		UPDATE pipeline_items_v2
		SET state = ?, last_error = ?,
		    claimed_by = '', claimed_at = NULL, lease_until = NULL,
		    updated_at = ?
		WHERE id = ?`),
		StateFailed, lastError, now, id,
	)
	return err
}

// AppendAttemptLog appends a JSON object to the attempt_log array.
func (s *Store) AppendAttemptLog(id string, entry map[string]interface{}) error {
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal attempt log entry: %w", err)
	}

	now := s.nowValue()
	if s.backend == platform.DBBackendPostgres {
		_, err = s.db.Exec(`
			UPDATE pipeline_items_v2
			SET attempt_log = COALESCE(attempt_log, '[]'::jsonb) || $1::jsonb,
			    updated_at = $2
			WHERE id = $3`,
			string(entryJSON), now, id,
		)
	} else {
		// SQLite: manipulate as TEXT JSON array
		_, err = s.db.Exec(`
			UPDATE pipeline_items_v2
			SET attempt_log = CASE
				WHEN attempt_log IS NULL OR attempt_log = '' THEN '[' || ? || ']'
				ELSE SUBSTR(attempt_log, 1, LENGTH(attempt_log)-1) || ',' || ? || ']'
			END,
			updated_at = ?
			WHERE id = ?`,
			string(entryJSON), string(entryJSON), now, id,
		)
	}
	return err
}

// UpdateRetryState sets state to targetState, increments the specified attempts field, clears claim.
func (s *Store) UpdateRetryState(id, targetState string, incrementField string) error {
	// Validate incrementField to prevent SQL injection
	validFields := map[string]bool{
		"translate_attempts": true,
		"format_attempts":    true,
		"score_attempts":     true,
	}
	if !validFields[incrementField] {
		return fmt.Errorf("invalid increment field: %s", incrementField)
	}

	now := s.nowValue()
	_, err := s.db.Exec(s.rebind(fmt.Sprintf(`
		UPDATE pipeline_items_v2
		SET state = ?, %s = %s + 1,
		    claimed_by = '', claimed_at = NULL, lease_until = NULL,
		    updated_at = ?
		WHERE id = ?`, incrementField, incrementField)),
		targetState, now, id,
	)
	return err
}

// CleanupStaleClaims reclaims items stuck in working_* states past their lease.
// Resets working_translate -> pending_translate, working_format -> pending_format,
// working_score -> pending_score.
func (s *Store) CleanupStaleClaims(olderThanSec int) (int64, error) {
	now := s.nowValue()
	var total int64

	staleMap := map[string]string{
		StateWorkingTranslate: StatePendingTranslate,
		StateWorkingFormat:    StatePendingFormat,
		StateWorkingScore:     StatePendingScore,
	}

	for workingState, pendingState := range staleMap {
		var result sql.Result
		var err error
		if s.backend == platform.DBBackendPostgres {
			result, err = s.db.Exec(`
				UPDATE pipeline_items_v2
				SET state = $1, claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = $2
				WHERE state = $3 AND lease_until IS NOT NULL AND lease_until < NOW() - ($4 || ' seconds')::interval`,
				pendingState, now, workingState, fmt.Sprintf("%d", olderThanSec),
			)
		} else {
			cutoff := time.Now().UTC().Add(-time.Duration(olderThanSec) * time.Second).Format(time.RFC3339)
			result, err = s.db.Exec(`
				UPDATE pipeline_items_v2
				SET state = ?, claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ?
				WHERE state = ? AND lease_until IS NOT NULL AND lease_until < ?`,
				pendingState, now, workingState, cutoff,
			)
		}
		if err != nil {
			return total, err
		}
		affected, _ := result.RowsAffected()
		total += affected
	}

	return total, nil
}

// CountByState returns counts of items grouped by state.
func (s *Store) CountByState() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT state, COUNT(*) FROM pipeline_items_v2 GROUP BY state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var state string
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			return nil, err
		}
		counts[state] = count
	}
	return counts, rows.Err()
}

// GetItem retrieves a single pipeline item by ID.
func (s *Store) GetItem(id string) (*contracts.V2PipelineItem, error) {
	row := s.db.QueryRow(s.rebind(`
		SELECT id, sort_index, source_file, knot, content_type, speaker, choice, gate, source_raw, source_hash, has_tags, state, ko_raw, ko_formatted, translate_attempts, format_attempts, score_attempts, score_final, failure_type, last_error, attempt_log, claimed_by, batch_id
		FROM pipeline_items_v2
		WHERE id = ?`),
		id,
	)

	item, err := s.scanItemRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

// QueryDone returns all items in state=done, ordered by sort_index.
func (s *Store) QueryDone() ([]contracts.V2PipelineItem, error) {
	rows, err := s.db.Query(s.rebind(`
		SELECT id, sort_index, source_file, knot, content_type, speaker, choice, gate,
		       source_raw, source_hash, has_tags, state, ko_raw, ko_formatted,
		       translate_attempts, format_attempts, score_attempts, score_final,
		       failure_type, last_error, attempt_log, claimed_by, batch_id
		FROM pipeline_items_v2
		WHERE state = ?
		ORDER BY sort_index`),
		contracts.StateDone,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []contracts.V2PipelineItem
	for rows.Next() {
		item, err := s.scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if items == nil {
		items = []contracts.V2PipelineItem{}
	}
	return items, rows.Err()
}

// scanItem scans a row from sql.Rows into a V2PipelineItem.
func (s *Store) scanItem(rows *sql.Rows) (contracts.V2PipelineItem, error) {
	var item contracts.V2PipelineItem
	var koRaw, koFormatted, attemptLog sql.NullString
	var hasTags interface{}

	err := rows.Scan(
		&item.ID, &item.SortIndex, &item.SourceFile, &item.Knot, &item.ContentType,
		&item.Speaker, &item.Choice, &item.Gate,
		&item.SourceRaw, &item.SourceHash, &hasTags, &item.State,
		&koRaw, &koFormatted,
		&item.TranslateAttempts, &item.FormatAttempts, &item.ScoreAttempts,
		&item.ScoreFinal, &item.FailureType, &item.LastError,
		&attemptLog, &item.ClaimedBy, &item.BatchID,
	)
	if err != nil {
		return item, err
	}
	item.HasTags = parseBool(hasTags)
	if koRaw.Valid {
		item.KORaw = koRaw.String
	}
	if koFormatted.Valid {
		item.KOFormatted = koFormatted.String
	}
	if attemptLog.Valid {
		item.AttemptLog = attemptLog.String
	}
	return item, nil
}

// scanItemRow scans a single row from sql.Row into a V2PipelineItem.
func (s *Store) scanItemRow(row *sql.Row) (contracts.V2PipelineItem, error) {
	var item contracts.V2PipelineItem
	var koRaw, koFormatted, attemptLog sql.NullString
	var hasTags interface{}

	err := row.Scan(
		&item.ID, &item.SortIndex, &item.SourceFile, &item.Knot, &item.ContentType,
		&item.Speaker, &item.Choice, &item.Gate,
		&item.SourceRaw, &item.SourceHash, &hasTags, &item.State,
		&koRaw, &koFormatted,
		&item.TranslateAttempts, &item.FormatAttempts, &item.ScoreAttempts,
		&item.ScoreFinal, &item.FailureType, &item.LastError,
		&attemptLog, &item.ClaimedBy, &item.BatchID,
	)
	if err != nil {
		return item, err
	}
	item.HasTags = parseBool(hasTags)
	if koRaw.Valid {
		item.KORaw = koRaw.String
	}
	if koFormatted.Valid {
		item.KOFormatted = koFormatted.String
	}
	if attemptLog.Valid {
		item.AttemptLog = attemptLog.String
	}
	return item, nil
}

// rebind converts ? placeholders to $N for PostgreSQL.
func (s *Store) rebind(query string) string {
	if s.backend != platform.DBBackendPostgres {
		return query
	}
	var out strings.Builder
	out.Grow(len(query) + 8)
	argIndex := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			out.WriteString(fmt.Sprintf("$%d", argIndex))
			argIndex++
		} else {
			out.WriteByte(query[i])
		}
	}
	return out.String()
}

// nowValue returns the current time in the appropriate format for the backend.
func (s *Store) nowValue() interface{} {
	now := time.Now().UTC()
	if s.backend == platform.DBBackendPostgres {
		return now
	}
	return now.Format(time.RFC3339)
}

// timeValue converts a time.Time to the appropriate format for the backend.
func (s *Store) timeValue(t time.Time) interface{} {
	if s.backend == platform.DBBackendPostgres {
		return t.UTC()
	}
	return t.UTC().Format(time.RFC3339)
}

// boolValue converts a bool to the appropriate format for the backend.
func (s *Store) boolValue(b bool) interface{} {
	if s.backend == platform.DBBackendPostgres {
		return b
	}
	if b {
		return 1
	}
	return 0
}

// parseBool handles bool values from both SQLite (int) and PostgreSQL (bool).
func parseBool(v interface{}) bool {
	switch x := v.(type) {
	case bool:
		return x
	case int64:
		return x != 0
	case int:
		return x != 0
	case float64:
		return x != 0
	default:
		return false
	}
}

// MarkDonePassthrough sets state=done with ko_formatted=source text for punctuation-only blocks.
func (s *Store) MarkDonePassthrough(id, koFormatted string) error {
	now := s.nowValue()
	_, err := s.db.Exec(s.rebind(`
		UPDATE pipeline_items_v2
		SET state = ?, ko_raw = ?, ko_formatted = ?,
		    claimed_by = '', claimed_at = NULL, lease_until = NULL,
		    updated_at = ?
		WHERE id = ?`),
		StateDone, koFormatted, koFormatted, now, id,
	)
	return err
}

// GetPrevGateLines returns the last N source_raw texts from the previous gate
// in the same knot, ordered by sort_index descending. Used for D-03 context injection.
func (s *Store) GetPrevGateLines(knot, currentGate string, limit int) ([]string, error) {
	if knot == "" || currentGate == "" {
		return nil, nil
	}

	rows, err := s.db.Query(s.rebind(`
		SELECT source_raw FROM pipeline_items_v2
		WHERE knot = ? AND gate != ? AND gate != ''
		  AND sort_index < (SELECT MIN(sort_index) FROM pipeline_items_v2 WHERE knot = ? AND gate = ?)
		ORDER BY sort_index DESC
		LIMIT ?`),
		knot, currentGate, knot, currentGate, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lines []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	// Reverse so lines are in chronological order (oldest first).
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return lines, rows.Err()
}

// nullableText converts empty strings to NULL for ko_raw/ko_formatted.
func nullableText(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
