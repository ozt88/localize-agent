package clustertranslate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// numberedLineRe matches lines starting with [NN] marker.
var numberedLineRe = regexp.MustCompile(`^\[(\d+)\]\s*(.*)$`)

// speakerRe matches "SpeakerName: text" patterns in LLM output.
// Requires uppercase first letter to prevent false positives on Korean text
// containing lowercase "english: ..." patterns (e.g., "Level: 조심해").
// Character names in this game are always capitalized (Braxo, She'lia, Captain Morgan).
var speakerRe = regexp.MustCompile(`^([A-Z][A-Za-z0-9_' ]*?):\s*(.*)$`)

// ParseNumberedOutput parses LLM output into TranslatedLine slice.
// Expects lines in format: [NN] optional-speaker: "translated text"
func ParseNumberedOutput(raw string) ([]TranslatedLine, error) {
	rawLines := strings.Split(raw, "\n")
	var result []TranslatedLine

	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		match := numberedLineRe.FindStringSubmatch(line)
		if match == nil {
			continue // skip non-numbered lines
		}

		num, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}

		remainder := strings.TrimSpace(match[2])
		tl := TranslatedLine{Number: num}

		// Check for [CHOICE] marker
		if strings.HasPrefix(remainder, "[CHOICE]") {
			tl.IsChoice = true
			remainder = strings.TrimSpace(strings.TrimPrefix(remainder, "[CHOICE]"))
		}

		// Strip surrounding quotes
		remainder = stripQuotes(remainder)

		// Check for speaker pattern
		if speakerMatch := speakerRe.FindStringSubmatch(remainder); speakerMatch != nil {
			tl.Speaker = strings.TrimSpace(speakerMatch[1])
			tl.Text = stripQuotes(strings.TrimSpace(speakerMatch[2]))
		} else {
			tl.Text = remainder
		}

		result = append(result, tl)
	}

	return result, nil
}

// MapLinesToIDs maps parsed lines to block IDs using PromptMeta.BlockIDOrder.
// Per TRANS-03: [NN] markers map back to source block IDs.
func MapLinesToIDs(lines []TranslatedLine, meta PromptMeta) (map[string]string, error) {
	if len(lines) != len(meta.BlockIDOrder) {
		return nil, fmt.Errorf("line count mismatch: got %d lines, expected %d block IDs", len(lines), len(meta.BlockIDOrder))
	}

	mapping := make(map[string]string, len(lines))
	for i, line := range lines {
		mapping[meta.BlockIDOrder[i]] = line.Text
	}
	return mapping, nil
}

// stripQuotes removes surrounding double quotes from a string.
func stripQuotes(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}
