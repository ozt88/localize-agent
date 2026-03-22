package inkparse

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

// CaptureEntry represents one entry from full_text_capture_clean.json.
type CaptureEntry struct {
	Text         string `json:"text"`
	Origin       string `json:"origin"`
	HasTags      bool   `json:"has_tags"`
	Length       int    `json:"length"`
	DialogSource string `json:"dialog_source"`
}

// CaptureData is the root structure of the capture JSON file.
type CaptureData struct {
	Count   int            `json:"count"`
	Entries []CaptureEntry `json:"entries"`
}

// ValidationReport holds results of comparing parser output to capture data.
type ValidationReport struct {
	TotalCapture   int             `json:"total_capture"`    // ink_dialogue + ink_choice entries
	Matched        int             `json:"matched"`
	Unmatched      int             `json:"unmatched"`
	MatchRate      float64         `json:"match_rate"`       // 0.0 to 1.0
	UnmatchedItems []UnmatchedItem `json:"unmatched_items"`
	SkippedOrigins map[string]int  `json:"skipped_origins"` // menu_scan, tmp_text counts
}

// UnmatchedItem records a capture entry that was not found in parser output.
type UnmatchedItem struct {
	Text   string `json:"text"`
	Origin string `json:"origin"`
}

// Rendering wrapper patterns added by the game engine (D-07).
// These are stripped before comparison. Content tags (<b>, <i>, <color=...>) are preserved.
var (
	reLineIndentOpen  = regexp.MustCompile(`<line-indent=[^>]*>`)
	reLineIndentClose = regexp.MustCompile(`</line-indent>`)
	reHexColor        = regexp.MustCompile(`<#[0-9A-Fa-f]{6,8}>`)
	reSizeOpen        = regexp.MustCompile(`<size=[^>]*>`)
	reSizeClose       = regexp.MustCompile(`</size>`)
	reSmallcapsOpen   = regexp.MustCompile(`<smallcaps>`)
	reSmallcapsClose  = regexp.MustCompile(`</smallcaps>`)
	reLinkOpen        = regexp.MustCompile(`<link="[^"]*">`)
	reLinkClose       = regexp.MustCompile(`</link>`)
	reColorOpen       = regexp.MustCompile(`<color=[^>]*>`)
	reMultiSpace      = regexp.MustCompile(`\s+`)

	// Ink command tokens the game engine processes and strips from display.
	// These appear at end of block text or as E- prefix (event trigger).
	reInkCommands = regexp.MustCompile(`\b(?:UpdateEntities|PlayNormalMusic|PlayAmbient|PlayMusic|StopMusic|StopAmbient|VOTE\s+[A-Z]+)\b`)
	reCondPrefix  = regexp.MustCompile(`^\.[A-Za-z_][A-Za-z0-9_]*[=<>!]+[^-]*-`)
	// Single-letter command prefix: E- (event), M- (move), R- (response), etc.
	reCmdPrefix = regexp.MustCompile(`^[A-Z]-`)
	// Trailing Q (queue marker) at end of text
	reTrailingQ = regexp.MustCompile(`\s+Q$`)
	// Game-engine-added parenthetical actions in choices
	reParenAction = regexp.MustCompile(`\s*\((?:Leave|Move on|Leave\.)\.\)\s*$`)
)

