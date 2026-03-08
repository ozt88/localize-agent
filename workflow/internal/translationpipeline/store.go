package translationpipeline

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(dbPath string) (*Store, error) {
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
	if err := initSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func initSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS pipeline_items (
			id TEXT PRIMARY KEY,
			state TEXT NOT NULL,
			retry_count INTEGER NOT NULL DEFAULT 0,
			score_final REAL NOT NULL DEFAULT -1,
			last_error TEXT NOT NULL DEFAULT '',
			claimed_by TEXT NOT NULL DEFAULT '',
			claimed_at TEXT NOT NULL DEFAULT '',
			lease_until TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)
	`)
	if err != nil {
		return err
	}
	for _, ddl := range []string{
		`ALTER TABLE pipeline_items ADD COLUMN claimed_by TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE pipeline_items ADD COLUMN claimed_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE pipeline_items ADD COLUMN lease_until TEXT NOT NULL DEFAULT ''`,
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

func (s *Store) Seed(ids []string, limit int) error {
	if limit > 0 && limit < len(ids) {
		ids = ids[:limit]
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	stmt, err := tx.Prepare(`
		INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at)
		VALUES(?, ?, 0, -1, '', '', '', '', ?)
		ON CONFLICT(id) DO NOTHING
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, id := range ids {
		state := StatePendingTranslate
		if isDone, err := checkpointRowDone(tx, id); err != nil {
			return err
		} else if isDone {
			state = StatePendingScore
		}
		if _, err := stmt.Exec(id, state, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Reset() error {
	_, err := s.db.Exec(`DELETE FROM pipeline_items`)
	return err
}

func (s *Store) RequeueFailedNoDoneRow(limit int) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	var (
		res sql.Result
		err error
	)
	if limit > 0 {
		res, err = s.db.Exec(
			fmt.Sprintf(`WITH picked AS (
				SELECT id
				FROM pipeline_items
				WHERE state = ? AND last_error = ?
				ORDER BY updated_at DESC
				LIMIT %d
			)
			UPDATE pipeline_items
			SET state = ?, last_error = '', claimed_by = '', claimed_at = '', lease_until = '', updated_at = ?
			WHERE id IN (SELECT id FROM picked)`, limit),
			StateFailed, "translator produced no done row", StatePendingTranslate, now,
		)
	} else {
		res, err = s.db.Exec(
			`UPDATE pipeline_items
			 SET state = ?, last_error = '', claimed_by = '', claimed_at = '', lease_until = '', updated_at = ?
			 WHERE state = ? AND last_error = ?`,
			StatePendingTranslate, now, StateFailed, "translator produced no done row",
		)
	}
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func checkpointRowDone(tx *sql.Tx, id string) (bool, error) {
	var status string
	err := tx.QueryRow(`SELECT status FROM items WHERE id = ?`, id).Scan(&status)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return status == "done", nil
}

func (s *Store) ListByState(state string, limit int) ([]PipelineItem, error) {
	query := `SELECT id, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until FROM pipeline_items WHERE state = ? ORDER BY id`
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
		if err := rows.Scan(&it.ID, &it.State, &it.RetryCount, &it.ScoreFinal, &it.LastError, &it.ClaimedBy, &it.ClaimedAt, &it.LeaseUntil); err != nil {
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
	nowRFC := now.Format(time.RFC3339)
	leaseUntil := now.Add(leaseDuration).Format(time.RFC3339)

	rows, err := s.db.Query(
		fmt.Sprintf(`WITH picked AS (
				SELECT id
				FROM pipeline_items
				WHERE state = ?
				  AND (claimed_by = '' OR lease_until = '' OR lease_until < ?)
				ORDER BY id
				LIMIT %d
			)
			UPDATE pipeline_items
			SET state = ?, claimed_by = ?, claimed_at = ?, lease_until = ?, updated_at = ?
			WHERE id IN (SELECT id FROM picked)
			RETURNING id, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until`, limit),
		pendingState, nowRFC, workingState, workerID, nowRFC, leaseUntil, nowRFC,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var claimed []PipelineItem
	for rows.Next() {
		var it PipelineItem
		if err := rows.Scan(&it.ID, &it.State, &it.RetryCount, &it.ScoreFinal, &it.LastError, &it.ClaimedBy, &it.ClaimedAt, &it.LeaseUntil); err != nil {
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
	now := time.Now().UTC().Format(time.RFC3339)
	return s.updateMany(ids, `UPDATE pipeline_items SET state = ?, last_error = ?, updated_at = ? WHERE id = ?`, func(stmt *sql.Stmt, id string) error {
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

	now := time.Now().UTC().Format(time.RFC3339)
	workingState := StateWorkingTranslate
	if isRetry {
		workingState = StateWorkingRetranslate
	}
	stmtDone, err := tx.Prepare(`UPDATE pipeline_items SET state = ?, last_error = '', claimed_by = '', claimed_at = '', lease_until = '', updated_at = ?, retry_count = retry_count + ? WHERE id = ? AND state = ? AND claimed_by = ?`)
	if err != nil {
		return err
	}
	defer stmtDone.Close()
	stmtFail, err := tx.Prepare(`UPDATE pipeline_items SET state = ?, last_error = ?, claimed_by = '', claimed_at = '', lease_until = '', updated_at = ? WHERE id = ? AND state = ? AND claimed_by = ?`)
	if err != nil {
		return err
	}
	defer stmtFail.Close()

	retryInc := 0
	if isRetry {
		retryInc = 1
	}
	for _, id := range ids {
		done, err := checkpointRowDone(tx, id)
		if err != nil {
			return err
		}
		switch {
		case done:
			if _, err := stmtDone.Exec(StatePendingScore, now, retryInc, id, workingState, workerID); err != nil {
				return err
			}
		case isSystemPassthrough(sourceTextByID[id]):
			if _, err := stmtFail.Exec(StateDone, "system passthrough", now, id, workingState, workerID); err != nil {
				return err
			}
		default:
			if isRetry {
				if _, err := stmtFail.Exec(StateFailed, "translator produced no done row", now, id, workingState, workerID); err != nil {
					return err
				}
			} else {
				if _, err := stmtFail.Exec(StatePendingRetranslate, "translator produced no done row", now, id, workingState, workerID); err != nil {
					return err
				}
			}
		}
	}
	return tx.Commit()
}

func (s *Store) ApplyScores(rows []PipelineItem, reports map[string]float64, threshold float64, maxRetries int, workerID string) error {
	ids := pipelineIDs(rows)
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	retryByID := make(map[string]int, len(rows))
	for _, row := range rows {
		retryByID[row.ID] = row.RetryCount
	}
	for _, id := range ids {
		score, ok := reports[id]
		if !ok {
			if _, err := tx.Exec(`UPDATE pipeline_items SET state = ?, last_error = ?, claimed_by = '', claimed_at = '', lease_until = '', updated_at = ? WHERE id = ? AND state = ? AND claimed_by = ?`, StateFailed, "missing score", now, id, StateWorkingScore, workerID); err != nil {
				return err
			}
			continue
		}
		retryCount := retryByID[id]
		nextState := StateDone
		lastError := ""
		if score >= threshold {
			if retryCount < maxRetries {
				nextState = StatePendingRetranslate
			} else {
				nextState = StateFailed
				lastError = fmt.Sprintf("score %.3f >= threshold %.3f after max retries", score, threshold)
			}
		}
		if _, err := tx.Exec(`UPDATE pipeline_items SET state = ?, score_final = ?, last_error = ?, claimed_by = '', claimed_at = '', lease_until = '', updated_at = ? WHERE id = ? AND state = ? AND claimed_by = ?`, nextState, score, lastError, now, id, StateWorkingScore, workerID); err != nil {
			return err
		}
	}
	return tx.Commit()
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
		`INSERT INTO pipeline_worker_stats(worker_id, role, processed_count, elapsed_ms, started_at, finished_at)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		workerID,
		role,
		processedCount,
		elapsedMs,
		startedAt.UTC().Format(time.RFC3339),
		finishedAt.UTC().Format(time.RFC3339),
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
		if err := rows.Scan(&it.ID, &it.WorkerID, &it.Role, &it.ProcessedCount, &it.ElapsedMs, &it.StartedAt, &it.FinishedAt); err != nil {
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
