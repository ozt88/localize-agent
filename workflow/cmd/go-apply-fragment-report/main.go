package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"localize-agent/workflow/pkg/platform"
	"localize-agent/workflow/pkg/shared"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type reportEntry struct {
	ClusterName string   `json:"cluster_name"`
	Status      string   `json:"status"`
	IDs         []string `json:"ids"`
	AfterKO     []string `json:"after_ko"`
}

type backupRow struct {
	ID            string `json:"id"`
	ItemStatus    string `json:"item_status"`
	KOJSON        string `json:"ko_json"`
	PackJSON      string `json:"pack_json"`
	PipelineState string `json:"pipeline_state"`
	RetryCount    int    `json:"retry_count"`
	ScoreFinal    string `json:"score_final"`
	LastError     string `json:"last_error"`
}

type update struct {
	ID    string
	After string
}

func main() {
	var projectDir string
	var reportPath string
	var backupPath string

	fs := flag.NewFlagSet("go-apply-fragment-report", flag.ExitOnError)
	fs.StringVar(&projectDir, "project-dir", "", "project directory containing project.json")
	fs.StringVar(&reportPath, "report-path", "", "fragment batch report JSON to apply")
	fs.StringVar(&backupPath, "backup-path", "", "path to write backup JSON before apply")
	fs.Parse(os.Args[1:])

	if strings.TrimSpace(projectDir) == "" || strings.TrimSpace(reportPath) == "" || strings.TrimSpace(backupPath) == "" {
		fmt.Fprintln(os.Stderr, "--project-dir, --report-path, and --backup-path are required")
		os.Exit(2)
	}

	projectCfg, _, err := shared.LoadProjectConfig("", projectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "project load error: %v\n", err)
		os.Exit(1)
	}
	db, err := platform.OpenTranslationCheckpointDB(projectCfg.Translation.CheckpointBackend, projectCfg.Translation.CheckpointDB, projectCfg.Translation.CheckpointDSN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "checkpoint open error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	raw, err := os.ReadFile(reportPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read report error: %v\n", err)
		os.Exit(1)
	}
	var report []reportEntry
	if err := json.Unmarshal(raw, &report); err != nil {
		fmt.Fprintf(os.Stderr, "decode report error: %v\n", err)
		os.Exit(1)
	}

	updates := make([]update, 0)
	seen := map[string]bool{}
	for _, entry := range report {
		if entry.Status != "ok" {
			continue
		}
		if len(entry.IDs) != len(entry.AfterKO) {
			fmt.Fprintf(os.Stderr, "mismatched ids/after_ko in cluster %s\n", entry.ClusterName)
			os.Exit(1)
		}
		for i, id := range entry.IDs {
			if seen[id] {
				continue
			}
			seen[id] = true
			updates = append(updates, update{ID: id, After: entry.AfterKO[i]})
		}
	}
	if len(updates) == 0 {
		fmt.Fprintln(os.Stderr, "no ok entries to apply")
		os.Exit(1)
	}

	backups, err := backupRows(db, projectCfg.Translation.CheckpointBackend, updates)
	if err != nil {
		fmt.Fprintf(os.Stderr, "backup query error: %v\n", err)
		os.Exit(1)
	}
	backupRaw, _ := json.MarshalIndent(backups, "", "  ")
	if err := os.WriteFile(backupPath, backupRaw, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "backup write error: %v\n", err)
		os.Exit(1)
	}

	if err := applyUpdates(db, projectCfg.Translation.CheckpointBackend, updates); err != nil {
		fmt.Fprintf(os.Stderr, "apply error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("applied=%d backup=%s\n", len(updates), backupPath)
}

func backupRows(db *sql.DB, backend string, updates []update) ([]backupRow, error) {
	placeholders := make([]string, 0, len(updates))
	args := make([]any, 0, len(updates))
	for _, u := range updates {
		placeholders = append(placeholders, "?")
		args = append(args, u.ID)
	}
	query := platform.RebindSQL(backend, `
select i.id, i.status, coalesce(i.ko_json::text,''), coalesce(i.pack_json::text,''),
       coalesce(p.state,''), coalesce(p.retry_count,0), coalesce(p.score_final::text,''), coalesce(p.last_error,'')
from items i
left join pipeline_items p on p.id = i.id
where i.id in (`+strings.Join(placeholders, ",")+`)`)
	if backend != platform.DBBackendPostgres {
		query = platform.RebindSQL(backend, `
select i.id, i.status, coalesce(i.ko_json,''), coalesce(i.pack_json,''),
       coalesce(p.state,''), coalesce(p.retry_count,0), coalesce(cast(p.score_final as text),''), coalesce(p.last_error,'')
from items i
left join pipeline_items p on p.id = i.id
where i.id in (`+strings.Join(placeholders, ",")+`)`)
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]backupRow, 0, len(updates))
	for rows.Next() {
		var row backupRow
		if err := rows.Scan(&row.ID, &row.ItemStatus, &row.KOJSON, &row.PackJSON, &row.PipelineState, &row.RetryCount, &row.ScoreFinal, &row.LastError); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func applyUpdates(db *sql.DB, backend string, updates []update) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	nowValue := dbTimeValueForBackend(backend, now)

	for _, u := range updates {
		if err := applyOne(tx, backend, u, nowValue); err != nil {
			return fmt.Errorf("%s: %w", u.ID, err)
		}
	}
	return tx.Commit()
}

func applyOne(tx *sql.Tx, backend string, u update, now any) error {
	var koRaw, packRaw string
	selectQ := `select coalesce(ko_json::text,''), coalesce(pack_json::text,'') from items where id = ?`
	if backend != platform.DBBackendPostgres {
		selectQ = `select coalesce(ko_json,''), coalesce(pack_json,'') from items where id = ?`
	}
	if err := tx.QueryRow(rebindForBackend(backend, selectQ), u.ID).Scan(&koRaw, &packRaw); err != nil {
		return err
	}

	koObj := map[string]any{}
	packObj := map[string]any{}
	if strings.TrimSpace(koRaw) != "" {
		_ = json.Unmarshal([]byte(koRaw), &koObj)
	}
	if strings.TrimSpace(packRaw) != "" {
		_ = json.Unmarshal([]byte(packRaw), &packObj)
	}

	koObj["Text"] = u.After
	packObj["fresh_ko"] = u.After
	packObj["proposed_ko_restored"] = u.After

	koOut, _ := json.Marshal(koObj)
	packOut, _ := json.Marshal(packObj)

	updateItemQ := `update items set status='done', ko_json = ?, pack_json = ?, updated_at = ? where id = ?`
	if backend == platform.DBBackendPostgres {
		updateItemQ = `update items set status='done', ko_json = ?::jsonb, pack_json = ?::jsonb, updated_at = ? where id = ?`
	}
	if _, err := tx.Exec(rebindForBackend(backend, updateItemQ), string(koOut), string(packOut), now, u.ID); err != nil {
		return err
	}

	nextReady, err := checkpointNextReadyForApply(tx, backend, u.ID)
	if err != nil {
		return err
	}
	nextState := "blocked_score"
	if nextReady {
		nextState = "pending_score"
	}
	updatePipeQ := `update pipeline_items set state = ?, retry_count = 0, score_final = -1, last_error = '', claimed_by = '', claimed_at = NULL, lease_until = NULL, updated_at = ? where id = ?`
	if _, err := tx.Exec(rebindForBackend(backend, updatePipeQ), nextState, now, u.ID); err != nil {
		return err
	}
	return nil
}

func checkpointNextReadyForApply(tx *sql.Tx, backend string, id string) (bool, error) {
	nextID, err := checkpointLineIDFromPackDB(tx, backend, id, "next_line_id")
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(nextID) == "" {
		return true, nil
	}
	var status string
	if err := tx.QueryRow(rebindForBackend(backend, `select status from items where id = ?`), nextID).Scan(&status); err != nil {
		if err == sql.ErrNoRows {
			return true, nil
		}
		return false, err
	}
	return status == "done", nil
}

func checkpointLineIDFromPackDB(tx *sql.Tx, backend string, id string, key string) (string, error) {
	var packRaw string
	selectQ := `select coalesce(pack_json::text,'') from items where id = ?`
	if backend != platform.DBBackendPostgres {
		selectQ = `select coalesce(pack_json,'') from items where id = ?`
	}
	if err := tx.QueryRow(rebindForBackend(backend, selectQ), id).Scan(&packRaw); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	packObj := map[string]any{}
	if strings.TrimSpace(packRaw) != "" {
		_ = json.Unmarshal([]byte(packRaw), &packObj)
	}
	v, _ := packObj[key].(string)
	return strings.TrimSpace(v), nil
}

func rebindForBackend(backend string, q string) string {
	return platform.RebindSQL(backend, q)
}

func dbTimeValueForBackend(backend string, t time.Time) any {
	if backend == platform.DBBackendPostgres {
		return t
	}
	return t.Format(time.RFC3339Nano)
}
