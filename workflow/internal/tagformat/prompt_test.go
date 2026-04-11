package tagformat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildFormatPrompt_Single(t *testing.T) {
	tasks := []FormatTask{
		{BlockID: "blk-1", ENSource: "<b>Watch</b> your step.", KOPlain: "조심해."},
	}
	prompt := BuildFormatPrompt(tasks)
	if !strings.Contains(prompt, `"pairs"`) {
		t.Error("prompt should contain pairs JSON key")
	}
	if !strings.Contains(prompt, "<b>Watch</b> your step.") {
		t.Error("prompt should contain EN source")
	}
	if !strings.Contains(prompt, "조심해.") {
		t.Error("prompt should contain KO plain")
	}
	// Verify it's valid JSON.
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(prompt), &parsed); err != nil {
		t.Errorf("prompt should be valid JSON: %v", err)
	}
}

func TestBuildFormatPrompt_Multiple(t *testing.T) {
	tasks := []FormatTask{
		{BlockID: "blk-1", ENSource: "<b>Watch</b> your step.", KOPlain: "조심해."},
		{BlockID: "blk-2", ENSource: "The <shake>ground trembled</shake>.", KOPlain: "땅이 흔들렸다."},
		{BlockID: "blk-3", ENSource: "<i>Hello</i>.", KOPlain: "안녕."},
	}
	prompt := BuildFormatPrompt(tasks)
	var parsed struct {
		Pairs []struct {
			EN string `json:"en"`
			KO string `json:"ko"`
		} `json:"pairs"`
	}
	if err := json.Unmarshal([]byte(prompt), &parsed); err != nil {
		t.Fatalf("prompt should be valid JSON: %v", err)
	}
	if len(parsed.Pairs) != 3 {
		t.Errorf("expected 3 pairs, got %d", len(parsed.Pairs))
	}
}

func TestParseFormatResponse_Valid(t *testing.T) {
	raw := `{"results": ["<b>조심해</b>."]}`
	results, err := ParseFormatResponse(raw, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0] != "<b>조심해</b>." {
		t.Errorf("got %q, want %q", results[0], "<b>조심해</b>.")
	}
}

func TestParseFormatResponse_CountMismatch(t *testing.T) {
	raw := `{"results": ["<b>조심해</b>.", "<i>안녕</i>."]}`
	_, err := ParseFormatResponse(raw, 1)
	if err == nil {
		t.Fatal("expected error for count mismatch")
	}
}

func TestParseFormatResponse_InvalidJSON(t *testing.T) {
	raw := "not json at all"
	_, err := ParseFormatResponse(raw, 1)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseFormatResponse_CodeFence(t *testing.T) {
	raw := "```json\n{\"results\": [\"<b>조심해</b>.\"]}\n```"
	results, err := ParseFormatResponse(raw, 1)
	if err != nil {
		t.Fatalf("should handle code fence: %v", err)
	}
	if len(results) != 1 || results[0] != "<b>조심해</b>." {
		t.Errorf("unexpected results: %v", results)
	}
}
