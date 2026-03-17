package translation

import "testing"

func TestSanitizePromptKoreanReference_StripsUnexpectedForeignScript(t *testing.T) {
	got := sanitizePromptKoreanReference("No, Ragn.", "아니다, 라그н.")
	if got != "" {
		t.Fatalf("got=%q, want empty", got)
	}
}

func TestSanitizePromptKoreanReference_KeepsAllowedForeignScriptPresentInSource(t *testing.T) {
	got := sanitizePromptKoreanReference("Bjôrn says hello.", "그래. Bjôrn이라는 놈이 그랬어.")
	if got == "" {
		t.Fatalf("expected reference to be kept")
	}
}
