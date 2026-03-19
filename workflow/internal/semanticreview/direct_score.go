package semanticreview

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"localize-agent/workflow/pkg/platform"
	"localize-agent/workflow/pkg/shared"
)

type directScoreResult struct {
	ID           string   `json:"id"`
	CurrentScore float64  `json:"current_score"`
	FreshScore   float64  `json:"fresh_score"`
	ReasonTags   []string `json:"reason_tags,omitempty"`
	ShortReason  string   `json:"short_reason,omitempty"`
}

type DirectScorer struct {
	client        llmClient
	profile       platform.LLMProfile
	sessionPrefix string
	scoreOnly     bool
	promptVariant string
	traceSink      platform.LLMTraceSink
}

func newDirectScorer(cfg Config, traceSink platform.LLMTraceSink) (*DirectScorer, error) {
	client, profile, sessionPrefix, err := newReviewClientAndProfile(cfg, traceSink, directScoreWarmup(cfg), nil)
	if err != nil {
		return nil, err
	}
	return &DirectScorer{
		client:        client,
		profile:       profile,
		sessionPrefix: sessionPrefix,
		scoreOnly:     cfg.ScoreOnly,
		promptVariant: strings.TrimSpace(cfg.PromptVariant),
		traceSink:      traceSink,
	}, nil
}

func (d *DirectScorer) ScoreBatch(slotKey string, items []ReviewItem) (map[string]directScoreResult, error) {
	sessionKey := d.sessionPrefix + "#" + slotKey
	firstID := ""
	lastID := ""
	if len(items) > 0 {
		firstID = items[0].ID
		lastID = items[len(items)-1].ID
	}
	writeRunnerPhase(d.traceSink, "score_batch_enter", map[string]any{
		"worker":     slotKey,
		"batch_size": len(items),
		"first_id":   firstID,
		"last_id":    lastID,
	})
	writeRunnerPhase(d.traceSink, "score_batch_send_prompt_start", map[string]any{
		"worker":     slotKey,
		"batch_size": len(items),
		"first_id":   firstID,
		"last_id":    lastID,
	})
	raw, err := d.client.SendPrompt(sessionKey, d.profile, buildMinimalDirectScorePrompt(items, d.promptVariant))
	if err != nil {
		writeRunnerPhase(d.traceSink, "score_batch_send_prompt_error", map[string]any{
			"worker":     slotKey,
			"batch_size": len(items),
			"first_id":   firstID,
			"last_id":    lastID,
			"error":      err.Error(),
		})
		return nil, err
	}
	writeRunnerPhase(d.traceSink, "score_batch_send_prompt_done", map[string]any{
		"worker":         slotKey,
		"batch_size":     len(items),
		"first_id":       firstID,
		"last_id":        lastID,
		"raw_len":        len(raw),
		"raw_trimmed_len": len(strings.TrimSpace(raw)),
	})
	writeRunnerPhase(d.traceSink, "score_batch_extract_start", map[string]any{
		"worker":     slotKey,
		"batch_size": len(items),
		"first_id":   firstID,
		"last_id":    lastID,
	})
	objects := extractDirectScoreObjects(strings.TrimSpace(raw), items, d.scoreOnly)
	if len(objects) == 0 {
		writeRunnerPhase(d.traceSink, "score_batch_extract_empty", map[string]any{
			"worker":     slotKey,
			"batch_size": len(items),
			"first_id":   firstID,
			"last_id":    lastID,
		})
		return nil, fmt.Errorf("no direct score objects in response")
	}
	writeRunnerPhase(d.traceSink, "score_batch_extract_done", map[string]any{
		"worker":      slotKey,
		"batch_size":  len(items),
		"first_id":    firstID,
		"last_id":     lastID,
		"object_count": len(objects),
	})
	out := map[string]directScoreResult{}
	for _, obj := range objects {
		out[obj.ID] = obj
	}
	return out, nil
}

