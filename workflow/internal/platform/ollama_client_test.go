package platform

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"localize-agent/workflow/internal/shared"
)

func TestNormalizeLLMBackend(t *testing.T) {
	got, err := NormalizeLLMBackend(" OLLAMA ")
	if err != nil {
		t.Fatalf("NormalizeLLMBackend error: %v", err)
	}
	if got != LLMBackendOllama {
		t.Fatalf("got=%q, want=%q", got, LLMBackendOllama)
	}

	got, err = NormalizeLLMBackend("")
	if err != nil {
		t.Fatalf("NormalizeLLMBackend empty error: %v", err)
	}
	if got != LLMBackendOpencode {
		t.Fatalf("got=%q, want=%q", got, LLMBackendOpencode)
	}

	if _, err := NormalizeLLMBackend("unknown"); err == nil {
		t.Fatalf("NormalizeLLMBackend unknown error=nil, want error")
	}
}

func TestOllamaLLMClient_SendPrompt_UsesHistory(t *testing.T) {
	call := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		call++

		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			Stream bool `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if body.Model != "qwen3:8b" {
			t.Fatalf("model=%q, want qwen3:8b", body.Model)
		}
		if body.Stream {
			t.Fatalf("stream=true, want false")
		}

		if call == 1 {
			if len(body.Messages) != 2 {
				t.Fatalf("messages len=%d, want 2", len(body.Messages))
			}
			if body.Messages[0].Role != "system" || body.Messages[0].Content != "warmup text" {
				t.Fatalf("first message=%+v, want system warmup", body.Messages[0])
			}
			if body.Messages[1].Role != "user" || body.Messages[1].Content != "q1" {
				t.Fatalf("second message=%+v, want user q1", body.Messages[1])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"message": map[string]any{"role": "assistant", "content": "a1"},
			})
			return
		}

		if len(body.Messages) != 4 {
			t.Fatalf("messages len=%d, want 4", len(body.Messages))
		}
		if body.Messages[2].Role != "assistant" || body.Messages[2].Content != "a1" {
			t.Fatalf("third message=%+v, want assistant a1", body.Messages[2])
		}
		if body.Messages[3].Role != "user" || body.Messages[3].Content != "q2" {
			t.Fatalf("fourth message=%+v, want user q2", body.Messages[3])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{"role": "assistant", "content": "a2"},
		})
	}))
	defer ts.Close()

	c := NewOllamaLLMClient(ts.URL, 2, &shared.MetricCollector{}, nil)
	profile := LLMProfile{
		ProviderID: "ollama",
		ModelID:    "qwen3:8b",
		Warmup:     "warmup text",
	}

	out1, err := c.SendPrompt("k1", profile, "q1")
	if err != nil {
		t.Fatalf("SendPrompt #1 error: %v", err)
	}
	out2, err := c.SendPrompt("k1", profile, "q2")
	if err != nil {
		t.Fatalf("SendPrompt #2 error: %v", err)
	}
	if out1 != "a1" || out2 != "a2" {
		t.Fatalf("outputs=%q,%q want a1,a2", out1, out2)
	}
	if call != 2 {
		t.Fatalf("calls=%d, want 2", call)
	}
}

func TestOllamaLLMClient_SendPrompt_ResetHistoryAndOptions(t *testing.T) {
	call := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		call++

		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			Stream    bool           `json:"stream"`
			KeepAlive string         `json:"keep_alive"`
			Format    map[string]any `json:"format"`
			Options   map[string]any `json:"options"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if body.KeepAlive != "12h" {
			t.Fatalf("keep_alive=%q, want 12h", body.KeepAlive)
		}
		if len(body.Format) == 0 {
			t.Fatalf("format is empty, want schema")
		}
		if body.Options["num_ctx"] != float64(8192) {
			t.Fatalf("num_ctx=%v, want 8192", body.Options["num_ctx"])
		}
		if body.Options["temperature"] != float64(0) {
			t.Fatalf("temperature=%v, want 0", body.Options["temperature"])
		}
		if len(body.Messages) != 2 {
			t.Fatalf("messages len=%d, want 2", len(body.Messages))
		}
		if body.Messages[0].Role != "system" || body.Messages[0].Content != "warmup text" {
			t.Fatalf("first message=%+v, want system warmup", body.Messages[0])
		}
		if call == 2 && body.Messages[1].Content != "q2" {
			t.Fatalf("second user content=%q, want q2", body.Messages[1].Content)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{"role": "assistant", "content": "[]"},
		})
	}))
	defer ts.Close()

	c := NewOllamaLLMClient(ts.URL, 2, &shared.MetricCollector{}, nil)
	profile := LLMProfile{
		ProviderID:     "ollama",
		ModelID:        "qwen3:8b",
		Warmup:         "warmup text",
		KeepAlive:      "12h",
		ResponseFormat: map[string]any{"type": "array"},
		Options:        map[string]any{"num_ctx": 8192, "temperature": 0},
		ResetHistory:   true,
	}

	if _, err := c.SendPrompt("k1", profile, "q1"); err != nil {
		t.Fatalf("SendPrompt #1 error: %v", err)
	}
	if _, err := c.SendPrompt("k1", profile, "q2"); err != nil {
		t.Fatalf("SendPrompt #2 error: %v", err)
	}
	if call != 2 {
		t.Fatalf("calls=%d, want 2", call)
	}
}
