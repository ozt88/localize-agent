package dbmigration

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"localize-agent/workflow/pkg/platform"
	"localize-agent/workflow/internal/translationpipeline"
)

type TableMigrationCount struct {
	Table string
	Src   int64
	Dst   int64
}

type CheckpointToPostgresSummary struct {
	Counts []TableMigrationCount
}

func MigrateSQLiteCheckpointToPostgres(sqlitePath string, postgresDSN string, truncateDst bool) (*CheckpointToPostgresSummary, error) {
	if strings.TrimSpace(sqlitePath) == "" {
		return nil, fmt.Errorf("sqlite path required")
	}
	if strings.TrimSpace(postgresDSN) == "" {
		return nil, fmt.Errorf("postgres dsn required")
	}

	checkpointStore, err := platform.NewPostgresCheckpointStore(postgresDSN)
	if err != nil {
		return nil, err
	}
	defer checkpointStore.Close()

	pipelineStore, err := translationpipeline.OpenStore(platform.DBBackendPostgres, "", postgresDSN)
	if err != nil {
		return nil, err
	}
	defer pipelineStore.Close()

	srcDB, err := platform.OpenTranslationCheckpointDB(platform.DBBackendSQLite, sqlitePath, "")
	if err != nil {
		return nil, err
	}
	defer srcDB.Close()

	dstDB, err := platform.OpenTranslationCheckpointDB(platform.DBBackendPostgres, "", postgresDSN)
	if err != nil {
		return nil, err
	}
	defer dstDB.Close()

	if truncateDst {
		if err := truncateDestination(dstDB); err != nil {
			return nil, err
		}
	}

	if err := migrateJobs(srcDB, dstDB); err != nil {
		return nil, err
	}
	if err := migrateItems(srcDB, dstDB); err != nil {
		return nil, err
	}
	if err := migratePipelineItems(srcDB, dstDB); err != nil {
		return nil, err
	}
	if err := migratePipelineWorkerStats(srcDB, dstDB); err != nil {
		return nil, err
	}

	summary, err := validateCounts(srcDB, dstDB)
	if err != nil {
		return nil, err
	}
	return summary, nil
}

func truncateDestination(dstDB *sql.DB) error {
	_, err := dstDB.Exec(`TRUNCATE TABLE pipeline_worker_stats, pipeline_items, items, jobs RESTART IDENTITY CASCADE`)
	return err
}

