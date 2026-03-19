package fragmentcluster

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

type Line struct {
	ID        string `json:"id"`
	EN        string `json:"en"`
	CurrentKO string `json:"current_ko,omitempty"`
	TextRole  string `json:"text_role,omitempty"`
}

var (
	clusterPlaceholderRe = regexp.MustCompile(`\[T\d+\]`)
	clusterEmphasisRe    = regexp.MustCompile(`\[\[/?E\d+\]\]`)
	clusterTagTokenRe    = regexp.MustCompile(`(<[^>]+>|\{[^}]+\}|\$[A-Za-z0-9_]+)`)
	clusterOpenEmphasisRe   = regexp.MustCompile(`\[\[E\d+\]\]`)
	clusterCloseEmphasisRe  = regexp.MustCompile(`\[\[/E\d+\]\]`)
	clusterSupportedEmphasisTagRe = regexp.MustCompile(`</?(i|b)>`)
)

type PromptInput struct {
	ClusterID       string   `json:"cluster_id,omitempty"`
	SourceFile      string   `json:"source_file,omitempty"`
	SegmentID       string   `json:"segment_id,omitempty"`
	ContextBeforeEN string   `json:"context_before_en,omitempty"`
	ContextAfterEN  string   `json:"context_after_en,omitempty"`
	ClusterJoinHint string   `json:"cluster_join_hint,omitempty"`
	ClusterPatterns []string `json:"cluster_patterns,omitempty"`
	Lines           []Line   `json:"lines"`
}

func BuildPrompt(in PromptInput) string {
	b, _ := json.Marshal(in)
	parts := []string{
		"You are improving Korean localization for a context cluster.",
		"Read ALL lines together as one meaning unit before translating any individual line.",
		"Return one Korean line per input line.",
		"Keep the same number of output lines as input lines.",
		"Preserve input order exactly.",
		"Do not merge lines.",
		"Do not drop lines.",
		"Do not invent content that is not implied by the cluster and nearby context.",
		"If the English is fragmentary, Korean may remain fragmentary, but adjacent lines should read naturally together.",
		"Keep each line's Korean speech level and register aligned with current_ko when current_ko already sounds natural.",
		"Do not add honorifics or extra politeness unless the source or current_ko clearly requires it.",
		"If a line already carries emphasis in current_ko, preserve that emphasis unless there is a strong reason not to.",
	}
	// Pattern-specific hints
	for _, hint := range patternHints(in.ClusterPatterns) {
		parts = append(parts, hint)
	}
	parts = append(parts,
		"Return only one JSON array of Korean strings.",
		"Input cluster JSON: "+string(b),
	)
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// patternHints returns extra prompt instructions based on cluster patterns.
func patternHints(patterns []string) []string {
	if len(patterns) == 0 {
		return nil
	}
	set := map[string]bool{}
	for _, p := range patterns {
		set[p] = true
	}
	var hints []string
	if set["pronoun_ref"] {
		hints = append(hints, "Some lines contain pronouns (he, she, it, they, etc.) whose antecedent is in a neighboring line. Resolve pronoun references using surrounding context to choose the correct Korean subject/object.")
	}
	if set["speaker_change"] {
		hints = append(hints, "This cluster has dialogue from multiple speakers. Match tone, register, and speech level to each speaker's character. Short responses should reflect the responding speaker's personality.")
	}
	if set["incomplete"] {
		hints = append(hints, "Some lines are incomplete (trailing ..., -, or unfinished quotes). Understand the full intent from surrounding lines before translating. Keep the Korean equally incomplete where the English is intentionally cut off.")
	}
	if set["action_reaction"] {
		hints = append(hints, "This cluster pairs a narration/action line with a short emotional reaction. Use the action context to inform the correct emotional tone and nuance of the reaction line.")
	}
	if set["fragment_chain"] {
		hints = append(hints, "These lines form a fragment chain where a single thought or action spans multiple short lines. Translate them as parts of one coherent meaning unit.")
	}
	return hints
}

func ParseOutput(raw string, expected int) ([]string, error) {
	raw = strings.TrimSpace(raw)
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		if len(arr) != expected {
			return nil, fmt.Errorf("output line count %d != expected %d", len(arr), expected)
		}
		return arr, nil
	}
	type wrapped struct {
		Items []string `json:"items"`
	}
	var w wrapped
	if err := json.Unmarshal([]byte(raw), &w); err == nil {
		if len(w.Items) != expected {
			return nil, fmt.Errorf("output line count %d != expected %d", len(w.Items), expected)
		}
		return w.Items, nil
	}
	return nil, fmt.Errorf("invalid cluster output")
}

func NormalizeOutputLines(lines []string, sourceLines []Line) ([]string, error) {
	if len(lines) != len(sourceLines) {
		return nil, fmt.Errorf("output line count %d != source line count %d", len(lines), len(sourceLines))
	}
	out := make([]string, 0, len(lines))
	for idx, line := range lines {
		normalized, err := normalizeOneLine(line, sourceLines[idx])
		if err != nil {
			return nil, fmt.Errorf("line %d (%s): %w", idx, sourceLines[idx].ID, err)
		}
		out = append(out, normalized)
	}
	return out, nil
}