// normalizeForComparison strips rendering wrappers added by the game engine
// and normalizes whitespace. Content tags (<b>, <i>, <color=...>) are preserved.
func normalizeForComparison(s string) string {
	// Strip rendering wrappers
	s = reLineIndentOpen.ReplaceAllString(s, "")
	s = reLineIndentClose.ReplaceAllString(s, "")
	s = reHexColor.ReplaceAllString(s, "")
	s = reSizeOpen.ReplaceAllString(s, "")
	s = reSizeClose.ReplaceAllString(s, "")
	s = reSmallcapsOpen.ReplaceAllString(s, "")
	s = reSmallcapsClose.ReplaceAllString(s, "")
	s = reLinkOpen.ReplaceAllString(s, "")
	s = reLinkClose.ReplaceAllString(s, "")

	// Strip excess </color> tags: keep only those matching <color=...> openers.
	// After stripping <#hex> wrappers, any </color> without a <color=...> opener is a wrapper closer.
	colorOpeners := len(reColorOpen.FindAllString(s, -1))
	kept := 0
	s = strings.ReplaceAll(s, "</color>", "\x00COLOR_CLOSE\x00")
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		idx := strings.Index(s[i:], "\x00COLOR_CLOSE\x00")
		if idx < 0 {
			b.WriteString(s[i:])
			break
		}
		b.WriteString(s[i : i+idx])
		if kept < colorOpeners {
			b.WriteString("</color>")
			kept++
		}
		i += idx + len("\x00COLOR_CLOSE\x00")
	}
	s = b.String()

	// Normalize whitespace: collapse runs to single space, trim
	s = reMultiSpace.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)

	return s
}

// normalizeBlockText normalizes a parser block text for comparison.
// Strips ink command tokens, event prefixes (E-), conditional prefixes,
// and trailing queue markers that the game engine processes and removes
// before rendering.
func normalizeBlockText(s string) string {
	// Strip conditional prefix: ".VarName==1-"
	s = reCondPrefix.ReplaceAllString(s, "")
	// Strip single-letter command prefix: E-, M-, R-, etc.
	s = reCmdPrefix.ReplaceAllString(s, "")
	// Strip ink command tokens
	s = reInkCommands.ReplaceAllString(s, "")
	// Strip trailing Q marker
	s = reTrailingQ.ReplaceAllString(s, "")
	// Standard normalization
	s = normalizeForComparison(s)
	return s
}

// LoadCaptureData reads and parses a capture JSON file.
func LoadCaptureData(path string) (CaptureData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CaptureData{}, err
	}
	var cd CaptureData
	if err := json.Unmarshal(data, &cd); err != nil {
		return CaptureData{}, err
	}
	return cd, nil
}

// Patterns for game-rendered headers in capture data.
// The game engine prepends speaker names, DC checks, and choice numbers
// that are not part of the ink text content.
var (
	// DC check headers: "Intelligence dc 8: Success", "dc 14: Failure", etc.
	reDCCheck = regexp.MustCompile(`^(?:[A-Z][a-z]+ )?dc \d+: (?:Success|Failure)\s*`)
	// Choice number prefix: "1.   ", "2.   ", etc. (number + dot + spaces)
	reChoiceNum = regexp.MustCompile(`^\d+\.\s+`)
)

// extractCaptureDialogueText extracts the actual dialogue text from a capture
// entry, stripping game-rendered headers (speaker name, DC check, choice number).
//
// Capture entries are multi-line: the first line is often a speaker header or DC
// check, and subsequent lines contain the dialogue text. For choices, a number
// prefix is prepended.
func extractCaptureDialogueText(text string, origin string) []string {
	// First normalize rendering wrappers
	norm := normalizeForComparison(text)
	if norm == "" {
		return nil
	}

	if origin == "ink_choice" {
		// Strip choice number prefix: "1.   text" -> "text"
		stripped := reChoiceNum.ReplaceAllString(norm, "")
		// Strip game-added parenthetical action: "(Leave.)", "(Move on.)"
		stripped = reParenAction.ReplaceAllString(stripped, "")
		stripped = strings.TrimSpace(stripped)
		if stripped != "" {
			return []string{stripped}
		}
		return []string{norm}
	}

	// For ink_dialogue, the capture entry may have:
	// - Speaker header line (short, no punctuation typically) followed by dialogue
	// - DC check header followed by dialogue
	// - Just dialogue text

	// Split on line breaks that survived normalization
	// (normalizeForComparison collapses whitespace, but we need to detect
	// the boundary between header and text)

	// Re-normalize preserving newline boundaries: normalize each line separately
	lines := strings.Split(text, "\n")
	var dialogueLines []string
	for _, line := range lines {
		normLine := normalizeForComparison(line)
		if normLine == "" {
			continue
		}
		// Strip DC check header from line
		normLine = reDCCheck.ReplaceAllString(normLine, "")
		normLine = strings.TrimSpace(normLine)
		if normLine == "" {
			continue
		}
		dialogueLines = append(dialogueLines, normLine)
	}

	// If we have multiple lines, the first might be just a speaker name/title.
	// Speaker headers are rendered by the game engine and not in ink text.
	// Examples: "Jor", "Visken, Local Mortician", "Intelligence", etc.
	if len(dialogueLines) > 1 {
		first := dialogueLines[0]
		// Speaker names: no sentence-ending punctuation (. ! ? or quotes),
		// typically short-ish (< 50 chars). Commas are OK in titles.
		if len(first) < 50 && !strings.ContainsAny(first, ".!?\"'") {
			dialogueLines = dialogueLines[1:]
		}
	}

	return dialogueLines
}

