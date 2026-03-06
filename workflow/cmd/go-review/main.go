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

type itemRow struct {
	RunName          string `json:"run_name"`
	ID               string `json:"id"`
	Status           string `json:"status"`
	EN               string `json:"en"`
	Original         string `json:"original_ko"`
	FinalKO          string `json:"final_ko"`
	FinalRisk        string `json:"final_risk"`
	FinalNote        string `json:"final_notes"`
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
	db   *sql.DB
	page *template.Template
}

func main() {
	var dbPath string
	var addr string

	flag.StringVar(&dbPath, "db", "workflow/output/evaluation_unified.db", "unified evaluation DB path")
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
	if err := ensureSchema(db); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	a := &app{
		db:   db,
		page: template.Must(template.New("index").Parse(indexHTML)),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleIndex)
	mux.HandleFunc("/api/runs", a.handleRuns)
	mux.HandleFunc("/api/stats", a.handleStats)
	mux.HandleFunc("/api/items", a.handleItems)
	mux.HandleFunc("/api/update", a.handleUpdate)
	mux.HandleFunc("/api/delete-run", a.handleDeleteRun)

	log.Printf("review viewer: http://%s (db=%s)", addr, dbPath)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func ensureSchema(db *sql.DB) error {
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
	_, err = db.Exec(`UPDATE eval_items SET source_id=id WHERE source_id=''`)
	if err != nil {
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
	_ = a.page.Execute(w, nil)
}

func (a *app) handleRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
	writeJSON(w, map[string]any{"runs": out})
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
	writeJSON(w, map[string]any{"run_name": runName, "total": total, "counts": counts})
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
	limit := parseIntDefault(r.URL.Query().Get("limit"), 200)
	if limit < 1 {
		limit = 1
	}
	if limit > 1000 {
		limit = 1000
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
	writeJSON(w, map[string]any{"items": items, "count": len(items), "run_name": runName, "compare_run": compareRun})
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
	if !isValidStatus(in.Status) {
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

func (a *app) handleDeleteRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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

func isValidStatus(s string) bool {
	switch s {
	case "pending", "evaluating", "pass", "revise", "reject", "applied":
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
  <title>RT Eval DB Review</title>
  <style>
    body { font-family: "Segoe UI", sans-serif; margin: 16px; background: #f4f7fb; color: #111827; }
    h1 { margin: 0 0 12px; font-size: 20px; }
    .row { display: flex; gap: 8px; flex-wrap: wrap; margin-bottom: 10px; }
    input, select, button, textarea { font-size: 13px; padding: 8px; border: 1px solid #cbd5e1; border-radius: 6px; }
    button { background: #0f766e; color: white; border: none; cursor: pointer; }
    button.secondary { background: #334155; }
    button.warn { background: #b45309; }
    button.danger { background: #b91c1c; }
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
  <h1>Unified Evaluation DB Review</h1>
  <div class="panel">
    <div class="row">
      <label>Run <select id="run"></select></label>
      <label>Compare <select id="compare"><option value="">(none)</option></select></label>
      <button class="secondary" onclick="loadRuns()">Refresh runs</button>
      <button class="danger" onclick="deleteRun()">Delete selected run</button>
    </div>
    <div class="row">
      <input id="q" placeholder="search id / en / ko" style="min-width:240px;">
      <input id="status" value="" placeholder="status csv (empty = all)">
      <input id="limit" value="100" style="width:90px;">
      <button onclick="loadItems()">Load</button>
      <button class="secondary" onclick="loadStats()">Stats</button>
    </div>
    <div id="stats" class="muted"></div>
  </div>

  <div class="cols">
    <div class="panel">
      <table id="tbl">
        <thead>
          <tr><th>id</th><th>status</th><th>risk</th><th>updated</th><th>final_ko</th></tr>
        </thead>
        <tbody></tbody>
      </table>
    </div>
    <div id="editor" class="panel">
      <div class="muted">Select a row to edit</div>
      <div>Run: <span id="erun"></span> | ID: <span id="eid"></span></div>
      <div class="row">
        <select id="estatus"><option>pending</option><option>evaluating</option><option>pass</option><option>revise</option><option>reject</option><option>applied</option></select>
        <input id="erisk" placeholder="risk">
      </div>
      <div class="muted">EN</div>
      <textarea id="een" readonly></textarea>
      <div class="muted">Original KO</div>
      <textarea id="eorig" readonly></textarea>
      <div class="muted">Final KO</div>
      <textarea id="efinal"></textarea>
      <div class="muted">Notes</div>
      <textarea id="enote"></textarea>
      <div class="cmp">Compare status: <span id="cstatus"></span></div>
      <div class="cmp">Compare risk: <span id="crisk"></span></div>
      <div class="cmp">Compare final_ko: <span id="cko"></span></div>
      <div class="row">
        <button onclick="saveCurrent()">Save</button>
        <button class="secondary" onclick="setStatus('pass')">Mark pass</button>
        <button class="warn" onclick="setStatus('revise')">Mark revise</button>
        <button class="danger" onclick="setStatus('reject')">Mark reject</button>
      </div>
      <div id="msg" class="muted"></div>
    </div>
  </div>

<script>
let current = null;

function runValue() { return document.getElementById('run').value; }
function compareValue() { return document.getElementById('compare').value; }

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
    cmp.add(new Option(label, x.run_name));
  });
  if (prevRun && [...run.options].some(o => o.value === prevRun)) run.value = prevRun;
  if (!run.value && run.options.length > 0) run.selectedIndex = 0;
  if (prevCmp && [...cmp.options].some(o => o.value === prevCmp)) cmp.value = prevCmp;
  await loadStats();
  await loadItems();
}

async function loadStats() {
  const run = runValue();
  if (!run) { document.getElementById('stats').textContent = 'no runs'; return; }
  const r = await fetch('/api/stats?' + new URLSearchParams({run}));
  const d = await r.json();
  document.getElementById('stats').textContent = 'run=' + run + ' total=' + d.total + ' ' + JSON.stringify(d.counts);
}

async function loadItems() {
  const run = runValue();
  if (!run) { document.getElementById('msg').textContent = 'no runs'; return; }
  const q = document.getElementById('q').value.trim();
  const status = document.getElementById('status').value.trim();
  const limit = document.getElementById('limit').value.trim() || '100';
  const compare_run = compareValue();
  const url = '/api/items?' + new URLSearchParams({run, compare_run, q, status, limit});
  const r = await fetch(url);
  const d = await r.json();
  const tb = document.querySelector('#tbl tbody');
  tb.innerHTML = '';
  d.items.forEach(it => {
    const tr = document.createElement('tr');
    const cmpTag = it.compare_status ? (' / cmp:' + it.compare_status) : '';
    tr.innerHTML = '<td>' + esc(it.id) + '</td><td>' + esc(it.status + cmpTag) + '</td><td>' + esc(it.final_risk || '') + '</td><td>' + esc(it.updated_at || '') + '</td><td>' + esc((it.final_ko || '').slice(0, 120)) + '</td>';
    tr.onclick = () => bindEditor(it);
    tb.appendChild(tr);
  });
  document.getElementById('msg').textContent = 'loaded: ' + d.count;
}

function bindEditor(it) {
  current = it;
  document.getElementById('erun').textContent = it.run_name;
  document.getElementById('eid').textContent = it.id;
  document.getElementById('estatus').value = it.status || 'pending';
  document.getElementById('erisk').value = it.final_risk || '';
  document.getElementById('een').value = it.en || '';
  document.getElementById('eorig').value = it.original_ko || '';
  document.getElementById('efinal').value = it.final_ko || '';
  document.getElementById('enote').value = it.final_notes || '';
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

loadRuns();
</script>
</body>
</html>`
