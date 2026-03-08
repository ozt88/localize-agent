package semanticreview

import (
	"fmt"
	"sort"
	"sync"

	"localize-agent/workflow/internal/platform"
)

func LoadDoneItemsByIDs(dbPath string, ids []string) ([]ReviewItem, error) {
	return loadDoneItemsFiltered(dbPath, ids, 0)
}

func ReviewDirectItems(cfg Config, items []ReviewItem) ([]ReportItem, error) {
	traceSink, err := platform.NewJSONLTraceSink(cfg.TraceOut)
	if err != nil {
		return nil, fmt.Errorf("trace init error: %w", err)
	}
	if traceSink != nil {
		defer traceSink.Close()
	}
	report, err := runDirectItems(cfg, items, traceSink)
	if err != nil {
		return nil, err
	}
	sort.Slice(report, func(i, j int) bool { return report[i].ScoreFinal > report[j].ScoreFinal })
	return report, nil
}

func runDirectItems(cfg Config, items []ReviewItem, traceSink platform.LLMTraceSink) ([]ReportItem, error) {
	type batchJob struct {
		index int
		items []ReviewItem
	}
	type batchResult struct {
		index  int
		report []ReportItem
		err    error
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
			return nil, fmt.Errorf("semantic review client init error: %w", err)
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
		if result.err != nil {
			return nil, result.err
		}
		reportByBatch[result.index] = result.report
	}

	report := make([]ReportItem, 0, len(items))
	for i := 0; i < len(batches); i++ {
		report = append(report, reportByBatch[i]...)
	}
	return report, nil
}
