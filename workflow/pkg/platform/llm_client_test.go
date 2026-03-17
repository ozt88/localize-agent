package platform

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"localize-agent/workflow/pkg/shared"
)

type fakeTraceSink struct {
	mu     sync.Mutex
	events []LLMTraceEvent
}

func (f *fakeTraceSink) Write(event LLMTraceEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, event)
	return nil
}

func (f *fakeTraceSink) Close() error { return nil }

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type errReadCloser struct {
	data []byte
	read bool
}

func (e *errReadCloser) Read(p []byte) (int, error) {
	if !e.read {
		e.read = true
		n := copy(p, e.data)
		return n, io.ErrUnexpectedEOF
	}
	return 0, io.ErrUnexpectedEOF
}

func (e *errReadCloser) Close() error { return nil }

func TestParseModel(t *testing.T) {
	tests := []struct {
		name      string
		giveModel string
		wantProv  string
		wantModel string
		wantErr   bool
	}{
		{name: "valid", giveModel: "openai/gpt-5.2", wantProv: "openai", wantModel: "gpt-5.2"},
		{name: "missing slash", giveModel: "openai", wantErr: true},
		{name: "empty model side", giveModel: "openai/", wantProv: "openai", wantModel: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prov, model, err := ParseModel(tt.giveModel)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseModel(%q) error = nil, want error", tt.giveModel)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseModel(%q) unexpected error: %v", tt.giveModel, err)
			}
			if prov != tt.wantProv {
				t.Fatalf("provider = %q, want %q", prov, tt.wantProv)
			}
			if model != tt.wantModel {
				t.Fatalf("model = %q, want %q", model, tt.wantModel)
			}
		})
	}
}

func TestJoinTextParts(t *testing.T) {
	t.Run("joins text entries", func(t *testing.T) {
		out := map[string]any{
			"parts": []any{
				map[string]any{"type": "text", "text": "first"},
				map[string]any{"type": "tool", "text": "ignore"},
				map[string]any{"type": "text", "text": "second"},
			},
		}

		got, err := joinTextParts(out)
		if err != nil {
			t.Fatalf("joinTextParts returned error: %v", err)
		}
		if got != "first\nsecond" {
			t.Fatalf("joinTextParts=%q, want %q", got, "first\\nsecond")
		}
	})

	t.Run("returns error without text", func(t *testing.T) {
		out := map[string]any{
			"parts": []any{
				map[string]any{"type": "tool", "text": "ignore"},
				"invalid",
			},
		}

		_, err := joinTextParts(out)
		if err == nil {
			t.Fatalf("joinTextParts error=nil, want error")
		}
	})
}

func TestSessionLLMClient_EnsureContextWithoutWarmup_NoHTTPCall(t *testing.T) {
	hits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.NotFound(w, r)
	}))
	defer ts.Close()

	c := NewSessionLLMClient(ts.URL, 2, &shared.MetricCollector{}, nil)
	if err := c.EnsureContext("k", LLMProfile{}); err != nil {
		t.Fatalf("EnsureContext error: %v", err)
	}
	if hits != 0 {
		t.Fatalf("http hits=%d, want 0", hits)
	}
}

