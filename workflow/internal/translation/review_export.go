package translation

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"localize-agent/workflow/internal/platform"
)

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

func runReviewExport(c Config) int {
	statuses := parseCSV(c.ReviewStatuses)
	if len(statuses) == 0 {
		fmt.Fprintln(os.Stderr, "review export requires at least one status in --review-statuses")
		return 2
	}
	rows, err := platform.ExportTranslationCheckpointRows(c.CheckpointDB, statuses)
	if err != nil {
		fmt.Fprintf(os.Stderr, "review export error: %v\n", err)
		return 1
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		b, err := json.Marshal(row)
		if err != nil {
			continue
		}
		lines = append(lines, string(b))
	}
	files := platform.NewOSFileStore()
	if err := files.WriteLines(c.ReviewExportOut, lines); err != nil {
		fmt.Fprintf(os.Stderr, "review export write error: %v\n", err)
		return 1
	}
	fmt.Printf("review export -> %s (%d items)\n", c.ReviewExportOut, len(lines))
	return 0
}
