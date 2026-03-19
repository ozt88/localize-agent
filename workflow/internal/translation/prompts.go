package translation

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"localize-agent/workflow/pkg/shared"
)

func batchHasLoreContext(tasks []translationTask) bool {
	for _, t := range tasks {
		if len(t.LoreHints) > 0 {
			return true
		}
	}
	return false
}

func batchNeedsFragmentRules(tasks []translationTask) bool {
	for _, t := range tasks {
		fp, _, _ := deriveFragmentHints(t)
		sp, _, _ := deriveStructureHints(t)
		if fp != "" || sp != "" {
			return true
		}
	}
	return false
}

func buildBatchPrompt(tasks []translationTask, shapeHint string, plain bool) string {
	payload := normalizeBatchPrompt(tasks)
	b, _ := json.Marshal(payload)
	_ = shapeHint
	base := "Translate each items[*].en into Korean without repairing or completing broken source fragments. If the English is truncated, open-ended, or has an unbalanced quote, translate only the visible fragment naturally. Preserve gameplay prefixes, action markers, narration cues, and mixed action-plus-dialogue structure as meaningful source content. If a source span is clearly a non-English phrase, chant, or foreign-language fragment, preserve that span unchanged and translate only the surrounding English. If glossary is present for an item, treat each glossary mapping as mandatory terminology for that item. Use the provided target exactly for translate mappings and preserve the source exactly for preserve mappings."
	fragmentRules := " If fragment_pattern is present, use action_cue_en and spoken_fragment_en as reference-only structure hints for the same source line; keep the spoken part fragmentary when the source is fragmentary, and do not invent missing continuation or a closing quote. If structure_pattern is present, use lead_term_en and definition_body_en as reference-only structure hints; translate the line as Korean explanatory text rather than copying the English term-definition wording literally. If structure_pattern is expository_entry, render the whole line as fluent Korean explanatory prose and do not preserve the English wording verbatim. If structure_pattern is long_discourse, render the whole line as fluent Korean long-form dialogue or narration, preserving speaker tone while avoiding clause-by-clause literalism."
	loreRules := " If lore_context is present for an item, use it as background knowledge to inform tone, register, and contextual accuracy of the translation. Do not include lore_context text in the output."
	tail := " If [PLAYER OPTION] appears inside context, it is a reference-only anchor and must not be copied into output. Return ONLY one JSON array of exactly the requested number of Korean strings."
	rules := base
	if batchNeedsFragmentRules(tasks) {
		rules += fragmentRules
	}
	if batchHasLoreContext(tasks) {
		rules += loreRules
	}
	rules += tail
	if plain {
		return fmt.Sprintf(
			"%s Each item uses items[*].en as the source text and items[*].ctx/items[*].line as a reference anchor in contexts. Do not add prose before or after the JSON array. Input JSON: %s",
			rules,
			string(b),
		)
	}
	return fmt.Sprintf("%s Each item uses items[*].en as the source text and items[*].ctx/items[*].line as a reference anchor in contexts. Do not add prose before or after the JSON array. Input JSON: %s", rules, string(b))
}

func buildSinglePrompt(task translationTask, shapeHint string, plain bool) string {
	payload := normalizePromptInput(task)
	b, _ := json.Marshal(payload)
	_ = shapeHint
	base := "Translate the single `en` field into Korean without repairing or completing broken source fragments. If the English is truncated, open-ended, or has an unbalanced quote, translate only the visible fragment naturally. Preserve gameplay prefixes, action markers, narration cues, and mixed action-plus-dialogue structure as meaningful source content. If a source span is clearly a non-English phrase, chant, or foreign-language fragment, preserve that span unchanged and translate only the surrounding English. If glossary is present, treat each glossary mapping as mandatory terminology for this item. Use the provided target exactly for translate mappings and preserve the source exactly for preserve mappings."
	fragmentRules := " If fragment_pattern is present, use action_cue_en and spoken_fragment_en as reference-only structure hints for the same source line; keep the spoken part fragmentary when the source is fragmentary, and do not invent missing continuation or a closing quote. If structure_pattern is present, use lead_term_en and definition_body_en as reference-only structure hints; translate the line as Korean explanatory text rather than copying the English term-definition wording literally. If structure_pattern is expository_entry, render the whole line as fluent Korean explanatory prose and do not preserve the English wording verbatim. If structure_pattern is long_discourse, render the whole line as fluent Korean long-form dialogue or narration, preserving speaker tone while avoiding clause-by-clause literalism."
	loreRules := " If lore_context is present, use it as background knowledge to inform tone, register, and contextual accuracy of the translation. Do not include lore_context text in the output."
	tail := " If [PLAYER OPTION] appears inside context, it is a reference-only anchor and must not be copied into output."
	rules := base
	fp, _, _ := deriveFragmentHints(task)
	sp, _, _ := deriveStructureHints(task)
	if fp != "" || sp != "" {
		rules += fragmentRules
	}
	if len(task.LoreHints) > 0 {
		rules += loreRules
	}
	rules += tail
	if plain {
		return fmt.Sprintf("%s Return one valid JSON array with exactly 1 Korean string and nothing else. Do not add prose before or after the JSON array. Input JSON: %s", rules, string(b))
	}
	return fmt.Sprintf("%s Return one valid JSON array with exactly 1 Korean string and nothing else. Do not add prose before or after the JSON array. Input JSON: %s", rules, string(b))
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

func extractStringArray(raw string) []string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	candidates := stringArrayCandidates(s)
	for _, candidate := range candidates {
		var arr []string
		if err := json.Unmarshal([]byte(candidate), &arr); err == nil && len(arr) > 0 {
			return arr
		}
		var wrapped struct {
			Items []string `json:"items"`
		}
		if err := json.Unmarshal([]byte(candidate), &wrapped); err == nil && len(wrapped.Items) > 0 {
			return wrapped.Items
		}
		var single string
		if err := json.Unmarshal([]byte(candidate), &single); err == nil && strings.TrimSpace(single) != "" {
			return []string{single}
		}
	}
	for _, candidate := range candidates {
		if salvaged := salvageSingletonStringArray(candidate); len(salvaged) == 1 {
			return salvaged
		}
	}
	return nil
}

func stringArrayCandidates(s string) []string {
	candidates := []string{s}
	if unquoted, err := strconv.Unquote(s); err == nil {
		candidates = append(candidates, strings.TrimSpace(unquoted))
	}
	for _, raw := range candidates {
		normalized := strings.ReplaceAll(raw, `\[`, `[`)
		normalized = strings.ReplaceAll(normalized, `\]`, `]`)
		candidates = append(candidates, normalized)
		candidates = append(candidates, strings.ReplaceAll(normalized, `\"`, `"`))
	}
	return uniqueNonEmptyStrings(candidates)
}

func uniqueNonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func salvageSingletonStringArray(s string) []string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil
	}
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(s, "["), "]"))
	if inner == "" {
		return nil
	}
	if strings.HasPrefix(inner, "\"") && strings.HasSuffix(inner, "\"") && len(inner) >= 2 {
		if unquoted, err := strconv.Unquote(inner); err == nil {
			return []string{unquoted}
		}
		return []string{inner[1 : len(inner)-1]}
	}
	firstQuote := strings.Index(inner, "\"")
	lastQuote := strings.LastIndex(inner, "\"")
	if firstQuote >= 0 && lastQuote > firstQuote {
		return []string{inner[firstQuote+1 : lastQuote]}
	}
	return nil
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
		"type":  "array",
		"items": map[string]any{"type": "string"},
	}
}