func normalizeOneLine(line string, source Line) (string, error) {
	line = strings.TrimSpace(line)
	current := strings.TrimSpace(source.CurrentKO)
	if current != "" {
		line = restoreSimpleTagPlaceholders(line, current)
		line = restoreSimpleEmphasisMarkers(line, current)
		line = carryForwardSingleEmphasis(line, current)
	}
	line = carryForwardSourceEmphasis(line, source.EN)
	if clusterPlaceholderRe.MatchString(line) || clusterEmphasisRe.MatchString(line) {
		return "", fmt.Errorf("placeholder residue remained after normalization")
	}
	return line, nil
}

func restoreSimpleTagPlaceholders(line string, current string) string {
	placeholders := clusterPlaceholderRe.FindAllString(line, -1)
	if len(placeholders) == 0 {
		return line
	}
	tokens := clusterTagTokenRe.FindAllString(current, -1)
	if len(tokens) < len(placeholders) {
		return line
	}
	out := line
	for idx, placeholder := range placeholders {
		out = strings.Replace(out, placeholder, tokens[idx], 1)
	}
	return out
}

func restoreSimpleEmphasisMarkers(line string, current string) string {
	if !clusterEmphasisRe.MatchString(line) {
		return line
	}
	tags := clusterSupportedEmphasisTagRe.FindAllString(current, -1)
	if len(tags) < 2 {
		return line
	}
	openTag := tags[0]
	closeTag := tags[1]
	out := clusterOpenEmphasisRe.ReplaceAllString(line, openTag)
	out = clusterCloseEmphasisRe.ReplaceAllString(out, closeTag)
	return out
}

func carryForwardSingleEmphasis(line string, current string) string {
	if clusterSupportedEmphasisTagRe.MatchString(line) {
		return line
	}
	prefix, openTag, closeTag, suffix, ok := parseSingleEmphasisStructure(current)
	if !ok {
		return line
	}
	if !isWhitespaceOrPunctuation(prefix) || !isWhitespaceOrPunctuation(suffix) {
		return line
	}
	body := strings.TrimSpace(line)
	if body == "" {
		return line
	}
	if suffix != "" {
		trimmedBody, trailing := splitTrailingPunctuation(body)
		if strings.TrimSpace(trimmedBody) != "" {
			body = trimmedBody
			if trailing != "" {
				suffix = trailing
			}
		}
	}
	return strings.TrimSpace(prefix + openTag + body + closeTag + suffix)
}

func carryForwardSourceEmphasis(line string, sourceEN string) string {
	if clusterSupportedEmphasisTagRe.MatchString(line) {
		return line
	}
	prefix, suffix, ok := parseSingleSourceEmphasisStructure(sourceEN)
	if !ok {
		return line
	}
	if !isWhitespaceOrPunctuation(prefix) || !isWhitespaceOrPunctuation(suffix) {
		return line
	}
	body := strings.TrimSpace(line)
	if body == "" {
		return line
	}
	if suffix != "" {
		trimmedBody, trailing := splitTrailingPunctuation(body)
		if strings.TrimSpace(trimmedBody) != "" {
			body = trimmedBody
			if trailing != "" {
				suffix = trailing
			}
		}
	}
	return strings.TrimSpace(prefix + "<i>" + body + "</i>" + suffix)
}

func parseSingleEmphasisStructure(current string) (string, string, string, string, bool) {
	for _, tag := range []string{"i", "b"} {
		openTag := "<" + tag + ">"
		closeTag := "</" + tag + ">"
		openIdx := strings.Index(current, openTag)
		closeIdx := strings.Index(current, closeTag)
		if openIdx == -1 || closeIdx == -1 || closeIdx <= openIdx {
			continue
		}
		prefix := current[:openIdx]
		suffix := current[closeIdx+len(closeTag):]
		return prefix, openTag, closeTag, suffix, true
	}
	return "", "", "", "", false
}

func parseSingleSourceEmphasisStructure(sourceEN string) (string, string, bool) {
	if clusterEmphasisRe.MatchString(sourceEN) {
		open := clusterOpenEmphasisRe.FindStringIndex(sourceEN)
		close := clusterCloseEmphasisRe.FindStringIndex(sourceEN)
		if open == nil || close == nil || close[0] <= open[1] {
			return "", "", false
		}
		prefix := sourceEN[:open[0]]
		suffix := sourceEN[close[1]:]
		return prefix, suffix, true
	}
	prefix, _, _, suffix, ok := parseSingleEmphasisStructure(sourceEN)
	if !ok {
		return "", "", false
	}
	return prefix, suffix, true
}

func splitTrailingPunctuation(s string) (string, string) {
	runes := []rune(s)
	i := len(runes)
	for i > 0 {
		r := runes[i-1]
		if unicode.IsPunct(r) || unicode.IsSpace(r) {
			i--
			continue
		}
		break
	}
	return strings.TrimSpace(string(runes[:i])), string(runes[i:])
}

func isWhitespaceOrPunctuation(s string) bool {
	if strings.TrimSpace(s) == "" {
		return true
	}
	for _, r := range s {
		if !(unicode.IsPunct(r) || unicode.IsSpace(r)) {
			return false
		}
	}
	return true
}
