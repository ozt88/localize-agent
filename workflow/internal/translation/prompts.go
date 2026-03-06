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
		"Return JSON Lines only, one line per item: %s. Input items: %s",
		shapeHint, string(b),
	)
}

func buildSinglePrompt(id, en, cur, shapeHint string) string {
	b, _ := json.Marshal(map[string]string{"id": id, "en": en, "current_ko": cur})
	return fmt.Sprintf("Return ONE JSON line only: %s. Input: %s", shapeHint, string(b))
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
		"Return ONE JSON line only: %s. This is a placeholder recovery task. Preserve expected_placeholders exactly once and in order. Input: %s",
		shapeHint, string(b),
	)
}

func extractObjects(raw string) []proposal {
	out := []proposal{}
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
