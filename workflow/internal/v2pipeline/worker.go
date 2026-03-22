package v2pipeline

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"localize-agent/workflow/internal/clustertranslate"
	"localize-agent/workflow/internal/contracts"
	"localize-agent/workflow/internal/glossary"
	"localize-agent/workflow/internal/inkparse"
	"localize-agent/workflow/internal/scorellm"
	"localize-agent/workflow/internal/tagformat"
	"localize-agent/workflow/pkg/platform"
)

// TranslateWorker runs the translate stage loop for a single worker.
// Session key: "v2-translate-{workerID}" per Pitfall 3.
func TranslateWorker(ctx context.Context, cfg Config, store contracts.V2PipelineStore,
	llm *platform.SessionLLMClient, glossarySet *glossary.GlossarySet,
	translateProfile, highProfile platform.LLMProfile,
	workerID string) error {

	sessionKey := "v2-translate-" + workerID

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		items, err := store.ClaimPending(StatePendingTranslate, StateWorkingTranslate, workerID, cfg.TranslateBatchSize, cfg.LeaseSec)
		if err != nil {
			return fmt.Errorf("translate claim: %w", err)
		}
		if len(items) == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(cfg.IdleSleepSec) * time.Second):
				continue
			}
		}

		// Group items by batch_id for scene-context translation (TRANS-01).
		batches := groupByBatchID(items)
		for batchID, batchItems := range batches {
			if err := translateBatch(ctx, cfg, store, llm, glossarySet, translateProfile, highProfile, sessionKey, workerID, batchID, batchItems); err != nil {
				// Log error but continue processing other batches.
				fmt.Fprintf(os.Stderr, "[translate-%s] batch %s error: %v\n", workerID, batchID, err)
			}
		}

		if cfg.Once {
			return nil
		}
	}
}

