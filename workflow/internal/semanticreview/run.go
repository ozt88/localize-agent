package semanticreview

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"localize-agent/workflow/internal/platform"
)

func Run(cfg Config) int {
	items, err := LoadDoneItems(cfg.CheckpointDB, cfg.Limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "semantic review load error: %v\n", err)
		return 1
	}
	traceSink, err := platform.NewJSONLTraceSink(cfg.TraceOut)
	if err != nil {
		fmt.Fprintf(os.Stderr, "trace init error: %v\n", err)
		return 1
	}
	if traceSink != nil {
		defer traceSink.Close()
	}

	if strings.EqualFold(cfg.Mode, "direct") {
		return runDirect(cfg, items, traceSink)
	}

	bt, err := NewBacktranslator(cfg, traceSink)
	if err != nil {
		fmt.Fprintf(os.Stderr, "semantic review client init error: %v\n", err)
		return 1
	}
	report := make([]ReportItem, 0, len(items))
	pairs := make([]embeddingPair, 0, len(items))
	backs := map[string]string{}

	for i := 0; i < len(items); i += max(1, cfg.BatchSize) {
		end := i + max(1, cfg.BatchSize)
		if end > len(items) {
			end = len(items)
		}
		batch := items[i:end]
		slotKey := fmt.Sprintf("semanticreview-%d", (i/max(1, cfg.BatchSize))%max(1, cfg.Concurrency))
		backMap, err := bt.BacktranslateBatch(slotKey, batch)
		if err != nil {
			for _, item := range batch {
				backs[item.ID] = "__ERROR__: " + err.Error()
			}
		} else {
			for _, item := range batch {
				if back, ok := backMap[item.ID]; ok && strings.TrimSpace(back) != "" {
					backs[item.ID] = back
				} else {
					backs[item.ID] = "__ERROR__: missing backtranslation"
				}
			}
		}
	}

	for _, item := range items {
		back := backs[item.ID]
		if strings.HasPrefix(back, "__ERROR__:") {
			pairs = append(pairs, embeddingPair{ID: item.ID, A: item.SourceEN, B: item.SourceEN})
			continue
		}
		pairs = append(pairs, embeddingPair{ID: item.ID, A: item.SourceEN, B: back})
	}

	sims, err := ComputeSemanticSimilarities(cfg.OutputDir, pairs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "semantic review embedding error: %v\n", err)
		return 1
	}

	for _, item := range items {
		back := backs[item.ID]
		sim := sims[item.ID]
		report = append(report, BuildReportItem(item, back, sim))
	}
	sort.Slice(report, func(i, j int) bool { return report[i].ScoreFinal > report[j].ScoreFinal })

	if err := WriteReports(cfg.OutputDir, report); err != nil {
		fmt.Fprintf(os.Stderr, "semantic review write error: %v\n", err)
		return 1
	}

	fmt.Printf("Semantic review analyzed=%d\n", len(report))
	fmt.Printf("Output dir: %s\n", filepath.Clean(cfg.OutputDir))
	return 0
}

func runDirect(cfg Config, items []ReviewItem, traceSink platform.LLMTraceSink) int {
	type batchJob struct {
		index int
		items []ReviewItem
	}
	type batchResult struct {
		index  int
		report []ReportItem
	}

	batches := splitReviewBatches(items, max(1, cfg.BatchSize))
	jobs := make(chan batchJob)
	results := make(chan batchResult, len(batches))
	workerCount := min(max(1, cfg.Concurrency), len(batches))
	if workerCount == 0 {
		workerCount = 1
	}

	var wg sync.WaitGroup
	for workerIdx := 0; workerIdx < workerCount; workerIdx++ {
		scorer, err := NewDirectScorer(cfg, traceSink)
		if err != nil {
			fmt.Fprintf(os.Stderr, "semantic review client init error: %v\n", err)
			return 1
		}
		wg.Add(1)
		go func(worker int, scorer *DirectScorer) {
			defer wg.Done()
			for job := range jobs {
				slotKey := fmt.Sprintf("semanticreview-%d", worker)
				results <- batchResult{
					index:  job.index,
					report: scoreDirectBatch(scorer, slotKey, job.items),
				}
			}
		}(workerIdx, scorer)
	}

	go func() {
		for i, batch := range batches {
			jobs <- batchJob{index: i, items: batch}
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	reportByBatch := make(map[int][]ReportItem, len(batches))
	for result := range results {
		reportByBatch[result.index] = result.report
	}

	report := make([]ReportItem, 0, len(items))
	for i := 0; i < len(batches); i++ {
		report = append(report, reportByBatch[i]...)
	}
	sort.Slice(report, func(i, j int) bool { return report[i].ScoreFinal > report[j].ScoreFinal })
	if err := WriteReports(cfg.OutputDir, report); err != nil {
		fmt.Fprintf(os.Stderr, "semantic review write error: %v\n", err)
		return 1
	}
	fmt.Printf("Semantic review analyzed=%d\n", len(report))
	fmt.Printf("Output dir: %s\n", filepath.Clean(cfg.OutputDir))
	return 0
}

func scoreDirectBatch(scorer *DirectScorer, slotKey string, batch []ReviewItem) []ReportItem {
	scoreMap, err := scorer.ScoreBatch(slotKey, batch)
	if err != nil {
		report := make([]ReportItem, 0, len(batch))
		for _, item := range batch {
			report = append(report, ReportItem{
				ID:           item.ID,
				SourceEN:     item.SourceEN,
				TranslatedKO: item.TranslatedKO,
				ScoreFinal:   1.0,
				ReasonTags:   []string{"scoring_error"},
				ShortReason:  err.Error(),
			})
		}
		return report
	}

	report := make([]ReportItem, 0, len(batch))
	for _, item := range batch {
		score, ok := scoreMap[item.ID]
		if !ok {
			report = append(report, ReportItem{
				ID:           item.ID,
				SourceEN:     item.SourceEN,
				TranslatedKO: item.TranslatedKO,
				ScoreFinal:   1.0,
				ReasonTags:   []string{"missing_score"},
				ShortReason:  "model returned no score for item",
			})
			continue
		}
		report = append(report, BuildDirectScoreReportItem(item, score))
	}
	return report
}

func splitReviewBatches(items []ReviewItem, batchSize int) [][]ReviewItem {
	if batchSize <= 0 {
		batchSize = 1
	}
	out := make([][]ReviewItem, 0, (len(items)+batchSize-1)/batchSize)
	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}
		out = append(out, items[i:end])
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
