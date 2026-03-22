package clustertranslate

import (
	"fmt"
	"strings"
	"unicode"
)

// ValidateTranslation validates LLM output against prompt metadata and source texts.
// Per TRANS-04: line count must match. Per D-13: degenerate output is rejected.
func ValidateTranslation(rawOutput string, meta PromptMeta, sourceTexts []string) error {
	parsed, err := ParseNumberedOutput(rawOutput)
	if err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	// Check line count (TRANS-04)
	if err := ValidateLineCount(meta.LineCount, len(parsed)); err != nil {
		return err
	}

	// Check each line for degenerate output (D-13)
	for i, line := range parsed {
		if i >= len(sourceTexts) {
			break
		}
		reason := degenerateReason(sourceTexts[i], line.Text)
		if reason != "" {
			return fmt.Errorf("degenerate: %s (line %d)", reason, line.Number)
		}
	}

	return nil
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

	// ASCII-heavy check: if >80% of non-space characters are ASCII,
	// the LLM likely did not actually translate (v1 ascii_heavy check).
	if len(koTrim) > 5 {
		asciiCount, totalCount := 0, 0
		for _, r := range koTrim {
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