func buildMinimalDirectScorePrompt(items []ReviewItem, variant string) string {
	type payloadItem struct {
		ID        string `json:"id"`
		SourceEN  string `json:"source_en"`
		CurrentKO string `json:"current_ko,omitempty"`
		FreshKO   string `json:"fresh_ko,omitempty"`
		TextRole  string `json:"text_role,omitempty"`
	}
	payload := make([]payloadItem, 0, len(items))
	ultra := strings.EqualFold(strings.TrimSpace(variant), "ultra")
	stripQuotes := func(s string) string { return strings.ReplaceAll(s, "\"", "") }
	for _, item := range items {
		row := payloadItem{
			ID:        item.ID,
			SourceEN:  stripQuotes(item.SourceEN),
			CurrentKO: stripQuotes(item.CurrentKO),
			FreshKO:   stripQuotes(item.FreshKO),
		}
		if !ultra {
			row.TextRole = item.TextRole
		}
		payload = append(payload, row)
	}
	b, _ := json.Marshal(payload)
	prompt := ""
	prompt += "Return exactly one JSON array.\n"
	prompt += "Each output entry must be [current_score, fresh_score].\n"
	prompt += "Rules:\n"
	prompt += "- integers 0 to 100, use the full rubric range from warmup\n"
	prompt += "- 90+: meaning preserved + natural Korean + correct tone\n"
	prompt += "- 80-89: meaning preserved + natural, minor issues\n"
	prompt += "- 70-79: meaning OK but awkward, or natural but meaning drifts\n"
	prompt += "- <70: meaning loss, mistranslation, or broken output\n"
	prompt += "- fragmentary source → fragmentary Korean is CORRECT, score the fragment fairly\n"
	prompt += "- short valid translations (1-5 words) deserve 85+ if meaning is preserved\n"
	if !ultra {
		prompt += "- if text_role is choice, prefer concise actionable option wording\n"
	}
	prompt += "- return exactly N entries for N input items\n"
	prompt += "- preserve input order exactly\n"
	prompt += "- no explanation, no extra text\n"
	prompt += "Example output:\n"
	prompt += "[[91,84],[70,88]]\n"
	return prompt + "Input items: " + string(b)
}

