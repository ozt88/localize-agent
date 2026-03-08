package translation

import (
	"fmt"
	"sync"
	"time"

	"localize-agent/workflow/internal/contracts"
)

type translationRuntime struct {
	cfg                Config
	sourceStrings      map[string]map[string]any
	currentStrings     map[string]map[string]any
	ids                []string
	idIndex            map[string]int
	lineContexts       map[string]lineContext
	chunkBatches       [][]string
	doneFromCheckpoint map[string]bool
	client             *serverClient
	highClient         *serverClient
	skill              *translateSkill
	checkpoint         contracts.TranslationCheckpointStore
}

type pipelineResult struct {
	completedCount       int
	skippedInvalid       int
	skippedTimeout       int
	skippedTranslatorErr int
	skippedLong          int
	skippedLongIDs       []string
	checkpointErr        error
}

func runPipeline(rt translationRuntime) pipelineResult {
	done := map[string]map[string]any{}
	pack := []map[string]any{}
	var doneMu sync.Mutex
	var cpWriter *checkpointBatchWriter
	if rt.checkpoint.IsEnabled() {
		cpWriter = newCheckpointBatchWriter(rt.checkpoint, 64, 100*time.Millisecond)
		cpWriter.Start()
	}

	skippedInvalid := 0
	skippedTimeout := 0
	skippedTranslatorErr := 0
	skippedLong := 0
	skippedLongIDs := make([]string, 0)
	var countMu sync.Mutex

	worker := func(slot int, jobs <-chan []string, wg *sync.WaitGroup) {
		defer wg.Done()
		slotKey := fmt.Sprintf("go_pool%d", slot)
		_ = rt.client.ensureContext(rt.client.sessionKey(slotKey))
		if rt.highClient != nil {
			_ = rt.highClient.ensureContext(rt.highClient.sessionKey(slotKey))
		}

		for batchIDs := range jobs {
			batchSizeRequested := len(batchIDs)
			batch := buildBatch(rt, batchIDs)
			runItems := batch.runItems
			metas := batch.metas
			if len(runItems) == 0 {
				countMu.Lock()
				skippedInvalid += batch.skippedInvalid
				skippedLong += batch.skippedLong
				if len(batch.skippedLongIDs) > 0 {
					skippedLongIDs = append(skippedLongIDs, batch.skippedLongIDs...)
				}
				countMu.Unlock()
				continue
			}
			countMu.Lock()
			skippedInvalid += batch.skippedInvalid
			skippedLong += batch.skippedLong
			if len(batch.skippedLongIDs) > 0 {
				skippedLongIDs = append(skippedLongIDs, batch.skippedLongIDs...)
			}
			countMu.Unlock()

			proposals, batchInvalid, batchTransErr := collectProposals(rt, slotKey, runItems)
			countMu.Lock()
			skippedInvalid += batchInvalid
			skippedTranslatorErr += batchTransErr
			countMu.Unlock()

			persist := persistResults(rt, slotKey, proposals, metas, done, pack, &doneMu, cpWriter)
			pack = persist.pack
			countMu.Lock()
			skippedInvalid += persist.skippedInvalid
			countMu.Unlock()
			if persist.abortWorker {
				return
			}

			doneMu.Lock()
			doneNow := len(done)
			doneMu.Unlock()
			countMu.Lock()
			si, st, se, sl := skippedInvalid, skippedTimeout, skippedTranslatorErr, skippedLong
			countMu.Unlock()
			fmt.Printf("[slot=%d] batch done requested=%d accepted=%d completed=%d skipped(i/t/e/l)=%d/%d/%d/%d\n", slot, batchSizeRequested, len(runItems), doneNow, si, st, se, sl)
		}
	}

	jobQueue := make(chan []string, rt.cfg.Concurrency*2)
	var wg sync.WaitGroup
	for i := 0; i < rt.cfg.Concurrency; i++ {
		wg.Add(1)
		go worker(i+1, jobQueue, &wg)
	}

	jobBatches := buildJobBatches(rt)
	totalBatches := len(jobBatches)
	for idx, batchIDs := range jobBatches {
		jobQueue <- append([]string(nil), batchIDs...)
		batchNum := idx + 1
		if idx == 0 || batchNum%10 == 0 || batchNum == totalBatches {
			fmt.Printf("Queued batches: %d/%d\n", batchNum, totalBatches)
		}
	}
	close(jobQueue)
	wg.Wait()
	var checkpointErr error
	if cpWriter != nil {
		checkpointErr = cpWriter.Close()
	}

	return pipelineResult{
		completedCount:       len(done),
		skippedInvalid:       skippedInvalid,
		skippedTimeout:       skippedTimeout,
		skippedTranslatorErr: skippedTranslatorErr,
		skippedLong:          skippedLong,
		skippedLongIDs:       skippedLongIDs,
		checkpointErr:        checkpointErr,
	}
}

