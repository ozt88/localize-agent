package translationpipeline

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"localize-agent/workflow/pkg/platform"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type Store struct {
	db      *sql.DB
	backend string
}

type StaleClaimCleanupSummary struct {
	Translate   int64
	Score       int64
	Retranslate int64
}

type ScoreRecoverySummary struct {
	ToPendingRetranslate int64
	ToPendingTranslate   int64
	ToFailed             int64
}

type FailedNoRowRoutingSummary struct {
	OpenQuoteOther   int64
	ActionOpenQuote  int64
	StatLikeOpenQuote int64
	LongDialogue     int64
	Expository       int64
	Passthrough      int64
	CurrentRescue    int64
	Total            int64
}

type BlockedTranslateRepairSummary struct {
	Released int64
	StillBlocked int64
}

type PreserveApplySummary struct {
	Applied int64
}

const defaultKeepMargin = 3.0

type checkpointTexts struct {
	currentKO string
	freshKO   string
}

type scoreDecision struct {
	Winner           string
	Decision         string
	SelectedKO       string
	CurrentScore     float64
	FreshScore       float64
	ScoreFinal       float64
	ScoreDelta       float64
	RewriteTriggered bool
}

//go:embed postgres_pipeline_schema.sql
var postgresPipelineSchema string

func Open(dbPath string) (*Store, error) {
	return OpenStore(platform.DBBackendSQLite, dbPath, "")
}

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

func openSQLiteStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	for _, pragma := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=FULL`,
		`PRAGMA busy_timeout=5000`,
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, err
		}
	}
	if err := initSQLiteSchema(db); err != nil {
		db.Close()
		return nil, err
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
		return nil, err
	}
	if err := initPostgresSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, backend: platform.DBBackendPostgres}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func initSQLiteSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS pipeline_items (
			id TEXT PRIMARY KEY,
			sort_index INTEGER NOT NULL DEFAULT 0,
			state TEXT NOT NULL,
			retry_count INTEGER NOT NULL DEFAULT 0,
			score_final REAL NOT NULL DEFAULT -1,
			last_error TEXT NOT NULL DEFAULT '',
			claimed_by TEXT NOT NULL DEFAULT '',
			claimed_at TEXT,
			lease_until TEXT,
			updated_at TEXT NOT NULL
		)
	`)
	if err != nil {
		return err
	}
	for _, ddl := range []string{
		`ALTER TABLE pipeline_items ADD COLUMN sort_index INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE pipeline_items ADD COLUMN claimed_by TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE pipeline_items ADD COLUMN claimed_at TEXT`,
		`ALTER TABLE pipeline_items ADD COLUMN lease_until TEXT`,
	} {
		if _, err := db.Exec(ddl); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			return err
		}
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_pipeline_items_state ON pipeline_items(state)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_pipeline_items_state_lease ON pipeline_items(state, lease_until)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS pipeline_worker_stats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			worker_id TEXT NOT NULL,
			role TEXT NOT NULL,
			processed_count INTEGER NOT NULL,
			elapsed_ms INTEGER NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL
		)
	`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_pipeline_worker_stats_role_finished ON pipeline_worker_stats(role, finished_at DESC)`)
	return err
}

func initPostgresSchema(db *sql.DB) error {
	_, err := db.Exec(postgresPipelineSchema)
	return err
}

func (s *Store) rebind(query string) string {
	if s == nil {
		return query
	}
	return rebindForBackend(s.backend, query)
}

func rebindForBackend(backend string, query string) string {
	if backend != platform.DBBackendPostgres {
		return query
	}
	var out strings.Builder
	out.Grow(len(query) + 8)
	argIndex := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			out.WriteString(fmt.Sprintf("$%d", argIndex))
			argIndex++
			continue
		}
		out.WriteByte(query[i])
	}
	return out.String()
}

func (s *Store) dbTimeValue(t time.Time) any {
	return dbTimeValueForBackend(s.backend, t)
}

func dbTimeValueForBackend(backend string, t time.Time) any {
	if backend == platform.DBBackendPostgres {
		return t.UTC()
	}
	return t.UTC().Format(time.RFC3339)
}

func normalizeNullableText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []byte:
		return string(x)
	case time.Time:
		return x.UTC().Format(time.RFC3339)
	default:
		return fmt.Sprint(x)
	}
}

func scanNullableString(rows interface{ Scan(...any) error }, target *string) error {
	var raw any
	if err := rows.Scan(&raw); err != nil {
		return err
	}
	*target = normalizeNullableText(raw)
	return nil
}

func scanPipelineItem(scanner interface{ Scan(...any) error }, it *PipelineItem) error {
	var claimedAt any
	var leaseUntil any
	if err := scanner.Scan(&it.ID, &it.SortIndex, &it.State, &it.RetryCount, &it.ScoreFinal, &it.LastError, &it.ClaimedBy, &claimedAt, &leaseUntil); err != nil {
		return err
	}
	it.ClaimedAt = normalizeNullableText(claimedAt)
	it.LeaseUntil = normalizeNullableText(leaseUntil)
	return nil
}

func scanWorkerBatchStat(scanner interface{ Scan(...any) error }, it *WorkerBatchStat) error {
	var startedAt any
	var finishedAt any
	if err := scanner.Scan(&it.ID, &it.WorkerID, &it.Role, &it.ProcessedCount, &it.ElapsedMs, &startedAt, &finishedAt); err != nil {
		return err
	}
	it.StartedAt = normalizeNullableText(startedAt)
	it.FinishedAt = normalizeNullableText(finishedAt)
	return nil
}