func buildBatchDirectScorePrompt(items []ReviewItem, detailed bool) string {
	type payloadItem struct {
		ID          string `json:"id"`
		SourceEN    string `json:"source_en"`
		CurrentKO   string `json:"current_ko,omitempty"`
		FreshKO     string `json:"fresh_ko,omitempty"`
		PrevEN      string `json:"prev_en,omitempty"`
		NextEN      string `json:"next_en,omitempty"`
		PrevKO      string `json:"prev_ko,omitempty"`
		NextKO      string `json:"next_ko,omitempty"`
		TextRole    string `json:"text_role,omitempty"`
		SpeakerHint string `json:"speaker_hint,omitempty"`
		ContextEN   string `json:"context_en,omitempty"`
		RetryReason string `json:"retry_reason,omitempty"`
	}
	payload := make([]payloadItem, 0, len(items))
	for _, item := range items {
		payload = append(payload, payloadItem{
			ID:          item.ID,
			SourceEN:    item.SourceEN,
			CurrentKO:   item.CurrentKO,
			FreshKO:     item.FreshKO,
			PrevEN:      item.PrevEN,
			NextEN:      item.NextEN,
			PrevKO:      item.PrevKO,
			NextKO:      item.NextKO,
			TextRole:    item.TextRole,
			SpeakerHint: item.SpeakerHint,
			ContextEN:   item.ContextEN,
			RetryReason: item.RetryReason,
		})
	}
	b, _ := json.Marshal(payload)
	_ = detailed
	prompt := ""
	prompt += "Return one JSON object per line.\n"
	prompt += "You will receive a JSON array of input items.\n"
	prompt += "Each input item has this schema:\n"
	prompt += "{\n"
	prompt += "  \"id\": string,\n"
	prompt += "  \"source_en\": string,\n"
	prompt += "  \"current_ko\": string,\n"
	prompt += "  \"fresh_ko\": string,\n"
	prompt += "  \"prev_en\": string,\n"
	prompt += "  \"next_en\": string,\n"
	prompt += "  \"prev_ko\": string,\n"
	prompt += "  \"next_ko\": string,\n"
	prompt += "  \"text_role\": string,\n"
	prompt += "  \"speaker_hint\": string,\n"
	prompt += "  \"context_en\": string,\n"
	prompt += "  \"retry_reason\": string\n"
	prompt += "}\n"
	prompt += "Task:\n"
	prompt += "For each input item, return exactly one output object on its own line in the same order.\n"
	prompt += "Required output schema:\n"
	prompt += "{\"current_score\": 0, \"fresh_score\": 0}\n"
	prompt += "{\"current_score\": 0, \"fresh_score\": 0}\n"
	prompt += "Output rules:\n"
	prompt += "- Return exactly N lines for N input items.\n"
	prompt += "- Preserve input order exactly.\n"
	prompt += "- Each object must contain exactly two keys: current_score and fresh_score.\n"
	prompt += "- Both values must be integers from 0 to 100.\n"
	prompt += "- If current_ko is empty, broken, or unusable, set current_score to 0.\n"
	prompt += "- If fresh_ko is empty, broken, or unusable, set fresh_score to 0.\n"
	prompt += "- If a candidate is ambiguous, still assign one integer score.\n"
	prompt += "- Never omit an item.\n"
	prompt += "- Never omit a field.\n"
	prompt += "- Never return null.\n"
	prompt += "- Never return strings for scores.\n"
	prompt += "- Never return decimals.\n"
	prompt += "Example 1\n"
	prompt += "Input:\n"
	prompt += "[\n"
	prompt += "  {\n"
	prompt += "    \"id\": \"line-1\",\n"
	prompt += "    \"source_en\": \"Do you trust him?\",\n"
	prompt += "    \"current_ko\": \"당신은 그를 신뢰합니까?\",\n"
	prompt += "    \"fresh_ko\": \"넌 그를 믿어?\",\n"
	prompt += "    \"prev_en\": \"He looks away.\",\n"
	prompt += "    \"next_en\": \"Answer me.\",\n"
	prompt += "    \"prev_ko\": \"그는 시선을 피한다.\",\n"
	prompt += "    \"next_ko\": \"대답해.\",\n"
	prompt += "    \"text_role\": \"dialogue\",\n"
	prompt += "    \"speaker_hint\": \"Snell\",\n"
	prompt += "    \"context_en\": \"He looks away.\\nDo you trust him?\\nAnswer me.\",\n"
	prompt += "    \"retry_reason\": \"\"\n"
	prompt += "  }\n"
	prompt += "]\n"
	prompt += "Output:\n"
	prompt += "{\"current_score\":54,\"fresh_score\":86}\n"
	prompt += "Example 2\n"
	prompt += "Input:\n"
	prompt += "[\n"
	prompt += "  {\n"
	prompt += "    \"id\": \"line-2\",\n"
	prompt += "    \"source_en\": \"Force him to return the papers.\",\n"
	prompt += "    \"current_ko\": \"그가 서류를 돌려주도록 강요한다.\",\n"
	prompt += "    \"fresh_ko\": \"[힘 14] 서류를 돌려놓으라고 밀어붙인다.\",\n"
	prompt += "    \"prev_en\": \"He hesitates.\",\n"
	prompt += "    \"next_en\": \"\",\n"
	prompt += "    \"prev_ko\": \"그는 머뭇거린다.\",\n"
	prompt += "    \"next_ko\": \"\",\n"
	prompt += "    \"text_role\": \"choice\",\n"
	prompt += "    \"speaker_hint\": \"\",\n"
	prompt += "    \"context_en\": \"He hesitates.\\nCHOICE OPTION: Force him to return the papers.\",\n"
	prompt += "    \"retry_reason\": \"\"\n"
	prompt += "  }\n"
	prompt += "]\n"
	prompt += "Output:\n"
	prompt += "{\"current_score\":58,\"fresh_score\":90}\n"
	prompt += "Formatting rules:\n"
	prompt += "- Output only NDJSON: one JSON object per line.\n"
	prompt += "- Do not wrap it in markdown.\n"
	prompt += "- Do not add any text before the first {.\n"
	prompt += "- Do not add any text after the last }.\n"
	prompt += "- Each line must be a complete JSON object.\n"
	prompt += "- Do not use a JSON array.\n"
	return prompt + "Input items: " + string(b)
}

