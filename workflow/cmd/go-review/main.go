package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"localize-agent/workflow/pkg/platform"
)

const (
	dbModeEval       = "eval"
	dbModeCheckpoint = "checkpoint"
	checkpointRun    = "checkpoint"
)

type itemRow struct {
	RunName           string  `json:"run_name"`
	ID                string  `json:"id"`
	Status            string  `json:"status"`
	PipelineState     string  `json:"pipeline_state,omitempty"`
	RetryCount        int     `json:"retry_count"`
	RetryText         string  `json:"retry_text"`
	ScoreFinal        float64 `json:"score_final"`
	ScoreText         string  `json:"score_text"`
	PipelineError     string  `json:"pipeline_error,omitempty"`
	EN                string  `json:"en"`
	SourceRaw         string  `json:"source_raw,omitempty"`
	Original          string  `json:"original_ko"`
	FreshKO           string  `json:"fresh_ko,omitempty"`
	FinalKO           string  `json:"final_ko"`
	FinalRisk         string  `json:"final_risk"`
	FinalNote         string  `json:"final_notes"`
	Winner            string  `json:"winner,omitempty"`
	WinnerScore       string  `json:"winner_score,omitempty"`
	ReplacementKO     string  `json:"replacement_ko,omitempty"`
	CurrentScore      string  `json:"current_score,omitempty"`
	FreshScore        string  `json:"fresh_score,omitempty"`
	ScoreDecision     string  `json:"score_decision,omitempty"`
	ScoreDelta        string  `json:"score_delta,omitempty"`
	RewriteTriggered  string  `json:"rewrite_triggered,omitempty"`
	RetryReason       string  `json:"retry_reason,omitempty"`
	PipelineVersion   string  `json:"pipeline_version,omitempty"`
	SourceType        string  `json:"source_type,omitempty"`
	SourceFile        string  `json:"source_file,omitempty"`
	SceneHint         string  `json:"scene_hint,omitempty"`
	ResourceKey       string  `json:"resource_key,omitempty"`
	MetaPathLabel     string  `json:"meta_path_label,omitempty"`
	SegmentID         string  `json:"segment_id,omitempty"`
	SegmentPos        string  `json:"segment_pos,omitempty"`
	ChoiceBlockID     string  `json:"choice_block_id,omitempty"`
	PrevLineID        string  `json:"prev_line_id,omitempty"`
	NextLineID        string  `json:"next_line_id,omitempty"`
	MigratedFromOldID string  `json:"migrated_from_old_id,omitempty"`
	ChunkID           string  `json:"chunk_id,omitempty"`
	ParentSegmentID   string  `json:"parent_segment_id,omitempty"`
	TextRole          string  `json:"text_role,omitempty"`
	SpeakerHint       string  `json:"speaker_hint,omitempty"`
	LineFlags         string  `json:"line_flags,omitempty"`
	PrevEN            string  `json:"prev_en,omitempty"`
	PrevKO            string  `json:"prev_ko,omitempty"`
	NextEN            string  `json:"next_en,omitempty"`
	NextKO            string  `json:"next_ko,omitempty"`
	ChunkEN           string  `json:"chunk_en,omitempty"`
	RawKOJSON         string  `json:"raw_ko_json,omitempty"`
	RawPackJSON       string  `json:"raw_pack_json,omitempty"`
	Revised           bool    `json:"revised"`
	UpdatedAt         string  `json:"updated_at"`
	CompareStatus     string  `json:"compare_status,omitempty"`
	CompareFinalKO    string  `json:"compare_final_ko,omitempty"`
	CompareFinalRisk  string  `json:"compare_final_risk,omitempty"`
}

type updateRequest struct {
	RunName   string `json:"run_name"`
	ID        string `json:"id"`
	Status    string `json:"status"`
	FinalKO   string `json:"final_ko"`
	FinalRisk string `json:"final_risk"`
	FinalNote string `json:"final_notes"`
}

type deleteRunRequest struct {
	RunName string `json:"run_name"`
}

type forceDoneRequest struct {
	RunName   string `json:"run_name"`
	ID        string `json:"id"`
	FinalKO   string `json:"final_ko"`
	FinalRisk string `json:"final_risk"`
	FinalNote string `json:"final_notes"`
}

type app struct {
	db                 *sql.DB
	page               *template.Template
	dbBackend          string
	dbMode             string
	hasPipelineItems   bool
	hasPipelineWorkers bool
}

type workerStatRow struct {
	WorkerID       string  `json:"worker_id"`
	Role           string  `json:"role"`
	ProcessedCount int     `json:"processed_count"`
	ElapsedMs      int64   `json:"elapsed_ms"`
	ItemsPerSec    float64 `json:"items_per_sec"`
	FinishedAt     string  `json:"finished_at"`
}

type workerClaimRow struct {
	WorkerID   string `json:"worker_id"`
	State      string `json:"state"`
	Claimed    int    `json:"claimed"`
	ClaimedAt  string `json:"claimed_at"`
	LeaseUntil string `json:"lease_until"`
}

type workerOverviewRow struct {
	WorkerID        string  `json:"worker_id"`
	Role            string  `json:"role"`
	Status          string  `json:"status"`
	ActiveClaimed   int     `json:"active_claimed"`
	ActiveState     string  `json:"active_state"`
	LastProcessed   int     `json:"last_processed"`
	LastElapsedMs   int64   `json:"last_elapsed_ms"`
	LastItemsPerSec float64 `json:"last_items_per_sec"`
	LastFinishedAt  string  `json:"last_finished_at"`
}

func (a *app) rebind(query string) string {
	return platform.RebindSQL(a.dbBackend, query)
}

func scanTextRow(scanner interface{ Scan(...any) error }, dest ...*string) error {
	raw := make([]any, len(dest))
	args := make([]any, len(dest))
	for i := range raw {
		args[i] = &raw[i]
	}
	if err := scanner.Scan(args...); err != nil {
		return err
	}
	for i := range dest {
		*dest[i] = platform.NormalizeSQLValue(raw[i])
	}
	return nil
}

