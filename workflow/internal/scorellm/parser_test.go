package scorellm

import (
	"testing"
)

func TestParseScoreResponse_Valid(t *testing.T) {
	raw := `{"translation_score": 8, "format_score": 9, "failure_type": "pass", "reason": ""}`
	result, err := ParseScoreResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TranslationScore != 8 {
		t.Errorf("translation_score: got %v, want 8", result.TranslationScore)
	}
	if result.FormatScore != 9 {
		t.Errorf("format_score: got %v, want 9", result.FormatScore)
	}
	if result.FailureType != "pass" {
		t.Errorf("failure_type: got %q, want %q", result.FailureType, "pass")
	}
}

func TestParseScoreResponse_Failure(t *testing.T) {
	raw := `{"translation_score": 3, "format_score": 7, "failure_type": "translation", "reason": "unnatural phrasing"}`
	result, err := ParseScoreResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FailureType != "translation" {
		t.Errorf("failure_type: got %q, want %q", result.FailureType, "translation")
	}
	if result.Reason != "unnatural phrasing" {
		t.Errorf("reason: got %q, want %q", result.Reason, "unnatural phrasing")
	}
}

func TestParseScoreResponse_InvalidJSON(t *testing.T) {
	raw := "This is not JSON, just free text explaining the score"
	_, err := ParseScoreResponse(raw)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseScoreResponse_MissingFields(t *testing.T) {
	raw := `{"score": 5}`
	_, err := ParseScoreResponse(raw)
	if err == nil {
		t.Fatal("expected error for missing failure_type")
	}
}

func TestParseScoreResponse_InvalidFailureType(t *testing.T) {
	raw := `{"translation_score": 5, "format_score": 5, "failure_type": "unknown", "reason": ""}`
	_, err := ParseScoreResponse(raw)
	if err == nil {
		t.Fatal("expected error for invalid failure_type")
	}
}

func TestParseScoreResponse_ScoreOutOfRange(t *testing.T) {
	raw := `{"translation_score": 15, "format_score": 5, "failure_type": "pass", "reason": ""}`
	_, err := ParseScoreResponse(raw)
	if err == nil {
		t.Fatal("expected error for score out of range")
	}
}

func TestParseScoreResponse_CodeFence(t *testing.T) {
	raw := "```json\n{\"translation_score\": 8, \"format_score\": 9, \"failure_type\": \"pass\", \"reason\": \"\"}\n```"
	result, err := ParseScoreResponse(raw)
	if err != nil {
		t.Fatalf("should handle code fence: %v", err)
	}
	if result.FailureType != "pass" {
		t.Errorf("failure_type: got %q, want %q", result.FailureType, "pass")
	}
}

func TestScoreResult_TargetState(t *testing.T) {
	tests := []struct {
		failureType string
		wantState   string
	}{
		{"pass", "done"},
		{"translation", "pending_translate"},
		{"format", "pending_format"},
		{"both", "pending_translate"},
	}
	for _, tt := range tests {
		r := &ScoreResult{FailureType: tt.failureType}
		got := r.TargetState()
		if got != tt.wantState {
			t.Errorf("TargetState(%q) = %q, want %q", tt.failureType, got, tt.wantState)
		}
	}
}