// ValidateAgainstCapture compares parser-produced dialogue blocks against
// game runtime capture data. Only ink_dialogue and ink_choice origin entries
// are compared. Rendering wrappers and game-rendered headers are stripped
// before matching.
//
// Matching strategy: each dialogue text line extracted from a capture entry
// is checked against the set of parser block texts. A capture entry is
// considered matched if ALL its dialogue lines appear (as substrings) in
// at least one parser block.
func ValidateAgainstCapture(blocks []DialogueBlock, captureData CaptureData) ValidationReport {
	// Build set of normalized parser block texts for exact lookup,
	// plus a slice for substring matching.
	// normalizeBlockText strips ink commands, E- prefix, conditional prefix
	// in addition to standard rendering wrapper normalization.
	blockTexts := make(map[string]bool, len(blocks))
	var blockTextList []string
	for _, b := range blocks {
		norm := normalizeBlockText(b.Text)
		if norm != "" {
			blockTexts[norm] = true
			blockTextList = append(blockTextList, norm)
		}
	}

	report := ValidationReport{
		SkippedOrigins: make(map[string]int),
	}

	for _, entry := range captureData.Entries {
		if entry.Origin != "ink_dialogue" && entry.Origin != "ink_choice" {
			report.SkippedOrigins[entry.Origin]++
			continue
		}

		report.TotalCapture++

		// First try exact match on the whole normalized text
		norm := normalizeForComparison(entry.Text)
		if blockTexts[norm] {
			report.Matched++
			continue
		}

		// Extract dialogue text lines (strip headers)
		dialogueLines := extractCaptureDialogueText(entry.Text, entry.Origin)
		if len(dialogueLines) == 0 {
			report.Unmatched++
			report.UnmatchedItems = append(report.UnmatchedItems, UnmatchedItem{
				Text:   entry.Text,
				Origin: entry.Origin,
			})
			continue
		}

		// Check if all dialogue lines match (exact or substring)
		allMatched := true
		for _, line := range dialogueLines {
			if blockTexts[line] {
				continue
			}
			// Try substring match: line appears in a parser block
			found := false
			for _, bt := range blockTextList {
				if strings.Contains(bt, line) {
					found = true
					break
				}
			}
			if !found {
				allMatched = false
				break
			}
		}

		if allMatched {
			report.Matched++
		} else {
			report.Unmatched++
			report.UnmatchedItems = append(report.UnmatchedItems, UnmatchedItem{
				Text:   entry.Text,
				Origin: entry.Origin,
			})
		}
	}

	if report.TotalCapture == 0 {
		report.MatchRate = 1.0
	} else {
		report.MatchRate = float64(report.Matched) / float64(report.TotalCapture)
	}

	return report
}
