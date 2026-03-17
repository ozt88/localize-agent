package platform

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"localize-agent/workflow/internal/contracts"

	_ "modernc.org/sqlite"
)

type sqliteEvalStore struct {
	db      *sql.DB
	runName string
}

func NewSQLiteEvalStore(path, runName string) (contracts.EvalStore, error) {
	if strings.TrimSpace(runName) == "" {
		runName = "default"
	}
	db, err := openSQLite(path, []string{"PRAGMA journal_mode=WAL", "PRAGMA synchronous=NORMAL", "PRAGMA foreign_keys=ON"})
	if err != nil {
		return nil, err
	}
	if err := initEvalSchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return &sqliteEvalStore{db: db, runName: runName}, nil
}

func initEvalSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS eval_items (
		  id           TEXT PRIMARY KEY,
		  run_name     TEXT NOT NULL DEFAULT 'default',
		  source_id    TEXT NOT NULL DEFAULT '',
		  status       TEXT NOT NULL DEFAULT 'pending',
		  original_ko  TEXT NOT NULL DEFAULT '',
		  final_ko     TEXT NOT NULL DEFAULT '',
		  final_risk   TEXT NOT NULL DEFAULT '',
		  final_notes  TEXT NOT NULL DEFAULT '',
		  revised      INTEGER NOT NULL DEFAULT 0,
		  eval_history TEXT NOT NULL DEFAULT '[]',
		  en           TEXT NOT NULL DEFAULT '',
		  current_ko   TEXT NOT NULL DEFAULT '',
		  updated_at   TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_status ON eval_items(status);
		CREATE INDEX IF NOT EXISTS idx_run_status ON eval_items(run_name, status);
	`)
	if err != nil {
		return err
	}
	if err := ensureColumn(db, "eval_items", "run_name", "TEXT NOT NULL DEFAULT 'default'"); err != nil {
		return err
	}
	if err := ensureColumn(db, "eval_items", "source_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE eval_items SET source_id=id WHERE source_id=''`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_run_source_id ON eval_items(run_name, source_id)`)
	return err
}

func (e *sqliteEvalStore) Close() { e.db.Close() }

func (e *sqliteEvalStore) LoadPack(items []contracts.EvalPackItem) (int, error) {
	tx, err := e.db.Begin()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO eval_items
		  (id, run_name, source_id, status, original_ko, final_ko, final_risk, final_notes, revised, eval_history, en, current_ko, updated_at)
		VALUES (?, ?, ?, 'pending', ?, ?, ?, ?, 0, '[]', ?, ?, ?)
	`)
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	defer stmt.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	inserted := 0
	for _, it := range items {
		rowID := makeEvalRowID(e.runName, it.ID)
		r, err := stmt.Exec(rowID, e.runName, it.ID, it.ProposedKORestored, it.ProposedKORestored, it.Risk, it.Notes, it.EN, it.CurrentKO, now)
		if err != nil {
			tx.Rollback()
			return 0, err
		}
		n, _ := r.RowsAffected()
		inserted += int(n)
	}
	return inserted, tx.Commit()
}

