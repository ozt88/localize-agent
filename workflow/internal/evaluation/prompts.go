package evaluation

import (
	"encoding/json"
	"fmt"
	"strings"

	"localize-agent/workflow/pkg/shared"
)

func buildEvalPrompt(unit map[string]any, shape string) string {
	b, _ := json.Marshal(unit)
	return fmt.Sprintf("Return ONE JSON line only: %s. Evaluate this translation unit: %s", shape, string(b))
}

func buildRevisePrompt(id, en, currentKO, prevKO string, issues []string, shape string) string {
	b, _ := json.Marshal(map[string]any{"id": id, "en": en, "current_ko": currentKO, "previous_ko": prevKO, "eval_issues": issues})
	return fmt.Sprintf("Return ONE JSON line only: %s. Revision: fix all eval_issues while preserving EN meaning and [Tn] placeholders. Input: %s", shape, string(b))
}

func extractEvalResults(raw string) []evalResult {
	var out []evalResult
	for _, ln := range strings.Split(raw, "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "{") && strings.HasSuffix(t, "}") {
			var e evalResult
			if json.Unmarshal([]byte(t), &e) == nil {
				out = append(out, e)
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, chunk := range shared.ExtractJSONObjectChunks(raw) {
		var e evalResult
		if json.Unmarshal([]byte(chunk), &e) == nil {
			out = append(out, e)
		}
	}
	return out
}

func extractRevised(raw string) []revisedProposal {
	var out []revisedProposal
	for _, ln := range strings.Split(raw, "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "{") && strings.HasSuffix(t, "}") {
			var r revisedProposal
			if json.Unmarshal([]byte(t), &r) == nil {
				out = append(out, r)
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, chunk := range shared.ExtractJSONObjectChunks(raw) {
		var r revisedProposal
		if json.Unmarshal([]byte(chunk), &r) == nil {
			out = append(out, r)
		}
	}
	return out
}
