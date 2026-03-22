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
	if !strings.Contains(err.Error(), "expected 5") && !strings.Contains(err.Error(), "got 3") {
		t.Errorf("error should contain counts: %q", err.Error())
	}
}

func TestValidateTranslation_DegenerateEmpty(t *testing.T) {
	raw := `[01] "안녕하세요."
[02] ""`

	meta := PromptMeta{
		LineCount:    2,
		BlockIDOrder: []string{"blk-0", "blk-1"},
	}
	sourceTexts := []string{"Hello.", "Goodbye."}

	err := ValidateTranslation(raw, meta, sourceTexts)
	if err == nil {
		t.Fatal("expected error for degenerate empty")
	}
	if !strings.Contains(err.Error(), "degenerate") || !strings.Contains(err.Error(), "empty") {
		t.Errorf("error = %q, want degenerate empty", err.Error())
	}
}

func TestValidateTranslation_DegenerateExactCopy(t *testing.T) {
	raw := `[01] "Hello."
[02] "잘 가요."`

	meta := PromptMeta{
		LineCount:    2,
		BlockIDOrder: []string{"blk-0", "blk-1"},
	}
	sourceTexts := []string{"Hello.", "Goodbye."}

	err := ValidateTranslation(raw, meta, sourceTexts)
	if err == nil {
		t.Fatal("expected error for degenerate exact_source_copy")
	}
	if !strings.Contains(err.Error(), "degenerate") || !strings.Contains(err.Error(), "exact_source_copy") {
		t.Errorf("error = %q, want degenerate exact_source_copy", err.Error())
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