func (e *sqliteEvalStore) PendingIDs() ([]string, error) {
	rows, err := e.db.Query(`SELECT id FROM eval_items WHERE run_name=? AND status='pending' ORDER BY rowid`, e.runName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (e *sqliteEvalStore) GetItem(id string) (*contracts.EvalPackItem, error) {
	row := e.db.QueryRow(`SELECT source_id, en, current_ko, original_ko FROM eval_items WHERE run_name=? AND id=?`, e.runName, id)
	var it contracts.EvalPackItem
	if err := row.Scan(&it.ID, &it.EN, &it.CurrentKO, &it.ProposedKORestored); err != nil {
		return nil, err
	}
	return &it, nil
}

func (e *sqliteEvalStore) MarkEvaluating(id string) error {
	_, err := e.db.Exec(`UPDATE eval_items SET status='evaluating', updated_at=? WHERE run_name=? AND id=?`, time.Now().UTC().Format(time.RFC3339), e.runName, id)
	return err
}

func (e *sqliteEvalStore) SaveResult(id, status, finalKO, finalRisk, finalNotes string, revised bool, history []contracts.EvalResult) error {
	histJSON, _ := json.Marshal(history)
	rev := 0
	if revised {
		rev = 1
	}
	_, err := e.db.Exec(`
		UPDATE eval_items
		SET status=?, final_ko=?, final_risk=?, final_notes=?, revised=?, eval_history=?, updated_at=?
		WHERE run_name=? AND id=?
	`, status, finalKO, finalRisk, finalNotes, rev, string(histJSON), time.Now().UTC().Format(time.RFC3339), e.runName, id)
	return err
}

func (e *sqliteEvalStore) ResetToStatus(statuses []string) (int, error) {
	if len(statuses) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(statuses))
	args := make([]any, len(statuses))
	for i, s := range statuses {
		placeholders[i] = "?"
		args[i] = s
	}
	q := fmt.Sprintf(`UPDATE eval_items SET status='pending', updated_at=? WHERE run_name=? AND status IN (%s)`, strings.Join(placeholders, ","))
	args = append([]any{time.Now().UTC().Format(time.RFC3339), e.runName}, args...)
	r, err := e.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	n, _ := r.RowsAffected()
	return int(n), nil
}

func (e *sqliteEvalStore) ResetIDs(ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tx, err := e.db.Begin()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare(`UPDATE eval_items SET status='pending', updated_at=? WHERE run_name=? AND source_id=?`)
	if err != nil {
		tx.Rollback()
		return 0, err
	}
	defer stmt.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	total := 0
	for _, id := range ids {
		r, err := stmt.Exec(now, e.runName, id)
		if err != nil {
			tx.Rollback()
			return 0, err
		}
		n, _ := r.RowsAffected()
		total += int(n)
	}
	return total, tx.Commit()
}

func (e *sqliteEvalStore) ResetEvaluating() (int, error) {
	r, err := e.db.Exec(`UPDATE eval_items SET status='pending', updated_at=? WHERE run_name=? AND status='evaluating'`, time.Now().UTC().Format(time.RFC3339), e.runName)
	if err != nil {
		return 0, err
	}
	n, _ := r.RowsAffected()
	return int(n), nil
}

func (e *sqliteEvalStore) StatusCounts() (map[string]int, error) {
	rows, err := e.db.Query(`SELECT status, COUNT(*) FROM eval_items WHERE run_name=? GROUP BY status`, e.runName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		counts[s] = n
	}
	return counts, rows.Err()
}

func (e *sqliteEvalStore) ExportByStatus(statuses ...string) ([]map[string]any, error) {
	placeholders := make([]string, len(statuses))
	args := make([]any, len(statuses))
	for i, s := range statuses {
		placeholders[i] = "?"
		args[i] = s
	}
	q := fmt.Sprintf(
		`SELECT source_id, en, original_ko, final_ko, final_risk, final_notes, revised, eval_history, status
		 FROM eval_items WHERE run_name=? AND status IN (%s) ORDER BY rowid`,
		strings.Join(placeholders, ","),
	)
	args = append([]any{e.runName}, args...)
	rows, err := e.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, en, origKO, finalKO, finalRisk, finalNotes, histJSON, status string
		var revised int
		if err := rows.Scan(&id, &en, &origKO, &finalKO, &finalRisk, &finalNotes, &revised, &histJSON, &status); err != nil {
			return nil, err
		}
		var hist []contracts.EvalResult
		_ = json.Unmarshal([]byte(histJSON), &hist)
		out = append(out, map[string]any{
			"id": id, "en": en, "original_ko": origKO,
			"final_ko": finalKO, "final_risk": finalRisk, "final_notes": finalNotes,
			"revised": revised == 1, "status": status, "eval_history": hist,
		})
	}
	return out, rows.Err()
}

func makeEvalRowID(runName, sourceID string) string {
	return runName + "::" + sourceID
}

func ensureColumn(db *sql.DB, table, column, ddl string) error {
	var n int
	q := fmt.Sprintf(`SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name=?`, table)
	if err := db.QueryRow(q, column).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err := db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, ddl))
	return err
}
