package platform

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildLLMTracePathUsesFallbackAndSanitizes(t *testing.T) {
	path := BuildLLMTracePath("", filepath.Join("base", "run_logs", "llm_traces"), "score", "score-1")
	if path == "" {
		t.Fatal("expected non-empty trace path")
	}
	if !strings.Contains(path, filepath.Join("run_logs", "llm_traces")) {
		t.Fatalf("path=%q does not use fallback trace dir", path)
	}
	base := filepath.Base(path)
	if !strings.HasPrefix(base, "score__score-1__") {
		t.Fatalf("base=%q, want score__score-1__*", base)
	}
	if !strings.HasSuffix(base, ".jsonl") {
		t.Fatalf("base=%q, want .jsonl suffix", base)
	}
}

func TestBuildLLMTracePathSanitizesUnsafeChars(t *testing.T) {
	path := BuildLLMTracePath("C:\\trace root", "", "re/translate", "worker 1")
	base := filepath.Base(path)
	if strings.ContainsAny(base, " /\\") {
		t.Fatalf("base=%q contains unsafe path separators or spaces", base)
	}
	if !strings.HasPrefix(base, "re_translate__worker_1__") {
		t.Fatalf("base=%q, want sanitized prefix", base)
	}
}
