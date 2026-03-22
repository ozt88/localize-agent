package tagformat

import (
	"fmt"
	"strings"
)

// ValidateTagMatch compares tag frequency maps between EN source and KO formatted text.
// Order is ignored per D-07 (Korean word order differs from English).
// Returns nil if all tags present with correct counts.
func ValidateTagMatch(enSource, koFormatted string) error {
	enTags := ExtractTags(enSource)
	koTags := ExtractTags(koFormatted)

	// Both have zero tags -- pass.
	if len(enTags) == 0 && len(koTags) == 0 {
		return nil
	}

	// Build frequency maps.
	enFreq := buildFreqMap(enTags)
	koFreq := buildFreqMap(koTags)

	// Check EN tags present in KO with correct counts.
	var missing []string
	for tag, enCount := range enFreq {
		koCount := koFreq[tag]
		if koCount < enCount {
			missing = append(missing, fmt.Sprintf("%s (EN:%d, KO:%d)", tag, enCount, koCount))
		}
	}

	// Check for extra tags in KO not in EN.
	var extra []string
	for tag, koCount := range koFreq {
		enCount := enFreq[tag]
		if koCount > enCount {
			extra = append(extra, fmt.Sprintf("%s (EN:%d, KO:%d)", tag, enCount, koCount))
		}
	}

	if len(missing) > 0 || len(extra) > 0 {
		var parts []string
		if len(enTags) != len(koTags) {
			parts = append(parts, fmt.Sprintf("tag count mismatch: EN has %d, KO has %d", len(enTags), len(koTags)))
		}
		if len(missing) > 0 {
			parts = append(parts, "missing: "+strings.Join(missing, ", "))
		}
		if len(extra) > 0 {
			parts = append(parts, "extra: "+strings.Join(extra, ", "))
		}
		return &TagValidationError{
			Message: "tag mismatch: " + strings.Join(parts, "; "),
		}
	}

	return nil
}

// buildFreqMap creates a frequency map from a tag slice.
func buildFreqMap(tags []string) map[string]int {
	m := make(map[string]int, len(tags))
	for _, t := range tags {
		m[t]++
	}
	return m
}
