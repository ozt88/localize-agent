package translation

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"localize-agent/workflow/internal/shared"
)

func buildBatchPrompt(tasks []translationTask, shapeHint string, plain bool) string {
	payload := make([]normalizedPromptInput, 0, len(tasks))
	for _, task := range tasks {
		payload = append(payload, normalizePromptInput(task))
	}
	b, _ := json.Marshal(payload)
	_ = shapeHint
	if plain {
		return fmt.Sprintf(
			"Translate each input item into natural Korean. Return exactly one line per item in the same order. Each line must use the format <index>\\t<korean translation>. Use indexes 0 through %d. No JSON. No markdown. No commentary. Input items: %s",
			len(tasks)-1,
			string(b),
		)
	}
	return fmt.Sprintf("Translate each input item into natural Korean. Input items: %s", string(b))
}

func buildSinglePrompt(task translationTask, shapeHint string, plain bool) string {
	payload := normalizePromptInput(task)
	b, _ := json.Marshal(payload)
	_ = shapeHint
	if plain {
		return fmt.Sprintf("Translate the input item into natural Korean. Return only the Korean translation text. No JSON. No markdown. No commentary. Input: %s", string(b))
	}
	return fmt.Sprintf("Translate the input item into natural Korean. Input: %s", string(b))
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

func extractPlainTranslation(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	if unquoted, err := strconv.Unquote(s); err == nil {
		s = strings.TrimSpace(unquoted)
	}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if t == "" {
			continue
		}
		out = append(out, t)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func extractIndexedTranslations(raw string) map[int]string {
	out := map[int]string{}
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	for _, ln := range strings.Split(s, "\n") {
		line := strings.TrimSpace(ln)
		if line == "" {
			continue
		}
		idx, text, ok := parseIndexedLine(line)
		if ok {
			out[idx] = text
		}
	}
	return out
}

func parseIndexedLine(line string) (int, string, bool) {
	candidates := []string{"\t", "|", ":", " "}
	for _, sep := range candidates {
		parts := strings.SplitN(line, sep, 2)
		if len(parts) != 2 {
			continue
		}
		left := strings.TrimSpace(strings.Trim(parts[0], "[]"))
		right := strings.TrimSpace(parts[1])
		if left == "" || right == "" {
			continue
		}
		idx, err := strconv.Atoi(left)
		if err != nil {
			continue
		}
		return idx, right, true
	}
	return 0, "", false
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
			},
			"required":             []string{"id", "proposed_ko"},
			"additionalProperties": false,
		},
	}
}
