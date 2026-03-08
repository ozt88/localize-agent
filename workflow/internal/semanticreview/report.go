package semanticreview

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

func WriteReports(outputDir string, items []ReportItem) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}
	sorted := append([]ReportItem(nil), items...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ScoreFinal > sorted[j].ScoreFinal
	})

	if err := writeJSON(filepath.Join(outputDir, "semantic_review_full.json"), sorted); err != nil {
		return err
	}
	top10 := sorted
	if len(top10) > 10 {
		top10 = top10[:10]
	}
	if err := writeJSON(filepath.Join(outputDir, "semantic_review_top10.json"), top10); err != nil {
		return err
	}
	top50 := sorted
	if len(top50) > 50 {
		top50 = top50[:50]
	}
	return writeJSON(filepath.Join(outputDir, "semantic_review_top50.json"), top50)
}

func writeJSON(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0644)
}
