package clustertranslate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// numberedLineRe matches lines starting with [NN] marker at line start.
var numberedLineRe = regexp.MustCompile(`^\[(\d+)\]\s*(.*)$`)

// blockSplitRe splits raw LLM output into chunks, one per [NN] marker.
// Each chunk starts at a [NN] marker and ends just before the next one.
// This allows multiline text inside a single numbered entry.
var blockSplitRe = regexp.MustCompile(`(?m)^(\[\d+\])`)

// speakerRe matches "SpeakerName: text" patterns in LLM output.
// Requires uppercase first letter to prevent false positives on Korean text
// containing lowercase "english: ..." patterns (e.g., "Level: 조심해").
// Character names in this game are always capitalized (Braxo, She'lia, Captain Morgan).
var speakerRe = regexp.MustCompile(`^([A-Z][A-Za-z0-9_' ]*?):\s*(.*)$`)

// ParseNumberedOutput parses LLM output into TranslatedLine slice.
// Expects entries in format: [NN] optional-speaker: "translated text"
// Supports multiline text within a single numbered entry — the text may span
// multiple real newlines as produced by quoteForPrompt (no \n escaping).
func ParseNumberedOutput(raw string) ([]TranslatedLine, error) {
	// Split on [NN] markers to get one chunk per numbered entry.
	// indices[i] is the byte offset of the i-th [NN] marker in raw.
	indices := blockSplitRe.FindAllStringIndex(raw, -1)
	if len(indices) == 0 {
		return nil, nil
	}

	// Extract each chunk: from marker start to next marker start (or end of string).
	chunks := make([]string, len(indices))
	for i, loc := range indices {
		start := loc[0]
		var end int
		if i+1 < len(indices) {
			end = indices[i+1][0]
		} else {
			end = len(raw)
		}
		chunks[i] = strings.TrimRight(raw[start:end], "\n\r ")
	}

	var result []TranslatedLine

	for _, chunk := range chunks {
		// First line of the chunk is "[NN] <remainder>".
		nlIdx := strings.IndexByte(chunk, '\n')
		var firstLine, restLines string
		if nlIdx >= 0 {
			firstLine = chunk[:nlIdx]
			restLines = chunk[nlIdx+1:] // continuation lines (multiline text)
		} else {
			firstLine = chunk
		}

		match := numberedLineRe.FindStringSubmatch(firstLine)
		if match == nil {
			continue
		}

		num, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}

		remainder := strings.TrimSpace(match[2])

		// Append any continuation lines to remainder (multiline block).
		if restLines != "" {
			restTrimmed := strings.TrimSpace(restLines)
			if restTrimmed != "" {
				remainder = remainder + "\n" + restTrimmed
			}
		}

		tl := TranslatedLine{Number: num}

		// Check for [CHOICE] marker
		if strings.HasPrefix(remainder, "[CHOICE]") {
			tl.IsChoice = true
			remainder = strings.TrimSpace(strings.TrimPrefix(remainder, "[CHOICE]"))
		}

		// Check for speaker pattern before stripping quotes.
		// Speaker label (e.g. "Braxo:") is always unquoted, appearing before the text.
		// We match against the full remainder so multiline text after the speaker label
		// is captured in speakerMatch[2] + any continuation joined via afterSpeaker.
		//
		// Strategy:
		//   1. Try speaker match on the first line of remainder (before any \n or \\n).
		//   2. If matched, recombine speaker text with continuation and stripQuotes on
		//      the combined text value.
		//   3. If no speaker, stripQuotes on the full remainder.

		// Split remainder into first-line and continuation at the earliest newline
		// (real or escaped).  We handle both real \n (from quoteForPrompt) and
		// literal \n sequences (from old %q prompts that the LLM echoed back).
		splitAt := -1
		if idx := strings.Index(remainder, `\n`); idx >= 0 {
			splitAt = idx
		}
		if idx := strings.IndexByte(remainder, '\n'); idx >= 0 && (splitAt < 0 || idx < splitAt) {
			splitAt = idx
		}

		var firstSeg, afterFirst string
		if splitAt >= 0 {
			firstSeg = remainder[:splitAt]
			afterFirst = remainder[splitAt:]
		} else {
			firstSeg = remainder
		}

		if speakerMatch := speakerRe.FindStringSubmatch(firstSeg); speakerMatch != nil {
			tl.Speaker = strings.TrimSpace(speakerMatch[1])
			// Combine speaker's text segment with any continuation, then stripQuotes.
			rawText := strings.TrimSpace(speakerMatch[2]) + afterFirst
			tl.Text = stripQuotes(rawText)
		} else {
			// No speaker — stripQuotes on full remainder.
			tl.Text = stripQuotes(remainder)
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

// stripQuotes removes surrounding double quotes from a string and unescapes
// common Go escape sequences that the LLM may echo back when the prompt used %q.
//
// Multiline handling: when a block spans multiple real lines, the LLM may return
// the closing quote on a later line, or omit it entirely. We therefore strip a
// leading quote that has no matching trailing quote (unbalanced open quote).
//
// Unescape order: \\ must be last so we don't double-unescape already unescaped chars.
func stripQuotes(s string) string {
	// First, unescape escape sequences the LLM may echo from a %q-formatted prompt.
	// Note: \\ must be unescaped last to avoid double-processing.
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\t`, "\t")
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\\`, `\`)

	// Remove balanced surrounding quotes.
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}

	// Remove unbalanced leading quote (multiline block: closing quote may be on
	// a later line that the LLM omitted or the parser didn't join).
	if len(s) >= 1 && s[0] == '"' {
		return s[1:]
	}

	// Remove unbalanced trailing quote.
	if len(s) >= 1 && s[len(s)-1] == '"' {
		return s[:len(s)-1]
	}

	return s
}