func extractDirectScoreObjects(raw string, items []ReviewItem, scoreOnly bool) []directScoreResult {
	if rows := extractDirectScoreArray(raw, items); len(rows) > 0 {
		return rows
	}
	if rows := extractDirectScoreNDJSON(raw, items); len(rows) > 0 {
		return rows
	}
	if scoreOnly {
		if rows := extractPlainDirectScoreObjects(raw, items); len(rows) > 0 {
			return rows
		}
	}
	var arr []directScoreResult
	if err := json.Unmarshal([]byte(raw), &arr); err == nil && len(arr) > 0 {
		return hydrateDirectScoresByOrder(arr, items)
	}
	var wrapped struct {
		Items []directScoreResult `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &wrapped); err == nil && len(wrapped.Items) > 0 {
		return hydrateDirectScoresByOrder(wrapped.Items, items)
	}
	out := []directScoreResult{}
	for _, chunk := range shared.ExtractJSONObjectChunks(raw) {
		var row directScoreResult
		if err := json.Unmarshal([]byte(chunk), &row); err == nil && row.ID != "" {
			row.CurrentScore = normalizeJudgeScore(row.CurrentScore)
			row.FreshScore = normalizeJudgeScore(row.FreshScore)
			out = append(out, row)
		}
	}
	return out
}

func extractDirectScoreNDJSON(raw string, items []ReviewItem) []directScoreResult {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) == 0 {
		return nil
	}
	out := make([]directScoreResult, 0, len(lines))
	for idx, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row directScoreResult
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil
		}
		if strings.TrimSpace(row.ID) == "" && idx < len(items) {
			row.ID = items[idx].ID
		}
		row.CurrentScore = normalizeJudgeScore(row.CurrentScore)
		row.FreshScore = normalizeJudgeScore(row.FreshScore)
		out = append(out, row)
	}
	return out
}

func extractDirectScoreArray(raw string, items []ReviewItem) []directScoreResult {
	if len(items) == 0 {
		return nil
	}
	var arr []any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &arr); err != nil || len(arr) == 0 {
		return nil
	}
	if len(items) == 1 && len(arr) >= 2 {
		if currentScore, ok := numberToFloat(arr[0]); ok {
			if freshScore, ok := numberToFloat(arr[1]); ok {
				return []directScoreResult{{
					ID:           items[0].ID,
					CurrentScore: normalizeJudgeScore(currentScore),
					FreshScore:   normalizeJudgeScore(freshScore),
				}}
			}
		}
	}
	out := make([]directScoreResult, 0, len(arr))
	for idx, entry := range arr {
		if idx >= len(items) {
			break
		}
		parts, ok := entry.([]any)
		if !ok || len(parts) < 2 {
			return nil
		}
		currentScore, ok := numberToFloat(parts[0])
		if !ok {
			return nil
		}
		freshScore, ok := numberToFloat(parts[1])
		if !ok {
			return nil
		}
		row := directScoreResult{
			ID:           items[idx].ID,
			CurrentScore: normalizeJudgeScore(currentScore),
			FreshScore:   normalizeJudgeScore(freshScore),
		}
		out = append(out, row)
	}
	return out
}

func numberToFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func extractPlainDirectScoreObjects(raw string, items []ReviewItem) []directScoreResult {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	out := make([]directScoreResult, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) < 2 {
			continue
		}
		idx, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || idx < 0 || idx >= len(items) {
			continue
		}
		if len(parts) < 3 {
			continue
		}
		currentScore, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil {
			continue
		}
		freshScore, err := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
		if err != nil {
			continue
		}
		row := directScoreResult{
			ID:           items[idx].ID,
			CurrentScore: normalizeJudgeScore(currentScore),
			FreshScore:   normalizeJudgeScore(freshScore),
		}
		out = append(out, row)
	}
	return out
}

func normalizeJudgeScore(score float64) float64 {
	if score >= 0 && score <= 1 {
		score = score * 100.0
	}
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func hydrateDirectScoresByOrder(rows []directScoreResult, items []ReviewItem) []directScoreResult {
	out := make([]directScoreResult, 0, len(rows))
	for idx, row := range rows {
		if strings.TrimSpace(row.ID) == "" {
			if idx >= len(items) {
				break
			}
			row.ID = items[idx].ID
		}
		row.CurrentScore = normalizeJudgeScore(row.CurrentScore)
		row.FreshScore = normalizeJudgeScore(row.FreshScore)
		out = append(out, row)
	}
	return out
}
