package clustertranslate

import (
	"strings"
	"testing"
)

func TestValidateTranslation_LineCountMismatch(t *testing.T) {
	raw := `[01] "안녕하세요."
[02] "잘 가요."
[03] "다시 만나요."`

	meta := PromptMeta{
		LineCount:    5,
		BlockIDOrder: []string{"blk-0", "blk-1", "blk-2", "blk-3", "blk-4"},
	}
	sourceTexts := []string{"Hello.", "Goodbye.", "See you.", "Later.", "End."}

	err := ValidateTranslation(raw, meta, sourceTexts)
	if err == nil {
		t.Fatal("expected error for line count mismatch")
	}
	if !strings.Contains(err.Error(), "line count mismatch") {
		t.Errorf("error = %q, want line count mismatch", err.Error())
	}
}

func TestValidateTranslation_AllDegenerate(t *testing.T) {
	// Both lines are exact copies — 100% degenerate → reject
	raw := `[01] "Hello."
[02] "Goodbye."`

	meta := PromptMeta{
		LineCount:    2,
		BlockIDOrder: []string{"blk-0", "blk-1"},
	}
	sourceTexts := []string{"Hello.", "Goodbye."}

	err := ValidateTranslation(raw, meta, sourceTexts)
	if err == nil {
		t.Fatal("expected error when all lines degenerate")
	}
	if !strings.Contains(err.Error(), "degenerate") {
		t.Errorf("error = %q, want degenerate", err.Error())
	}
}

func TestValidateTranslation_SingleLineDegenerateAccepted(t *testing.T) {
	// 1 of 4 lines is an exact copy — 25% degenerate → accept (≤50%)
	raw := `[01] "안녕하세요."
[02] "Goodbye."
[03] "다시 만나요."
[04] "또 봐요."`

	meta := PromptMeta{
		LineCount:    4,
		BlockIDOrder: []string{"blk-0", "blk-1", "blk-2", "blk-3"},
	}
	sourceTexts := []string{"Hello.", "Goodbye.", "See you.", "Later."}

	err := ValidateTranslation(raw, meta, sourceTexts)
	if err != nil {
		t.Errorf("expected pass (25%% degenerate ≤50%%), got error: %v", err)
	}
}

func TestValidateTranslation_MajorityDegenerateRejected(t *testing.T) {
	// 3 of 4 lines are exact copies — 75% degenerate → reject
	raw := `[01] "Hello."
[02] "Goodbye."
[03] "See you."
[04] "또 봐요."`

	meta := PromptMeta{
		LineCount:    4,
		BlockIDOrder: []string{"blk-0", "blk-1", "blk-2", "blk-3"},
	}
	sourceTexts := []string{"Hello.", "Goodbye.", "See you.", "Later."}

	err := ValidateTranslation(raw, meta, sourceTexts)
	if err == nil {
		t.Fatal("expected error when 75% degenerate")
	}
	if !strings.Contains(err.Error(), "3/4") {
		t.Errorf("error = %q, want ratio info", err.Error())
	}
}

func TestValidateTranslation_Pass(t *testing.T) {
	raw := `[01] "안녕하세요."
[02] "잘 가요."`

	meta := PromptMeta{
		LineCount:    2,
		BlockIDOrder: []string{"blk-0", "blk-1"},
	}
	sourceTexts := []string{"Hello.", "Goodbye."}

	err := ValidateTranslation(raw, meta, sourceTexts)
	if err != nil {
		t.Errorf("expected pass, got error: %v", err)
	}
}

func TestValidateLineCount(t *testing.T) {
	if err := ValidateLineCount(5, 5); err != nil {
		t.Errorf("expected nil for matching counts, got: %v", err)
	}
	if err := ValidateLineCount(5, 3); err == nil {
		t.Error("expected error for mismatching counts")
	}
}

func TestDegenerateReason_GameTagsExcluded(t *testing.T) {
	// Korean translation with HTML tags — tags are ASCII but should be excluded
	en := `<i>Vourgeni</i>. A great highland.`
	ko := `<i>보르게니</i>. 거대한 고원.`

	reason := degenerateReason(en, ko)
	if reason == "ascii_heavy" {
		t.Error("should not be ascii_heavy — HTML tags should be excluded from ASCII ratio")
	}
}

func TestDegenerateReason_GameTokensExcluded(t *testing.T) {
	// Game command mixed with Korean — ROLL/SPELL tokens should be excluded
	en := `ROLL25 dex-Crack the safe. .LOCKPICK-Use_your_Esoteric`
	ko := `ROLL25 dex-금고를 연다. .LOCKPICK-에소테릭을 사용하라`

	reason := degenerateReason(en, ko)
	if reason == "ascii_heavy" {
		t.Error("should not be ascii_heavy — game tokens should be excluded from ASCII ratio")
	}
}

func TestDegenerateReason_PureAsciiStillCaught(t *testing.T) {
	// Truly untranslated text with no game tokens
	en := `Hello adventurer, welcome to the tavern.`
	ko := `Hello adventurer, welcome to the tavern.`

	reason := degenerateReason(en, ko)
	if reason == "" {
		t.Error("expected degenerate reason for exact copy")
	}
}

func TestStripGameTokensAndTags(t *testing.T) {
	input := `<b>ROLL25</b> dex-금고를 .LOCKPICK-열쇠 <color=#FFF>보르게니</color>`
	got := stripGameTokensAndTags(input)

	if strings.Contains(got, "<b>") || strings.Contains(got, "</color>") {
		t.Errorf("HTML tags not stripped: %q", got)
	}
}

func TestCheckDegenerate(t *testing.T) {
	parsed := []TranslatedLine{
		{Number: 1, Text: "안녕하세요."},
		{Number: 2, Text: "Hello."},       // exact copy
		{Number: 3, Text: "다시 만나요."},
	}
	sourceTexts := []string{"Hello.", "Hello.", "See you."}

	result := CheckDegenerate(parsed, sourceTexts)
	if result.DegenerateN != 1 {
		t.Errorf("expected 1 degenerate, got %d", result.DegenerateN)
	}
	if result.TotalN != 3 {
		t.Errorf("expected 3 total, got %d", result.TotalN)
	}
}