func main() {
	var dbPath string
	var dbBackend string
	var dbDSN string
	var addr string

	flag.StringVar(&dbPath, "db", "workflow/output/evaluation_unified.db", "evaluation or translation checkpoint DB path")
	flag.StringVar(&dbBackend, "db-backend", "sqlite", "database backend: sqlite|postgres")
	flag.StringVar(&dbDSN, "db-dsn", "", "database dsn for postgres backend")
	flag.StringVar(&addr, "addr", "127.0.0.1:8091", "listen address")
	flag.Parse()

	db, err := platform.OpenTranslationCheckpointDB(dbBackend, dbPath, dbDSN)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	mode, err := detectDBMode(dbBackend, db)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if mode == dbModeEval {
		if err := ensureEvalSchema(db); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	title := "Unified Evaluation DB Review"
	if mode == dbModeCheckpoint {
		title = "Translation Checkpoint DB Review"
	}
	hasPipelineItems := false
	hasPipelineWorkers := false
	if mode == dbModeCheckpoint {
		hasPipelineItems, err = hasTableForBackend(dbBackend, db, "pipeline_items")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		hasWorkerStats, err := hasTableForBackend(dbBackend, db, "pipeline_worker_stats")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		hasPipelineWorkers = hasPipelineItems && hasWorkerStats
	}
	a := &app{
		db:                 db,
		dbBackend:          dbBackend,
		dbMode:             mode,
		page:               template.Must(template.New("index").Parse(indexHTML)),
		hasPipelineItems:   hasPipelineItems,
		hasPipelineWorkers: hasPipelineWorkers,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleIndex)
	mux.HandleFunc("/api/runs", a.handleRuns)
	mux.HandleFunc("/api/stats", a.handleStats)
	mux.HandleFunc("/api/items", a.handleItems)
	mux.HandleFunc("/api/item", a.handleItem)
	mux.HandleFunc("/api/pipeline-worker-claims", a.handlePipelineWorkerClaims)
	mux.HandleFunc("/api/pipeline-worker-stats", a.handlePipelineWorkerStats)
	mux.HandleFunc("/api/pipeline-worker-overview", a.handlePipelineWorkerOverview)
	mux.HandleFunc("/api/update", a.handleUpdate)
	mux.HandleFunc("/api/force-done", a.handleForceDone)
	mux.HandleFunc("/api/delete-run", a.handleDeleteRun)

	log.Printf("review viewer: http://%s (db=%s, mode=%s, title=%s)", addr, dbPath, mode, title)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func detectDBMode(backend string, db *sql.DB) (string, error) {
	hasEval, err := hasTableForBackend(backend, db, "eval_items")
	if err != nil {
		return "", err
	}
	if hasEval {
		return dbModeEval, nil
	}
	hasItems, err := hasTableForBackend(backend, db, "items")
	if err != nil {
		return "", err
	}
	if hasItems {
		return dbModeCheckpoint, nil
	}
	return "", fmt.Errorf("unsupported DB: expected table eval_items or items")
}

func hasTable(db *sql.DB, name string) (bool, error) {
	return hasTableForBackend(platform.DBBackendSQLite, db, name)
}

func hasTableForBackend(backend string, db *sql.DB, name string) (bool, error) {
	var n int
	switch strings.TrimSpace(backend) {
	case platform.DBBackendPostgres:
		err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public' AND table_name=$1`, name).Scan(&n)
		return n > 0, err
	default:
		err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
		return n > 0, err
	}
}

func ensureEvalSchema(db *sql.DB) error {
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
	if _, err := db.Exec(`UPDATE eval_items SET source_id=id WHERE source_id=''`); err != nil {
		return err
	}
	_, err = db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_run_source_id ON eval_items(run_name, source_id)`)
	return err
}

func (a *app) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	data := map[string]any{
		"Title":        "Unified Evaluation DB Review",
		"Mode":         a.dbMode,
		"CompareOK":    a.dbMode == dbModeEval,
		"DeleteRunOK":  a.dbMode == dbModeEval,
		"StatusValues": "pending,evaluating,pass,revise,reject,applied",
	}
	if a.dbMode == dbModeCheckpoint {
		data["Title"] = "Translation Checkpoint DB Review"
		data["StatusValues"] = "new,done,error"
	}
	_ = a.page.Execute(w, data)
}

func (a *app) handleRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.dbMode == dbModeCheckpoint {
		var count int
		if err := a.db.QueryRow(`SELECT COUNT(*) FROM items`).Scan(&count); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{
			"runs": []map[string]any{{"run_name": checkpointRun, "count": count}},
			"mode": a.dbMode,
		})
		return
	}

	rows, err := a.db.Query(`SELECT run_name, COUNT(*) FROM eval_items GROUP BY run_name ORDER BY run_name`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type runInfo struct {
		RunName string `json:"run_name"`
		Count   int    `json:"count"`
	}
	out := []runInfo{}
	for rows.Next() {
		var x runInfo
		if err := rows.Scan(&x.RunName, &x.Count); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		out = append(out, x)
	}
	writeJSON(w, map[string]any{"runs": out, "mode": a.dbMode})
}

func (a *app) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	runName := strings.TrimSpace(r.URL.Query().Get("run"))
	if runName == "" {
		http.Error(w, "run is required", http.StatusBadRequest)
		return
	}
	if a.dbMode == dbModeCheckpoint {
		query := `
SELECT key, value FROM (
  SELECT 'item:' || status AS key, COUNT(*) AS value FROM items GROUP BY status
`
		if a.hasPipelineItems {
			query += `
  UNION ALL
  SELECT 'pipeline:' || state AS key, COUNT(*) AS value FROM pipeline_items GROUP BY state
`
		}
		query += `
) ORDER BY key`
		rows, err := a.db.Query(query)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		counts := map[string]int{}
		total := 0
		for rows.Next() {
			var s string
			var n int
			if err := rows.Scan(&s, &n); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			counts[s] = n
			if strings.HasPrefix(s, "item:") {
				total += n
			}
		}
		failedSummary, err := a.loadCheckpointFailedSummary()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"run_name": checkpointRun, "total": total, "counts": counts, "failed_summary": failedSummary, "mode": a.dbMode})
		return
	}

	rows, err := a.db.Query(`SELECT status, COUNT(*) FROM eval_items WHERE run_name=? GROUP BY status`, runName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	counts := map[string]int{}
	total := 0
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		counts[s] = n
		total += n
	}
	writeJSON(w, map[string]any{"run_name": runName, "total": total, "counts": counts, "mode": a.dbMode})
}

func (a *app) loadCheckpointFailedSummary() (map[string]any, error) {
	summary := map[string]any{
		"total":               0,
		"translator_no_row":   0,
		"low_score_max_retry": 0,
		"missing_score":       0,
		"current_empty":       0,
	}
	if !a.hasPipelineItems {
		return summary, nil
	}
	query := `
SELECT
  COUNT(*) FILTER (WHERE state = 'failed') AS total,
  COUNT(*) FILTER (WHERE state = 'failed' AND last_error = 'translator produced no done row') AS translator_no_row,
  COUNT(*) FILTER (WHERE state = 'failed' AND last_error LIKE 'max score % after max retries') AS low_score_max_retry,
  COUNT(*) FILTER (WHERE state = 'failed' AND last_error = 'model returned no score for item') AS missing_score,
  COUNT(*) FILTER (WHERE state = 'failed' AND last_error = 'winner=current but current_ko is empty') AS current_empty
FROM pipeline_items`
	var total, translatorNoRow, lowScoreMaxRetry, missingScore, currentEmpty int
	if err := a.db.QueryRow(query).Scan(&total, &translatorNoRow, &lowScoreMaxRetry, &missingScore, &currentEmpty); err != nil {
		return nil, err
	}
	summary["total"] = total
	summary["translator_no_row"] = translatorNoRow
	summary["low_score_max_retry"] = lowScoreMaxRetry
	summary["missing_score"] = missingScore
	summary["current_empty"] = currentEmpty
	return summary, nil
}

func (a *app) handleItems(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	runName := strings.TrimSpace(r.URL.Query().Get("run"))
	if runName == "" {
		http.Error(w, "run is required", http.StatusBadRequest)
		return
	}
	compareRun := strings.TrimSpace(r.URL.Query().Get("compare_run"))
	queryText := strings.TrimSpace(r.URL.Query().Get("q"))
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	statuses := parseCSV(r.URL.Query().Get("status"))
	pipelineVersion := strings.TrimSpace(r.URL.Query().Get("pipeline_version"))
	sortBy := strings.TrimSpace(r.URL.Query().Get("sort"))
	limit := parseIntDefault(r.URL.Query().Get("limit"), 200)
	if limit < 1 {
		limit = 1
	}
	if limit > 1000 {
		limit = 1000
	}

	pipelineState := strings.TrimSpace(r.URL.Query().Get("pipeline_state"))
	sceneHint := strings.TrimSpace(r.URL.Query().Get("scene_hint"))
	textRole := strings.TrimSpace(r.URL.Query().Get("text_role"))
	failedReason := strings.TrimSpace(r.URL.Query().Get("failed_reason"))
	if a.dbMode == dbModeCheckpoint {
		items, err := a.loadCheckpointItems(queryText, id, statuses, pipelineVersion, pipelineState, failedReason, sceneHint, textRole, sortBy, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"items": items, "count": len(items), "run_name": checkpointRun, "compare_run": "", "mode": a.dbMode, "pipeline_version": pipelineVersion, "sort": sortBy})
		return
	}

	parts := []string{"base.run_name = ?"}
	args := []any{runName}
	if id != "" {
		parts = append(parts, "base.source_id = ?")
		args = append(args, id)
	}
	if len(statuses) > 0 {
		ph := make([]string, len(statuses))
		for i, s := range statuses {
			ph[i] = "?"
			args = append(args, s)
		}
		parts = append(parts, "base.status IN ("+strings.Join(ph, ",")+")")
	}
	if queryText != "" {
		parts = append(parts, "(base.source_id LIKE ? OR base.en LIKE ? OR base.original_ko LIKE ? OR base.final_ko LIKE ?)")
		p := "%" + queryText + "%"
		args = append(args, p, p, p, p)
	}
	join := ""
	selectCompare := "'' AS compare_status, '' AS compare_final_ko, '' AS compare_final_risk"
	if compareRun != "" {
		join = "LEFT JOIN eval_items cmp ON cmp.run_name = ? AND cmp.source_id = base.source_id"
		args = append([]any{compareRun}, args...)
		selectCompare = "COALESCE(cmp.status,''), COALESCE(cmp.final_ko,''), COALESCE(cmp.final_risk,'')"
	}
	args = append(args, limit)

	sqlQ := `
SELECT base.run_name, base.source_id, base.status, base.en, base.original_ko, base.final_ko, base.final_risk, base.final_notes, base.revised, base.updated_at,
` + selectCompare + `
FROM eval_items base
` + join + `
WHERE ` + strings.Join(parts, " AND ") + `
ORDER BY base.updated_at DESC
LIMIT ?`

	rows, err := a.db.Query(sqlQ, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	items := make([]itemRow, 0, limit)
	for rows.Next() {
		var it itemRow
		var revised int
		if err := rows.Scan(
			&it.RunName, &it.ID, &it.Status, &it.EN, &it.Original, &it.FinalKO, &it.FinalRisk, &it.FinalNote, &revised, &it.UpdatedAt,
			&it.CompareStatus, &it.CompareFinalKO, &it.CompareFinalRisk,
		); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		it.Revised = revised == 1
		items = append(items, it)
	}
	writeJSON(w, map[string]any{"items": items, "count": len(items), "run_name": runName, "compare_run": compareRun, "mode": a.dbMode})
}

func (a *app) handlePipelineWorkerStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.dbMode != dbModeCheckpoint {
		writeJSON(w, map[string]any{"items": []workerStatRow{}})
		return
	}
	if !a.hasPipelineWorkers {
		writeJSON(w, map[string]any{"items": []workerStatRow{}})
		return
	}
	limit := parseIntDefault(r.URL.Query().Get("limit"), 30)
	if limit < 1 {
		limit = 1
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := a.db.Query(a.rebind(fmt.Sprintf(`
SELECT worker_id, role, processed_count, elapsed_ms, finished_at
FROM pipeline_worker_stats
ORDER BY finished_at DESC
LIMIT %d`, limit)))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := make([]workerStatRow, 0, limit)
	for rows.Next() {
		var it workerStatRow
		var finishedAt any
		if err := rows.Scan(&it.WorkerID, &it.Role, &it.ProcessedCount, &it.ElapsedMs, &finishedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		it.FinishedAt = platform.NormalizeSQLValue(finishedAt)
		if it.ElapsedMs > 0 {
			it.ItemsPerSec = float64(it.ProcessedCount) / (float64(it.ElapsedMs) / 1000.0)
		}
		out = append(out, it)
	}
	writeJSON(w, map[string]any{"items": out})
}

func (a *app) handlePipelineWorkerClaims(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.dbMode != dbModeCheckpoint {
		writeJSON(w, map[string]any{"items": []workerClaimRow{}})
		return
	}
	if !a.hasPipelineItems {
		writeJSON(w, map[string]any{"items": []workerClaimRow{}})
		return
	}
	rows, err := a.db.Query(a.rebind(`
SELECT claimed_by, state, COUNT(*), MIN(claimed_at), MAX(lease_until)
FROM pipeline_items
WHERE claimed_by <> '' AND state LIKE 'working_%'
GROUP BY claimed_by, state
ORDER BY claimed_by ASC, state ASC`))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := make([]workerClaimRow, 0, 16)
	for rows.Next() {
		var it workerClaimRow
		var claimedAt, leaseUntil any
		if err := rows.Scan(&it.WorkerID, &it.State, &it.Claimed, &claimedAt, &leaseUntil); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		it.ClaimedAt = platform.NormalizeSQLValue(claimedAt)
		it.LeaseUntil = platform.NormalizeSQLValue(leaseUntil)
		out = append(out, it)
	}
	writeJSON(w, map[string]any{"items": out})
}

func (a *app) handlePipelineWorkerOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.dbMode != dbModeCheckpoint || !a.hasPipelineItems {
		writeJSON(w, map[string]any{"items": []workerOverviewRow{}, "summary": map[string]int{}})
		return
	}
	recentWindow := time.Now().UTC().Add(-15 * time.Minute).Format(time.RFC3339)

	claims := map[string]workerOverviewRow{}
	rows, err := a.db.Query(a.rebind(`
SELECT claimed_by, state, COUNT(*)
FROM pipeline_items
WHERE claimed_by <> '' AND state LIKE 'working_%'
GROUP BY claimed_by, state
ORDER BY claimed_by ASC, state ASC`))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var workerID, state string
		var claimed int
		if err := rows.Scan(&workerID, &state, &claimed); err != nil {
			rows.Close()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		it := claims[workerID]
		it.WorkerID = workerID
		it.ActiveClaimed += claimed
		if it.ActiveState == "" {
			it.ActiveState = state
		}
		if it.Role == "" {
			it.Role = strings.TrimPrefix(state, "working_")
		}
		claims[workerID] = it
	}
	rows.Close()

	latestStats := map[string]workerOverviewRow{}
	if a.hasPipelineWorkers {
		rows, err = a.db.Query(a.rebind(`
SELECT worker_id, role, processed_count, elapsed_ms, finished_at
FROM pipeline_worker_stats
ORDER BY finished_at DESC`))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for rows.Next() {
			var it workerOverviewRow
			var finishedAt any
			if err := rows.Scan(&it.WorkerID, &it.Role, &it.LastProcessed, &it.LastElapsedMs, &finishedAt); err != nil {
				rows.Close()
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			it.LastFinishedAt = platform.NormalizeSQLValue(finishedAt)
			if _, exists := latestStats[it.WorkerID]; exists {
				continue
			}
			if it.LastElapsedMs > 0 {
				it.LastItemsPerSec = float64(it.LastProcessed) / (float64(it.LastElapsedMs) / 1000.0)
			}
			latestStats[it.WorkerID] = it
		}
		rows.Close()
	}

	merged := map[string]workerOverviewRow{}
	for workerID, it := range latestStats {
		merged[workerID] = it
	}
	for workerID, claim := range claims {
		it := merged[workerID]
		if it.WorkerID == "" {
			it.WorkerID = workerID
		}
		if it.Role == "" {
			it.Role = claim.Role
		}
		it.ActiveClaimed = claim.ActiveClaimed
		it.ActiveState = claim.ActiveState
		merged[workerID] = it
	}

	out := make([]workerOverviewRow, 0, len(merged))
	summary := map[string]int{
		"active_total":          0,
		"idle_recent_total":     0,
		"active_translate":      0,
		"active_score":          0,
		"idle_recent_translate": 0,
		"idle_recent_score":     0,
	}
	for _, it := range merged {
		if it.ActiveClaimed > 0 {
			it.Status = "active"
			summary["active_total"]++
			if it.Role == "translate" {
				summary["active_translate"]++
			} else if it.Role == "score" {
				summary["active_score"]++
			}
		} else if it.LastFinishedAt != "" && it.LastFinishedAt >= recentWindow {
			it.Status = "idle_recent"
			summary["idle_recent_total"]++
			if it.Role == "translate" {
				summary["idle_recent_translate"]++
			} else if it.Role == "score" {
				summary["idle_recent_score"]++
			}
		} else {
			it.Status = "stale"
		}
		if it.Status == "stale" {
			continue
		}
		out = append(out, it)
	}
	sort.Slice(out, func(i, j int) bool {
		rank := func(status string) int {
			switch status {
			case "active":
				return 0
			case "idle_recent":
				return 1
			default:
				return 2
			}
		}
		if rank(out[i].Status) != rank(out[j].Status) {
			return rank(out[i].Status) < rank(out[j].Status)
		}
		if out[i].Role != out[j].Role {
			return out[i].Role < out[j].Role
		}
		return out[i].WorkerID < out[j].WorkerID
	})
	writeJSON(w, map[string]any{"items": out, "summary": summary})
}

func checkpointSortOrder(sortBy string, hasPipelineItems bool) string {
	switch sortBy {
	case "score_asc":
		if !hasPipelineItems {
			return "i.updated_at DESC"
		}
		return "CASE WHEN COALESCE(p.score_final, -1) < 0 THEN 1 ELSE 0 END ASC, COALESCE(p.score_final, 0) ASC, i.updated_at DESC"
	case "score_desc":
		if !hasPipelineItems {
			return "i.updated_at DESC"
		}
		return "CASE WHEN COALESCE(p.score_final, -1) < 0 THEN 1 ELSE 0 END ASC, COALESCE(p.score_final, 0) DESC, i.updated_at DESC"
	case "retry_desc":
		if !hasPipelineItems {
			return "i.updated_at DESC"
		}
		return "COALESCE(p.retry_count, 0) DESC, COALESCE(p.score_final, 0) DESC, i.updated_at DESC"
	case "updated_asc":
		return "i.updated_at ASC"
	default:
		return "i.updated_at DESC"
	}
}

func failedReasonFilterClause(kind string) (string, []any, bool) {
	switch kind {
	case "":
		return "", nil, false
	case "translator_no_row":
		return "p.state = ? AND p.last_error = ?", []any{"failed", "translator produced no done row"}, true
	case "low_score_max_retry":
		return "p.state = ? AND p.last_error LIKE ?", []any{"failed", "max score % after max retries"}, true
	case "missing_score":
		return "p.state = ? AND p.last_error = ?", []any{"failed", "model returned no score for item"}, true
	case "current_empty":
		return "p.state = ? AND p.last_error = ?", []any{"failed", "winner=current but current_ko is empty"}, true
	default:
		return "", nil, false
	}
}

func (a *app) handleItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	runName := strings.TrimSpace(r.URL.Query().Get("run"))
	if runName == "" {
		http.Error(w, "run is required", http.StatusBadRequest)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if a.dbMode == dbModeCheckpoint {
		it, err := a.loadCheckpointItemByID(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if it == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeJSON(w, it)
		return
	}
	http.Error(w, "not supported in eval mode", http.StatusBadRequest)
}

func (a *app) loadCheckpointItemByID(id string) (*itemRow, error) {
	query := `
SELECT i.id, i.status, i.ko_json, i.pack_json, i.updated_at,
`
	if a.hasPipelineItems {
		query += "       COALESCE(p.state,''), COALESCE(p.retry_count,0), COALESCE(p.score_final,0), COALESCE(p.last_error,'')\n"
	} else {
		query += "       '' AS pipeline_state, 0 AS retry_count, -1 AS score_final, '' AS last_error\n"
	}
	query += `FROM items i
`
	if a.hasPipelineItems {
		query += "LEFT JOIN pipeline_items p ON p.id = i.id\n"
	}
	query += `WHERE i.id = ? LIMIT 1`

	row := a.db.QueryRow(a.rebind(query), id)
	var itemID, status, koJSON, packJSON, updatedAt string
	var pipelineState, pipelineError string
	var retryCount int
	var scoreFinal float64
	var koJSONRaw, packJSONRaw, updatedAtRaw any
	if err := row.Scan(&itemID, &status, &koJSONRaw, &packJSONRaw, &updatedAtRaw, &pipelineState, &retryCount, &scoreFinal, &pipelineError); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	koJSON = platform.NormalizeSQLValue(koJSONRaw)
	packJSON = platform.NormalizeSQLValue(packJSONRaw)
	updatedAt = platform.NormalizeSQLValue(updatedAtRaw)
	it := &itemRow{
		RunName:       checkpointRun,
		ID:            itemID,
		Status:        status,
		PipelineState: pipelineState,
		RetryCount:    retryCount,
		RetryText:     fmt.Sprintf("%d", retryCount),
		ScoreFinal:    scoreFinal,
		ScoreText:     scoreText(scoreFinal),
		PipelineError: pipelineError,
		UpdatedAt:     updatedAt,
		RawKOJSON:     koJSON,
		RawPackJSON:   packJSON,
	}
	var koObj map[string]any
	if strings.TrimSpace(koJSON) != "" {
		_ = json.Unmarshal([]byte(koJSON), &koObj)
	}
	var packObj map[string]any
	if strings.TrimSpace(packJSON) != "" {
		_ = json.Unmarshal([]byte(packJSON), &packObj)
	}
	it.EN = stringField(packObj, "en")
	it.SourceRaw = stringField(packObj, "source_raw")
	if it.SourceRaw != "" {
		it.EN = it.SourceRaw
	}
	it.Original = stringField(packObj, "current_ko")
	it.FreshKO = stringField(packObj, "fresh_ko")
	it.FinalKO = stringField(packObj, "proposed_ko_restored")
	if it.FinalKO == "" {
		it.FinalKO = stringField(koObj, "Text")
	}
	it.FinalRisk = stringField(packObj, "risk")
	it.FinalNote = stringField(packObj, "notes")
	it.Winner = stringField(packObj, "winner")
	it.WinnerScore = numericStringField(packObj, "winner_score")
	it.ReplacementKO = stringField(packObj, "replacement_ko")
	it.CurrentScore = numericStringField(packObj, "current_score")
	it.FreshScore = numericStringField(packObj, "fresh_score")
	it.ScoreDecision = stringField(packObj, "score_decision")
	it.ScoreDelta = numericStringField(packObj, "score_delta")
	it.RewriteTriggered = boolStringField(packObj, "rewrite_triggered")
	it.RetryReason = stringField(packObj, "retry_reason")
	it.PipelineVersion = stringField(packObj, "pipeline_version")
	it.SourceType = stringField(packObj, "source_type")
	it.SourceFile = stringField(packObj, "source_file")
	it.SceneHint = stringField(packObj, "scene_hint")
	it.ResourceKey = stringField(packObj, "resource_key")
	it.MetaPathLabel = stringField(packObj, "meta_path_label")
	it.SegmentID = stringField(packObj, "segment_id")
	it.SegmentPos = intStringField(packObj, "segment_pos")
	it.ChoiceBlockID = stringField(packObj, "choice_block_id")
	it.PrevLineID = stringField(packObj, "prev_line_id")
	it.NextLineID = stringField(packObj, "next_line_id")
	it.MigratedFromOldID = stringField(packObj, "migrated_from_old_id")
	it.ChunkID = stringField(packObj, "chunk_id")
	it.ParentSegmentID = stringField(packObj, "parent_segment_id")
	it.TextRole = stringField(packObj, "text_role")
	it.SpeakerHint = stringField(packObj, "speaker_hint")
	it.PrevEN = stringField(packObj, "prev_en")
	it.PrevKO = stringField(packObj, "prev_ko")
	it.NextEN = stringField(packObj, "next_en")
	it.NextKO = stringField(packObj, "next_ko")
	it.ChunkEN = stringField(packObj, "chunk_en")
	it.LineFlags = buildLineFlags(packObj)
	return it, nil
}

func (a *app) loadCheckpointItems(queryText, id string, statuses []string, pipelineVersion string, pipelineState string, failedReason string, sceneHint string, textRole string, sortBy string, limit int) ([]itemRow, error) {
	parts := []string{"1=1"}
	args := []any{}
	if id != "" {
		parts = append(parts, "i.id = ?")
		args = append(args, id)
	}
	if len(statuses) > 0 {
		ph := make([]string, len(statuses))
		for i, s := range statuses {
			ph[i] = "?"
			args = append(args, s)
		}
		parts = append(parts, "i.status IN ("+strings.Join(ph, ",")+")")
	}
	if queryText != "" {
		p := "%" + queryText + "%"
		if a.dbBackend == platform.DBBackendPostgres {
			parts = append(parts, "(i.id LIKE ? OR i.status LIKE ? OR i.pack_json::text LIKE ? OR i.ko_json::text LIKE ?)")
		} else {
			parts = append(parts, "(i.id LIKE ? OR i.status LIKE ? OR i.pack_json LIKE ? OR i.ko_json LIKE ?)")
		}
		args = append(args, p, p, p, p)
	}
	if pipelineVersion != "" {
		if a.dbBackend == platform.DBBackendPostgres {
			parts = append(parts, "(i.pack_json->>'pipeline_version' = ? OR i.pack_json::text LIKE ?)")
		} else {
			parts = append(parts, "(json_extract(i.pack_json, '$.pipeline_version') = ? OR i.pack_json LIKE ?)")
		}
		args = append(args, pipelineVersion, "%\"pipeline_version\":\""+pipelineVersion+"\"%")
	}
	if pipelineState != "" && a.hasPipelineItems {
		parts = append(parts, "p.state = ?")
		args = append(args, pipelineState)
	}
	if sceneHint != "" {
		if a.dbBackend == platform.DBBackendPostgres {
			parts = append(parts, "COALESCE(i.pack_json->>'scene_hint','') = ?")
		} else {
			parts = append(parts, "COALESCE(json_extract(i.pack_json, '$.scene_hint'),'') = ?")
		}
		args = append(args, sceneHint)
	}
	if textRole != "" {
		if a.dbBackend == platform.DBBackendPostgres {
			parts = append(parts, "COALESCE(i.pack_json->>'text_role','') = ?")
		} else {
			parts = append(parts, "COALESCE(json_extract(i.pack_json, '$.text_role'),'') = ?")
		}
		args = append(args, textRole)
	}
	if clause, extraArgs, ok := failedReasonFilterClause(failedReason); ok && a.hasPipelineItems {
		parts = append(parts, clause)
		args = append(args, extraArgs...)
	}
	args = append(args, limit)
	orderBy := checkpointSortOrder(sortBy, a.hasPipelineItems)
	query := `
SELECT i.id, i.status, i.ko_json, i.pack_json, i.updated_at,
`
	if a.hasPipelineItems {
		query += "       COALESCE(p.state,''), COALESCE(p.retry_count,0), COALESCE(p.score_final,0), COALESCE(p.last_error,'')\n"
	} else {
		query += "       '' AS pipeline_state, 0 AS retry_count, -1 AS score_final, '' AS last_error\n"
	}
	query += `FROM items i
`
	if a.hasPipelineItems {
		query += "LEFT JOIN pipeline_items p ON p.id = i.id\n"
	}
	query += `WHERE ` + strings.Join(parts, " AND ") + `
ORDER BY ` + orderBy + `
LIMIT ?`

	rows, err := a.db.Query(a.rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]itemRow, 0, limit)
	for rows.Next() {
		var id, status, koJSON, packJSON, updatedAt string
		var pipelineState, pipelineError string
		var retryCount int
		var scoreFinal float64
		var koJSONRaw, packJSONRaw, updatedAtRaw any
		if err := rows.Scan(&id, &status, &koJSONRaw, &packJSONRaw, &updatedAtRaw, &pipelineState, &retryCount, &scoreFinal, &pipelineError); err != nil {
			return nil, err
		}
		koJSON = platform.NormalizeSQLValue(koJSONRaw)
		packJSON = platform.NormalizeSQLValue(packJSONRaw)
		updatedAt = platform.NormalizeSQLValue(updatedAtRaw)
		it := itemRow{
			RunName:       checkpointRun,
			ID:            id,
			Status:        status,
			PipelineState: pipelineState,
			RetryCount:    retryCount,
			RetryText:     fmt.Sprintf("%d", retryCount),
			ScoreFinal:    scoreFinal,
			ScoreText:     scoreText(scoreFinal),
			PipelineError: pipelineError,
			UpdatedAt:     updatedAt,
			RawKOJSON:     koJSON,
			RawPackJSON:   packJSON,
		}
		var koObj map[string]any
		if strings.TrimSpace(koJSON) != "" {
			_ = json.Unmarshal([]byte(koJSON), &koObj)
		}
		var packObj map[string]any
		if strings.TrimSpace(packJSON) != "" {
			_ = json.Unmarshal([]byte(packJSON), &packObj)
		}
		it.EN = stringField(packObj, "en")
		it.SourceRaw = stringField(packObj, "source_raw")
		if it.SourceRaw != "" {
			it.EN = it.SourceRaw
		}
		it.Original = stringField(packObj, "current_ko")
		it.FreshKO = stringField(packObj, "fresh_ko")
		it.FinalKO = stringField(packObj, "proposed_ko_restored")
		if it.FinalKO == "" {
			it.FinalKO = stringField(koObj, "Text")
		}
		it.FinalRisk = stringField(packObj, "risk")
		it.FinalNote = stringField(packObj, "notes")
		it.Winner = stringField(packObj, "winner")
		it.WinnerScore = numericStringField(packObj, "winner_score")
		it.ReplacementKO = stringField(packObj, "replacement_ko")
		it.CurrentScore = numericStringField(packObj, "current_score")
		it.FreshScore = numericStringField(packObj, "fresh_score")
		it.ScoreDecision = stringField(packObj, "score_decision")
		it.ScoreDelta = numericStringField(packObj, "score_delta")
		it.RewriteTriggered = boolStringField(packObj, "rewrite_triggered")
		it.RetryReason = stringField(packObj, "retry_reason")
		it.PipelineVersion = stringField(packObj, "pipeline_version")
		it.SourceType = stringField(packObj, "source_type")
		it.SourceFile = stringField(packObj, "source_file")
		it.SceneHint = stringField(packObj, "scene_hint")
		it.ResourceKey = stringField(packObj, "resource_key")
		it.MetaPathLabel = stringField(packObj, "meta_path_label")
		it.SegmentID = stringField(packObj, "segment_id")
		it.SegmentPos = intStringField(packObj, "segment_pos")
		it.ChoiceBlockID = stringField(packObj, "choice_block_id")
		it.PrevLineID = stringField(packObj, "prev_line_id")
		it.NextLineID = stringField(packObj, "next_line_id")
		it.MigratedFromOldID = stringField(packObj, "migrated_from_old_id")
		it.ChunkID = stringField(packObj, "chunk_id")
		it.ParentSegmentID = stringField(packObj, "parent_segment_id")
		it.TextRole = stringField(packObj, "text_role")
		it.SpeakerHint = stringField(packObj, "speaker_hint")
		it.PrevEN = stringField(packObj, "prev_en")
		it.PrevKO = stringField(packObj, "prev_ko")
		it.NextEN = stringField(packObj, "next_en")
		it.NextKO = stringField(packObj, "next_ko")
		it.ChunkEN = stringField(packObj, "chunk_en")
		it.LineFlags = buildLineFlags(packObj)
		out = append(out, it)
	}
	return out, rows.Err()
}

func (a *app) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in updateRequest
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&in); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	in.RunName = strings.TrimSpace(in.RunName)
	in.ID = strings.TrimSpace(in.ID)
	in.Status = strings.TrimSpace(in.Status)
	if in.RunName == "" || in.ID == "" {
		http.Error(w, "run_name and id are required", http.StatusBadRequest)
		return
	}

	if a.dbMode == dbModeCheckpoint {
		if !isValidCheckpointStatus(in.Status) {
			http.Error(w, "invalid status", http.StatusBadRequest)
			return
		}
		n, err := a.updateCheckpointItem(in)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "updated": n})
		return
	}

	if !isValidEvalStatus(in.Status) {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}
	res, err := a.db.Exec(
		`UPDATE eval_items
		 SET status=?, final_ko=?, final_risk=?, final_notes=?, updated_at=?
		 WHERE run_name=? AND source_id=?`,
		in.Status, in.FinalKO, in.FinalRisk, in.FinalNote, time.Now().UTC().Format(time.RFC3339), in.RunName, in.ID,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	n, _ := res.RowsAffected()
	writeJSON(w, map[string]any{"ok": true, "updated": n})
}

func (a *app) handleForceDone(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.dbMode != dbModeCheckpoint {
		http.Error(w, "force-done is only supported in checkpoint mode", http.StatusBadRequest)
		return
	}
	var in forceDoneRequest
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&in); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	in.RunName = strings.TrimSpace(in.RunName)
	in.ID = strings.TrimSpace(in.ID)
	in.FinalKO = strings.TrimSpace(in.FinalKO)
	in.FinalRisk = strings.TrimSpace(in.FinalRisk)
	in.FinalNote = strings.TrimSpace(in.FinalNote)
	if in.RunName == "" || in.ID == "" || in.FinalKO == "" {
		http.Error(w, "run_name, id, final_ko are required", http.StatusBadRequest)
		return
	}
	if err := a.forceDoneCheckpointItem(in); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "forced_done": 1})
}

func (a *app) updateCheckpointItem(in updateRequest) (int64, error) {
	var koJSON, packJSON string
	var koJSONRaw, packJSONRaw any
	err := a.db.QueryRow(a.rebind(`SELECT ko_json, pack_json FROM items WHERE id=?`), in.ID).Scan(&koJSONRaw, &packJSONRaw)
	if err != nil {
		return 0, err
	}
	koJSON = platform.NormalizeSQLValue(koJSONRaw)
	packJSON = platform.NormalizeSQLValue(packJSONRaw)
	koObj := map[string]any{}
	if strings.TrimSpace(koJSON) != "" {
		_ = json.Unmarshal([]byte(koJSON), &koObj)
	}
	packObj := map[string]any{}
	if strings.TrimSpace(packJSON) != "" {
		_ = json.Unmarshal([]byte(packJSON), &packObj)
	}
	koObj["Text"] = in.FinalKO
	packObj["proposed_ko_restored"] = in.FinalKO
	packObj["risk"] = in.FinalRisk
	packObj["notes"] = in.FinalNote

	koRaw, err := json.Marshal(koObj)
	if err != nil {
		return 0, err
	}
	packRaw, err := json.Marshal(packObj)
	if err != nil {
		return 0, err
	}
	updateQuery := `UPDATE items SET status=?, ko_json=?, pack_json=?, updated_at=? WHERE id=?`
	if a.dbBackend == platform.DBBackendPostgres {
		updateQuery = `UPDATE items SET status=?, ko_json=?::jsonb, pack_json=?::jsonb, updated_at=? WHERE id=?`
	}
	res, err := a.db.Exec(
		a.rebind(updateQuery),
		in.Status, string(koRaw), string(packRaw), time.Now().UTC().Format(time.RFC3339), in.ID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (a *app) forceDoneCheckpointItem(in forceDoneRequest) error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var koJSON, packJSON string
	var koJSONRaw, packJSONRaw any
	if err := tx.QueryRow(a.rebind(`SELECT ko_json, pack_json FROM items WHERE id=?`), in.ID).Scan(&koJSONRaw, &packJSONRaw); err != nil {
		return err
	}
	koJSON = platform.NormalizeSQLValue(koJSONRaw)
	packJSON = platform.NormalizeSQLValue(packJSONRaw)

	koObj := map[string]any{}
	if strings.TrimSpace(koJSON) != "" {
		_ = json.Unmarshal([]byte(koJSON), &koObj)
	}
	packObj := map[string]any{}
	if strings.TrimSpace(packJSON) != "" {
		_ = json.Unmarshal([]byte(packJSON), &packObj)
	}

	koObj["Text"] = in.FinalKO
	packObj["current_ko"] = in.FinalKO
	packObj["fresh_ko"] = in.FinalKO
	packObj["proposed_ko_restored"] = in.FinalKO
	packObj["risk"] = in.FinalRisk
	packObj["notes"] = in.FinalNote
	packObj["winner"] = "manual"
	packObj["winner_score"] = 100
	packObj["current_score"] = 100
	packObj["fresh_score"] = 100
	packObj["score_delta"] = 0
	packObj["score_decision"] = "manual_force_done"
	packObj["rewrite_triggered"] = false
	packObj["retry_reason"] = ""

	koRaw, err := json.Marshal(koObj)
	if err != nil {
		return err
	}
	packRaw, err := json.Marshal(packObj)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	updateQuery := `UPDATE items SET status=?, ko_json=?, pack_json=?, updated_at=? WHERE id=?`
	if a.dbBackend == platform.DBBackendPostgres {
		updateQuery = `UPDATE items SET status=?, ko_json=?::jsonb, pack_json=?::jsonb, updated_at=? WHERE id=?`
	}
	if _, err := tx.Exec(a.rebind(updateQuery), "done", string(koRaw), string(packRaw), now, in.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(a.rebind(`UPDATE pipeline_items SET state=?, retry_count=0, score_final=?, last_error='', claimed_by='', claimed_at=NULL, lease_until=NULL, updated_at=? WHERE id=?`), "done", 100, now, in.ID); err != nil {
		return err
	}
	if err := a.unlockNextTranslateReview(tx, in.ID, now); err != nil {
		return err
	}
	if err := a.unlockPrevScoreReview(tx, in.ID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *app) checkpointRowDoneReview(tx *sql.Tx, id string) (bool, error) {
	var status string
	err := tx.QueryRow(a.rebind(`SELECT status FROM items WHERE id=?`), id).Scan(&status)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return status == "done", nil
}

func (a *app) checkpointLineIDFromPackReview(tx *sql.Tx, id string, key string) (string, error) {
	var packJSONRaw any
	err := tx.QueryRow(a.rebind(`SELECT pack_json FROM items WHERE id=?`), id).Scan(&packJSONRaw)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	packJSON := platform.NormalizeSQLValue(packJSONRaw)
	if strings.TrimSpace(packJSON) == "" {
		return "", nil
	}
	var packObj map[string]any
	if err := json.Unmarshal([]byte(packJSON), &packObj); err != nil {
		return "", nil
	}
	return strings.TrimSpace(stringField(packObj, key)), nil
}

func (a *app) unlockNextTranslateReview(tx *sql.Tx, id string, now string) error {
	nextID, err := a.checkpointLineIDFromPackReview(tx, id, "next_line_id")
	if err != nil || strings.TrimSpace(nextID) == "" {
		return err
	}
	prevID, err := a.checkpointLineIDFromPackReview(tx, nextID, "prev_line_id")
	if err != nil {
		return err
	}
	if strings.TrimSpace(prevID) != "" {
		prevDone, err := a.checkpointRowDoneReview(tx, prevID)
		if err != nil || !prevDone {
			return err
		}
	}
	_, err = tx.Exec(a.rebind(`UPDATE pipeline_items SET state=?, last_error='', claimed_by='', claimed_at=NULL, lease_until=NULL, updated_at=? WHERE id=? AND state=?`), "pending_translate", now, nextID, "blocked_translate")
	return err
}

func (a *app) unlockPrevScoreReview(tx *sql.Tx, id string, now string) error {
	prevID, err := a.checkpointLineIDFromPackReview(tx, id, "prev_line_id")
	if err != nil || strings.TrimSpace(prevID) == "" {
		return err
	}
	prevDone, err := a.checkpointRowDoneReview(tx, prevID)
	if err != nil || !prevDone {
		return err
	}
	_, err = tx.Exec(a.rebind(`UPDATE pipeline_items SET state=?, last_error='', claimed_by='', claimed_at=NULL, lease_until=NULL, updated_at=? WHERE id=? AND state=?`), "pending_score", now, prevID, "blocked_score")
	return err
}

func (a *app) handleDeleteRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.dbMode == dbModeCheckpoint {
		http.Error(w, "delete-run is not supported for checkpoint DBs", http.StatusBadRequest)
		return
	}

	var in deleteRunRequest
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&in); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	in.RunName = strings.TrimSpace(in.RunName)
	if in.RunName == "" {
		http.Error(w, "run_name is required", http.StatusBadRequest)
		return
	}
	res, err := a.db.Exec(`DELETE FROM eval_items WHERE run_name=?`, in.RunName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	n, _ := res.RowsAffected()
	writeJSON(w, map[string]any{"ok": true, "deleted": n})
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}

func boolField(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	v, ok := m[key]
	if !ok {
		return false
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "true"
	default:
		return false
	}
}

func boolStringField(m map[string]any, key string) string {
	if !boolField(m, key) {
		if m == nil {
			return ""
		}
		if v, ok := m[key]; ok {
			switch x := v.(type) {
			case string:
				return x
			case bool:
				if !x {
					return "false"
				}
			}
		}
		return ""
	}
	return "true"
}

func intStringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case float64:
		return fmt.Sprintf("%.0f", x)
	case int:
		return fmt.Sprintf("%d", x)
	case int64:
		return fmt.Sprintf("%d", x)
	case json.Number:
		return x.String()
	case string:
		return x
	default:
		return fmt.Sprintf("%v", x)
	}
}

func numericStringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case float64:
		return fmt.Sprintf("%.4f", x)
	case int:
		return fmt.Sprintf("%d", x)
	case int64:
		return fmt.Sprintf("%d", x)
	case json.Number:
		return x.String()
	case string:
		return x
	default:
		return fmt.Sprintf("%v", x)
	}
}

func buildLineFlags(packObj map[string]any) string {
	flags := make([]string, 0, 2)
	if boolField(packObj, "line_is_imperative") {
		flags = append(flags, "imperative")
	}
	if boolField(packObj, "line_is_short_context_dependent") {
		flags = append(flags, "short-context")
	}
	return strings.Join(flags, ", ")
}

func parseCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func parseIntDefault(raw string, fallback int) int {
	var n int
	if _, err := fmt.Sscanf(raw, "%d", &n); err != nil {
		return fallback
	}
	return n
}

func scoreText(score float64) string {
	if score < 0 {
		return "pending"
	}
	return fmt.Sprintf("%.2f", score)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.Header().Set("cache-control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func isValidEvalStatus(s string) bool {
	switch s {
	case "pending", "evaluating", "pass", "revise", "reject", "applied":
		return true
	default:
		return false
	}
}

func isValidCheckpointStatus(s string) bool {
	switch s {
	case "new", "done", "error":
		return true
	default:
		return false
	}
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

const indexHTML = `<!doctype html>
<html lang="ko">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>{{.Title}}</title>
  <style>
    body { font-family: "Segoe UI", sans-serif; margin: 16px; background: #f4f7fb; color: #111827; }
    h1 { margin: 0 0 12px; font-size: 20px; }
    .row { display: flex; gap: 8px; flex-wrap: wrap; margin-bottom: 10px; }
    .tabs { display: flex; gap: 8px; margin-bottom: 10px; }
    .tab-btn { background: #e2e8f0; color: #0f172a; }
    .tab-btn.active { background: #0f766e; color: white; }
    .tab-pane { display: none; }
    .tab-pane.active { display: block; }
    input, select, button, textarea { font-size: 13px; padding: 8px; border: 1px solid #cbd5e1; border-radius: 6px; }
    button { background: #0f766e; color: white; border: none; cursor: pointer; }
    button.secondary { background: #334155; }
    button.warn { background: #b45309; }
    button.danger { background: #b91c1c; }
    button[disabled] { opacity: 0.5; cursor: not-allowed; }
    .panel { background: white; border: 1px solid #e2e8f0; border-radius: 8px; padding: 10px; margin-bottom: 10px; }
    table { width: 100%; border-collapse: collapse; font-size: 12px; }
    th, td { border-bottom: 1px solid #e5e7eb; text-align: left; padding: 6px; vertical-align: top; }
    tr:hover { background: #f8fafc; cursor: pointer; }
    .muted { color: #64748b; font-size: 12px; }
    #editor textarea { width: 100%; min-height: 65px; margin-bottom: 8px; }
    .cols { display: block; }
    .list-panel { width: 100%; }
    .editor-sidecar {
      position: fixed;
      top: 0;
      right: 0;
      width: min(720px, 88vw);
      height: 100vh;
      margin: 0;
      border-radius: 0;
      border-top: none;
      border-right: none;
      border-bottom: none;
      box-shadow: -10px 0 24px rgba(15, 23, 42, 0.16);
      overflow-y: auto;
      z-index: 40;
      transform: translateX(100%);
      transition: transform .18s ease;
    }
    .editor-sidecar.open { transform: translateX(0); }
    .editor-sidecar-header {
      display:flex;
      align-items:center;
      justify-content:space-between;
      gap:12px;
      position: sticky;
      top: 0;
      background: white;
      padding-bottom: 8px;
      margin-bottom: 8px;
      border-bottom: 1px solid #e5e7eb;
    }
    .editor-sidecar-close {
      background:#334155;
      color:white;
      border:none;
      border-radius:6px;
      padding:6px 10px;
      cursor:pointer;
    }
    .editor-backdrop {
      position: fixed;
      inset: 0;
      background: rgba(15, 23, 42, 0.26);
      z-index: 30;
      display: none;
    }
    .editor-backdrop.show { display: block; }
    .cmp { color: #0f766e; font-size: 11px; }
    .mini-grid { display: grid; gap: 4px; }
    .mini-cell { border: 1px solid #dbe4f0; border-radius: 6px; background: #f8fafc; padding: 4px 6px; }
    .mini-label { display: block; font-size: 10px; color: #64748b; text-transform: uppercase; letter-spacing: 0.03em; margin-bottom: 2px; }
    .mini-value { display: block; font-size: 12px; color: #0f172a; word-break: break-word; }
    .pill { display: inline-block; padding: 2px 6px; border-radius: 999px; font-size: 11px; font-weight: 600; margin: 0 6px 4px 0; }
    .pill.state { background: #d1fae5; color: #065f46; }
    .pill.retry { background: #fef3c7; color: #92400e; }
    .pill.item { background: #dbeafe; color: #1d4ed8; }
    .stats-block { margin-top: 10px; }
    .stats-heading { font-size: 12px; font-weight: 700; color: #334155; margin: 10px 0 6px; text-transform: uppercase; letter-spacing: 0.04em; }
    .stats-grid { display:grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap:8px; margin-top:8px; }
    .stats-card { border: 1px solid #dbe4f0; border-radius: 8px; background: white; padding: 8px 10px; }
    .stats-card.clickable { cursor: pointer; transition: transform .08s ease, box-shadow .08s ease, border-color .08s ease; }
    .stats-card.clickable:hover { transform: translateY(-1px); box-shadow: 0 4px 10px rgba(15,23,42,.08); border-color: #93c5fd; }
    .stats-card .stats-title { font-size: 10px; color: #64748b; text-transform: uppercase; letter-spacing: 0.03em; margin-bottom: 4px; }
    .stats-card .stats-value { font-size: 18px; font-weight: 700; color: #0f172a; }
    .stats-card .stats-sub { font-size: 11px; color: #475569; margin-top: 2px; white-space: pre-wrap; }
    .stats-card.failed { border-color: #fecaca; background: #fff7f7; }
    .stats-card.failed .stats-title { color: #991b1b; }
    .stats-card.failed .stats-value { color: #7f1d1d; }
    .active-metric-filter { display:none; align-items:center; gap:8px; margin: 10px 0 0; padding: 8px 10px; border:1px solid #cbd5e1; border-radius:8px; background:#f8fafc; }
    .active-metric-filter.show { display:flex; }
    .active-metric-filter .label { font-size: 12px; color:#334155; font-weight:600; }
    .active-metric-filter .value { font-size: 12px; color:#0f172a; }
    .active-metric-filter button { border:1px solid #cbd5e1; background:white; border-radius:6px; padding:4px 8px; cursor:pointer; }
    @media (max-width: 900px) {
      .editor-sidecar { width: 100vw; }
    }
  </style>
</head>
<body>
  <h1>{{.Title}}</h1>
  <div class="tabs">
    <button id="tab-items-btn" class="tab-btn active" onclick="showTab('items')">Items</button>
    <button id="tab-workers-btn" class="tab-btn" onclick="showTab('workers')">Workers</button>
  </div>

  <div id="tab-items" class="tab-pane active">
    <div class="panel">
      <div class="muted">mode: {{.Mode}}</div>
      <div class="row">
        <label>Run <select id="run"></select></label>
        <label>Compare <select id="compare" {{if not .CompareOK}}disabled{{end}}><option value="">(none)</option></select></label>
        <button class="secondary" onclick="loadRuns()">Refresh runs</button>
        <button class="danger" id="delete-run-btn" onclick="deleteRun()" {{if not .DeleteRunOK}}disabled{{end}}>Delete selected run</button>
      </div>
      <div class="row">
        <input id="q" placeholder="search id / en / ko" style="min-width:240px;">
        <input id="status" value="" placeholder="status csv (empty = all)">
        <input id="pipeline_version" value="" placeholder="pipeline_version (checkpoint only)" style="min-width:220px;">
        <input id="scene_hint" value="" placeholder="scene_hint (checkpoint only)" style="min-width:220px;">
        <select id="text_role_filter" style="min-width:150px;">
          <option value="">role all</option>
          <option value="dialogue">dialogue</option>
          <option value="narration">narration</option>
          <option value="reaction">reaction</option>
          <option value="choice">choice</option>
          <option value="fragment">fragment</option>
        </select>
        <select id="sort" style="min-width:170px;">
          <option value="updated_desc">updated desc</option>
          <option value="score_desc">score desc</option>
          <option value="score_asc">score asc</option>
          <option value="retry_desc">retry desc</option>
          <option value="updated_asc">updated asc</option>
        </select>
        <input id="limit" value="100" style="width:90px;">
        <button onclick="loadItems()">Load</button>
        <button class="secondary" onclick="loadStats()">Stats</button>
      </div>
      <div id="stats"></div>
      <div id="active-metric-filter" class="active-metric-filter">
        <span class="label">metric filter</span>
        <span id="active-metric-filter-value" class="value"></span>
        <button type="button" onclick="clearMetricFilter()">clear</button>
      </div>
    </div>

    <div class="cols">
      <div class="panel list-panel">
        <table id="tbl">
          <thead>
            <tr><th>id</th><th>pipeline</th><th>identity</th><th>text</th><th>score</th><th>updated</th><th>final_ko</th></tr>
          </thead>
          <tbody></tbody>
        </table>
      </div>
      <div id="editor-backdrop" class="editor-backdrop" onclick="closeEditor()"></div>
      <div id="editor" class="panel editor-sidecar">
        <div class="editor-sidecar-header">
          <div>
            <div class="muted">Selected row</div>
            <div>Run: <span id="erun"></span> | ID: <span id="eid"></span></div>
          </div>
          <div style="display:flex; gap:8px; align-items:center;">
            <button type="button" class="editor-sidecar-close" id="scene-dialogue-btn" onclick="filterCurrentSceneDialogue()" disabled>Scene Dialogue</button>
            <button type="button" class="editor-sidecar-close" id="prev-row-btn" onclick="jumpLinkedRow('prev')" disabled>Prev</button>
            <button type="button" class="editor-sidecar-close" id="next-row-btn" onclick="jumpLinkedRow('next')" disabled>Next</button>
            <button type="button" class="editor-sidecar-close" onclick="closeEditor()">Close</button>
          </div>
        </div>
        <div class="row">
          <select id="estatus"></select>
          <input id="erisk" placeholder="risk">
        </div>
        <div class="muted">EN</div>
        <textarea id="een" readonly></textarea>
        <div class="muted">Context Meta</div>
        <textarea id="emeta" readonly></textarea>
        <div class="muted">Original KO</div>
        <textarea id="eorig" readonly></textarea>
        <div class="muted">Chunk EN</div>
        <textarea id="echunk" readonly></textarea>
        <div class="muted">Final KO</div>
        <textarea id="efinal"></textarea>
        <div class="muted">Notes</div>
        <textarea id="enote"></textarea>
        <div class="muted">Pipeline version</div>
        <textarea id="epipeline" readonly></textarea>
        <div class="muted">Pipeline state</div>
        <textarea id="epipelinestate" readonly></textarea>
        <div class="muted">Prev EN</div>
        <textarea id="epreven" readonly></textarea>
        <div class="muted">Prev KO (checkpoint raw)</div>
        <textarea id="eprev" readonly></textarea>
        <div class="muted">Next EN</div>
        <textarea id="enexten" readonly></textarea>
        <div class="muted">Next KO (checkpoint raw)</div>
        <textarea id="enext" readonly></textarea>
        <div class="muted">Raw pack_json</div>
        <textarea id="epack" readonly></textarea>
        <div class="muted">Raw ko_json</div>
        <textarea id="ekojson" readonly></textarea>
        <div class="cmp">Compare status: <span id="cstatus"></span></div>
        <div class="cmp">Compare risk: <span id="crisk"></span></div>
        <div class="cmp">Compare final_ko: <span id="cko"></span></div>
        <div class="row">
          <button onclick="saveCurrent()">Save</button>
          <button class="warn" onclick="forceDoneCurrent()">Force Done 100</button>
          <button class="secondary" onclick="setStatus(defaultPassStatus())">Quick status</button>
        </div>
        <div id="msg" class="muted"></div>
      </div>
    </div>
  </div>

  <div id="tab-workers" class="tab-pane">
    <div class="panel">
      <div class="muted">Worker summary</div>
      <div id="wsummary" style="margin-top:6px;"></div>
      <div class="muted" style="margin-top:12px;">Active and recent workers</div>
      <table id="woverview" style="margin-top:6px;">
        <thead>
          <tr><th>worker</th><th>role</th><th>status</th><th>active_claimed</th><th>active_state</th><th>last_processed</th><th>last_items/sec</th><th>last_finished</th></tr>
        </thead>
        <tbody></tbody>
      </table>
      <div class="muted">Active workers</div>
      <table id="wclaims" style="margin-top:6px;">
        <thead>
          <tr><th>worker</th><th>state</th><th>claimed</th><th>claimed_at</th><th>lease_until</th></tr>
        </thead>
        <tbody></tbody>
      </table>
      <div class="muted" style="margin-top:12px;">Recent worker throughput</div>
      <table id="wtbl" style="margin-top:6px;">
        <thead>
          <tr><th>worker</th><th>role</th><th>processed</th><th>elapsed_ms</th><th>items/sec</th><th>finished</th></tr>
        </thead>
        <tbody></tbody>
      </table>
    </div>
  </div>

<script>
const MODE = {{printf "%q" .Mode}};
const STATUS_VALUES = {{printf "%q" .StatusValues}}.split(',');
let current = null;
let activeMetricFilter = { status: '', pipeline_state: '', failed_reason: '', label: '' };

function openEditor() {
  const editor = document.getElementById('editor');
  const backdrop = document.getElementById('editor-backdrop');
  if (editor) editor.classList.add('open');
  if (backdrop) backdrop.classList.add('show');
}

function closeEditor() {
  const editor = document.getElementById('editor');
  const backdrop = document.getElementById('editor-backdrop');
  if (editor) editor.classList.remove('open');
  if (backdrop) backdrop.classList.remove('show');
}

function showTab(name) {
  document.getElementById('tab-items').classList.toggle('active', name === 'items');
  document.getElementById('tab-workers').classList.toggle('active', name === 'workers');
  document.getElementById('tab-items-btn').classList.toggle('active', name === 'items');
  document.getElementById('tab-workers-btn').classList.toggle('active', name === 'workers');
}

function runValue() { return document.getElementById('run').value; }
function compareValue() { return document.getElementById('compare').value; }
function defaultPassStatus() { return MODE === 'checkpoint' ? 'done' : 'pass'; }

function initStatusOptions() {
  const el = document.getElementById('estatus');
  el.innerHTML = '';
  STATUS_VALUES.forEach(v => el.add(new Option(v, v)));
}

async function loadRuns() {
  const r = await fetch('/api/runs');
  const d = await r.json();
  const run = document.getElementById('run');
  const cmp = document.getElementById('compare');
  const prevRun = run.value;
  const prevCmp = cmp.value;
  run.innerHTML = '';
  cmp.innerHTML = '<option value="">(none)</option>';
  d.runs.forEach(x => {
    const label = x.run_name + ' (' + x.count + ')';
    run.add(new Option(label, x.run_name));
    if (!cmp.disabled) cmp.add(new Option(label, x.run_name));
  });
  if (prevRun && [...run.options].some(o => o.value === prevRun)) run.value = prevRun;
  if (!run.value && run.options.length > 0) run.selectedIndex = 0;
  if (!cmp.disabled && prevCmp && [...cmp.options].some(o => o.value === prevCmp)) cmp.value = prevCmp;
  await loadStats();
  await loadItems();
}

async function loadStats() {
  const run = runValue();
  if (!run) { document.getElementById('stats').textContent = 'no runs'; return; }
  const r = await fetch('/api/stats?' + new URLSearchParams({run}));
  const d = await r.json();
  renderStatsSummary(d);
  await loadWorkerClaims();
  await loadWorkerOverview();
  await loadWorkerStats();
}

async function loadWorkerOverview() {
  const tb = document.querySelector('#woverview tbody');
  const summary = document.getElementById('wsummary');
  if (!tb || !summary) return;
  const r = await fetch('/api/pipeline-worker-overview');
  const d = await r.json();
  tb.innerHTML = '';
  const s = d.summary || {};
  summary.textContent =
    'active total=' + String(s.active_total || 0) +
    ' | active translate=' + String(s.active_translate || 0) +
    ' | active score=' + String(s.active_score || 0) +
    ' | idle recent total=' + String(s.idle_recent_total || 0) +
    ' | idle recent translate=' + String(s.idle_recent_translate || 0) +
    ' | idle recent score=' + String(s.idle_recent_score || 0);
  (d.items || []).forEach(it => {
    const tr = document.createElement('tr');
    tr.innerHTML =
      '<td>' + esc(it.worker_id) + '</td>' +
      '<td>' + esc(it.role || '') + '</td>' +
      '<td>' + esc(it.status || '') + '</td>' +
      '<td>' + esc(String(it.active_claimed || 0)) + '</td>' +
      '<td>' + esc(it.active_state || '') + '</td>' +
      '<td>' + esc(String(it.last_processed || 0)) + '</td>' +
      '<td>' + esc(Number(it.last_items_per_sec || 0).toFixed(2)) + '</td>' +
      '<td>' + esc(it.last_finished_at || '') + '</td>';
    tb.appendChild(tr);
  });
}

async function loadWorkerClaims() {
  const tb = document.querySelector('#wclaims tbody');
  if (!tb) return;
  const r = await fetch('/api/pipeline-worker-claims');
  const d = await r.json();
  tb.innerHTML = '';
  (d.items || []).forEach(it => {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td>' + esc(it.worker_id) + '</td><td>' + esc(it.state) + '</td><td>' + esc(String(it.claimed)) + '</td><td>' + esc(it.claimed_at || '') + '</td><td>' + esc(it.lease_until || '') + '</td>';
    tb.appendChild(tr);
  });
}

async function loadWorkerStats() {
  const tb = document.querySelector('#wtbl tbody');
  if (!tb) return;
  const r = await fetch('/api/pipeline-worker-stats?limit=20');
  const d = await r.json();
  tb.innerHTML = '';
  (d.items || []).forEach(it => {
    const tr = document.createElement('tr');
    tr.innerHTML = '<td>' + esc(it.worker_id) + '</td><td>' + esc(it.role) + '</td><td>' + esc(String(it.processed_count)) + '</td><td>' + esc(String(it.elapsed_ms)) + '</td><td>' + esc(Number(it.items_per_sec || 0).toFixed(2)) + '</td><td>' + esc(it.finished_at || '') + '</td>';
    tb.appendChild(tr);
  });
}

async function loadItems() {
  const run = runValue();
  if (!run) { document.getElementById('msg').textContent = 'no runs'; return; }
  const q = document.getElementById('q').value.trim();
  const status = activeMetricFilter.status || document.getElementById('status').value.trim();
  const pipeline_version = document.getElementById('pipeline_version').value.trim();
  const scene_hint = document.getElementById('scene_hint').value.trim();
  const text_role = document.getElementById('text_role_filter').value.trim();
  const sort = document.getElementById('sort').value.trim() || 'updated_desc';
  const limit = document.getElementById('limit').value.trim() || '100';
  const compare_run = document.getElementById('compare').disabled ? '' : compareValue();
  const pipeline_state = activeMetricFilter.pipeline_state || '';
  const failed_reason = activeMetricFilter.failed_reason || '';
  const url = '/api/items?' + new URLSearchParams({run, compare_run, q, status, pipeline_version, scene_hint, text_role, pipeline_state, failed_reason, sort, limit});
  const r = await fetch(url);
  const d = await r.json();
  const tb = document.querySelector('#tbl tbody');
  tb.innerHTML = '';
  d.items.forEach(it => {
    const tr = document.createElement('tr');
    const pipelineTag = [
      pill('state', it.pipeline_state || '', 'state'),
      pill('retry', (it.retry_count || it.retry_count === 0) ? it.retry_count : '', 'retry'),
      pill('item', it.status || '', 'item'),
      it.pipeline_error ? miniCell('error', it.pipeline_error) : ''
    ].filter(Boolean).join('');
    const identityTag = [
      miniCell('role', it.text_role || ''),
      miniCell('speaker', it.speaker_hint || ''),
      miniCell('scene', it.scene_hint || ''),
      miniCell('source', it.source_type || ''),
      miniCell('file', it.source_file || ''),
      miniCell('resource', it.resource_key || ''),
      miniCell('segment', it.segment_id ? (it.segment_id + (it.segment_pos ? ('#' + it.segment_pos) : '')) : ''),
      miniCell('choice', it.choice_block_id || ''),
      miniCell('chunk', it.chunk_id || '')
    ].filter(Boolean).join('');
    const textTag = [
      miniCell('source', it.source_raw || ''),
      miniCell('prev', it.prev_en || ''),
      miniCell('next', it.next_en || '')
    ].filter(Boolean).join('');
    const scoreText = it.score_text || ((it.score_final || it.score_final === 0) ? (it.score_final < 0 ? 'pending' : Number(it.score_final).toFixed(2)) : '');
    const scoreMeta = [
      miniCell('score', scoreText || ''),
      miniCell('current', it.current_score || ''),
      miniCell('fresh', it.fresh_score || ''),
      miniCell('decision', it.score_decision || ''),
      miniCell('risk', it.final_risk || '')
    ].filter(Boolean).join('');
    tr.innerHTML =
      '<td>' + esc(it.id) + '</td>' +
      '<td><div class="mini-grid">' + pipelineTag + '</div></td>' +
      '<td><div class="mini-grid">' + identityTag + '</div></td>' +
      '<td><div class="mini-grid">' + textTag + '</div></td>' +
      '<td><div class="mini-grid">' + scoreMeta + '</div></td>' +
      '<td>' + esc(it.updated_at || '') + '</td>' +
      '<td>' + esc((it.final_ko || '').slice(0, 160)) + '</td>';
    tr.onclick = () => bindEditor(it);
    tb.appendChild(tr);
  });
  document.getElementById('msg').textContent = 'loaded: ' + d.count;
  syncMetricFilterBadge();
}

async function fetchItemByID(id) {
  const run = runValue();
  if (!run || !id) return null;
  const url = '/api/item?' + new URLSearchParams({ run, id });
  const r = await fetch(url);
  if (!r.ok) return null;
  return await r.json();
}

async function jumpLinkedRow(which) {
  if (!current) return;
  const targetID = which === 'prev' ? current.prev_line_id : current.next_line_id;
  if (!targetID) return;
  const item = await fetchItemByID(targetID);
  if (!item) {
    document.getElementById('msg').textContent = 'linked row not found: ' + targetID;
    return;
  }
  bindEditor(item);
}

function bindEditor(it) {
  current = it;
  openEditor();
  document.getElementById('erun').textContent = it.run_name;
  document.getElementById('eid').textContent = it.id;
  document.getElementById('estatus').value = it.status || STATUS_VALUES[0];
  document.getElementById('erisk').value = it.final_risk || '';
  document.getElementById('een').value = it.en || '';
  document.getElementById('emeta').value = [
    it.pipeline_version ? ('pipeline=' + it.pipeline_version) : '',
    it.source_type ? ('source_type=' + it.source_type) : '',
    it.source_file ? ('source_file=' + it.source_file) : '',
    it.scene_hint ? ('scene_hint=' + it.scene_hint) : '',
    it.resource_key ? ('resource_key=' + it.resource_key) : '',
    it.meta_path_label ? ('meta_path_label=' + it.meta_path_label) : '',
    it.segment_id ? ('segment_id=' + it.segment_id) : '',
    it.segment_pos ? ('segment_pos=' + it.segment_pos) : '',
    it.choice_block_id ? ('choice_block_id=' + it.choice_block_id) : '',
    it.migrated_from_old_id ? ('migrated_from_old_id=' + it.migrated_from_old_id) : '',
    it.chunk_id ? ('chunk=' + it.chunk_id) : '',
    it.parent_segment_id ? ('segment=' + it.parent_segment_id) : '',
    it.text_role ? ('role=' + it.text_role) : '',
    it.speaker_hint ? ('speaker=' + it.speaker_hint) : '',
    it.line_flags ? ('flags=' + it.line_flags) : '',
    it.winner ? ('winner=' + it.winner) : '',
    it.winner_score ? ('winner_score=' + it.winner_score) : '',
    it.current_score ? ('current_score=' + it.current_score) : '',
    it.fresh_score ? ('fresh_score=' + it.fresh_score) : '',
    it.score_decision ? ('score_decision=' + it.score_decision) : '',
    it.score_delta ? ('score_delta=' + it.score_delta) : '',
    it.rewrite_triggered ? ('rewrite_triggered=' + it.rewrite_triggered) : '',
    it.retry_reason ? ('retry_reason=' + it.retry_reason) : ''
  ].filter(Boolean).join('\n');
  document.getElementById('eorig').value = it.original_ko || '';
  document.getElementById('echunk').value = [
    it.chunk_en || '',
    it.fresh_ko ? ('[fresh_ko]\n' + it.fresh_ko) : '',
    it.replacement_ko ? ('[replacement_ko]\n' + it.replacement_ko) : '',
    it.retry_reason ? ('[retry_reason]\n' + it.retry_reason) : ''
  ].filter(Boolean).join('\n\n');
  document.getElementById('efinal').value = it.final_ko || '';
  document.getElementById('enote').value = it.final_notes || '';
  document.getElementById('epipeline').value = it.pipeline_version || '';
  document.getElementById('epipelinestate').value = [
    it.pipeline_state ? ('state=' + it.pipeline_state) : '',
    (it.retry_count || it.retry_count === 0) ? ('retry=' + it.retry_count) : '',
    (it.score_final || it.score_final === 0) ? ('score=' + (it.score_final < 0 ? 'pending' : it.score_final)) : '',
    it.pipeline_error ? ('error=' + it.pipeline_error) : ''
  ].filter(Boolean).join('\n');
  document.getElementById('epreven').value = it.prev_en || '';
  document.getElementById('eprev').value = it.prev_ko || '';
  document.getElementById('enexten').value = it.next_en || '';
  document.getElementById('enext').value = it.next_ko || '';
  document.getElementById('epack').value = it.raw_pack_json || '';
  document.getElementById('ekojson').value = it.raw_ko_json || '';
  const prevBtn = document.getElementById('prev-row-btn');
  const nextBtn = document.getElementById('next-row-btn');
  const sceneDialogueBtn = document.getElementById('scene-dialogue-btn');
  if (prevBtn) prevBtn.disabled = !it.prev_line_id;
  if (nextBtn) nextBtn.disabled = !it.next_line_id;
  if (sceneDialogueBtn) sceneDialogueBtn.disabled = !it.scene_hint;
  document.getElementById('cstatus').textContent = it.compare_status || '(none)';
  document.getElementById('crisk').textContent = it.compare_final_risk || '';
  document.getElementById('cko').textContent = (it.compare_final_ko || '').slice(0, 180);
  document.getElementById('msg').textContent = '';
}

function filterCurrentSceneDialogue() {
  if (!current || !current.scene_hint) return;
  document.getElementById('scene_hint').value = current.scene_hint;
  document.getElementById('text_role_filter').value = 'dialogue';
  loadItems();
}

function setStatus(s) { document.getElementById('estatus').value = s; }

async function saveCurrent() {
  if (!current) return;
  const payload = {
    run_name: current.run_name,
    id: current.id,
    status: document.getElementById('estatus').value,
    final_ko: document.getElementById('efinal').value,
    final_risk: document.getElementById('erisk').value,
    final_notes: document.getElementById('enote').value
  };
  const r = await fetch('/api/update', { method: 'POST', headers: {'content-type': 'application/json'}, body: JSON.stringify(payload) });
  const txt = await r.text();
  document.getElementById('msg').textContent = txt;
  if (r.ok) await loadItems();
}

async function deleteRun() {
  if (document.getElementById('delete-run-btn').disabled) return;
  const run = runValue();
  if (!run) return;
  if (!confirm('Delete run "' + run + '" ?')) return;
  const r = await fetch('/api/delete-run', { method: 'POST', headers: {'content-type':'application/json'}, body: JSON.stringify({run_name: run}) });
  const txt = await r.text();
  document.getElementById('msg').textContent = txt;
  await loadRuns();
}

function esc(s) {
  return (s || '').replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;');
}

function miniCell(label, value) {
  if (!value) return '';
  return '<div class="mini-cell"><span class="mini-label">' + esc(label) + '</span><span class="mini-value">' + esc(value) + '</span></div>';
}

function pill(label, value, klass) {
  if (value === '' || value === null || value === undefined) return '';
  const className = klass ? ('pill ' + klass) : 'pill';
  return '<span class="' + className + '">' + esc(label + ':' + String(value)) + '</span>';
}

async function forceDoneCurrent() {
  if (!current) return;
  const payload = {
    run_name: current.run_name,
    id: current.id,
    final_ko: document.getElementById('efinal').value,
    final_risk: document.getElementById('erisk').value,
    final_notes: document.getElementById('enote').value
  };
  const r = await fetch('/api/force-done', {
    method: 'POST',
    headers: {'content-type': 'application/json'},
    body: JSON.stringify(payload)
  });
  const txt = await r.text();
  document.getElementById('msg').textContent = txt;
  if (r.ok) {
    const refreshed = await fetchItemByID(current.id);
    if (refreshed) bindEditor(refreshed);
    await loadStats();
    await loadItems();
  }
}

function setMetricFilter(filter) {
  activeMetricFilter = {
    status: filter.status || '',
    pipeline_state: filter.pipeline_state || '',
    failed_reason: filter.failed_reason || '',
    label: filter.label || ''
  };
  loadItems();
}

function clearMetricFilter() {
  activeMetricFilter = { status: '', pipeline_state: '', failed_reason: '', label: '' };
  loadItems();
}

function syncMetricFilterBadge() {
  const box = document.getElementById('active-metric-filter');
  const value = document.getElementById('active-metric-filter-value');
  if (!box || !value) return;
  if (!activeMetricFilter.label) {
    box.classList.remove('show');
    value.textContent = '';
    return;
  }
  box.classList.add('show');
  value.textContent = activeMetricFilter.label;
}

function renderStatsSummary(d) {
  const stats = document.getElementById('stats');
  if (!stats) return;
  const counts = d.counts || {};
  const failed = d.failed_summary || {};
  const cards = [];
  const failedCards = [];
  const card = (title, value, sub='', klass='', filter=null) => {
    const classes = ['stats-card'];
    if (klass) classes.push(klass);
    if (filter) classes.push('clickable');
    const attrs = filter ? (
      ' onclick="setMetricFilter(' + escAttr(JSON.stringify(filter)).replaceAll('&quot;', '&quot;') + ')" title="filter items"'
    ) : '';
    return '<div class="' + classes.join(' ') + '"' + attrs + '><div class="stats-title">' + esc(title) + '</div><div class="stats-value">' + esc(String(value)) + '</div><div class="stats-sub">' + esc(sub) + '</div></div>';
  };
  cards.push(card('run', d.run_name || '', 'mode=' + (d.mode || '')));
  cards.push(card('total items', d.total || 0));
  cards.push(card('done', counts['item:done'] || counts['pipeline:done'] || 0, '', '', { status: 'done', label: 'item status = done' }));
  cards.push(card('failed', counts['pipeline:failed'] || 0, '', '', { pipeline_state: 'failed', label: 'pipeline state = failed' }));
  cards.push(card('blocked translate', counts['pipeline:blocked_translate'] || 0, '', '', { pipeline_state: 'blocked_translate', label: 'pipeline state = blocked_translate' }));
  cards.push(card('blocked score', counts['pipeline:blocked_score'] || 0, '', '', { pipeline_state: 'blocked_score', label: 'pipeline state = blocked_score' }));
  cards.push(card('working score', counts['pipeline:working_score'] || 0, '', '', { pipeline_state: 'working_score', label: 'pipeline state = working_score' }));
  cards.push(card('working translate', counts['pipeline:working_translate'] || 0, '', '', { pipeline_state: 'working_translate', label: 'pipeline state = working_translate' }));
  cards.push(card('pending overlay translate', counts['pipeline:pending_overlay_translate'] || 0, '', '', { pipeline_state: 'pending_overlay_translate', label: 'pipeline state = pending_overlay_translate' }));
  cards.push(card('working overlay translate', counts['pipeline:working_overlay_translate'] || 0, '', '', { pipeline_state: 'working_overlay_translate', label: 'pipeline state = working_overlay_translate' }));
  cards.push(card('pending failed translate', counts['pipeline:pending_failed_translate'] || 0, '', '', { pipeline_state: 'pending_failed_translate', label: 'pipeline state = pending_failed_translate' }));
  cards.push(card('working failed translate', counts['pipeline:working_failed_translate'] || 0, '', '', { pipeline_state: 'working_failed_translate', label: 'pipeline state = working_failed_translate' }));
  failedCards.push(card('failed total', failed.total || counts['pipeline:failed'] || 0, '', 'failed', { pipeline_state: 'failed', label: 'pipeline state = failed' }));
  failedCards.push(card('translator no row', failed.translator_no_row || 0, '', 'failed', { failed_reason: 'translator_no_row', label: 'failed reason = translator_no_row' }));
  failedCards.push(card('low score max retry', failed.low_score_max_retry || 0, '', 'failed', { failed_reason: 'low_score_max_retry', label: 'failed reason = low_score_max_retry' }));
  failedCards.push(card('missing score', failed.missing_score || 0, '', 'failed', { failed_reason: 'missing_score', label: 'failed reason = missing_score' }));
  failedCards.push(card('current empty winner', failed.current_empty || 0, '', 'failed', { failed_reason: 'current_empty', label: 'failed reason = current_empty_winner' }));
  stats.innerHTML =
    '<div class="stats-block"><div class="stats-heading">Pipeline Summary</div><div class="stats-grid">' + cards.join('') + '</div></div>' +
    '<div class="stats-block"><div class="stats-heading">Failed Pipeline</div><div class="stats-grid">' + failedCards.join('') + '</div></div>';
  syncMetricFilterBadge();
}

function escAttr(s) {
  return (s || '').replaceAll('&', '&amp;').replaceAll('\"', '&quot;').replaceAll('<', '&lt;').replaceAll('>', '&gt;');
}

initStatusOptions();
loadRuns();
if (MODE === 'checkpoint') {
  setInterval(() => {
    if (document.hidden) return;
    loadStats();
    loadItems();
  }, 5000);
}
</script>
</body>
</html>`
