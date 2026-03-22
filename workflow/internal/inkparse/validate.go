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

// ValidateAgainstCapture compares parser-produced dialogue blocks against
// game runtime capture data. Only ink_dialogue and ink_choice origin entries
// are compared. Rendering wrappers are stripped before matching.
func ValidateAgainstCapture(blocks []DialogueBlock, captureData CaptureData) ValidationReport {
	// Build set of normalized parser block texts
	blockTexts := make(map[string]bool, len(blocks))
	for _, b := range blocks {
		norm := normalizeForComparison(b.Text)
		if norm != "" {
			blockTexts[norm] = true
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
		norm := normalizeForComparison(entry.Text)

		if blockTexts[norm] {
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