func migrateJobs(srcDB, dstDB *sql.DB) error {
	exists, err := sqliteTableExists(srcDB, "jobs")
	if err != nil || !exists {
		return err
	}
	rows, err := srcDB.Query(`SELECT run_id, created_at, total_ids, config_json FROM jobs`)
	if err != nil {
		return err
	}
	defer rows.Close()

	tx, err := dstDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(platform.RebindSQL(platform.DBBackendPostgres, `
		INSERT INTO jobs(run_id, created_at, total_ids, config_json)
		VALUES(?, ?, ?, ?::jsonb)
		ON CONFLICT(run_id) DO UPDATE SET
			created_at = EXCLUDED.created_at,
			total_ids = EXCLUDED.total_ids,
			config_json = EXCLUDED.config_json
	`))
	if err != nil {
		return err
	}
	defer stmt.Close()

	for rows.Next() {
		var runID string
		var createdAtRaw string
		var totalIDs int
		var configJSON string
		if err := rows.Scan(&runID, &createdAtRaw, &totalIDs, &configJSON); err != nil {
			return err
		}
		createdAt := parseSQLiteTime(createdAtRaw)
		if strings.TrimSpace(configJSON) == "" {
			configJSON = "{}"
		}
		if _, err := stmt.Exec(runID, createdAt, totalIDs, configJSON); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateItems(srcDB, dstDB *sql.DB) error {
	exists, err := sqliteTableExists(srcDB, "items")
	if err != nil || !exists {
		return err
	}
	rows, err := srcDB.Query(`SELECT id, status, ko_json, pack_json, attempts, last_error, updated_at, latency_ms, source_hash FROM items`)
	if err != nil {
		return err
	}
	defer rows.Close()

	tx, err := dstDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(platform.RebindSQL(platform.DBBackendPostgres, `
		INSERT INTO items(id, status, ko_json, pack_json, attempts, last_error, updated_at, latency_ms, source_hash)
		VALUES(?, ?, ?::jsonb, ?::jsonb, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = EXCLUDED.status,
			ko_json = EXCLUDED.ko_json,
			pack_json = EXCLUDED.pack_json,
			attempts = EXCLUDED.attempts,
			last_error = EXCLUDED.last_error,
			updated_at = EXCLUDED.updated_at,
			latency_ms = EXCLUDED.latency_ms,
			source_hash = EXCLUDED.source_hash
	`))
	if err != nil {
		return err
	}
	defer stmt.Close()

	for rows.Next() {
		var id, status, koJSON, packJSON, lastError, updatedAtRaw, sourceHash string
		var attempts int
		var latencyMS float64
		if err := rows.Scan(&id, &status, &koJSON, &packJSON, &attempts, &lastError, &updatedAtRaw, &latencyMS, &sourceHash); err != nil {
			return err
		}
		if strings.TrimSpace(koJSON) == "" {
			koJSON = "{}"
		}
		if strings.TrimSpace(packJSON) == "" {
			packJSON = "{}"
		}
		if _, err := stmt.Exec(id, status, koJSON, packJSON, attempts, lastError, parseSQLiteTime(updatedAtRaw), latencyMS, sourceHash); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return tx.Commit()
}

func migratePipelineItems(srcDB, dstDB *sql.DB) error {
	exists, err := sqliteTableExists(srcDB, "pipeline_items")
	if err != nil || !exists {
		return err
	}
	rows, err := srcDB.Query(`SELECT id, sort_index, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at FROM pipeline_items`)
	if err != nil {
		return err
	}
	defer rows.Close()

	tx, err := dstDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(platform.RebindSQL(platform.DBBackendPostgres, `
		INSERT INTO pipeline_items(id, sort_index, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			sort_index = EXCLUDED.sort_index,
			state = EXCLUDED.state,
			retry_count = EXCLUDED.retry_count,
			score_final = EXCLUDED.score_final,
			last_error = EXCLUDED.last_error,
			claimed_by = EXCLUDED.claimed_by,
			claimed_at = EXCLUDED.claimed_at,
			lease_until = EXCLUDED.lease_until,
			updated_at = EXCLUDED.updated_at
	`))
	if err != nil {
		return err
	}
	defer stmt.Close()

	for rows.Next() {
		var id, state, lastError, claimedBy, claimedAtRaw, leaseUntilRaw, updatedAtRaw string
		var sortIndex, retryCount int
		var scoreFinal float64
		if err := rows.Scan(&id, &sortIndex, &state, &retryCount, &scoreFinal, &lastError, &claimedBy, &claimedAtRaw, &leaseUntilRaw, &updatedAtRaw); err != nil {
			return err
		}
		if _, err := stmt.Exec(
			id,
			sortIndex,
			state,
			retryCount,
			scoreFinal,
			lastError,
			claimedBy,
			parseNullableSQLiteTime(claimedAtRaw),
			parseNullableSQLiteTime(leaseUntilRaw),
			parseSQLiteTime(updatedAtRaw),
		); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return tx.Commit()
}

func migratePipelineWorkerStats(srcDB, dstDB *sql.DB) error {
	exists, err := sqliteTableExists(srcDB, "pipeline_worker_stats")
	if err != nil || !exists {
		return err
	}
	rows, err := srcDB.Query(`SELECT worker_id, role, processed_count, elapsed_ms, started_at, finished_at FROM pipeline_worker_stats`)
	if err != nil {
		return err
	}
	defer rows.Close()

	tx, err := dstDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(platform.RebindSQL(platform.DBBackendPostgres, `
		INSERT INTO pipeline_worker_stats(worker_id, role, processed_count, elapsed_ms, started_at, finished_at)
		VALUES(?, ?, ?, ?, ?, ?)
	`))
	if err != nil {
		return err
	}
	defer stmt.Close()

	for rows.Next() {
		var workerID, role, startedAtRaw, finishedAtRaw string
		var processedCount int
		var elapsedMS int64
		if err := rows.Scan(&workerID, &role, &processedCount, &elapsedMS, &startedAtRaw, &finishedAtRaw); err != nil {
			return err
		}
		if _, err := stmt.Exec(workerID, role, processedCount, elapsedMS, parseSQLiteTime(startedAtRaw), parseSQLiteTime(finishedAtRaw)); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return tx.Commit()
}

func validateCounts(srcDB, dstDB *sql.DB) (*CheckpointToPostgresSummary, error) {
	tables := []string{"jobs", "items", "pipeline_items", "pipeline_worker_stats"}
	out := &CheckpointToPostgresSummary{Counts: make([]TableMigrationCount, 0, len(tables))}
	for _, table := range tables {
		srcCount, err := countRowsSQLite(srcDB, table)
		if err != nil {
			return nil, err
		}
		dstCount, err := countRowsPostgres(dstDB, table)
		if err != nil {
			return nil, err
		}
		out.Counts = append(out.Counts, TableMigrationCount{
			Table: table,
			Src:   srcCount,
			Dst:   dstCount,
		})
	}
	return out, nil
}

func sqliteTableExists(db *sql.DB, table string) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n)
	return n > 0, err
}

func countRowsSQLite(db *sql.DB, table string) (int64, error) {
	exists, err := sqliteTableExists(db, table)
	if err != nil || !exists {
		return 0, err
	}
	var n int64
	err = db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n)
	return n, err
}

func countRowsPostgres(db *sql.DB, table string) (int64, error) {
	var exists bool
	if err := db.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, "public."+table).Scan(&exists); err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}
	var n int64
	err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n)
	return n, err
}

func parseSQLiteTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Unix(0, 0).UTC()
	}
	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC()
		}
	}
	return time.Unix(0, 0).UTC()
}

func parseNullableSQLiteTime(raw string) any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return parseSQLiteTime(raw)
}

func (s *CheckpointToPostgresSummary) String() string {
	if s == nil {
		return ""
	}
	rows := make([]map[string]any, 0, len(s.Counts))
	for _, count := range s.Counts {
		rows = append(rows, map[string]any{
			"table": count.Table,
			"src":   count.Src,
			"dst":   count.Dst,
		})
	}
	raw, _ := json.Marshal(rows)
	return string(raw)
}
