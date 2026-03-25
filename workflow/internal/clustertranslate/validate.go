package clustertranslate

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// htmlTagRe matches HTML-style tags like <b>, </i>, <color=#FFF>.
var htmlTagRe = regexp.MustCompile(`</?[a-zA-Z][^>]*>`)

// gameTokenRe matches game control tokens that are inherently ASCII:
// ROLL25, SPELL, ADV, .VariableName, .VAR>=10-, $var_ref
var gameTokenRe = regexp.MustCompile(`(?:^|\s)(?:ROLL\d*|SPELL|ADV|\.[\w'\-]+(?:[<>]=?\d+|==[^\s]+)?-?|\$[\w]+)(?:\s|$)`)

// ValidationResult holds per-line degenerate check results.
type ValidationResult struct {
	LineResults   []LineResult // per-line results in order
	DegenerateN   int          // number of degenerate lines
	TotalN        int          // total lines checked
	DegenerateIDs []int        // 1-based line numbers that are degenerate
}

// LineResult holds a single line's validation outcome.
type LineResult struct {
	LineNumber int
	Reason     string // empty = valid, non-empty = degenerate reason
}

// ValidateTranslation validates LLM output against prompt metadata and source texts.
// Per TRANS-04: line count must match (hard reject).
// Per D-13: degenerate lines are checked per-line; batch is rejected only if >50% are degenerate.
func ValidateTranslation(rawOutput string, meta PromptMeta, sourceTexts []string) error {
	parsed, err := ParseNumberedOutput(rawOutput)
	if err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	// Check line count (TRANS-04) — hard reject
	if err := ValidateLineCount(meta.LineCount, len(parsed)); err != nil {
		return err
	}

	// Check each line for degenerate output (D-13) — ratio-based
	result := CheckDegenerate(parsed, sourceTexts)

	if result.TotalN == 0 {
		return nil
	}

	ratio := float64(result.DegenerateN) / float64(result.TotalN)

	// Hard reject: all lines are degenerate
	if result.DegenerateN == result.TotalN {
		first := result.LineResults[0]
		return fmt.Errorf("degenerate: %s (all %d lines)", first.Reason, result.TotalN)
	}

	// Reject if >50% of lines are degenerate
	if ratio > 0.5 {
		return fmt.Errorf("degenerate: %d/%d lines (%.0f%%) failed quality check", result.DegenerateN, result.TotalN, ratio*100)
	}

	// ≤50% degenerate — accept the batch; individual degenerate lines
	// will have lower quality but the batch as a whole is usable.
	return nil
}

// CheckDegenerate runs per-line degenerate checks and returns results without rejecting.
func CheckDegenerate(parsed []TranslatedLine, sourceTexts []string) ValidationResult {
	var result ValidationResult
	result.TotalN = len(parsed)

	for i, line := range parsed {
		lr := LineResult{LineNumber: line.Number}
		if i < len(sourceTexts) {
			lr.Reason = degenerateReason(sourceTexts[i], line.Text)
		}
		result.LineResults = append(result.LineResults, lr)
		if lr.Reason != "" {
			result.DegenerateN++
			result.DegenerateIDs = append(result.DegenerateIDs, line.Number)
		}
	}
	return result
}

// ValidateLineCount checks that expected and actual line counts match.
func ValidateLineCount(expected, actual int) error {
	if expected != actual {
		return fmt.Errorf("line count mismatch: expected %d, got %d", expected, actual)
	}
	return nil
}

// degenerateReason checks if a translation is degenerate relative to its source.
// Returns reason string or empty string if valid.
func degenerateReason(en, ko string) string {
	koTrim := strings.TrimSpace(ko)
	enTrim := strings.TrimSpace(en)

	if koTrim == "" {
		return "empty"
	}

	// Exact source copy check (normalized)
	if normalizedEqual(enTrim, koTrim) {
		return "exact_source_copy"
	}

	// ASCII-heavy check: if >80% of non-space, non-tag, non-game-token characters
	// are ASCII, the LLM likely did not actually translate.
	// Skip for short texts (≤50 chars) — proper nouns, quotes, and game terms
	// make short translations inherently ASCII-heavy.
	if len(koTrim) > 50 {
		cleaned := stripGameTokensAndTags(koTrim)
		asciiCount, totalCount := 0, 0
		for _, r := range cleaned {
			if unicode.IsSpace(r) {
				continue
			}
			totalCount++
			if r < 128 {
				asciiCount++
			}
		}
		if totalCount > 0 && float64(asciiCount)/float64(totalCount) > 0.8 {
			return "ascii_heavy"
		}
	}

	return ""
}

// stripGameTokensAndTags removes HTML tags and game control tokens from text
// before ASCII ratio calculation. These are inherently ASCII but not indicators
// of failed translation.
func stripGameTokensAndTags(s string) string {
	// Remove HTML tags: <b>, </i>, <color=#FFF>, etc.
	s = htmlTagRe.ReplaceAllString(s, "")
	// Remove game tokens: ROLL25, SPELL, .Variable, $var, ADV
	s = gameTokenRe.ReplaceAllString(s, " ")
	return s
}

// normalizedEqual compares two strings after removing punctuation, spaces,
// and lowercasing.
func normalizedEqual(a, b string) bool {
	return normalizeForCompare(a) == normalizeForCompare(b)
}

// normalizeForCompare strips whitespace, punctuation, and lowercases.
func normalizeForCompare(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
