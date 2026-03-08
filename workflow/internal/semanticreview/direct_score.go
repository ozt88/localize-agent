package semanticreview

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"localize-agent/workflow/internal/platform"
	"localize-agent/workflow/internal/shared"
)

type directScoreResult struct {
	ID             string   `json:"id"`
	WeirdnessScore float64  `json:"weirdness_score"`
	ReasonTags     []string `json:"reason_tags"`
	ShortReason    string   `json:"short_reason"`
}

type DirectScorer struct {
	client        llmClient
	profile       platform.LLMProfile
	sessionPrefix string
	scoreOnly     bool
}

func NewDirectScorer(cfg Config, traceSink platform.LLMTraceSink) (*DirectScorer, error) {
	client, profile, sessionPrefix, err := newReviewClientAndProfile(cfg, traceSink, directScoreWarmup(cfg), nil)
	if err != nil {
		return nil, err
	}
	return &DirectScorer{
		client:        client,
		profile:       profile,
		sessionPrefix: sessionPrefix,
		scoreOnly:     cfg.ScoreOnly,
	}, nil
}

func (d *DirectScorer) ScoreBatch(slotKey string, items []ReviewItem) (map[string]directScoreResult, error) {
	sessionKey := d.sessionPrefix + "#" + slotKey
	raw, err := d.client.SendPrompt(sessionKey, d.profile, buildBatchDirectScorePrompt(items, !d.scoreOnly))
	if err != nil {
		return nil, err
	}
	objects := extractDirectScoreObjects(strings.TrimSpace(raw), items, d.scoreOnly)
	if len(objects) == 0 {
		return nil, fmt.Errorf("no direct score objects in response")
	}
	out := map[string]directScoreResult{}
	for _, obj := range objects {
		out[obj.ID] = obj
	}
	return out, nil
}

func buildBatchDirectScorePrompt(items []ReviewItem, detailed bool) string {
	type payloadItem struct {
		ID           string `json:"id"`
		SourceEN     string `json:"source_en"`
		TranslatedKO string `json:"translated_ko"`
	}
	payload := make([]payloadItem, 0, len(items))
	for _, item := range items {
		payload = append(payload, payloadItem{
			ID:           item.ID,
			SourceEN:     item.SourceEN,
			TranslatedKO: item.TranslatedKO,
		})
	}
	b, _ := json.Marshal(payload)
	prompt := ""
	if detailed {
		prompt += "Return a JSON array only.\n"
		prompt += "Each object must be {\"id\":\"...\",\"weirdness_score\":0.0,\"reason_tags\":[\"...\"],\"short_reason\":\"...\"}.\n"
	} else {
		prompt += "Return plain text only.\n" +
			"Return exactly one line per item in the same order as input.\n" +
			"Format each line as: <index>\\t<score>\n" +
			"Score must be a decimal between 0.00 and 1.00 inclusive.\n" +
			"Do not use percentages, whole-number scales, or values greater than 1.\n" +
			"If useful, you may append a third tab-separated field with comma-separated short tags.\n"
	}
	return prompt + "Input items: " + string(b)
}

func extractDirectScoreObjects(raw string, items []ReviewItem, scoreOnly bool) []directScoreResult {
	if scoreOnly {
		if rows := extractPlainDirectScoreObjects(raw, items); len(rows) > 0 {
			return rows
		}
	}
	var arr []directScoreResult
	if err := json.Unmarshal([]byte(raw), &arr); err == nil && len(arr) > 0 {
		return arr
	}
	var wrapped struct {
		Items []directScoreResult `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &wrapped); err == nil && len(wrapped.Items) > 0 {
		return wrapped.Items
	}
	out := []directScoreResult{}
	for _, chunk := range shared.ExtractJSONObjectChunks(raw) {
		var row directScoreResult
		if err := json.Unmarshal([]byte(chunk), &row); err == nil && row.ID != "" {
			out = append(out, row)
		}
	}
	return out
}

func extractPlainDirectScoreObjects(raw string, items []ReviewItem) []directScoreResult {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	out := make([]directScoreResult, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			continue
		}
		idx, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || idx < 0 || idx >= len(items) {
			continue
		}
		score, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil {
			continue
		}
		score = normalizeReviewScore(score)
		row := directScoreResult{
			ID:             items[idx].ID,
			WeirdnessScore: score,
		}
		if len(parts) == 3 {
			tagText := strings.TrimSpace(parts[2])
			if tagText != "" {
				for _, tag := range strings.Split(tagText, ",") {
					tag = strings.TrimSpace(tag)
					if tag != "" {
						row.ReasonTags = append(row.ReasonTags, tag)
					}
				}
			}
		}
		out = append(out, row)
	}
	return out
}

func normalizeReviewScore(score float64) float64 {
	if score > 1 && score <= 100 {
		score = score / 100.0
	}
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}
