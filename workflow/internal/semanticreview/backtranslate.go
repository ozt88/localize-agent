package semanticreview

import (
	"encoding/json"
	"fmt"
	"strings"

	"localize-agent/workflow/internal/platform"
	"localize-agent/workflow/internal/shared"
)

type backtranslationResult struct {
	ID               string `json:"id"`
	BacktranslatedEN string `json:"backtranslated_en"`
}

type llmClient interface {
	EnsureContext(key string, profile platform.LLMProfile) error
	SendPrompt(key string, profile platform.LLMProfile, prompt string) (string, error)
}

type Backtranslator struct {
	client llmClient
	profile platform.LLMProfile
	sessionPrefix string
}

func NewBacktranslator(cfg Config, traceSink platform.LLMTraceSink) (*Backtranslator, error) {
	client, profile, sessionPrefix, err := newReviewClientAndProfile(cfg, traceSink, backtranslationWarmup(cfg), backtranslationArraySchema())
	if err != nil {
		return nil, err
	}

	return &Backtranslator{
		client:        client,
		profile:       profile,
		sessionPrefix: sessionPrefix,
	}, nil
}

func (b *Backtranslator) BacktranslateBatch(slotKey string, items []ReviewItem) (map[string]string, error) {
	prompt := buildBatchBacktranslationPrompt(items)
	sessionKey := b.sessionPrefix + "#" + slotKey
	raw, err := b.client.SendPrompt(sessionKey, b.profile, prompt)
	if err != nil {
		return nil, err
	}
	objects := extractBacktranslationObjects(strings.TrimSpace(raw))
	if len(objects) == 0 {
		return nil, fmt.Errorf("no backtranslation objects in response")
	}
	out := map[string]string{}
	for _, obj := range objects {
		out[obj.ID] = strings.TrimSpace(obj.BacktranslatedEN)
	}
	return out, nil
}

func buildBatchBacktranslationPrompt(items []ReviewItem) string {
	type payloadItem struct {
		ID           string `json:"id"`
		TranslatedKO string `json:"translated_ko"`
	}
	payload := make([]payloadItem, 0, len(items))
	for _, item := range items {
		payload = append(payload, payloadItem{ID: item.ID, TranslatedKO: item.TranslatedKO})
	}
	b, _ := json.Marshal(payload)
	return "Input items: " + string(b)
}

func extractBacktranslationObjects(raw string) []backtranslationResult {
	var arr []backtranslationResult
	if err := json.Unmarshal([]byte(raw), &arr); err == nil && len(arr) > 0 {
		return arr
	}
	var wrapped struct {
		Items []backtranslationResult `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &wrapped); err == nil && len(wrapped.Items) > 0 {
		return wrapped.Items
	}
	out := []backtranslationResult{}
	for _, chunk := range shared.ExtractJSONObjectChunks(raw) {
		var row backtranslationResult
		if err := json.Unmarshal([]byte(chunk), &row); err == nil && row.ID != "" {
			out = append(out, row)
		}
	}
	return out
}

func backtranslationArraySchema() map[string]any {
	return map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string"},
				"backtranslated_en": map[string]any{"type": "string"},
			},
			"required": []string{"id", "backtranslated_en"},
			"additionalProperties": false,
		},
	}
}