func TestSessionLLMClient_SendPrompt_WarmupSessionReuseAndTrace(t *testing.T) {
	var mu sync.Mutex
	sessionCalls := 0
	warmupCalls := 0
	promptCalls := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			if r.URL.Query().Get("directory") == "" {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			mu.Lock()
			sessionCalls++
			mu.Unlock()
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{"id":"s1"}`))
			return
		case r.Method == http.MethodPost && r.URL.Path == "/session/s1/message":
			if r.URL.Query().Get("directory") == "" {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			parts, _ := body["parts"].([]any)
			p0, _ := parts[0].(map[string]any)
			prompt, _ := p0["text"].(string)

			if strings.Contains(prompt, "warmup") {
				mu.Lock()
				warmupCalls++
				mu.Unlock()
				_ = json.NewEncoder(w).Encode(map[string]any{"parts": []map[string]any{{"type": "text", "text": "OK"}}})
				return
			}
			mu.Lock()
			promptCalls++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"parts": []map[string]any{{"type": "text", "text": "answer:" + prompt}}})
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	trace := &fakeTraceSink{}
	c := NewSessionLLMClient(ts.URL, 2, &shared.MetricCollector{}, trace)
	profile := LLMProfile{ProviderID: "p", ModelID: "m", Warmup: "please warmup"}

	out1, err := c.SendPrompt("slot-a", profile, "q1")
	if err != nil {
		t.Fatalf("SendPrompt #1 error: %v", err)
	}
	out2, err := c.SendPrompt("slot-a", profile, "q2")
	if err != nil {
		t.Fatalf("SendPrompt #2 error: %v", err)
	}

	if out1 != "answer:q1" || out2 != "answer:q2" {
		t.Fatalf("outputs=%q,%q", out1, out2)
	}
	if sessionCalls != 1 {
		t.Fatalf("sessionCalls=%d, want 1", sessionCalls)
	}
	if warmupCalls != 1 {
		t.Fatalf("warmupCalls=%d, want 1", warmupCalls)
	}
	if promptCalls != 2 {
		t.Fatalf("promptCalls=%d, want 2", promptCalls)
	}
	if len(trace.events) != 7 {
		t.Fatalf("trace events=%d, want 7", len(trace.events))
	}
	kinds := []string{
		trace.events[0].Kind,
		trace.events[1].Kind,
		trace.events[2].Kind,
		trace.events[3].Kind,
		trace.events[4].Kind,
		trace.events[5].Kind,
		trace.events[6].Kind,
	}
	want := []string{"request", "request", "warmup", "request", "prompt", "request", "prompt"}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("trace kinds=%v, want %v", kinds, want)
		}
	}
}

func TestSessionLLMClient_SendPrompt_NoTextReturnsErrorAndTrace(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{"id":"s1"}`))
			return
		case r.Method == http.MethodPost && r.URL.Path == "/session/s1/message":
			_ = json.NewEncoder(w).Encode(map[string]any{"parts": []map[string]any{{"type": "tool", "text": "noop"}}})
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	trace := &fakeTraceSink{}
	c := NewSessionLLMClient(ts.URL, 2, &shared.MetricCollector{}, trace)
	profile := LLMProfile{ProviderID: "p", ModelID: "m", Warmup: ""}

	_, err := c.SendPrompt("slot-a", profile, "q")
	if err == nil {
		t.Fatalf("SendPrompt error=nil, want error")
	}
	if !strings.Contains(err.Error(), "no text in response") {
		t.Fatalf("error=%v", err)
	}
	if len(trace.events) != 3 {
		t.Fatalf("trace events=%d, want 3", len(trace.events))
	}
	if trace.events[0].Kind != "request" || trace.events[1].Kind != "request" || trace.events[2].Kind != "prompt_error" {
		t.Fatalf("trace kinds=%s/%s/%s, want request/request/prompt_error", trace.events[0].Kind, trace.events[1].Kind, trace.events[2].Kind)
	}
}

func TestSessionLLMClient_SendPrompt_ResponseParseErrorCapturesRawResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{"id":"s1"}`))
			return
		case r.Method == http.MethodPost && r.URL.Path == "/session/s1/message":
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{"parts":[{"type":"text","text":"unterminated"}`))
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	trace := &fakeTraceSink{}
	c := NewSessionLLMClient(ts.URL, 2, &shared.MetricCollector{}, trace)
	_, err := c.SendPrompt("slot-a", LLMProfile{ProviderID: "openai", ModelID: "gpt-5.2"}, "q")
	if err == nil {
		t.Fatalf("SendPrompt error=nil, want parse error")
	}
	if len(trace.events) != 4 {
		t.Fatalf("trace events=%d, want 4", len(trace.events))
	}
	if trace.events[0].Kind != "request" {
		t.Fatalf("trace event[0].Kind=%q, want request", trace.events[0].Kind)
	}
	if trace.events[2].Kind != "response_parse_error" {
		t.Fatalf("trace event[2].Kind=%q, want response_parse_error", trace.events[2].Kind)
	}
	if !strings.Contains(trace.events[2].ResponseRaw, `"unterminated"`) {
		t.Fatalf("response_raw=%q, want malformed payload captured", trace.events[2].ResponseRaw)
	}
	if trace.events[3].Kind != "prompt_error" {
		t.Fatalf("trace event[3].Kind=%q, want prompt_error", trace.events[3].Kind)
	}
}

