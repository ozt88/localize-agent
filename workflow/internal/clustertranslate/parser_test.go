package clustertranslate

import (
	"testing"
)

func TestParseNumberedOutput_Basic(t *testing.T) {
	raw := `[01] Braxo: "안녕하세요."
[02] "잘 지내셨어요?"
[03] "좋습니다."`

	lines, err := ParseNumberedOutput(raw)
	if err != nil {
		t.Fatalf("ParseNumberedOutput error: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	// Line 1: speaker
	if lines[0].Number != 1 {
		t.Errorf("line[0].Number = %d, want 1", lines[0].Number)
	}
	if lines[0].Speaker != "Braxo" {
		t.Errorf("line[0].Speaker = %q, want Braxo", lines[0].Speaker)
	}
	if lines[0].Text != "안녕하세요." {
		t.Errorf("line[0].Text = %q, want 안녕하세요.", lines[0].Text)
	}

	// Line 2: no speaker
	if lines[1].Number != 2 {
		t.Errorf("line[1].Number = %d, want 2", lines[1].Number)
	}
	if lines[1].Speaker != "" {
		t.Errorf("line[1].Speaker = %q, want empty", lines[1].Speaker)
	}
	if lines[1].Text != "잘 지내셨어요?" {
		t.Errorf("line[1].Text = %q", lines[1].Text)
	}
}

func TestParseNumberedOutput_Choice(t *testing.T) {
	raw := `[01] [CHOICE] "떠나기"`

	lines, err := ParseNumberedOutput(raw)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if !lines[0].IsChoice {
		t.Error("expected IsChoice=true")
	}
	if lines[0].Text != "떠나기" {
		t.Errorf("text = %q, want 떠나기", lines[0].Text)
	}
}

func TestParseNumberedOutput_NoQuotes(t *testing.T) {
	raw := `[01] 안녕하세요.
[02] Braxo: 잘 지내요.`

	lines, err := ParseNumberedOutput(raw)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0].Text != "안녕하세요." {
		t.Errorf("line[0].Text = %q, want 안녕하세요.", lines[0].Text)
	}
	if lines[1].Speaker != "Braxo" {
		t.Errorf("line[1].Speaker = %q, want Braxo", lines[1].Speaker)
	}
	if lines[1].Text != "잘 지내요." {
		t.Errorf("line[1].Text = %q, want 잘 지내요.", lines[1].Text)
	}
}

func TestParseNumberedOutput_SkipsNonNumberedLines(t *testing.T) {
	raw := `Some preamble text
[01] "안녕하세요."
Some extra commentary
[02] "잘 가요."`

	lines, err := ParseNumberedOutput(raw)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
}

func TestMapLinesToIDs(t *testing.T) {
	lines := []TranslatedLine{
		{Number: 1, Text: "안녕하세요."},
		{Number: 2, Text: "잘 가요."},
	}
	meta := PromptMeta{
		BlockIDOrder: []string{"knot/g-0/blk-0", "knot/g-0/blk-1"},
		LineCount:    2,
	}

	mapping, err := MapLinesToIDs(lines, meta)
	if err != nil {
		t.Fatalf("MapLinesToIDs error: %v", err)
	}
	if mapping["knot/g-0/blk-0"] != "안녕하세요." {
		t.Errorf("mapping[blk-0] = %q", mapping["knot/g-0/blk-0"])
	}
	if mapping["knot/g-0/blk-1"] != "잘 가요." {
		t.Errorf("mapping[blk-1] = %q", mapping["knot/g-0/blk-1"])
	}
}

func TestMapLinesToIDs_Mismatch(t *testing.T) {
	lines := []TranslatedLine{
		{Number: 1, Text: "안녕하세요."},
	}
	meta := PromptMeta{
		BlockIDOrder: []string{"blk-0", "blk-1"},
		LineCount:    2,
	}

	_, err := MapLinesToIDs(lines, meta)
	if err == nil {
		t.Error("expected error for line count mismatch")
	}
}
