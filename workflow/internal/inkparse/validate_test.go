package inkparse

import (
	"testing"
)

func TestNormalizeForComparisonStripLineIndent(t *testing.T) {
	input := `<line-indent=2em>Hello world</line-indent>`
	got := normalizeForComparison(input)
	want := "Hello world"
	if got != want {
		t.Errorf("normalizeForComparison(%q) = %q, want %q", input, got, want)
	}
}

func TestNormalizeForComparisonStripHexColor(t *testing.T) {
	input := `<#AABBCC>Hello</color>`
	got := normalizeForComparison(input)
	want := "Hello"
	if got != want {
		t.Errorf("normalizeForComparison(%q) = %q, want %q", input, got, want)
	}
}

func TestNormalizeForComparisonStripSize(t *testing.T) {
	input := `<size=12>Hello</size>`
	got := normalizeForComparison(input)
	want := "Hello"
	if got != want {
		t.Errorf("normalizeForComparison(%q) = %q, want %q", input, got, want)
	}
}

func TestNormalizeForComparisonStripSmallcaps(t *testing.T) {
	input := `<smallcaps>Hello</smallcaps>`
	got := normalizeForComparison(input)
	want := "Hello"
	if got != want {
		t.Errorf("normalizeForComparison(%q) = %q, want %q", input, got, want)
	}
}

func TestNormalizeForComparisonWhitespace(t *testing.T) {
	input := "  Hello   world  \n  "
	got := normalizeForComparison(input)
	want := "Hello world"
	if got != want {
		t.Errorf("normalizeForComparison(%q) = %q, want %q", input, got, want)
	}
}

func TestNormalizeForComparisonPreservesContentTags(t *testing.T) {
	input := `<b>Hello</b> <i>world</i> <color=#FF0000>red</color>`
	got := normalizeForComparison(input)
	want := `<b>Hello</b> <i>world</i> <color=#FF0000>red</color>`
	if got != want {
		t.Errorf("normalizeForComparison(%q) = %q, want %q", input, got, want)
	}
}

func TestNormalizeForComparisonStripHexColor8Digit(t *testing.T) {
	input := `<#AABBCCDD>Hello</color>`
	got := normalizeForComparison(input)
	want := "Hello"
	if got != want {
		t.Errorf("normalizeForComparison(%q) = %q, want %q", input, got, want)
	}
}

func TestNormalizeForComparisonStripLink(t *testing.T) {
	input := `<link="1">1.   Hello</link>`
	got := normalizeForComparison(input)
	want := "1. Hello"
	if got != want {
		t.Errorf("normalizeForComparison(%q) = %q, want %q", input, got, want)
	}
}

func TestNormalizeForComparisonComplexDialogue(t *testing.T) {
	input := `<line-indent=-5%><#FFFFFFFF><#9AE5D5FF><size=120%><smallcaps>Jor</smallcaps><size=100%></color></line-indent>`
	got := normalizeForComparison(input)
	want := "Jor"
	if got != want {
		t.Errorf("normalizeForComparison(%q) = %q, want %q", input, got, want)
	}
}

func TestValidateAgainstCaptureAllMatch(t *testing.T) {
	blocks := []DialogueBlock{
		{Text: "Hello world"},
		{Text: "Goodbye world"},
		{Text: "Choice one"},
	}
	capture := CaptureData{
		Count: 5,
		Entries: []CaptureEntry{
			{Text: "Hello world", Origin: "ink_dialogue"},
			{Text: "Goodbye world", Origin: "ink_dialogue"},
			{Text: "Choice one", Origin: "ink_choice"},
			{Text: "Menu item", Origin: "menu_scan"},
			{Text: "UI text", Origin: "tmp_text"},
		},
	}
	report := ValidateAgainstCapture(blocks, capture)
	if report.TotalCapture != 3 {
		t.Errorf("TotalCapture = %d, want 3", report.TotalCapture)
	}
	if report.Matched != 3 {
		t.Errorf("Matched = %d, want 3", report.Matched)
	}
	if report.MatchRate != 1.0 {
		t.Errorf("MatchRate = %f, want 1.0", report.MatchRate)
	}
	if len(report.UnmatchedItems) != 0 {
		t.Errorf("UnmatchedItems = %d, want 0", len(report.UnmatchedItems))
	}
}

func TestValidateAgainstCaptureUnmatched(t *testing.T) {
	blocks := []DialogueBlock{
		{Text: "Hello world"},
	}
	capture := CaptureData{
		Count: 2,
		Entries: []CaptureEntry{
			{Text: "Hello world", Origin: "ink_dialogue"},
			{Text: "Missing text", Origin: "ink_dialogue"},
		},
	}
	report := ValidateAgainstCapture(blocks, capture)
	if report.Unmatched != 1 {
		t.Errorf("Unmatched = %d, want 1", report.Unmatched)
	}
	if len(report.UnmatchedItems) != 1 {
		t.Errorf("UnmatchedItems = %d, want 1", len(report.UnmatchedItems))
	}
	if report.UnmatchedItems[0].Text != "Missing text" {
		t.Errorf("UnmatchedItems[0].Text = %q, want %q", report.UnmatchedItems[0].Text, "Missing text")
	}
}

func TestValidateAgainstCaptureSkipsNonInkOrigins(t *testing.T) {
	blocks := []DialogueBlock{}
	capture := CaptureData{
		Count: 3,
		Entries: []CaptureEntry{
			{Text: "Menu item", Origin: "menu_scan"},
			{Text: "UI text", Origin: "tmp_text"},
			{Text: "Another menu", Origin: "menu_scan"},
		},
	}
	report := ValidateAgainstCapture(blocks, capture)
	if report.TotalCapture != 0 {
		t.Errorf("TotalCapture = %d, want 0", report.TotalCapture)
	}
	if report.SkippedOrigins["menu_scan"] != 2 {
		t.Errorf("SkippedOrigins[menu_scan] = %d, want 2", report.SkippedOrigins["menu_scan"])
	}
	if report.SkippedOrigins["tmp_text"] != 1 {
		t.Errorf("SkippedOrigins[tmp_text] = %d, want 1", report.SkippedOrigins["tmp_text"])
	}
}

func TestValidateAgainstCaptureEmptyData(t *testing.T) {
	blocks := []DialogueBlock{{Text: "Hello"}}
	capture := CaptureData{Count: 0, Entries: nil}
	report := ValidateAgainstCapture(blocks, capture)
	if report.TotalCapture != 0 {
		t.Errorf("TotalCapture = %d, want 0", report.TotalCapture)
	}
	if report.MatchRate != 1.0 {
		t.Errorf("MatchRate = %f, want 1.0 for empty capture", report.MatchRate)
	}
}

func TestValidateAgainstCaptureNormalizesWrappers(t *testing.T) {
	// Parser produces clean text, capture has rendering wrappers
	blocks := []DialogueBlock{
		{Text: "Hello world"},
	}
	capture := CaptureData{
		Count: 1,
		Entries: []CaptureEntry{
			{Text: "<line-indent=0%><#FFFFFFFF><#DAE5CFFF>Hello world\n</color></color></line-indent>", Origin: "ink_dialogue"},
		},
	}
	report := ValidateAgainstCapture(blocks, capture)
	if report.Matched != 1 {
		t.Errorf("Matched = %d, want 1 (should match after normalizing wrappers)", report.Matched)
	}
}
