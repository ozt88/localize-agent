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

type sqliteCheckpointStore struct {
	db      *sql.DB
	enabled bool
}

func NewSQLiteCheckpointStore(path string) (contracts.TranslationCheckpointStore, error) {
	if path == "" {
		return &sqliteCheckpointStore{enabled: false}, nil
	}
	db, err := openSQLite(path, []string{"PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL", "PRAGMA foreign_keys=ON"})
	if err != nil {
		return nil, fmt.Errorf("failed to open checkpoint db: %w", err)
	}
	if err := initCheckpointSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to init schema: %w", err)
	}
	return &sqliteCheckpointStore{db: db, enabled: true}, nil
}

func initCheckpointSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS jobs (
		  run_id TEXT PRIMARY KEY,
		  created_at TEXT NOT NULL,
		  total_ids INTEGER NOT NULL,
		  config_json TEXT NOT NULL
		)
	`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS items (
		  id TEXT PRIMARY KEY,
		  status TEXT NOT NULL,
		  ko_json TEXT,
		  pack_json TEXT,
		  attempts INTEGER NOT NULL DEFAULT 0,
		  last_error TEXT NOT NULL DEFAULT '',
		  updated_at TEXT NOT NULL,
		  latency_ms REAL NOT NULL DEFAULT 0,
		  source_hash TEXT NOT NULL DEFAULT ''
		)
	`)
	if err != nil {
		return err
	}
	_, err = db.Exec("CREATE INDEX IF NOT EXISTS idx_items_status ON items(status)")
	return err
}

func (cs *sqliteCheckpointStore) IsEnabled() bool { return cs.enabled }

func (cs *sqliteCheckpointStore) Close() error {
	if !cs.enabled || cs.db == nil {
		return nil
	}
	return cs.db.Close()
}

func (cs *sqliteCheckpointStore) UpsertItem(entryID, status, sourceHash string, attempts int, lastError string, latencyMs float64, koObj, packObj map[string]any) error {
	return cs.UpsertItems([]contracts.TranslationCheckpointItem{
		{
			EntryID:    entryID,
			Status:     status,
			SourceHash: sourceHash,
			Attempts:   attempts,
			LastError:  lastError,
			LatencyMs:  latencyMs,
			KOObj:      koObj,
			PackObj:    packObj,
		},
	})
}

func (cs *sqliteCheckpointStore) UpsertItems(items []contracts.TranslationCheckpointItem) error {
	if !cs.enabled {
		return nil
	}
	if len(items) == 0 {
		return nil
	}

	tx, err := cs.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`
		INSERT INTO items(id, status, ko_json, pack_json, attempts, last_error, updated_at, latency_ms, source_hash)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  status=excluded.status,
		  ko_json=COALESCE(excluded.ko_json, items.ko_json),
		  pack_json=COALESCE(excluded.pack_json, items.pack_json),
		  attempts=excluded.attempts,
		  last_error=excluded.last_error,
		  updated_at=excluded.updated_at,
		  latency_ms=excluded.latency_ms,
		  source_hash=excluded.source_hash
	`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, it := range items {
		koJSON, _ := json.Marshal(it.KOObj)
		packJSON, _ := json.Marshal(it.PackObj)
		if _, err := stmt.Exec(
			it.EntryID, it.Status, string(koJSON), string(packJSON),
			it.Attempts, it.LastError, now, it.LatencyMs, it.SourceHash,
		); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (cs *sqliteCheckpointStore) LoadDoneIDs() (map[string]bool, error) {
	if !cs.enabled {
		return nil, nil
	}
	rows, err := cs.db.Query("SELECT id FROM items WHERE status='done'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	done := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		done[id] = true
	}
	return done, rows.Err()
}

func LoadDonePackItems(dbPath string) ([]contracts.EvalPackItem, error) {
	db, err := openSQLite(dbPath, nil)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query("SELECT pack_json FROM items WHERE status='done' AND pack_json IS NOT NULL")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []contracts.EvalPackItem{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if raw == "" {
			continue
		}
		var item contracts.EvalPackItem
		if err := json.Unmarshal([]byte(raw), &item); err != nil {
			continue
		}
		if item.ID == "" {
			continue
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func ExportTranslationCheckpointRows(dbPath string, statuses []string) ([]map[string]any, error) {
	if len(statuses) == 0 {
		return nil, fmt.Errorf("at least one status is required")
	}
	db, err := openSQLite(dbPath, nil)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	ph := make([]string, len(statuses))
	args := make([]any, len(statuses))
	for i, s := range statuses {
		ph[i] = "?"
		args[i] = s
	}
	q := fmt.Sprintf(`SELECT id, status, pack_json, ko_json, updated_at FROM items WHERE status IN (%s) ORDER BY updated_at DESC`, strings.Join(ph, ","))
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var id, status, packJSON, koJSON, updatedAt string
		if err := rows.Scan(&id, &status, &packJSON, &koJSON, &updatedAt); err != nil {
			return nil, err
		}
		row := map[string]any{
			"id":         id,
			"status":     status,
			"updated_at": updatedAt,
		}
		if strings.TrimSpace(packJSON) != "" {
			var packObj map[string]any
			if json.Unmarshal([]byte(packJSON), &packObj) == nil {
				row["pack"] = packObj
			}
		}
		if strings.TrimSpace(koJSON) != "" {
			var koObj map[string]any
			if json.Unmarshal([]byte(koJSON), &koObj) == nil {
				row["ko"] = koObj
			}
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
