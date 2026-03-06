package evaluation

import (
	"encoding/json"
	"fmt"
	"os"

	"localize-agent/workflow/internal/contracts"
)

func runStatusMode(c Config, store contracts.EvalStore) int {
	counts, err := store.StatusCounts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	total := 0
	for _, n := range counts {
		total += n
	}
	fmt.Printf("DB: %s\n", c.DB)
	fmt.Printf("Run: %s\n", c.RunName)
	fmt.Printf("Total: %d\n", total)
	for _, s := range []string{statusPending, statusEvaluating, statusPass, statusRevise, statusReject} {
		fmt.Printf("  %-12s %d\n", s, counts[s])
	}
	return 0
}

func runExportMode(c Config, store contracts.EvalStore, files contracts.FileStore) int {
	all, err := store.ExportByStatus(statusPass, statusRevise, statusReject)
	if err != nil {
		fmt.Fprintf(os.Stderr, "export error: %v\n", err)
		return 1
	}
	_ = files.WriteJSON(c.ReportOut, map[string]any{"items": all})
	fmt.Printf("report  -> %s (%d items)\n", c.ReportOut, len(all))

	rej, _ := store.ExportByStatus(statusReject)
	if len(rej) > 0 {
		_ = files.WriteJSON(c.RejectedOut, map[string]any{"items": rej})
		fmt.Printf("rejected-> %s (%d items)\n", c.RejectedOut, len(rej))
	}

	passItems, _ := store.ExportByStatus(statusPass, statusRevise)
	revised := selectRevised(passItems)
	if len(revised) > 0 {
		_ = files.WriteJSON(c.RevisedOut, map[string]any{"items": revised})
		fmt.Printf("revised -> %s (%d items)\n", c.RevisedOut, len(revised))
	}
	return 0
}

func runResetMode(c Config, store contracts.EvalStore) int {
	n, err := store.ResetToStatus(parseCSV(c.ResetStatus))
	if err != nil {
		fmt.Fprintf(os.Stderr, "reset error: %v\n", err)
		return 1
	}
	fmt.Printf("Reset %d items (%s) -> pending\n", n, c.ResetStatus)
	return 0
}

func runReviewExportMode(c Config, store contracts.EvalStore, files contracts.FileStore) int {
	statuses := parseCSV(c.ReviewStatuses)
	if len(statuses) == 0 {
		fmt.Fprintln(os.Stderr, "review export requires at least one status in --review-statuses")
		return 2
	}
	items, err := store.ExportByStatus(statuses...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "review export error: %v\n", err)
		return 1
	}
	lines := make([]string, 0, len(items))
	for _, it := range items {
		row := map[string]any{
			"id":           it["id"],
			"status":       it["status"],
			"en":           it["en"],
			"final_ko":     it["final_ko"],
			"final_risk":   it["final_risk"],
			"final_notes":  it["final_notes"],
			"eval_history": it["eval_history"],
		}
		b, err := json.Marshal(row)
		if err != nil {
			continue
		}
		lines = append(lines, string(b))
	}
	if err := files.WriteLines(c.ReviewExportOut, lines); err != nil {
		fmt.Fprintf(os.Stderr, "review export write error: %v\n", err)
		return 1
	}
	fmt.Printf("review export -> %s (%d items)\n", c.ReviewExportOut, len(lines))
	return 0
}
