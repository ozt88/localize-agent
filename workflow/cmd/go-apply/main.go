package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

func loadStringsRoot(path string) (map[string]any, map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, nil, err
	}
	stringsObj, ok := root["strings"].(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("invalid JSON: expected top-level 'strings' object: %s", path)
	}
	return root, stringsObj, nil
}

func writeJSONFile(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func parseCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
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

func loadReadyEntries(db *sql.DB, runName string, statuses []string) (map[string]string, error) {
	if len(statuses) == 0 {
		return nil, fmt.Errorf("at least one ready status is required")
	}
	ph := make([]string, len(statuses))
	args := make([]any, len(statuses))
	for i, s := range statuses {
		ph[i] = "?"
		args[i] = s
	}
	q := fmt.Sprintf(
		`SELECT source_id, final_ko
		 FROM eval_items
		 WHERE run_name=? AND status IN (%s) AND final_ko IS NOT NULL AND final_ko <> ''`,
		strings.Join(ph, ","),
	)
	args = append([]any{runName}, args...)
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var id, ko string
		if err := rows.Scan(&id, &ko); err != nil {
			return nil, err
		}
		out[id] = ko
	}
	return out, rows.Err()
}

func updateAppliedStatus(db *sql.DB, runName string, ids []string, fromStatuses []string, nextStatus string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	idPH := make([]string, len(ids))
	idArgs := make([]any, len(ids))
	for i, id := range ids {
		idPH[i] = "?"
		idArgs[i] = id
	}
	stPH := make([]string, len(fromStatuses))
	stArgs := make([]any, len(fromStatuses))
	for i, s := range fromStatuses {
		stPH[i] = "?"
		stArgs[i] = s
	}
	q := fmt.Sprintf(
		`UPDATE eval_items
		 SET status=?, updated_at=?
		 WHERE run_name=? AND source_id IN (%s) AND status IN (%s)`,
		strings.Join(idPH, ","),
		strings.Join(stPH, ","),
	)
	args := make([]any, 0, 3+len(idArgs)+len(stArgs))
	args = append(args, nextStatus, time.Now().UTC().Format(time.RFC3339), runName)
	args = append(args, idArgs...)
	args = append(args, stArgs...)

	r, err := db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return r.RowsAffected()
}

func main() {
	var dbPath string
	var current string
	var out string
	var inPlace bool
	var runName string
	var readyStatus string
	var nextStatus string
	var dryRun bool

	flag.StringVar(&dbPath, "db", "workflow/output/evaluation_unified.db", "evaluation DB path")
	flag.StringVar(&runName, "run-name", "default", "logical run name inside unified DB")
	flag.StringVar(&current, "current", "enGB_new.json", "current localization JSON")
	flag.StringVar(&out, "out", "workflow/output/enGB_new_applied.json", "output JSON path")
	flag.BoolVar(&inPlace, "in-place", false, "write directly to --current")
	flag.StringVar(&readyStatus, "ready-status", "pass", "comma-separated source statuses to apply")
	flag.StringVar(&nextStatus, "next-status", "applied", "status to set after apply")
	flag.BoolVar(&dryRun, "dry-run", false, "apply in-memory only, do not write file or DB status")
	flag.Parse()

	statuses := parseCSV(readyStatus)
	if len(statuses) == 0 {
		fmt.Fprintln(os.Stderr, "--ready-status must include at least one status")
		os.Exit(2)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()
	if err := ensureColumn(db, "eval_items", "run_name", "TEXT NOT NULL DEFAULT 'default'"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := ensureColumn(db, "eval_items", "source_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if _, err := db.Exec(`UPDATE eval_items SET source_id=id WHERE source_id=''`); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	readyEntries, err := loadReadyEntries(db, runName, statuses)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	currentRoot, currentStrings, err := loadStringsRoot(current)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	appliedIDs := make([]string, 0, len(readyEntries))
	for entryID, finalKO := range readyEntries {
		curRaw, ok := currentStrings[entryID]
		if !ok {
			continue
		}
		curObj, ok := curRaw.(map[string]any)
		if !ok {
			continue
		}
		curObj["Text"] = finalKO
		currentStrings[entryID] = curObj
		appliedIDs = append(appliedIDs, entryID)
	}

	outputPath := out
	if inPlace {
		outputPath = current
	}

	if !dryRun {
		currentRoot["strings"] = currentStrings
		if err := writeJSONFile(outputPath, currentRoot); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		updatedRows, err := updateAppliedStatus(db, runName, appliedIDs, statuses, nextStatus)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("Applied %d entries\n", len(appliedIDs))
		fmt.Printf("Run: %s\n", runName)
		fmt.Printf("DB status updated: %d rows -> %s\n", updatedRows, nextStatus)
		fmt.Printf("Output: %s\n", outputPath)
		return
	}

	fmt.Printf("[DRY-RUN] Ready in DB: %d\n", len(readyEntries))
	fmt.Printf("[DRY-RUN] Applicable to current file: %d\n", len(appliedIDs))
	fmt.Printf("[DRY-RUN] No file written, no DB status updated\n")
}
