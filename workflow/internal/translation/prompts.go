package translation

import (
	"encoding/json"
	"fmt"
	"strings"

	"localize-agent/workflow/internal/shared"
)

func buildBatchPrompt(items []map[string]string, shapeHint string) string {
	b, _ := json.Marshal(items)
	return fmt.Sprintf(
		"Return a JSON array only. Each array item must match this shape: %s. Input items: %s",
		shapeHint, string(b),
	)
}

func buildSinglePrompt(id, en, cur, shapeHint string) string {
	b, _ := json.Marshal(map[string]string{"id": id, "en": en, "current_ko": cur})
	return fmt.Sprintf("Return a JSON array with exactly one object. Object shape: %s. Input: %s", shapeHint, string(b))
}

func buildRecoveryPrompt(id, en, cur, failed string, placeholders []string, shapeHint string) string {
	p := map[string]any{
		"id":                    id,
		"en":                    en,
		"current_ko":            cur,
		"failed_proposed_ko":    failed,
		"expected_placeholders": placeholders,
	}
	b, _ := json.Marshal(p)
	return fmt.Sprintf(
		"Return a JSON array with exactly one object. Object shape: %s. This is a placeholder recovery task. Preserve expected_placeholders exactly once and in order. Input: %s",
		shapeHint, string(b),
	)
}

func extractObjects(raw string) []proposal {
	out := []proposal{}
	var arr []proposal
	if err := json.Unmarshal([]byte(raw), &arr); err == nil && len(arr) > 0 {
		return arr
	}
	var wrapped struct {
		Items []proposal `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &wrapped); err == nil && len(wrapped.Items) > 0 {
		return wrapped.Items
	}
	for _, ln := range strings.Split(raw, "\n") {
		t := strings.TrimSpace(ln)
		if t == "" || !strings.HasPrefix(t, "{") || !strings.HasSuffix(t, "}") {
			continue
		}
		var p proposal
		if err := json.Unmarshal([]byte(t), &p); err == nil {
			out = append(out, p)
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, chunk := range shared.ExtractJSONObjectChunks(raw) {
		var p proposal
		if err := json.Unmarshal([]byte(chunk), &p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func proposalArraySchema() map[string]any {
	return map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type": "string",
				},
				"proposed_ko": map[string]any{
					"type": "string",
				},
				"risk": map[string]any{
					"type": "string",
					"enum": []string{"low", "med", "high"},
				},
				"notes": map[string]any{
					"type": "string",
				},
			},
			"required":             []string{"id", "proposed_ko", "risk", "notes"},
			"additionalProperties": false,
		},
	}
}