func (s *Store) Seed(ids []string, limit int) error {
	if limit > 0 && limit < len(ids) {
		ids = ids[:limit]
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	nowTime := time.Now().UTC()
	now := s.dbTimeValue(nowTime)
	stmt, err := tx.Prepare(s.rebind(`
		INSERT INTO pipeline_items(id, sort_index, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at)
		VALUES(?, ?, ?, 0, -1, '', '', NULL, NULL, ?)
		ON CONFLICT(id) DO UPDATE SET sort_index = excluded.sort_index
	`))
	if err != nil {
		return err
	}
	defer stmt.Close()

	for idx, id := range ids {
		state := StatePendingTranslate
		prevLineID, err := checkpointPrevLineID(s.backend, tx, id)
		if err != nil {
			return err
		}
		if isDone, err := checkpointRowDone(s.backend, tx, id); err != nil {
			return err
		} else if isDone {
			nextReady, err := checkpointNextReady(s.backend, tx, id)
			if err != nil {
				return err
			}
			if nextReady {
				state = StatePendingScore
			} else {
				state = StateBlockedScore
			}
		} else if strings.TrimSpace(prevLineID) != "" {
			if prevDone, err := checkpointRowDone(s.backend, tx, prevLineID); err != nil {
				return err
			} else if !prevDone {
				state = StateBlockedTranslate
			}
		}
		if _, err := stmt.Exec(id, idx, state, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Reset() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM pipeline_items`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM pipeline_worker_stats`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ResetScoringState() (int64, error) {
	nowTime := time.Now().UTC()
	now := s.dbTimeValue(nowTime)
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT id FROM items WHERE status = 'done'`)
	if err != nil {
		return 0, err
	}
	doneIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		doneIDs = append(doneIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	var updated int64
	for _, id := range doneIDs {
		state := StateBlockedScore
		nextReady, err := checkpointNextReady(s.backend, tx, id)
		if err != nil {
			return 0, err
		}
		if nextReady {
			state = StatePendingScore
		}
		res, err := tx.Exec(
			s.rebind(`UPDATE pipeline_items
			 SET state = ?, retry_count = 0, score_final = -1, last_error = '', claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ?
			 WHERE id = ?`),
			state, now, id,
		)
		if err != nil {
			return 0, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, err
		}
		updated += n
	}
	if _, err := tx.Exec(`DELETE FROM pipeline_items WHERE id NOT IN (SELECT id FROM items WHERE status = 'done')`); err != nil {
		return 0, err
	}
	for _, id := range doneIDs {
		state := StateBlockedScore
		nextReady, err := checkpointNextReady(s.backend, tx, id)
		if err != nil {
			return 0, err
		}
		if nextReady {
			state = StatePendingScore
		}
		if _, err := tx.Exec(
			s.rebind(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at)
			 SELECT ?, ?, 0, -1, '', '', NULL, NULL, ?
			 WHERE NOT EXISTS (SELECT 1 FROM pipeline_items WHERE id = ?)`),
			id, state, now, id,
		); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return updated, nil
}

func (s *Store) RequeueFailedNoDoneRow(limit int) (int64, error) {
	now := s.dbTimeValue(time.Now().UTC())
	var (
		res sql.Result
		err error
	)
	if limit > 0 {
		res, err = s.db.Exec(
			s.rebind(fmt.Sprintf(`WITH picked AS (
				SELECT id
				FROM pipeline_items
				WHERE state = ? AND last_error = ?
				ORDER BY updated_at DESC
				LIMIT %d
			)
			UPDATE pipeline_items
			SET state = ?, last_error = '', claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ?
			WHERE id IN (SELECT id FROM picked)`, limit)),
			StateFailed, "translator produced no done row", StatePendingTranslate, now,
		)
	} else {
		res, err = s.db.Exec(
			s.rebind(`UPDATE pipeline_items
			 SET state = ?, last_error = '', claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ?
			 WHERE state = ? AND last_error = ?`),
			StatePendingTranslate, now, StateFailed, "translator produced no done row",
		)
	}
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) RepairBlockedTranslate(limit int) (BlockedTranslateRepairSummary, error) {
	query := `SELECT p.id, i.pack_json
		FROM pipeline_items p
		JOIN items i ON i.id = p.id
		WHERE p.state = ?
		ORDER BY p.updated_at ASC, p.id`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.db.Query(s.rebind(query), StateBlockedTranslate)
	if err != nil {
		return BlockedTranslateRepairSummary{}, err
	}
	type blockedRow struct {
		id      string
		packJSON string
	}
	blockedRows := make([]blockedRow, 0)
	defer rows.Close()
	summary := BlockedTranslateRepairSummary{}
	for rows.Next() {
		var id string
		var packRaw any
		if err := rows.Scan(&id, &packRaw); err != nil {
			return BlockedTranslateRepairSummary{}, err
		}
		blockedRows = append(blockedRows, blockedRow{
			id: id,
			packJSON: normalizeNullableText(packRaw),
		})
	}
	if err := rows.Err(); err != nil {
		return BlockedTranslateRepairSummary{}, err
	}
	rows.Close()
	releaseIDs := make([]string, 0)
	for _, row := range blockedRows {
		prevID := checkpointLineIDFromPack(row.packJSON, "prev_line_id")
		release := false
		if strings.TrimSpace(prevID) == "" {
			release = true
		} else {
			prevDone, err := checkpointRowDoneDB(s.db, s.backend, prevID)
			if err != nil {
				return BlockedTranslateRepairSummary{}, err
			}
			release = prevDone
		}
		if release {
			releaseIDs = append(releaseIDs, row.id)
		} else {
			summary.StillBlocked++
		}
	}
	if len(releaseIDs) == 0 {
		return summary, nil
	}
	now := s.dbTimeValue(time.Now().UTC())
	err = s.updateMany(releaseIDs, s.rebind(`UPDATE pipeline_items
		SET state = ?, last_error = '', claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ?
		WHERE id = ? AND state = ?`), func(stmt *sql.Stmt, id string) error {
		res, err := stmt.Exec(StatePendingTranslate, now, id, StateBlockedTranslate)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		summary.Released += n
		return nil
	})
	if err != nil {
		return BlockedTranslateRepairSummary{}, err
	}
	return summary, nil
}

func checkpointRowDoneDB(db *sql.DB, backend string, id string) (bool, error) {
	var status string
	err := db.QueryRow(rebindForBackend(backend, `SELECT status FROM items WHERE id = ?`), id).Scan(&status)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return status == "done", nil
}

var (
	failedNoRowActionOpenQuoteRe  = regexp.MustCompile(`^\([^)]*\)\s*"`)
	failedNoRowStatLikeQuoteRe    = regexp.MustCompile(`^(DC|ROLL|FC)\d+\s+[A-Za-z]+-"`)
	failedNoRowControlTokenRe     = regexp.MustCompile(`(?i)^\.[A-Za-z0-9_'\-]+(?:[<>]=?\d+|==[^\s]+)?-$|^[A-Za-z0-9_'\-]+==[^\s]+-$|^SPELL [A-Za-z0-9_'\-]+-$`)
)

func (s *Store) RouteKnownFailedNoDoneRow(limit int) (FailedNoRowRoutingSummary, error) {
	rows, err := s.db.Query(
		s.rebind(`SELECT p.id, i.pack_json
			FROM pipeline_items p
			JOIN items i ON i.id = p.id
			WHERE p.state = ? AND p.last_error = ?
			ORDER BY p.updated_at DESC`),
		StateFailed, "translator produced no done row",
	)
	if err != nil {
		return FailedNoRowRoutingSummary{}, err
	}
	defer rows.Close()

	type routedRow struct {
		id     string
		family string
	}
	selected := make([]routedRow, 0)
	summary := FailedNoRowRoutingSummary{}
	for rows.Next() {
		var id string
		var packRaw any
		if err := rows.Scan(&id, &packRaw); err != nil {
			return FailedNoRowRoutingSummary{}, err
		}
		packJSON := normalizeNullableText(packRaw)
		packObj := map[string]any{}
		if strings.TrimSpace(packJSON) != "" {
			_ = json.Unmarshal([]byte(packJSON), &packObj)
		}
		family := classifyKnownFailedNoRowFamily(packObj)
		if family == "" {
			continue
		}
		selected = append(selected, routedRow{id: id, family: family})
		switch family {
		case "open_quote_other":
			summary.OpenQuoteOther++
		case "action_open_quote":
			summary.ActionOpenQuote++
		case "stat_like_open_quote":
			summary.StatLikeOpenQuote++
		case "long_dialogue":
			summary.LongDialogue++
		case "expository":
			summary.Expository++
		case "passthrough":
			summary.Passthrough++
		case "current_rescue":
			summary.CurrentRescue++
		}
		if limit > 0 && len(selected) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return FailedNoRowRoutingSummary{}, err
	}
	if len(selected) == 0 {
		return summary, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return FailedNoRowRoutingSummary{}, err
	}
	defer tx.Rollback()

	now := s.dbTimeValue(time.Now().UTC())
	stmt, err := tx.Prepare(s.rebind(`UPDATE pipeline_items
		SET state = ?, retry_count = 0, score_final = -1, last_error = '', claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ?
		WHERE id = ? AND state = ? AND last_error = ?`))
	if err != nil {
		return FailedNoRowRoutingSummary{}, err
	}
	defer stmt.Close()

	for _, row := range selected {
		res, err := stmt.Exec(StatePendingFailedTranslate, now, row.id, StateFailed, "translator produced no done row")
		if err != nil {
			return FailedNoRowRoutingSummary{}, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return FailedNoRowRoutingSummary{}, err
		}
		summary.Total += n
	}
	if err := tx.Commit(); err != nil {
		return FailedNoRowRoutingSummary{}, err
	}
	return summary, nil
}

func (s *Store) ResolveAfterFailedTranslate(ids []string, sourceTextByID map[string]string, workerID string) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := s.dbTimeValue(time.Now().UTC())
	stmtDone, err := tx.Prepare(s.rebind(`UPDATE pipeline_items SET state = ?, last_error = '', claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ? WHERE id = ? AND state = ? AND claimed_by = ?`))
	if err != nil {
		return err
	}
	defer stmtDone.Close()
	stmtEscalate, err := tx.Prepare(s.rebind(`UPDATE pipeline_items SET state = ?, last_error = '', claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ? WHERE id = ? AND state = ? AND claimed_by = ?`))
	if err != nil {
		return err
	}
	defer stmtEscalate.Close()

	for _, id := range ids {
		done, err := checkpointRowDone(s.backend, tx, id)
		if err != nil {
			return err
		}
		switch {
		case done:
			nextReady, err := checkpointNextReady(s.backend, tx, id)
			if err != nil {
				return err
			}
			nextState := StateBlockedScore
			if nextReady {
				nextState = StatePendingScore
			}
			if _, err := stmtDone.Exec(nextState, now, id, StateWorkingFailedTranslate, workerID); err != nil {
				return err
			}
		case isSystemPassthrough(sourceTextByID[id]):
			if _, err := stmtDone.Exec(StateDone, now, id, StateWorkingFailedTranslate, workerID); err != nil {
				return err
			}
		default:
			if _, err := stmtEscalate.Exec(StatePendingRetranslate, now, id, StateWorkingFailedTranslate, workerID); err != nil {
				return err
			}
		}
	}
	sort.Strings(ids)
	processedFinal := make(map[string]bool, len(ids))
	for _, id := range ids {
		done, err := checkpointRowDone(s.backend, tx, id)
		if err != nil {
			return err
		}
		processedFinal[id] = done || isSystemPassthrough(sourceTextByID[id])
	}
	for _, id := range ids {
		if !processedFinal[id] {
			continue
		}
		if err := unlockNextTranslate(s.backend, tx, id, now); err != nil {
			return err
		}
		if err := unlockPrevScore(s.backend, tx, id, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) RouteOverlayUI(limit int) (int64, error) {
	query := `SELECT p.id
		FROM pipeline_items p
		JOIN items i ON i.id = p.id
		WHERE i.id LIKE 'ovl-mainmenu-%'
		  AND p.state IN (?, ?, ?, ?, ?)
		ORDER BY p.updated_at ASC, p.id`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.db.Query(s.rebind(query),
		StateFailed,
		StatePendingTranslate,
		StateWorkingTranslate,
		StatePendingRetranslate,
		StateWorkingRetranslate,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	selected := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		selected = append(selected, id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(selected) == 0 {
		return 0, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := s.dbTimeValue(time.Now().UTC())
	stmt, err := tx.Prepare(s.rebind(`UPDATE pipeline_items
		SET state = ?, retry_count = 0, score_final = -1, last_error = '', claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ?
		WHERE id = ?`))
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	var total int64
	for _, id := range selected {
		res, err := stmt.Exec(StatePendingOverlayTranslate, now, id)
		if err != nil {
			return 0, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return 0, err
		}
		total += n
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return total, nil
}

func (s *Store) ApplyPreservePolicy(limit int) (PreserveApplySummary, error) {
	query := `SELECT p.id, p.state, i.ko_json, i.pack_json
		FROM pipeline_items p
		JOIN items i ON i.id = p.id
		WHERE p.state <> ?
		ORDER BY p.updated_at ASC, p.id`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.db.Query(s.rebind(query), StateDone)
	if err != nil {
		return PreserveApplySummary{}, err
	}
	defer rows.Close()

	type preserveRow struct {
		id       string
		state    string
		koJSON   string
		packJSON string
	}
	selected := make([]preserveRow, 0)
	for rows.Next() {
		var id, state string
		var koRaw any
		var packRaw any
		if err := rows.Scan(&id, &state, &koRaw, &packRaw); err != nil {
			return PreserveApplySummary{}, err
		}
		packJSON := normalizeNullableText(packRaw)
		if strings.TrimSpace(packJSON) == "" {
			continue
		}
		var packObj map[string]any
		if err := json.Unmarshal([]byte(packJSON), &packObj); err != nil {
			continue
		}
		if strings.TrimSpace(stringFieldAny(packObj, "translation_policy")) != "preserve" {
			continue
		}
		selected = append(selected, preserveRow{
			id:       id,
			state:    state,
			koJSON:   normalizeNullableText(koRaw),
			packJSON: packJSON,
		})
	}
	if err := rows.Err(); err != nil {
		return PreserveApplySummary{}, err
	}
	if len(selected) == 0 {
		return PreserveApplySummary{}, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return PreserveApplySummary{}, err
	}
	defer tx.Rollback()

	nowTime := time.Now().UTC()
	now := s.dbTimeValue(nowTime)
	summary := PreserveApplySummary{}
	for _, row := range selected {
		var koObj map[string]any
		if strings.TrimSpace(row.koJSON) != "" {
			_ = json.Unmarshal([]byte(row.koJSON), &koObj)
		}
		if koObj == nil {
			koObj = map[string]any{}
		}
		var packObj map[string]any
		if err := json.Unmarshal([]byte(row.packJSON), &packObj); err != nil {
			return PreserveApplySummary{}, err
		}
		sourceRaw := strings.TrimSpace(stringFieldAny(packObj, "source_raw"))
		if sourceRaw == "" {
			sourceRaw = strings.TrimSpace(stringFieldAny(packObj, "en"))
		}
		if sourceRaw == "" {
			continue
		}
		koObj["Text"] = sourceRaw
		packObj["current_ko"] = sourceRaw
		packObj["fresh_ko"] = sourceRaw
		packObj["proposed_ko_restored"] = sourceRaw
		packObj["winner"] = "preserve"
		packObj["winner_score"] = 100
		packObj["current_score"] = 100
		packObj["fresh_score"] = 100
		packObj["score_delta"] = 0
		packObj["rewrite_triggered"] = false
		packObj["score_decision"] = "preserve"
		packObj["risk"] = "low"
		packObj["notes"] = "preserve policy"
		koRaw, err := json.Marshal(koObj)
		if err != nil {
			return PreserveApplySummary{}, err
		}
		packRaw, err := json.Marshal(packObj)
		if err != nil {
			return PreserveApplySummary{}, err
		}
		if _, err := tx.Exec(rebindForBackend(s.backend, `UPDATE items SET status = ?, ko_json = ?, pack_json = ?, updated_at = ? WHERE id = ?`), "done", string(koRaw), string(packRaw), now, row.id); err != nil {
			return PreserveApplySummary{}, err
		}
		if _, err := tx.Exec(s.rebind(`UPDATE pipeline_items
			SET state = ?, retry_count = 0, score_final = ?, last_error = '', claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ?
			WHERE id = ? AND state = ?`), StateDone, 100, now, row.id, row.state); err != nil {
			return PreserveApplySummary{}, err
		}
		if err := unlockNextTranslate(s.backend, tx, row.id, now); err != nil {
			return PreserveApplySummary{}, err
		}
		if err := unlockPrevScore(s.backend, tx, row.id, now); err != nil {
			return PreserveApplySummary{}, err
		}
		summary.Applied++
	}
	if err := tx.Commit(); err != nil {
		return PreserveApplySummary{}, err
	}
	return summary, nil
}

func (s *Store) ResolveAfterOverlayTranslate(ids []string, sourceTextByID map[string]string, workerID string) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := s.dbTimeValue(time.Now().UTC())
	stmtDone, err := tx.Prepare(s.rebind(`UPDATE pipeline_items SET state = ?, last_error = '', claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ? WHERE id = ? AND state = ? AND claimed_by = ?`))
	if err != nil {
		return err
	}
	defer stmtDone.Close()
	stmtFail, err := tx.Prepare(s.rebind(`UPDATE pipeline_items SET state = ?, last_error = ?, claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ? WHERE id = ? AND state = ? AND claimed_by = ?`))
	if err != nil {
		return err
	}
	defer stmtFail.Close()

	for _, id := range ids {
		done, err := checkpointRowDone(s.backend, tx, id)
		if err != nil {
			return err
		}
		switch {
		case done:
			nextReady, err := checkpointNextReady(s.backend, tx, id)
			if err != nil {
				return err
			}
			nextState := StateBlockedScore
			if nextReady {
				nextState = StatePendingScore
			}
			if _, err := stmtDone.Exec(nextState, now, id, StateWorkingOverlayTranslate, workerID); err != nil {
				return err
			}
		case isSystemPassthrough(sourceTextByID[id]):
			if _, err := stmtDone.Exec(StateDone, now, id, StateWorkingOverlayTranslate, workerID); err != nil {
				return err
			}
		default:
			if _, err := stmtFail.Exec(StateFailed, "overlay translator produced no done row", now, id, StateWorkingOverlayTranslate, workerID); err != nil {
				return err
			}
		}
	}
	sort.Strings(ids)
	processedFinal := make(map[string]bool, len(ids))
	for _, id := range ids {
		done, err := checkpointRowDone(s.backend, tx, id)
		if err != nil {
			return err
		}
		processedFinal[id] = done || isSystemPassthrough(sourceTextByID[id])
	}
	for _, id := range ids {
		if !processedFinal[id] {
			continue
		}
		if err := unlockNextTranslate(s.backend, tx, id, now); err != nil {
			return err
		}
		if err := unlockPrevScore(s.backend, tx, id, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func classifyKnownFailedNoRowFamily(packObj map[string]any) string {
	source := strings.TrimSpace(stringFieldAny(packObj, "source_raw"))
	if source == "" {
		source = strings.TrimSpace(stringFieldAny(packObj, "en"))
	}
	role := strings.TrimSpace(stringFieldAny(packObj, "text_role"))
	currentKO := strings.TrimSpace(stringFieldAny(packObj, "current_ko"))
	if source == "" {
		return ""
	}
	switch {
	case failedNoRowStatLikeQuoteRe.MatchString(source):
		return "stat_like_open_quote"
	case failedNoRowActionOpenQuoteRe.MatchString(source):
		return "action_open_quote"
	case isKnownLongDialogueFailedNoRow(role, source):
		return "long_dialogue"
	case strings.Contains(source, "\""):
		return "open_quote_other"
	case isKnownExpositoryFailedNoRow(role, source):
		return "expository"
	case isKnownPassthroughFailedNoRow(source):
		return "passthrough"
	case isKnownCurrentRescueFailedNoRow(role, source, currentKO):
		return "current_rescue"
	default:
		return ""
	}
}

func isKnownLongDialogueFailedNoRow(role string, source string) bool {
	if role != "dialogue" {
		return false
	}
	length := len([]rune(strings.TrimSpace(source)))
	if length >= 180 {
		return true
	}
	return strings.Contains(source, "<i>") && length >= 80
}

func isKnownExpositoryFailedNoRow(role string, source string) bool {
	if role != "glossary" && role != "quest" && role != "system" && role != "narration" {
		return false
	}
	if strings.Contains(source, " - ") {
		parts := strings.SplitN(source, " - ", 2)
		if len(parts) == 2 && len([]rune(strings.TrimSpace(parts[1]))) >= 40 {
			return true
		}
	}
	return len([]rune(strings.TrimSpace(source))) >= 140
}

func isKnownPassthroughFailedNoRow(source string) bool {
	if failedNoRowControlTokenRe.MatchString(source) {
		return true
	}
	if nonEnglishPassthroughLocal(source) {
		return true
	}
	if strings.Contains(source, "<wiggle>") {
		stripped := stripSimpleTagsLocal(source)
		return stripped != "" && punctuationOnlyLocal(stripped)
	}
	stripped := stripSimpleTagsLocal(source)
	return stripped != "" && len([]rune(stripped)) <= 8 && upperishTokenLocal(stripped)
}

func stripSimpleTagsLocal(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch r {
		case '<':
			inTag = true
			continue
		case '>':
			inTag = false
			continue
		}
		if !inTag {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func punctuationOnlyLocal(s string) bool {
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		if ('0' <= r && r <= '9') || ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z') || ('가' <= r && r <= '힣') {
			return false
		}
	}
	return true
}

func upperishTokenLocal(s string) bool {
	hasLetter := false
	for _, r := range s {
		switch {
		case 'A' <= r && r <= 'Z':
			hasLetter = true
		case 'a' <= r && r <= 'z':
			return false
		case '0' <= r && r <= '9':
		case r == ' ' || r == '.' || r == '!' || r == '?' || r == '-' || r == '_' || r == '\'':
		default:
			return false
		}
	}
	return hasLetter
}

func nonEnglishPassthroughLocal(source string) bool {
	stripped := stripSimpleTagsLocal(strings.TrimSpace(source))
	if stripped == "" {
		return false
	}
	lower := " " + strings.ToLower(stripped) + " "
	for _, marker := range []string{" the ", " and ", " of ", " is ", " are ", " your ", " you ", " this ", " that ", " with ", " from ", " for ", " to "} {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	asciiLetters := 0
	nonASCIILetters := 0
	for _, r := range stripped {
		switch {
		case ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z'):
			asciiLetters++
		case r > 127:
			nonASCIILetters++
		}
	}
	if nonASCIILetters < 3 {
		return false
	}
	return asciiLetters+nonASCIILetters >= 8
}

func isKnownCurrentRescueFailedNoRow(role, source, currentKO string) bool {
	if strings.TrimSpace(currentKO) == "" {
		return false
	}
	switch role {
	case "dialogue", "narration", "fragment", "reaction":
		return true
	default:
		return false
	}
}

func (s *Store) RequeueTranslateNoDoneRowAsRetranslate(limit int) (int64, error) {
	now := s.dbTimeValue(time.Now().UTC())
	var (
		res sql.Result
		err error
	)
	if limit > 0 {
		res, err = s.db.Exec(
			s.rebind(fmt.Sprintf(`WITH picked AS (
				SELECT id
				FROM pipeline_items
				WHERE state = ? AND last_error = ?
				ORDER BY updated_at DESC
				LIMIT %d
			)
			UPDATE pipeline_items
			SET state = ?, retry_count = 0, last_error = '', claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ?
			WHERE id IN (SELECT id FROM picked)`, limit)),
			StateFailed, "translator produced no done row", StatePendingRetranslate, now,
		)
	} else {
		res, err = s.db.Exec(
			s.rebind(`UPDATE pipeline_items
			 SET state = ?, retry_count = 0, last_error = '', claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ?
			 WHERE state = ? AND last_error = ?`),
			StatePendingRetranslate, now, StateFailed, "translator produced no done row",
		)
	}
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) RequeueExpiredWorking(workingState string, pendingState string) (int64, error) {
	nowTime := time.Now().UTC()
	now := s.dbTimeValue(nowTime)
	res, err := s.db.Exec(
		s.rebind(`UPDATE pipeline_items
		 SET state = ?, last_error = '', claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ?
		 WHERE state = ? AND claimed_by != '' AND lease_until IS NOT NULL AND lease_until < ?`),
		pendingState, now, workingState, now,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) CleanupStaleClaims() (StaleClaimCleanupSummary, error) {
	translate, err := s.RequeueExpiredWorking(StateWorkingTranslate, StatePendingTranslate)
	if err != nil {
		return StaleClaimCleanupSummary{}, err
	}
	score, err := s.RequeueExpiredWorking(StateWorkingScore, StatePendingScore)
	if err != nil {
		return StaleClaimCleanupSummary{}, err
	}
	retranslate, err := s.RequeueExpiredWorking(StateWorkingRetranslate, StatePendingRetranslate)
	if err != nil {
		return StaleClaimCleanupSummary{}, err
	}
	return StaleClaimCleanupSummary{
		Translate:   translate,
		Score:       score,
		Retranslate: retranslate,
	}, nil
}

func (s *Store) RecoverUnscoreableWorkingScore(allIDs []string, loadedIDs []string, workerID string) (ScoreRecoverySummary, error) {
	if len(allIDs) == 0 || workerID == "" {
		return ScoreRecoverySummary{}, nil
	}
	loaded := make(map[string]struct{}, len(loadedIDs))
	for _, id := range loadedIDs {
		loaded[id] = struct{}{}
	}
	missing := make([]string, 0, len(allIDs))
	for _, id := range allIDs {
		if _, ok := loaded[id]; !ok {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return ScoreRecoverySummary{}, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return ScoreRecoverySummary{}, err
	}
	defer tx.Rollback()

	now := s.dbTimeValue(time.Now().UTC())
	placeholders := make([]string, 0, len(missing))
	args := make([]any, 0, len(missing))
	for _, id := range missing {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}

	query := fmt.Sprintf(`SELECT p.id, i.status, i.ko_json, i.pack_json
		FROM pipeline_items p
		JOIN items i ON i.id = p.id
		WHERE p.state = ? AND p.claimed_by = ? AND p.id IN (%s)`, strings.Join(placeholders, ","))
	query = s.rebind(query)
	baseArgs := []any{StateWorkingScore, workerID}
	rows, err := tx.Query(query, append(baseArgs, args...)...)
	if err != nil {
		return ScoreRecoverySummary{}, err
	}
	type recoveryRow struct {
		id         string
		itemStatus string
		koJSON     string
		packJSON   string
	}
	recoveryRows := make([]recoveryRow, 0, len(missing))

	summary := ScoreRecoverySummary{}
	for rows.Next() {
		var rr recoveryRow
		var koJSONRaw any
		var packJSONRaw any
		if err := rows.Scan(&rr.id, &rr.itemStatus, &koJSONRaw, &packJSONRaw); err != nil {
			rows.Close()
			return ScoreRecoverySummary{}, err
		}
		rr.koJSON = normalizeNullableText(koJSONRaw)
		rr.packJSON = normalizeNullableText(packJSONRaw)
		recoveryRows = append(recoveryRows, rr)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return ScoreRecoverySummary{}, err
	}
	rows.Close()

	for _, rr := range recoveryRows {
		koText := ""
		if strings.TrimSpace(rr.koJSON) != "" {
			var koObj map[string]any
			if json.Unmarshal([]byte(rr.koJSON), &koObj) == nil {
				koText = strings.TrimSpace(stringFieldAny(koObj, "Text"))
			}
		}
		packObj := map[string]any{}
		if strings.TrimSpace(rr.packJSON) != "" {
			_ = json.Unmarshal([]byte(rr.packJSON), &packObj)
		}
		freshKO := strings.TrimSpace(stringFieldAny(packObj, "fresh_ko"))
		sourceRaw := strings.TrimSpace(stringFieldAny(packObj, "source_raw"))
		if sourceRaw == "" {
			sourceRaw = strings.TrimSpace(stringFieldAny(packObj, "en"))
		}

		nextState := StateFailed
		lastError := "unscoreable score row"
		switch {
		case rr.itemStatus != "done":
			nextState = StatePendingTranslate
			lastError = ""
			summary.ToPendingTranslate++
		case sourceRaw != "" && freshKO == "" && koText == "":
			nextState = StatePendingRetranslate
			lastError = ""
			summary.ToPendingRetranslate++
		default:
			summary.ToFailed++
		}

		if _, err := tx.Exec(
			s.rebind(`UPDATE pipeline_items
				SET state = ?, score_final = -1, last_error = ?, claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ?
				WHERE id = ? AND state = ? AND claimed_by = ?`),
			nextState, lastError, now, rr.id, StateWorkingScore, workerID,
		); err != nil {
			return ScoreRecoverySummary{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return ScoreRecoverySummary{}, err
	}
	return summary, nil
}

func (s *Store) ExtendLease(ids []string, workingState string, workerID string, leaseDuration time.Duration) (int64, error) {
	if len(ids) == 0 || workerID == "" || leaseDuration <= 0 {
		return 0, nil
	}
	now := time.Now().UTC()
	nowRFC := s.dbTimeValue(now)
	leaseUntil := s.dbTimeValue(now.Add(leaseDuration))
	updated := int64(0)
	err := s.updateMany(ids, s.rebind(`UPDATE pipeline_items SET lease_until = ?, updated_at = ? WHERE id = ? AND state = ? AND claimed_by = ?`), func(stmt *sql.Stmt, id string) error {
		res, err := stmt.Exec(leaseUntil, nowRFC, id, workingState, workerID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		updated += n
		return nil
	})
	return updated, err
}

func (s *Store) RequeueClaimsByWorker(workingState string, pendingState string, workerID string) (int64, error) {
	if workerID == "" {
		return 0, nil
	}
	now := s.dbTimeValue(time.Now().UTC())
	res, err := s.db.Exec(
		s.rebind(`UPDATE pipeline_items
		 SET state = ?, last_error = '', claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ?
		 WHERE state = ? AND claimed_by = ?`),
		pendingState, now, workingState, workerID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) RequeueClaimedIDsByWorker(ids []string, workingState string, pendingState string, workerID string, lastError string) (int64, error) {
	if workerID == "" || len(ids) == 0 {
		return 0, nil
	}
	now := s.dbTimeValue(time.Now().UTC())
	var updated int64
	err := s.updateMany(ids, s.rebind(`UPDATE pipeline_items
		 SET state = ?, last_error = ?, claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ?
		 WHERE id = ? AND state = ? AND claimed_by = ?`), func(stmt *sql.Stmt, id string) error {
		res, err := stmt.Exec(pendingState, lastError, now, id, workingState, workerID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		return nil
	})
	return updated, err
}

func checkpointRowDone(backend string, tx *sql.Tx, id string) (bool, error) {
	var status string
	err := tx.QueryRow(rebindForBackend(backend, `SELECT status FROM items WHERE id = ?`), id).Scan(&status)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return status == "done", nil
}

func checkpointPrevLineID(backend string, tx *sql.Tx, id string) (string, error) {
	var packJSON string
	err := scanNullableString(tx.QueryRow(rebindForBackend(backend, `SELECT pack_json FROM items WHERE id = ?`), id), &packJSON)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return checkpointLineIDFromPack(packJSON, "prev_line_id"), nil
}

func checkpointNextLineID(backend string, tx *sql.Tx, id string) (string, error) {
	var packJSON string
	err := scanNullableString(tx.QueryRow(rebindForBackend(backend, `SELECT pack_json FROM items WHERE id = ?`), id), &packJSON)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return checkpointLineIDFromPack(packJSON, "next_line_id"), nil
}

func checkpointNextReady(backend string, tx *sql.Tx, id string) (bool, error) {
	nextID, err := checkpointNextLineID(backend, tx, id)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(nextID) == "" {
		return true, nil
	}
	return checkpointRowDone(backend, tx, nextID)
}

func checkpointLineIDFromPack(packJSON string, key string) string {
	if strings.TrimSpace(packJSON) == "" {
		return ""
	}
	var packObj map[string]any
	if err := json.Unmarshal([]byte(packJSON), &packObj); err != nil {
		return ""
	}
	return strings.TrimSpace(stringFieldAny(packObj, key))
}

func (s *Store) ListByState(state string, limit int) ([]PipelineItem, error) {
	query := s.rebind(`SELECT id, sort_index, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until FROM pipeline_items WHERE state = ? ORDER BY sort_index, id`)
	args := []any{state}
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PipelineItem
	for rows.Next() {
		var it PipelineItem
		if err := scanPipelineItem(rows, &it); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) ClaimPending(pendingState string, workingState string, workerID string, limit int, leaseDuration time.Duration) ([]PipelineItem, error) {
	if workerID == "" {
		return nil, fmt.Errorf("workerID required")
	}
	if limit <= 0 {
		return nil, nil
	}
	now := time.Now().UTC()
	nowRFC := s.dbTimeValue(now)
	leaseUntil := s.dbTimeValue(now.Add(leaseDuration))

	rows, err := s.db.Query(
		s.rebind(fmt.Sprintf(`WITH picked AS (
				SELECT id
				FROM pipeline_items
				WHERE state = ?
				  AND (claimed_by = '' OR lease_until IS NULL OR lease_until < ?)
				ORDER BY sort_index, id
				LIMIT %d
			)
			UPDATE pipeline_items
			SET state = ?, claimed_by = ?, claimed_at = ?, lease_until = ?, updated_at = ?
			WHERE id IN (SELECT id FROM picked)
			RETURNING id, sort_index, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until`, limit)),
		pendingState, nowRFC, workingState, workerID, nowRFC, leaseUntil, nowRFC,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var claimed []PipelineItem
	for rows.Next() {
		var it PipelineItem
		if err := scanPipelineItem(rows, &it); err != nil {
			return nil, err
		}
		claimed = append(claimed, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return claimed, nil
}

func (s *Store) MarkState(ids []string, state string, lastError string) error {
	if len(ids) == 0 {
		return nil
	}
	now := s.dbTimeValue(time.Now().UTC())
	return s.updateMany(ids, s.rebind(`UPDATE pipeline_items SET state = ?, last_error = ?, updated_at = ? WHERE id = ?`), func(stmt *sql.Stmt, id string) error {
		_, err := stmt.Exec(state, lastError, now, id)
		return err
	})
}

func (s *Store) ResolveAfterTranslate(ids []string, sourceTextByID map[string]string, isRetry bool, workerID string) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := s.dbTimeValue(time.Now().UTC())
	workingState := StateWorkingTranslate
	if isRetry {
		workingState = StateWorkingRetranslate
	}
	stmtDone, err := tx.Prepare(s.rebind(`UPDATE pipeline_items SET state = ?, last_error = '', claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ?, retry_count = retry_count + ? WHERE id = ? AND state = ? AND claimed_by = ?`))
	if err != nil {
		return err
	}
	defer stmtDone.Close()
	stmtFail, err := tx.Prepare(s.rebind(`UPDATE pipeline_items SET state = ?, last_error = ?, claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ? WHERE id = ? AND state = ? AND claimed_by = ?`))
	if err != nil {
		return err
	}
	defer stmtFail.Close()

	retryInc := 0
	if isRetry {
		retryInc = 1
	}
	for _, id := range ids {
		done, err := checkpointRowDone(s.backend, tx, id)
		if err != nil {
			return err
		}
		switch {
		case done:
			nextReady, err := checkpointNextReady(s.backend, tx, id)
			if err != nil {
				return err
			}
			nextState := StateBlockedScore
			if nextReady {
				nextState = StatePendingScore
			}
			if _, err := stmtDone.Exec(nextState, now, retryInc, id, workingState, workerID); err != nil {
				return err
			}
		case isSystemPassthrough(sourceTextByID[id]):
			if _, err := stmtFail.Exec(StateDone, "system passthrough", now, id, workingState, workerID); err != nil {
				return err
			}
		default:
			nextState := StateFailed
			lastError := "translator produced no done row"
			if !isRetry {
				nextState = StatePendingRetranslate
				lastError = ""
			}
			if _, err := stmtFail.Exec(nextState, lastError, now, id, workingState, workerID); err != nil {
				return err
			}
		}
	}
	sort.Strings(ids)
	processedDone := make(map[string]bool, len(ids))
	processedFinal := make(map[string]bool, len(ids))
	for _, id := range ids {
		done, err := checkpointRowDone(s.backend, tx, id)
		if err != nil {
			return err
		}
		processedDone[id] = done
		processedFinal[id] = done || isSystemPassthrough(sourceTextByID[id])
	}
	for _, id := range ids {
		if !processedFinal[id] {
			continue
		}
		if err := unlockNextTranslate(s.backend, tx, id, now); err != nil {
			return err
		}
		if err := unlockPrevScore(s.backend, tx, id, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ApplyScores(rows []PipelineItem, reports map[string]ScoreResult, threshold float64, maxRetries int, workerID string) error {
	ids := pipelineIDs(rows)
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := s.dbTimeValue(time.Now().UTC())
	retryByID := make(map[string]int, len(rows))
	for _, row := range rows {
		retryByID[row.ID] = row.RetryCount
	}
	for _, id := range ids {
		report, ok := reports[id]
		if !ok {
			if _, err := tx.Exec(s.rebind(`UPDATE pipeline_items SET state = ?, last_error = ?, claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ? WHERE id = ? AND state = ? AND claimed_by = ?`), StatePendingScore, "missing score", now, id, StateWorkingScore, workerID); err != nil {
				return err
			}
			continue
		}
		if isScoreTransportFailure(report) {
			if _, err := tx.Exec(s.rebind(`UPDATE pipeline_items SET state = ?, last_error = ?, claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ? WHERE id = ? AND state = ? AND claimed_by = ?`), StatePendingScore, report.ShortReason, now, id, StateWorkingScore, workerID); err != nil {
				return err
			}
			continue
		}
		retryCount := retryByID[id]
		nextState := StateDone
		lastError := ""
		decision, err := applyScoreDecisionToCheckpoint(s.backend, tx, id, report, normalizeScoreThreshold(threshold), defaultKeepMargin)
		if err != nil {
			if _, updateErr := tx.Exec(s.rebind(`UPDATE pipeline_items SET state = ?, last_error = ?, claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ? WHERE id = ? AND state = ? AND claimed_by = ?`), StateFailed, err.Error(), now, id, StateWorkingScore, workerID); updateErr != nil {
				return updateErr
			}
			continue
		}
		if decision.RewriteTriggered {
			lastError = buildRetryReason(report, decision)
			if retryCount < maxRetries {
				nextState = StatePendingRetranslate
			} else {
				nextState = StateFailed
				lastError = fmt.Sprintf("max score %.1f < threshold %.1f after max retries", decision.ScoreFinal, normalizeScoreThreshold(threshold))
			}
		}
		if _, err := tx.Exec(s.rebind(`UPDATE pipeline_items SET state = ?, score_final = ?, last_error = ?, claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ? WHERE id = ? AND state = ? AND claimed_by = ?`), nextState, decision.ScoreFinal, lastError, now, id, StateWorkingScore, workerID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func isScoreTransportFailure(report ScoreResult) bool {
	for _, tag := range report.ReasonTags {
		switch strings.TrimSpace(tag) {
		case "scoring_error", "missing_score":
			return true
		}
	}
	reason := strings.ToLower(strings.TrimSpace(report.ShortReason))
	if reason == "" {
		return false
	}
	fragments := []string{
		"unexpected end of json input",
		"model returned no score",
		"empty response body",
		"connection refused",
		"transport connection broken",
		"http ",
		"dial tcp",
	}
	for _, fragment := range fragments {
		if strings.Contains(reason, fragment) {
			return true
		}
	}
	return false
}

func buildRetryReason(report ScoreResult, decision scoreDecision) string {
	parts := make([]string, 0, len(report.ReasonTags)+1)
	if strings.TrimSpace(report.ShortReason) != "" {
		parts = append(parts, strings.TrimSpace(report.ShortReason))
	}
	parts = append(parts, fmt.Sprintf("scores=current:%.1f fresh:%.1f max:%.1f delta:%.1f decision:%s", decision.CurrentScore, decision.FreshScore, decision.ScoreFinal, decision.ScoreDelta, decision.Decision))
	if len(report.ReasonTags) > 0 {
		parts = append(parts, "tags="+strings.Join(report.ReasonTags, ","))
	}
	return strings.Join(parts, " | ")
}

func unlockNextTranslate(backend string, tx *sql.Tx, id string, now any) error {
	nextID, err := checkpointNextLineID(backend, tx, id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(nextID) == "" {
		return nil
	}
	prevID, err := checkpointPrevLineID(backend, tx, nextID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(prevID) != "" {
		prevDone, err := checkpointRowDone(backend, tx, prevID)
		if err != nil {
			return err
		}
		if !prevDone {
			return nil
		}
	}
	_, err = tx.Exec(
		rebindForBackend(backend, `UPDATE pipeline_items
		 SET state = ?, last_error = '', claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ?
		 WHERE id = ? AND state = ?`),
		StatePendingTranslate, now, nextID, StateBlockedTranslate,
	)
	return err
}

func unlockPrevScore(backend string, tx *sql.Tx, id string, now any) error {
	prevID, err := checkpointPrevLineID(backend, tx, id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(prevID) == "" {
		return nil
	}
	prevDone, err := checkpointRowDone(backend, tx, prevID)
	if err != nil {
		return err
	}
	if !prevDone {
		return nil
	}
	_, err = tx.Exec(
		rebindForBackend(backend, `UPDATE pipeline_items
		 SET state = ?, last_error = '', claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ?
		 WHERE id = ? AND state = ?`),
		StatePendingScore, now, prevID, StateBlockedScore,
	)
	return err
}

func applyScoreDecisionToCheckpoint(backend string, tx *sql.Tx, id string, report ScoreResult, threshold float64, keepMargin float64) (scoreDecision, error) {
	var koJSON string
	var packJSON string
	row := tx.QueryRow(rebindForBackend(backend, `SELECT ko_json, pack_json FROM items WHERE id = ?`), id)
	var koValue any
	var packValue any
	if err := row.Scan(&koValue, &packValue); err != nil {
		return scoreDecision{}, fmt.Errorf("load checkpoint row: %w", err)
	}
	koJSON = normalizeNullableText(koValue)
	packJSON = normalizeNullableText(packValue)

	koObj := map[string]any{}
	if strings.TrimSpace(koJSON) != "" {
		if err := json.Unmarshal([]byte(koJSON), &koObj); err != nil {
			return scoreDecision{}, fmt.Errorf("decode ko_json: %w", err)
		}
	}
	packObj := map[string]any{}
	if strings.TrimSpace(packJSON) != "" {
		if err := json.Unmarshal([]byte(packJSON), &packObj); err != nil {
			return scoreDecision{}, fmt.Errorf("decode pack_json: %w", err)
		}
	}

	texts := checkpointTexts{
		currentKO: stringFieldAny(packObj, "current_ko"),
		freshKO:   stringFieldAny(packObj, "fresh_ko"),
	}
	if texts.freshKO == "" {
		texts.freshKO = stringFieldAny(koObj, "Text")
	}
	decision, err := deriveScoreDecision(texts, report, threshold, keepMargin)
	if err != nil {
		return scoreDecision{}, err
	}

	koObj["Text"] = decision.SelectedKO
	packObj["current_ko"] = decision.SelectedKO
	packObj["fresh_ko"] = decision.SelectedKO
	packObj["winner"] = decision.Winner
	packObj["winner_score"] = decision.ScoreFinal
	packObj["current_score"] = decision.CurrentScore
	packObj["fresh_score"] = decision.FreshScore
	packObj["score_delta"] = decision.ScoreDelta
	packObj["rewrite_triggered"] = decision.RewriteTriggered
	packObj["score_decision"] = decision.Decision
	if decision.RewriteTriggered {
		packObj["retry_reason"] = buildRetryReason(report, decision)
	} else {
		delete(packObj, "retry_reason")
	}
	packObj["proposed_ko_restored"] = decision.SelectedKO
	if report.ShortReason != "" {
		packObj["notes"] = report.ShortReason
	} else {
		delete(packObj, "notes")
	}

	koRaw, err := json.Marshal(koObj)
	if err != nil {
		return scoreDecision{}, fmt.Errorf("encode ko_json: %w", err)
	}
	packRaw, err := json.Marshal(packObj)
	if err != nil {
		return scoreDecision{}, fmt.Errorf("encode pack_json: %w", err)
	}
	if _, err := tx.Exec(rebindForBackend(backend, `UPDATE items SET ko_json = ?, pack_json = ?, updated_at = ? WHERE id = ?`), string(koRaw), string(packRaw), dbTimeValueForBackend(backend, time.Now().UTC()), id); err != nil {
		return scoreDecision{}, fmt.Errorf("update checkpoint row: %w", err)
	}
	return decision, nil
}

func deriveScoreDecision(texts checkpointTexts, report ScoreResult, threshold float64, keepMargin float64) (scoreDecision, error) {
	currentScore := normalizeJudgeScore(report.CurrentScore)
	freshScore := normalizeJudgeScore(report.FreshScore)
	currentKO := strings.TrimSpace(texts.currentKO)
	freshKO := strings.TrimSpace(texts.freshKO)

	if currentKO == "" {
		currentScore = 0
	}
	if freshKO == "" {
		freshScore = 0
	}
	if currentKO == "" && freshKO == "" {
		return scoreDecision{}, fmt.Errorf("both current_ko and fresh_ko are empty")
	}

	scoreFinal := currentScore
	if freshScore > scoreFinal {
		scoreFinal = freshScore
	}
	delta := freshScore - currentScore
	if delta < 0 {
		delta = -delta
	}

	winner := "current"
	selectedKO := currentKO
	if currentKO == "" || (freshKO != "" && freshScore > currentScore) {
		winner = "fresh"
		selectedKO = freshKO
	}
	if freshKO == "" {
		winner = "current"
		selectedKO = currentKO
	}

	decision := scoreDecision{
		Winner:       winner,
		CurrentScore: round1(currentScore),
		FreshScore:   round1(freshScore),
		ScoreFinal:   round1(scoreFinal),
		ScoreDelta:   round1(delta),
	}
	if decision.ScoreFinal < threshold {
		decision.Decision = "retranslate"
		decision.RewriteTriggered = true
		decision.SelectedKO = strings.TrimSpace(selectedKO)
		if decision.SelectedKO == "" {
			if freshKO != "" {
				decision.SelectedKO = freshKO
				decision.Winner = "fresh"
			} else {
				decision.SelectedKO = currentKO
				decision.Winner = "current"
			}
		}
		return decision, nil
	}
	if currentKO != "" && delta < keepMargin {
		decision.Winner = "current"
		decision.Decision = "current"
		decision.SelectedKO = currentKO
		return decision, nil
	}
	if decision.Winner == "fresh" {
		decision.Decision = "fresh"
		decision.SelectedKO = freshKO
		return decision, nil
	}
	decision.Decision = "current"
	decision.SelectedKO = currentKO
	return decision, nil
}

func normalizeJudgeScore(score float64) float64 {
	if score >= 0 && score <= 1 {
		score = score * 100.0
	}
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func normalizeScoreThreshold(threshold float64) float64 {
	return round1(normalizeJudgeScore(threshold))
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}

func stringFieldAny(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}

func (s *Store) CountStates() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT state, COUNT(*) FROM pipeline_items GROUP BY state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var state string
		var n int
		if err := rows.Scan(&state, &n); err != nil {
			return nil, err
		}
		out[state] = n
	}
	return out, rows.Err()
}

func (s *Store) RecordWorkerBatchStat(workerID, role string, processedCount int, startedAt, finishedAt time.Time) error {
	if workerID == "" || role == "" || processedCount <= 0 {
		return nil
	}
	elapsedMs := finishedAt.Sub(startedAt).Milliseconds()
	if elapsedMs < 0 {
		elapsedMs = 0
	}
	_, err := s.db.Exec(
		s.rebind(`INSERT INTO pipeline_worker_stats(worker_id, role, processed_count, elapsed_ms, started_at, finished_at)
		 VALUES(?, ?, ?, ?, ?, ?)`),
		workerID,
		role,
		processedCount,
		elapsedMs,
		s.dbTimeValue(startedAt),
		s.dbTimeValue(finishedAt),
	)
	return err
}

func (s *Store) ListRecentWorkerBatchStats(limit int) ([]WorkerBatchStat, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		fmt.Sprintf(`SELECT id, worker_id, role, processed_count, elapsed_ms, started_at, finished_at
			FROM pipeline_worker_stats
			ORDER BY finished_at DESC
			LIMIT %d`, limit),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]WorkerBatchStat, 0, limit)
	for rows.Next() {
		var it WorkerBatchStat
		if err := scanWorkerBatchStat(rows, &it); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) updateMany(ids []string, query string, exec func(*sql.Stmt, string) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, id := range ids {
		if err := exec(stmt, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func isSystemPassthrough(source string) bool {
	source = strings.TrimSpace(source)
	if source == "" {
		return false
	}
	pureControlPatterns := []string{
		`^\.[A-Za-z0-9_'\-]+==[^\s]+-$`,
		`^\.[A-Za-z0-9_'\-]+[<>]=?\d+-$`,
		`^[A-Za-z0-9_'\-]+==[^\s]+-$`,
		`^SPELL [A-Za-z0-9_'\-]+-$`,
	}
	for _, pattern := range pureControlPatterns {
		if matched, _ := regexp.MatchString(pattern, source); matched {
			return true
		}
	}
	return false
}