// translateBatch processes a single batch group of items through translation.
func translateBatch(ctx context.Context, cfg Config, store contracts.V2PipelineStore,
	llm *platform.SessionLLMClient, glossarySet *glossary.GlossarySet,
	translateProfile, highProfile platform.LLMProfile,
	sessionKey, workerID, batchID string, items []contracts.V2PipelineItem) error {

	// Build ClusterTask from items.
	blocks := make([]inkparse.DialogueBlock, len(items))
	sourceTexts := make([]string, len(items))
	for i, item := range items {
		blocks[i] = inkparse.DialogueBlock{
			ID:      item.ID,
			Text:    item.SourceRaw,
			Speaker: item.Speaker,
			Choice:  item.Choice,
		}
		sourceTexts[i] = item.SourceRaw
	}

	batch := inkparse.Batch{
		ID:          batchID,
		Blocks:      blocks,
		ContentType: items[0].ContentType,
	}

	// Build per-batch glossary (D-11).
	var glossaryJSON string
	if glossarySet != nil {
		batchText := collectBatchText(items)
		warmupTerms := glossarySet.WarmupTerms(50)
		batchTerms := glossarySet.FilterForBatch(batchText, warmupTerms)
		if len(batchTerms) > 0 {
			glossaryJSON = glossarySet.FormatJSON(batchTerms)
		}
	}

	// Fetch previous gate context for D-03 injection.
	var prevGateLines []string
	if items[0].Gate != "" {
		prevGateLines, _ = store.GetPrevGateLines(items[0].Knot, items[0].Gate, 3)
	}

	task := clustertranslate.ClusterTask{
		Batch:         batch,
		GlossaryJSON:  glossaryJSON,
		PrevGateLines: prevGateLines,
	}

	prompt, meta := clustertranslate.BuildScriptPrompt(task)

	// Determine which profile to use based on max attempts among items.
	maxAttempts := maxTranslateAttempts(items)
	profile := translateProfile
	if maxAttempts >= 2 {
		profile = highProfile // D-15: escalation after 2 attempts.
	}

	// Warmup and send.
	if err := llm.EnsureContext(sessionKey, profile); err != nil {
		return fmt.Errorf("translate warmup: %w", err)
	}
	rawOutput, err := llm.SendPrompt(sessionKey, profile, prompt)
	if err != nil {
		// Mark all items for retry on LLM error.
		for _, item := range items {
			logAttempt(store, item.ID, "translate", profile.ModelID, "", err.Error(), -1, item.TranslateAttempts+1, cfg.MaxRetries)
			handleRetry(store, item, "translate", cfg.MaxRetries, err.Error())
		}
		return fmt.Errorf("translate send: %w", err)
	}

	// Filter sourceTexts to only translatable (non-punctuation) items.
	var translatableSourceTexts []string
	for _, blockID := range meta.BlockIDOrder {
		for _, item := range items {
			if item.ID == blockID {
				translatableSourceTexts = append(translatableSourceTexts, item.SourceRaw)
				break
			}
		}
	}

	// Validate output (TRANS-04, D-13).
	if err := clustertranslate.ValidateTranslation(rawOutput, meta, translatableSourceTexts); err != nil {
		// Validation failed -- retry all items in batch.
		for _, item := range items {
			reason := fmt.Sprintf("validation: %v", err)
			logAttempt(store, item.ID, "translate", profile.ModelID, "", reason, -1, item.TranslateAttempts+1, cfg.MaxRetries)
			handleRetry(store, item, "translate", cfg.MaxRetries, reason)
		}
		return nil // not a fatal error; items retried
	}

	// Parse and map output.
	parsed, parseErr := clustertranslate.ParseNumberedOutput(rawOutput)
	if parseErr != nil {
		for _, item := range items {
			reason := fmt.Sprintf("parse: %v", parseErr)
			logAttempt(store, item.ID, "translate", profile.ModelID, "", reason, -1, item.TranslateAttempts+1, cfg.MaxRetries)
			handleRetry(store, item, "translate", cfg.MaxRetries, reason)
		}
		return nil
	}

	mapping, mapErr := clustertranslate.MapLinesToIDs(parsed, meta)
	if mapErr != nil {
		for _, item := range items {
			reason := fmt.Sprintf("map: %v", mapErr)
			logAttempt(store, item.ID, "translate", profile.ModelID, "", reason, -1, item.TranslateAttempts+1, cfg.MaxRetries)
			handleRetry(store, item, "translate", cfg.MaxRetries, reason)
		}
		return nil
	}

	// Mark translated for mapped items (routes by has_tags).
	for blockID, koRaw := range mapping {
		if err := store.MarkTranslated(blockID, koRaw); err != nil {
			return fmt.Errorf("mark translated %s: %w", blockID, err)
		}
		logAttempt(store, blockID, "translate", profile.ModelID, "", "", -1, 0, 0)
	}

	// Mark excluded items (punctuation-only per D-13) as done with source preserved.
	for _, excludedID := range meta.ExcludedBlockIDs {
		// Find source_raw for this excluded block to preserve as output.
		var sourceRaw string
		for _, item := range items {
			if item.ID == excludedID {
				sourceRaw = item.SourceRaw
				break
			}
		}
		if err := store.MarkDonePassthrough(excludedID, sourceRaw); err != nil {
			return fmt.Errorf("mark excluded %s: %w", excludedID, err)
		}
	}

	return nil
}

// FormatWorker runs the format stage loop for a single worker.
// Session key: "v2-format-{workerID}" per Pitfall 3.
func FormatWorker(ctx context.Context, cfg Config, store contracts.V2PipelineStore,
	llm *platform.SessionLLMClient,
	formatProfile, highProfile platform.LLMProfile,
	workerID string) error {

	sessionKey := "v2-format-" + workerID

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		items, err := store.ClaimPending(StatePendingFormat, StateWorkingFormat, workerID, cfg.FormatBatchSize, cfg.LeaseSec)
		if err != nil {
			return fmt.Errorf("format claim: %w", err)
		}
		if len(items) == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(cfg.IdleSleepSec) * time.Second):
				continue
			}
		}

		// Process in small sub-batches of 3-5 per D-06.
		subBatchSize := 5
		if subBatchSize > len(items) {
			subBatchSize = len(items)
		}
		for i := 0; i < len(items); i += subBatchSize {
			end := i + subBatchSize
			if end > len(items) {
				end = len(items)
			}
			subBatch := items[i:end]

			if err := formatSubBatch(ctx, cfg, store, llm, formatProfile, highProfile, sessionKey, workerID, subBatch); err != nil {
				fmt.Fprintf(os.Stderr, "[format-%s] sub-batch error: %v\n", workerID, err)
			}
		}

		if cfg.Once {
			return nil
		}
	}
}

