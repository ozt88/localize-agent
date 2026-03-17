package semanticreview

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestDirectReviewRunnerResetsSessionAcrossCallsWhenResetHistoryEnabled(t *testing.T) {
	var mu sync.Mutex
	sessionCalls := 0
	warmupCalls := 0
	promptCalls := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			mu.Lock()
			sessionCalls++
			mu.Unlock()
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{"id":"s1"}`))
			return
		case r.Method == http.MethodPost && r.URL.Path == "/session/s1/message":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			parts, _ := body["parts"].([]any)
			part0, _ := parts[0].(map[string]any)
			text, _ := part0["text"].(string)
			if strings.Contains(text, "Reply to this warmup") {
				mu.Lock()
				warmupCalls++
				mu.Unlock()
				_ = json.NewEncoder(w).Encode(map[string]any{"parts": []map[string]any{{"type": "text", "text": "OK"}}})
				return
			}
			mu.Lock()
			promptCalls++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"parts": []map[string]any{{"type": "text", "text": `{"current_score":91,"fresh_score":92}`}}})
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	cfg := Config{
		Mode:        "direct",
		LLMBackend:  "opencode",
		ServerURL:   ts.URL,
		Model:       "openai/gpt-5.2",
		Concurrency: 1,
		BatchSize:   1,
		TimeoutSec:  2,
	}
	runner, err := NewDirectReviewRunner(cfg)
	if err != nil {
		t.Fatalf("NewDirectReviewRunner error: %v", err)
	}
	defer runner.Close()

	items := []ReviewItem{{
		ID:          "x",
		SourceEN:    "Hello.",
		CurrentKO:   "안녕.",
		FreshKO:     "안녕하세요.",
		TextRole:    "dialogue",
		ContextEN:   "Hello.",
		RetryReason: "",
	}}

	if _, err := runner.ReviewItems(items); err != nil {
		t.Fatalf("ReviewItems #1 error: %v", err)
	}
	if _, err := runner.ReviewItems(items); err != nil {
		t.Fatalf("ReviewItems #2 error: %v", err)
	}

	if sessionCalls != 2 {
		t.Fatalf("sessionCalls=%d, want 2", sessionCalls)
	}
	if warmupCalls != 2 {
		t.Fatalf("warmupCalls=%d, want 2", warmupCalls)
	}
	if promptCalls != 2 {
		t.Fatalf("promptCalls=%d, want 2", promptCalls)
	}
}