func TestSessionLLMClient_SendPrompt_ResponseReadErrorCapturesRawResponse(t *testing.T) {
	trace := &fakeTraceSink{}
	c := NewSessionLLMClient("http://example.invalid", 2, &shared.MetricCollector{}, trace)
	c.http = &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path == "/session" {
				return &http.Response{
					StatusCode: 200,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"id":"s1"}`)),
				}, nil
			}
			if r.URL.Path == "/session/s1/message" {
				return &http.Response{
					StatusCode: 200,
					Header:     make(http.Header),
					Body:       &errReadCloser{data: []byte(`{"parts":[{"type":"text","text":"partial"}`)},
				}, nil
			}
			return &http.Response{
				StatusCode: 404,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`not found`)),
			}, nil
		}),
	}

	_, err := c.SendPrompt("slot-a", LLMProfile{ProviderID: "openai", ModelID: "gpt-5.2"}, "q")
	if err == nil {
		t.Fatalf("SendPrompt error=nil, want read error")
	}
	if !strings.Contains(err.Error(), "read response body") {
		t.Fatalf("error=%v, want read response body", err)
	}
	if len(trace.events) != 4 {
		t.Fatalf("trace events=%d, want 4", len(trace.events))
	}
	if trace.events[2].Kind != "response_read_error" {
		t.Fatalf("trace event[2].Kind=%q, want response_read_error", trace.events[2].Kind)
	}
	if !strings.Contains(trace.events[2].ResponseRaw, `"partial"`) {
		t.Fatalf("response_raw=%q, want partial payload captured", trace.events[2].ResponseRaw)
	}
	if trace.events[3].Kind != "prompt_error" {
		t.Fatalf("trace event[3].Kind=%q, want prompt_error", trace.events[3].Kind)
	}
}

func TestSessionLLMClient_SendPrompt_EmptyResponseBodyTracesExplicitly(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{"id":"s1"}`))
			return
		case r.Method == http.MethodPost && r.URL.Path == "/session/s1/message":
			w.WriteHeader(http.StatusOK)
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	trace := &fakeTraceSink{}
	c := NewSessionLLMClient(ts.URL, 2, &shared.MetricCollector{}, trace)
	_, err := c.SendPrompt("slot-a", LLMProfile{ProviderID: "openai", ModelID: "gpt-5.2"}, "q")
	if err == nil {
		t.Fatalf("SendPrompt error=nil, want empty response error")
	}
	if !strings.Contains(err.Error(), "empty response body") {
		t.Fatalf("error=%v, want empty response body", err)
	}
	if len(trace.events) != 4 {
		t.Fatalf("trace events=%d, want 4", len(trace.events))
	}
	if trace.events[2].Kind != "response_empty" {
		t.Fatalf("trace event[2].Kind=%q, want response_empty", trace.events[2].Kind)
	}
	if trace.events[2].ResponseBytes != 0 {
		t.Fatalf("response bytes=%d, want 0", trace.events[2].ResponseBytes)
	}
	if !trace.events[2].ResponseEmpty {
		t.Fatalf("response empty flag=false, want true")
	}
	if trace.events[3].Kind != "prompt_error" {
		t.Fatalf("trace event[3].Kind=%q, want prompt_error", trace.events[3].Kind)
	}
}

func TestSessionLLMClient_SendPrompt_SessionIDMissing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/session" {
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{"x":"y"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	c := NewSessionLLMClient(ts.URL, 2, &shared.MetricCollector{}, nil)
	_, err := c.SendPrompt("slot-a", LLMProfile{ProviderID: "p", ModelID: "m"}, "q")
	if err == nil {
		t.Fatalf("SendPrompt error=nil, want error")
	}
	if want := "session id missing in response"; !strings.Contains(fmt.Sprint(err), want) {
		t.Fatalf("error=%v, want contains %q", err, want)
	}
}