// formatSubBatch processes a small group of items through tag formatting.
func formatSubBatch(ctx context.Context, cfg Config, store contracts.V2PipelineStore,
	llm *platform.SessionLLMClient,
	formatProfile, highProfile platform.LLMProfile,
	sessionKey, workerID string, items []contracts.V2PipelineItem) error {

	// Build FormatTasks.
	tasks := make([]tagformat.FormatTask, len(items))
	for i, item := range items {
		tasks[i] = tagformat.FormatTask{
			BlockID:  item.ID,
			ENSource: item.SourceRaw,
			KOPlain:  item.KORaw,
		}
	}

	// Determine profile based on attempts (D-15 escalation).
	maxAttempts := maxFormatAttempts(items)
	profile := formatProfile
	if maxAttempts >= 2 {
		profile = highProfile
	}

	// Warmup and send.
	if err := llm.EnsureContext(sessionKey, profile); err != nil {
		return fmt.Errorf("format warmup: %w", err)
	}
	prompt := tagformat.BuildFormatPrompt(tasks)
	rawOutput, err := llm.SendPrompt(sessionKey, profile, prompt)
	if err != nil {
		for _, item := range items {
			logAttempt(store, item.ID, "format", profile.ModelID, "", err.Error(), -1, item.FormatAttempts+1, cfg.MaxRetries)
			handleRetry(store, item, "format", cfg.MaxRetries, err.Error())
		}
		return fmt.Errorf("format send: %w", err)
	}

	// Parse response.
	results, parseErr := tagformat.ParseFormatResponse(rawOutput, len(tasks))
	if parseErr != nil {
		for _, item := range items {
			reason := fmt.Sprintf("parse: %v", parseErr)
			logAttempt(store, item.ID, "format", profile.ModelID, "", reason, -1, item.FormatAttempts+1, cfg.MaxRetries)
			handleRetry(store, item, "format", cfg.MaxRetries, reason)
		}
		return nil
	}

	// Validate each result per D-07.
	for i, item := range items {
		koFormatted := results[i]
		if err := tagformat.ValidateTagMatch(item.SourceRaw, koFormatted); err != nil {
			reason := fmt.Sprintf("tag mismatch: %v", err)
			logAttempt(store, item.ID, "format", profile.ModelID, "", reason, -1, item.FormatAttempts+1, cfg.MaxRetries)
			handleRetry(store, item, "format", cfg.MaxRetries, reason)
			continue
		}

		// Tag validation passed -- mark formatted.
		if err := store.MarkFormatted(item.ID, koFormatted); err != nil {
			return fmt.Errorf("mark formatted %s: %w", item.ID, err)
		}
		logAttempt(store, item.ID, "format", profile.ModelID, "", "", -1, 0, 0)
	}

	return nil
}

// ScoreWorker runs the score stage loop for a single worker.
// Session key: "v2-score-{workerID}" per Pitfall 3.
func ScoreWorker(ctx context.Context, cfg Config, store contracts.V2PipelineStore,
	llm *platform.SessionLLMClient,
	scoreProfile platform.LLMProfile,
	workerID string) error {

	sessionKey := "v2-score-" + workerID

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		items, err := store.ClaimPending(StatePendingScore, StateWorkingScore, workerID, cfg.ScoreBatchSize, cfg.LeaseSec)
		if err != nil {
			return fmt.Errorf("score claim: %w", err)
		}
		if len(items) == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(cfg.IdleSleepSec) * time.Second):
				continue
			}
		}

		// Score each item individually (Open Question 2 in RESEARCH.md).
		for _, item := range items {
			if err := scoreItem(ctx, cfg, store, llm, scoreProfile, sessionKey, workerID, item); err != nil {
				fmt.Fprintf(os.Stderr, "[score-%s] item %s error: %v\n", workerID, item.ID, err)
			}
		}

		if cfg.Once {
			return nil
		}
	}
}