func (rt translationRuntime) clientForLane(lane string) *serverClient {
	if lane == laneHigh && rt.highClient != nil {
		return rt.highClient
	}
	return rt.client
}

func buildJobBatches(rt translationRuntime) [][]string {
	if len(rt.chunkBatches) > 0 {
		out := coalesceChunkBatches(rt)
		if len(out) > 0 {
			return out
		}
	}
	batchSize := rt.cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 1
	}
	maxBatchChars := rt.cfg.MaxBatchChars
	if maxBatchChars <= 0 {
		out := make([][]string, 0, (len(rt.ids)+batchSize-1)/batchSize)
		for pos := 0; pos < len(rt.ids); pos += batchSize {
			end := pos + batchSize
			if end > len(rt.ids) {
				end = len(rt.ids)
			}
			out = append(out, rt.ids[pos:end])
		}
		return out
	}

	out := make([][]string, 0, (len(rt.ids)+batchSize-1)/batchSize)
	cur := make([]string, 0, batchSize)
	curChars := 0
	for _, id := range rt.ids {
		weight := estimateBatchItemChars(rt, id)
		if len(cur) > 0 && (len(cur) >= batchSize || curChars+weight > maxBatchChars) {
			out = append(out, cur)
			cur = make([]string, 0, batchSize)
			curChars = 0
		}
		cur = append(cur, id)
		curChars += weight
		if len(cur) >= batchSize {
			out = append(out, cur)
			cur = make([]string, 0, batchSize)
			curChars = 0
		}
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

type chunkBatchPlan struct {
	key       string
	taskCount int
	charCount int
}

func coalesceChunkBatches(rt translationRuntime) [][]string {
	batchSize := rt.cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 1
	}
	maxBatchChars := rt.cfg.MaxBatchChars
	out := make([][]string, 0, len(rt.chunkBatches))
	curIDs := make([]string, 0, batchSize)
	curKey := ""
	curTasks := 0
	curChars := 0
	flush := func() {
		if len(curIDs) == 0 {
			return
		}
		out = append(out, append([]string(nil), curIDs...))
		curIDs = make([]string, 0, batchSize)
		curKey = ""
		curTasks = 0
		curChars = 0
	}
	for _, chunkIDs := range rt.chunkBatches {
		if len(chunkIDs) == 0 {
			continue
		}
		plan := planChunkBatch(rt, chunkIDs)
		if plan.taskCount == 0 || plan.key == "" {
			flush()
			out = append(out, append([]string(nil), chunkIDs...))
			continue
		}
		exceedsSize := curTasks > 0 && curTasks+plan.taskCount > batchSize
		exceedsChars := maxBatchChars > 0 && curChars > 0 && curChars+plan.charCount > maxBatchChars
		incompatible := curKey != "" && curKey != plan.key
		if exceedsSize || exceedsChars || incompatible {
			flush()
		}
		curIDs = append(curIDs, chunkIDs...)
		curKey = plan.key
		curTasks += plan.taskCount
		curChars += plan.charCount
		if curTasks >= batchSize {
			flush()
		}
	}
	flush()
	return out
}

func planChunkBatch(rt translationRuntime, ids []string) chunkBatchPlan {
	key := ""
	taskCount := 0
	charCount := 0
	for _, id := range ids {
		enObj, ok := rt.sourceStrings[id]
		if !ok {
			continue
		}
		if _, ok := rt.currentStrings[id]; !ok {
			continue
		}
		enText, _ := enObj["Text"].(string)
		if rt.cfg.MaxPlainLen > 0 && len([]rune(enText)) > rt.cfg.MaxPlainLen {
			continue
		}
		profile := effectiveTextProfile(rt, id, enText)
		prepared := preparePromptText(enText, enText, profile)
		if prepared.passthrough {
			continue
		}
		textRole := ""
		isShortContext := false
		if ctx, ok := rt.lineContexts[id]; ok {
			textRole = ctx.TextRole
			isShortContext = ctx.LineIsShortContextDependent
		}
		itemKey := decideTranslationLane(enText, profile, textRole, isShortContext) + "::" + profileGroupKey(profile)
		if key == "" {
			key = itemKey
		} else if key != itemKey {
			return chunkBatchPlan{}
		}
		taskCount++
		n := len([]rune(prepared.source))
		if n < 1 {
			n = 1
		}
		charCount += n
	}
	return chunkBatchPlan{
		key:       key,
		taskCount: taskCount,
		charCount: charCount,
	}
}

func estimateBatchItemChars(rt translationRuntime, id string) int {
	enObj, ok := rt.sourceStrings[id]
	if !ok {
		return 1
	}
	enText, _ := enObj["Text"].(string)
	if enText == "" {
		return 1
	}
	masked, _ := maskTags(enText)
	n := len([]rune(masked))
	if n < 1 {
		return 1
	}
	return n
}
