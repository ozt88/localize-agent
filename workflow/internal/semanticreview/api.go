package semanticreview

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"localize-agent/workflow/pkg/platform"
)

type DirectReviewRunner struct {
	cfg       Config
	traceSink platform.LLMTraceSink
	scorers   []*DirectScorer
}

func writeRunnerPhase(traceSink platform.LLMTraceSink, phase string, details map[string]any) {
	if traceSink == nil {
		return
	}
	event := platform.LLMTraceEvent{
		Kind: "runner_phase",
		Path: phase,
	}
	if worker, ok := details["worker"].(string); ok {
		event.SessionKey = worker
	}
	if payload, err := jsonMarshalNoError(details); err == nil {
		event.Request = payload
	}
	_ = traceSink.Write(event)
}

func jsonMarshalNoError(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func LoadDoneItemsByIDs(cfg Config, ids []string) ([]ReviewItem, error) {
	return loadDoneItemsFiltered(cfg, ids, 0)
}

func NewDirectReviewRunner(cfg Config) (*DirectReviewRunner, error) {
	traceSink, err := platform.NewJSONLTraceSink(cfg.TraceOut)
	if err != nil {
		return nil, fmt.Errorf("trace init error: %w", err)
	}
	workerCount := max(1, cfg.Concurrency)
	scorers := make([]*DirectScorer, 0, workerCount)
	for workerIdx := 0; workerIdx < workerCount; workerIdx++ {
		scorer, err := newDirectScorer(cfg, traceSink)
		if err != nil {
			if traceSink != nil {
				_ = traceSink.Close()
			}
			return nil, fmt.Errorf("semantic review client init error: %w", err)
		}
		scorers = append(scorers, scorer)
	}
	return &DirectReviewRunner{
		cfg:       cfg,
		traceSink: traceSink,
		scorers:   scorers,
	}, nil
}

func (r *DirectReviewRunner) Close() error {
	if r == nil || r.traceSink == nil {
		return nil
	}
	return r.traceSink.Close()
}

func (r *DirectReviewRunner) ReviewItems(items []ReviewItem) ([]ReportItem, error) {
	if r == nil {
		return nil, fmt.Errorf("direct review runner is nil")
	}
	report, err := runDirectItemsWithScorers(r.cfg, items, r.scorers, r.traceSink)
	if err != nil {
		return nil, err
	}
	sort.Slice(report, func(i, j int) bool { return report[i].ScoreFinal > report[j].ScoreFinal })
	return report, nil
}

func runDirectItemsWithScorers(cfg Config, items []ReviewItem, scorers []*DirectScorer, traceSink platform.LLMTraceSink) ([]ReportItem, error) {
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
	workerCount := min(len(scorers), len(batches))
	if workerCount == 0 {
		workerCount = 1
	}

	var wg sync.WaitGroup
	for workerIdx := 0; workerIdx < workerCount; workerIdx++ {
		scorer := scorers[workerIdx]
		wg.Add(1)
		go func(worker int, scorer *DirectScorer) {
			defer wg.Done()
			workerName := fmt.Sprintf("semanticreview-worker-%d", worker)
			writeRunnerPhase(traceSink, "worker_start", map[string]any{
				"worker": workerName,
			})
			for job := range jobs {
				slotKey := fmt.Sprintf("semanticreview-%d", worker)
				writeRunnerPhase(traceSink, "job_dequeued", map[string]any{
					"worker":     workerName,
					"batch_index": job.index,
					"batch_size": len(job.items),
					"first_id":   job.items[0].ID,
					"last_id":    job.items[len(job.items)-1].ID,
				})
				writeRunnerPhase(traceSink, "score_direct_batch_start", map[string]any{
					"worker":     workerName,
					"batch_index": job.index,
					"batch_size": len(job.items),
				})
				report := scoreDirectBatch(scorer, slotKey, job.items)
				writeRunnerPhase(traceSink, "score_direct_batch_done", map[string]any{
					"worker":      workerName,
					"batch_index": job.index,
					"batch_size":  len(job.items),
					"report_size": len(report),
				})
				results <- batchResult{
					index:  job.index,
					report: report,
				}
				writeRunnerPhase(traceSink, "result_sent", map[string]any{
					"worker":      workerName,
					"batch_index": job.index,
					"report_size": len(report),
				})
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
		writeRunnerPhase(traceSink, "result_received", map[string]any{
			"batch_index": result.index,
			"report_size": len(result.report),
		})
		reportByBatch[result.index] = result.report
	}

	report := make([]ReportItem, 0, len(items))
	for i := 0; i < len(batches); i++ {
		report = append(report, reportByBatch[i]...)
	}
	return report, nil
}