// scoreItem processes a single item through the Score LLM.
func scoreItem(ctx context.Context, cfg Config, store contracts.V2PipelineStore,
	llm *platform.SessionLLMClient, scoreProfile platform.LLMProfile,
	sessionKey, workerID string, item contracts.V2PipelineItem) error {

	// Build ScoreTask. Use KOFormatted if available, otherwise KORaw.
	koText := item.KOFormatted
	if koText == "" {
		koText = item.KORaw
	}
	task := scorellm.ScoreTask{
		BlockID:     item.ID,
		ENSource:    item.SourceRaw,
		KOFormatted: koText,
		HasTags:     item.HasTags,
	}

	// Warmup and send.
	if err := llm.EnsureContext(sessionKey, scoreProfile); err != nil {
		return fmt.Errorf("score warmup: %w", err)
	}
	prompt := scorellm.BuildScorePrompt(task)
	rawOutput, err := llm.SendPrompt(sessionKey, scoreProfile, prompt)
	if err != nil {
		logAttempt(store, item.ID, "score", scoreProfile.ModelID, "", err.Error(), -1, item.ScoreAttempts+1, cfg.MaxRetries)
		// Score LLM error -> retry score, not translation failure.
		_ = store.UpdateRetryState(item.ID, StatePendingScore, "score_attempts")
		return fmt.Errorf("score send: %w", err)
	}

	// Parse response (Pitfall 5: handle invalid JSON).
	result, parseErr := scorellm.ParseScoreResponse(rawOutput)
	if parseErr != nil {
		reason := fmt.Sprintf("parse: %v", parseErr)
		logAttempt(store, item.ID, "score", scoreProfile.ModelID, "", reason, -1, item.ScoreAttempts+1, cfg.MaxRetries)
		_ = store.UpdateRetryState(item.ID, StatePendingScore, "score_attempts")
		return nil // not fatal; will retry
	}

	// Mark scored -- this auto-routes per D-14.
	scoreFinal := result.ScoreFinal()
	if err := store.MarkScored(item.ID, scoreFinal, result.FailureType, result.Reason); err != nil {
		return fmt.Errorf("mark scored %s: %w", item.ID, err)
	}

	logAttempt(store, item.ID, "score", scoreProfile.ModelID, result.FailureType, result.Reason, scoreFinal, 0, 0)

	return nil
}

// handleRetry implements D-15 retry strategy: same model 2x with hint, then escalation, then fail.
func handleRetry(store contracts.V2PipelineStore, item contracts.V2PipelineItem, stage string, maxRetries int, lastError string) {
	var attempts int
	var incrementField, targetState string

	switch stage {
	case "translate":
		attempts = item.TranslateAttempts
		incrementField = "translate_attempts"
		targetState = StatePendingTranslate
	case "format":
		attempts = item.FormatAttempts
		incrementField = "format_attempts"
		targetState = StatePendingFormat
	default:
		return
	}

	if attempts+1 >= maxRetries {
		// Max retries reached -- mark failed (D-16).
		_ = store.MarkFailed(item.ID, lastError)
		return
	}

	// Retry: same model with hint (attempts < 2) or escalation (attempts >= 2).
	_ = store.UpdateRetryState(item.ID, targetState, incrementField)
}

// logAttempt appends an attempt log entry per D-16.
func logAttempt(store contracts.V2PipelineStore, id, stage, model, failureType, reason string, score float64, attemptNum, maxRetries int) {
	entry := map[string]interface{}{
		"stage":     stage,
		"model":     model,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	if failureType != "" {
		entry["failure_type"] = failureType
	}
	if reason != "" {
		entry["reason"] = reason
	}
	if score >= 0 {
		entry["score"] = score
	}
	_ = store.AppendAttemptLog(id, entry)
}

// groupByBatchID groups items by their BatchID for scene-context translation.
func groupByBatchID(items []contracts.V2PipelineItem) map[string][]contracts.V2PipelineItem {
	groups := make(map[string][]contracts.V2PipelineItem)
	for _, item := range items {
		key := item.BatchID
		if key == "" {
			key = item.ID // fallback: treat each item as its own batch
		}
		groups[key] = append(groups[key], item)
	}
	return groups
}

// collectBatchText concatenates source text from all items for glossary filtering.
func collectBatchText(items []contracts.V2PipelineItem) string {
	var b strings.Builder
	for _, item := range items {
		b.WriteString(item.SourceRaw)
		b.WriteByte(' ')
	}
	return b.String()
}

// maxTranslateAttempts returns the maximum translate_attempts among items.
func maxTranslateAttempts(items []contracts.V2PipelineItem) int {
	max := 0
	for _, item := range items {
		if item.TranslateAttempts > max {
			max = item.TranslateAttempts
		}
	}
	return max
}

// maxFormatAttempts returns the maximum format_attempts among items.
func maxFormatAttempts(items []contracts.V2PipelineItem) int {
	max := 0
	for _, item := range items {
		if item.FormatAttempts > max {
			max = item.FormatAttempts
		}
	}
	return max
}
