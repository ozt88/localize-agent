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
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	dbModeEval       = "eval"
	dbModeCheckpoint = "checkpoint"
	checkpointRun    = "checkpoint"
)

type itemRow struct {
	RunName          string `json:"run_name"`
	ID               string `json:"id"`
	Status           string `json:"status"`
	PipelineState    string `json:"pipeline_state,omitempty"`
	RetryCount       int    `json:"retry_count"`
	ScoreFinal       float64 `json:"score_final"`
	PipelineError    string `json:"pipeline_error,omitempty"`
	EN               string `json:"en"`
	SourceRaw        string `json:"source_raw,omitempty"`
	Original         string `json:"original_ko"`
	FinalKO          string `json:"final_ko"`
	FinalRisk        string `json:"final_risk"`
	FinalNote        string `json:"final_notes"`
	PipelineVersion  string `json:"pipeline_version,omitempty"`
	ChunkID          string `json:"chunk_id,omitempty"`
	ParentSegmentID  string `json:"parent_segment_id,omitempty"`
	TextRole         string `json:"text_role,omitempty"`
	SpeakerHint      string `json:"speaker_hint,omitempty"`
	LineFlags        string `json:"line_flags,omitempty"`
	PrevEN           string `json:"prev_en,omitempty"`
	PrevKO           string `json:"prev_ko,omitempty"`
	NextEN           string `json:"next_en,omitempty"`
	NextKO           string `json:"next_ko,omitempty"`
	ChunkEN          string `json:"chunk_en,omitempty"`
	RawKOJSON        string `json:"raw_ko_json,omitempty"`
	RawPackJSON      string `json:"raw_pack_json,omitempty"`
	Revised          bool   `json:"revised"`
	UpdatedAt        string `json:"updated_at"`
	CompareStatus    string `json:"compare_status,omitempty"`
	CompareFinalKO   string `json:"compare_final_ko,omitempty"`
	CompareFinalRisk string `json:"compare_final_risk,omitempty"`
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

type app struct {
	db     *sql.DB
	page   *template.Template
	dbMode string
}

type workerStatRow struct {
	WorkerID       string  `json:"worker_id"`
	Role           string  `json:"role"`
	ProcessedCount int     `json:"processed_count"`
	ElapsedMs      int64   `json:"elapsed_ms"`
	ItemsPerSec    float64 `json:"items_per_sec"`
	FinishedAt     string  `json:"finished_at"`
}

func main() {
	var dbPath string
	var addr string

	flag.StringVar(&dbPath, "db", "workflow/output/evaluation_unified.db", "evaluation or translation checkpoint DB path")
	flag.StringVar(&addr, "addr", "127.0.0.1:8091", "listen address")
	flag.Parse()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	mode, err := detectDBMode(db)
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
	a := &app{
		db:     db,
		dbMode: mode,
		page:   template.Must(template.New("index").Parse(indexHTML)),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleIndex)
	mux.HandleFunc("/api/runs", a.handleRuns)
	mux.HandleFunc("/api/stats", a.handleStats)
	mux.HandleFunc("/api/items", a.handleItems)
	mux.HandleFunc("/api/pipeline-worker-stats", a.handlePipelineWorkerStats)
	mux.HandleFunc("/api/update", a.handleUpdate)
	mux.HandleFunc("/api/delete-run", a.handleDeleteRun)

	log.Printf("review viewer: http://%s (db=%s, mode=%s, title=%s)", addr, dbPath, mode, title)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func detectDBMode(db *sql.DB) (string, error) {
	hasEval, err := hasTable(db, "eval_items")
	if err != nil {
		return "", err
	}
	if hasEval {
		return dbModeEval, nil
	}
	hasItems, err := hasTable(db, "items")
	if err != nil {
		return "", err
	}
	if hasItems {
		return dbModeCheckpoint, nil
	}
	return "", fmt.Errorf("unsupported DB: expected table eval_items or items")
}

func hasTable(db *sql.DB, name string) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
	return n > 0, err
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
		rows, err := a.db.Query(`
SELECT key, value FROM (
  SELECT 'item:' || status AS key, COUNT(*) AS value FROM items GROUP BY status
  UNION ALL
  SELECT 'pipeline:' || state AS key, COUNT(*) AS value FROM pipeline_items GROUP BY state
) ORDER BY key`)
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
		writeJSON(w, map[string]any{"run_name": checkpointRun, "total": total, "counts": counts, "mode": a.dbMode})
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

	if a.dbMode == dbModeCheckpoint {
		items, err := a.loadCheckpointItems(queryText, id, statuses, pipelineVersion, sortBy, limit)
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
	limit := parseIntDefault(r.URL.Query().Get("limit"), 30)
	if limit < 1 {
		limit = 1
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := a.db.Query(fmt.Sprintf(`
SELECT worker_id, role, processed_count, elapsed_ms, finished_at
FROM pipeline_worker_stats
ORDER BY finished_at DESC
LIMIT %d`, limit))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := make([]workerStatRow, 0, limit)
	for rows.Next() {
		var it workerStatRow
		if err := rows.Scan(&it.WorkerID, &it.Role, &it.ProcessedCount, &it.ElapsedMs, &it.FinishedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if it.ElapsedMs > 0 {
			it.ItemsPerSec = float64(it.ProcessedCount) / (float64(it.ElapsedMs) / 1000.0)
		}
		out = append(out, it)
	}
	writeJSON(w, map[string]any{"items": out})
}

func checkpointSortOrder(sortBy string) string {
	switch sortBy {
	case "score_asc":
		return "CASE WHEN COALESCE(p.score_final, -1) < 0 THEN 1 ELSE 0 END ASC, COALESCE(p.score_final, 0) ASC, i.updated_at DESC"
	case "score_desc":
		return "CASE WHEN COALESCE(p.score_final, -1) < 0 THEN 1 ELSE 0 END ASC, COALESCE(p.score_final, 0) DESC, i.updated_at DESC"
	case "retry_desc":
		return "COALESCE(p.retry_count, 0) DESC, COALESCE(p.score_final, 0) DESC, i.updated_at DESC"
	case "updated_asc":
		return "i.updated_at ASC"
	default:
		return "i.updated_at DESC"
	}
}

func (a *app) loadCheckpointItems(queryText, id string, statuses []string, pipelineVersion string, sortBy string, limit int) ([]itemRow, error) {
	parts := []string{"1=1"}
	args := []any{}
	if id != "" {
		parts = append(parts, "id = ?")
		args = append(args, id)
	}
	if len(statuses) > 0 {
		ph := make([]string, len(statuses))
		for i, s := range statuses {
			ph[i] = "?"
			args = append(args, s)
		}
		parts = append(parts, "status IN ("+strings.Join(ph, ",")+")")
	}
	if queryText != "" {
		p := "%" + queryText + "%"
		parts = append(parts, "(id LIKE ? OR status LIKE ? OR pack_json LIKE ? OR ko_json LIKE ?)")
		args = append(args, p, p, p, p)
	}
	if pipelineVersion != "" {
		parts = append(parts, "(json_extract(pack_json, '$.pipeline_version') = ? OR pack_json LIKE ?)")
		args = append(args, pipelineVersion, "%\"pipeline_version\":\""+pipelineVersion+"\"%")
	}
	args = append(args, limit)
	orderBy := checkpointSortOrder(sortBy)

	rows, err := a.db.Query(`
SELECT i.id, i.status, i.ko_json, i.pack_json, i.updated_at,
       COALESCE(p.state,''), COALESCE(p.retry_count,0), COALESCE(p.score_final,0), COALESCE(p.last_error,'')
FROM items i
LEFT JOIN pipeline_items p ON p.id = i.id
WHERE `+strings.Join(parts, " AND ")+`
ORDER BY `+orderBy+`
LIMIT ?`, args...)
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
		if err := rows.Scan(&id, &status, &koJSON, &packJSON, &updatedAt, &pipelineState, &retryCount, &scoreFinal, &pipelineError); err != nil {
			return nil, err
		}
		it := itemRow{
			RunName:   checkpointRun,
			ID:        id,
			Status:    status,
			PipelineState: pipelineState,
			RetryCount: retryCount,
			ScoreFinal: scoreFinal,
			PipelineError: pipelineError,
			UpdatedAt: updatedAt,
			RawKOJSON:   koJSON,
			RawPackJSON: packJSON,
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
		it.FinalKO = stringField(packObj, "proposed_ko_restored")
		if it.FinalKO == "" {
			it.FinalKO = stringField(koObj, "Text")
		}
		it.FinalRisk = stringField(packObj, "risk")
		it.FinalNote = stringField(packObj, "notes")
		it.PipelineVersion = stringField(packObj, "pipeline_version")
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

func (a *app) updateCheckpointItem(in updateRequest) (int64, error) {
	var koJSON, packJSON string
	err := a.db.QueryRow(`SELECT ko_json, pack_json FROM items WHERE id=?`, in.ID).Scan(&koJSON, &packJSON)
	if err != nil {
		return 0, err
	}
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
	res, err := a.db.Exec(
		`UPDATE items
		 SET status=?, ko_json=?, pack_json=?, updated_at=?
		 WHERE id=?`,
		in.Status, string(koRaw), string(packRaw), time.Now().UTC().Format(time.RFC3339), in.ID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
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

func buildLineFlags(packObj map[string]any) string {
	flags := make([]string, 0, 2)
	if stringField(packObj, "line_is_imperative") == "true" {
		flags = append(flags, "imperative")
	}
	if stringField(packObj, "line_is_short_context_dependent") == "true" {
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

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
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
    .cols { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
    .cmp { color: #0f766e; font-size: 11px; }
    @media (max-width: 900px) { .cols { grid-template-columns: 1fr; } }
  </style>
</head>
<body>
  <h1>{{.Title}}</h1>
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
    <div id="stats" class="muted"></div>
    <div class="muted" style="margin-top:8px;">Recent worker throughput</div>
    <table id="wtbl" style="margin-top:6px;">
      <thead>
        <tr><th>worker</th><th>role</th><th>processed</th><th>elapsed_ms</th><th>items/sec</th><th>finished</th></tr>
      </thead>
      <tbody></tbody>
    </table>
  </div>

  <div class="cols">
    <div class="panel">
      <table id="tbl">
        <thead>
          <tr><th>id</th><th>status</th><th>pipeline</th><th>score</th><th>retry</th><th>risk</th><th>updated</th><th>final_ko</th></tr>
        </thead>
        <tbody></tbody>
      </table>
    </div>
    <div id="editor" class="panel">
      <div class="muted">Select a row to edit</div>
      <div>Run: <span id="erun"></span> | ID: <span id="eid"></span></div>
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
        <button class="secondary" onclick="setStatus(defaultPassStatus())">Quick status</button>
      </div>
      <div id="msg" class="muted"></div>
    </div>
  </div>

<script>
const MODE = {{printf "%q" .Mode}};
const STATUS_VALUES = {{printf "%q" .StatusValues}}.split(',');
let current = null;

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
  document.getElementById('stats').textContent = 'run=' + d.run_name + ' total=' + d.total + ' ' + JSON.stringify(d.counts);
  await loadWorkerStats();
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
  const status = document.getElementById('status').value.trim();
  const pipeline_version = document.getElementById('pipeline_version').value.trim();
  const sort = document.getElementById('sort').value.trim() || 'updated_desc';
  const limit = document.getElementById('limit').value.trim() || '100';
  const compare_run = document.getElementById('compare').disabled ? '' : compareValue();
  const url = '/api/items?' + new URLSearchParams({run, compare_run, q, status, pipeline_version, sort, limit});
  const r = await fetch(url);
  const d = await r.json();
  const tb = document.querySelector('#tbl tbody');
  tb.innerHTML = '';
  d.items.forEach(it => {
    const tr = document.createElement('tr');
    const cmpTag = it.compare_status ? (' / cmp:' + it.compare_status) : '';
    const pipelineTag = it.pipeline_state ? (it.pipeline_state + (it.retry_count ? (' r' + it.retry_count) : '')) : '';
    const scoreText = (it.score_final || it.score_final === 0) ? (it.score_final < 0 ? 'pending' : Number(it.score_final).toFixed(2)) : '';
    const retryText = String((it.retry_count || it.retry_count === 0) ? it.retry_count : '');
    tr.innerHTML = '<td>' + esc(it.id) + '</td><td>' + esc(it.status + cmpTag) + '</td><td>' + esc(pipelineTag) + '</td><td>' + esc(scoreText) + '</td><td>' + esc(retryText) + '</td><td>' + esc(it.final_risk || '') + '</td><td>' + esc(it.updated_at || '') + '</td><td>' + esc((it.final_ko || '').slice(0, 120)) + '</td>';
    tr.onclick = () => bindEditor(it);
    tb.appendChild(tr);
  });
  document.getElementById('msg').textContent = 'loaded: ' + d.count;
}

function bindEditor(it) {
  current = it;
  document.getElementById('erun').textContent = it.run_name;
  document.getElementById('eid').textContent = it.id;
  document.getElementById('estatus').value = it.status || STATUS_VALUES[0];
  document.getElementById('erisk').value = it.final_risk || '';
  document.getElementById('een').value = it.en || '';
  document.getElementById('emeta').value = [
    it.pipeline_version ? ('pipeline=' + it.pipeline_version) : '',
    it.chunk_id ? ('chunk=' + it.chunk_id) : '',
    it.parent_segment_id ? ('segment=' + it.parent_segment_id) : '',
    it.text_role ? ('role=' + it.text_role) : '',
    it.speaker_hint ? ('speaker=' + it.speaker_hint) : '',
    it.line_flags ? ('flags=' + it.line_flags) : ''
  ].filter(Boolean).join('\n');
  document.getElementById('eorig').value = it.original_ko || '';
  document.getElementById('echunk').value = it.chunk_en || '';
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
  document.getElementById('cstatus').textContent = it.compare_status || '(none)';
  document.getElementById('crisk').textContent = it.compare_final_risk || '';
  document.getElementById('cko').textContent = (it.compare_final_ko || '').slice(0, 180);
  document.getElementById('msg').textContent = '';
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

initStatusOptions();
loadRuns();
</script>
</body>
</html>`
