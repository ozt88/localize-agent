package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// A real duplex byte stream exercises framing, response correlation and event
// ordering (including notifications arriving before turn/start's response).
func mockCodex(t *testing.T, status string, expectedEffort ...string) (*CodexServer, <-chan string) {
	t.Helper()
	client, peer := net.Pipe()
	s := newCodexTransport(client, "")
	methods := make(chan string, 100)
	t.Cleanup(func() { s.Close(); peer.Close() })
	go s.readLoop(client)
	go func() {
		defer peer.Close()
		dec, enc := json.NewDecoder(peer), json.NewEncoder(peer)
		n := 0
		persistent := map[string]bool{}
		for {
			var req struct {
				ID     json.RawMessage            `json:"id"`
				Method string                     `json:"method"`
				Params map[string]json.RawMessage `json:"params"`
			}
			if dec.Decode(&req) != nil {
				return
			}
			methods <- req.Method
			var result any = map[string]any{}
			str := func(k string) string { var v string; _ = json.Unmarshal(req.Params[k], &v); return v }
			notify := func(method string, p any) { _ = enc.Encode(map[string]any{"method": method, "params": p}) }
			switch req.Method {
			case "model/list":
				if str("cursor") == "second" {
					result = map[string]any{"data": []any{map[string]any{"model": "gpt-6-sol"}}, "nextCursor": nil}
				} else {
					result = map[string]any{"data": []any{map[string]any{"model": "gpt-6-luna"}}, "nextCursor": "second"}
				}
			case "thread/start":
				n++
				result = map[string]any{"thread": map[string]any{"id": fmt.Sprintf("thread-%d", n)}}
				var ephemeral bool
				_ = json.Unmarshal(req.Params["ephemeral"], &ephemeral)
				persistent[fmt.Sprintf("thread-%d", n)] = !ephemeral
				if (!ephemeral && str("developerInstructions") == "") || str("sandbox") != "read-only" || str("approvalPolicy") != "never" {
					t.Error("thread is not isolated/read-only")
				}
				if strings.Contains(str("developerInstructions"), "Reply with: OK") {
					t.Error("warmup acknowledgement leaked")
				}
			case "thread/fork":
				n++
				result = map[string]any{"thread": map[string]any{"id": fmt.Sprintf("thread-%d", n)}}
			case "turn/start":
				if persistent[str("threadId")] && !strings.Contains(string(req.Params["input"]), "No document is supplied") {
					t.Error("document content sent to general template")
				}
				if len(expectedEffort) > 0 && str("effort") != expectedEffort[0] {
					t.Errorf("effort = %q, want %q", str("effort"), expectedEffort[0])
				}
				id := str("threadId")
				turnID := "turn-" + id
				result = map[string]any{"turn": map[string]any{"id": turnID}}
				notify("item/completed", map[string]any{"threadId": id, "turnId": turnID, "item": map[string]any{"type": "agentMessage", "text": "working...", "phase": "commentary"}})
				if status != "wait" {
					completionStatus := status
					if status == "stream" {
						completionStatus = "completed"
						for i := 0; i < 600; i++ {
							notify("item/agentMessage/delta", map[string]any{"threadId": id, "turnId": turnID, "delta": "text"})
						}
					}
					if status == "commentary-only" {
						completionStatus = "completed"
					} else {
						notify("item/completed", map[string]any{"threadId": id, "turnId": turnID, "item": map[string]any{"type": "agentMessage", "text": "번역-" + id, "phase": "final_answer"}})
					}
					notify("turn/completed", map[string]any{"threadId": id, "turn": map[string]any{"id": turnID, "status": completionStatus, "error": map[string]any{"message": "test failure"}}})
				}
			case "turn/interrupt":
				notify("thread/tokenUsage/updated", map[string]any{"threadId": str("threadId"), "tokenUsage": map[string]any{"total": map[string]any{"inputTokens": 100, "cachedInputTokens": 0, "outputTokens": 20, "reasoningOutputTokens": 5}}})
				notify("turn/completed", map[string]any{"threadId": str("threadId"), "turn": map[string]any{"id": str("turnId"), "status": "interrupted"}})
			}
			if enc.Encode(map[string]any{"id": req.ID, "result": result}) != nil {
				return
			}
		}
	}()
	return s, methods
}

func TestCodexExplicitReasoningEffort(t *testing.T) {
	for _, effort := range []string{"", "medium"} {
		s, _ := mockCodex(t, "completed", effort)
		c := NewCodexLLMClient(s, 2, nil, nil)
		if _, err := c.SendPrompt("effort", LLMProfile{ModelID: "test", ReasoningEffort: effort}, "hello"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCodexRoleSessionsRotateAndIsolate(t *testing.T) {
	s, _ := mockCodex(t, "completed")
	c := NewCodexLLMClient(s, 2, nil, nil)
	defer c.CloseSessions()
	p := LLMProfile{ModelID: "test", Warmup: "Shared terms v1"}
	ask := func(key string, profile LLMProfile) string {
		t.Helper()
		v, err := c.SendSessionPromptContext(context.Background(), key, profile, "full snapshot", "delta")
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	first := ask("translate", p)
	if ask("translate", p) != first {
		t.Fatal("healthy session not reused")
	}
	if ask("review", p) == first {
		t.Fatal("review shares translator history")
	}
	for i := 0; i < 6; i++ {
		if ask("translate", p) != first {
			t.Fatal("rotated prematurely")
		}
	}
	rotated := ask("translate", p)
	if rotated == first {
		t.Fatal("turn limit did not rotate")
	}
	p.Warmup = "Shared terms v2"
	changed := ask("translate", p)
	if changed == rotated {
		t.Fatal("changed terminology reused stale session")
	}
	c.sessions["translate"].contextTokens = 180000
	if ask("translate", p) == changed {
		t.Fatal("context limit did not rotate")
	}
	p.ResetHistory = true
	reset := ask("translate", p)
	if ask("translate", p) == reset {
		t.Fatal("explicit reset ignored")
	}
}

func TestCodexFailedReusableSessionDiscarded(t *testing.T) {
	s, _ := mockCodex(t, "failed")
	c := NewCodexLLMClient(s, 2, nil, nil)
	_, err := c.SendSessionPromptContext(context.Background(), "review", LLMProfile{}, "full", "delta")
	if err == nil || c.sessions["review"].id != "" {
		t.Fatal("failed history retained")
	}
}

func TestCodexDocumentForksReuseOnlyTemplate(t *testing.T) {
	s, _ := mockCodex(t, "completed")
	c := NewCodexLLMClient(s, 2, nil, nil)
	defer c.CloseSessions()
	p := LLMProfile{ModelID: "test", Warmup: "General rules"}
	ask := func(delta string) string {
		t.Helper()
		v, e := c.SendDocumentPromptContext(context.Background(), "review", p, "whole document", delta)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	first := ask("")
	base := c.sessions["review"].baseID
	if base == "" {
		t.Fatal("no general template")
	}
	if ask("delta") != first {
		t.Fatal("same-document continuation was forked")
	}
	second := ask("")
	if second == first || c.sessions["review"].baseID != base {
		t.Fatal("document history or template lifecycle wrong")
	}
	p.Warmup = "New general rules"
	ask("")
	if c.sessions["review"].baseID == base {
		t.Fatal("changed general rules retained template")
	}
}

func TestCodexIsolatedDocumentRetries(t *testing.T) {
	s, _ := mockCodex(t, "completed")
	c := NewCodexLLMClient(s, 2, nil, nil)
	defer c.CloseSessions()
	p := LLMProfile{ModelID: "test", Warmup: "General rules"}
	ask := func(delta string) string {
		t.Helper()
		v, e := c.SendIsolatedDocumentPromptContext(context.Background(), "review", p, "full document", delta)
		if e != nil {
			t.Fatal(e)
		}
		if c.sessions["review"].baseID != "" {
			t.Fatal("unexpected template initializer")
		}
		return v
	}
	first := ask("")
	if ask("retry delta") != first {
		t.Fatal("retry lost document context")
	}
	if ask("") == first {
		t.Fatal("next document inherited previous history")
	}
}

func TestCodexLiveGeneralTemplateFork(t *testing.T) {
	if os.Getenv("LOCALIZE_CODEX_FORK_TEST") != "1" {
		t.Skip("opt-in isolated general-template inference")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	s, e := StartCodexServer(ctx, os.Getenv("LOCALIZE_CODEX_BIN"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	c := NewCodexLLMClient(s, 50, nil, nil)
	defer c.CloseSessions()
	p := LLMProfile{ModelID: "gpt-6-sol", ReasoningEffort: "medium", Warmup: "Always return rule=general-context-test. In value return the requested secret verbatim, or NONE if it has never appeared in this conversation.", ResponseFormat: map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"rule", "value"}, "properties": map[string]any{"rule": map[string]any{"type": "string"}, "value": map[string]any{"type": "string"}},
	}}
	full := []string{"Document A: its secret is ALPHA-1847. Return its secret.", "Document B: its active secret is BETA-9362. Return the secret of Document A if it appeared earlier, else NONE.", "Document B: active secret BETA-9362. Return its secret."}
	wants := []string{"ALPHA-1847", "NONE", "BETA-9362"}
	var first, base, second string
	for i, prompt := range full {
		delta := ""
		if i == 2 {
			delta = "Return the active secret of Document B from our preceding turn."
		}
		v, e := c.SendDocumentPromptContext(ctx, "review", p, prompt, delta)
		if e != nil {
			t.Fatal(e)
		}
		var result map[string]string
		if e = json.Unmarshal([]byte(v), &result); e != nil {
			t.Fatal(e)
		}
		if result["rule"] != "general-context-test" || result["value"] != wants[i] {
			t.Fatalf("turn %d incorrect: %s", i, v)
		}
		session := c.sessions["review"]
		if i == 0 {
			first = session.id
			base = session.baseID
		}
		if i == 1 {
			second = session.id
			if second == first || session.baseID != base {
				t.Fatal("wrong fork lineage")
			}
		}
		if i == 2 && session.id != second {
			t.Fatal("retry did not retain document")
		}
		t.Logf("turn=%d general rule inherited; value=%s", i, result["value"])
	}
}

func TestCodexContinuationFallsBackToFullAfterRotation(t *testing.T) {
	s, _ := mockCodex(t, "completed")
	path := t.TempDir() + "/trace.jsonl"
	trace, err := NewJSONLTraceSink(path)
	if err != nil {
		t.Fatal(err)
	}
	c := NewCodexLLMClient(s, 2, nil, trace)
	p := LLMProfile{ModelID: "test", Warmup: "context"}
	for i := 0; i < 3; i++ {
		p.ResetHistory = i == 2
		if _, err = c.SendSessionPromptContext(context.Background(), "review", p, "FULL", "DELTA"); err != nil {
			t.Fatal(err)
		}
	}
	c.CloseSessions()
	trace.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var prompts []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var event LLMTraceEvent
		if err = json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Kind == "codex_prompt" {
			prompts = append(prompts, event.Prompt)
		}
	}
	if strings.Join(prompts, ",") != "FULL,DELTA,FULL" {
		t.Fatalf("wrong payloads: %v", prompts)
	}
}

func TestCodexLiveSessionContinuation(t *testing.T) {
	if os.Getenv("LOCALIZE_CODEX_SESSION_TEST") != "1" {
		t.Skip("opt-in saved-auth session test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	s, err := StartCodexServer(ctx, os.Getenv("LOCALIZE_CODEX_BIN"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.CheckAuthentication(ctx); err != nil {
		t.Fatal(err)
	}
	c := NewCodexLLMClient(s, 50, nil, nil)
	defer c.CloseSessions()
	p := LLMProfile{ModelID: "gpt-6-sol", ReasoningEffort: "medium", Warmup: "Return the current two-item state exactly as JSON. Updates replace only named items.", ResponseFormat: map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"a", "b"}, "properties": map[string]any{"a": map[string]any{"type": "string"}, "b": map[string]any{"type": "string"}},
	}}
	first, err := c.SendSessionPromptContext(ctx, "review", p, `Initial state: a="안녕 {name}", b="비가스테스 병장". Return both.`, "")
	if err != nil {
		t.Fatal(err)
	}
	id := c.sessions["review"].id
	second, err := c.SendSessionPromptContext(ctx, "review", p, `Current full state: a="반갑습니다 {name}", b="비가스테스 병장". Return both.`, `Update only a="반갑습니다 {name}"; keep b from the previous state. Return both.`)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]string
	if err = json.Unmarshal([]byte(second), &value); err != nil {
		t.Fatal(err)
	}
	if value["a"] != "반갑습니다 {name}" || value["b"] != "비가스테스 병장" || c.sessions["review"].id != id {
		t.Fatalf("history not preserved: %s", second)
	}
	t.Logf("Same thread, two successful turns; untouched item preserved. First=%s Second=%s", first, second)
}

func TestCodexConcurrentIsolatedTurns(t *testing.T) {
	s, _ := mockCodex(t, "completed")
	c := NewCodexLLMClient(s, 2, nil, nil)
	var wg sync.WaitGroup
	results := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := c.SendPrompt("same-worker", LLMProfile{ModelID: "test", Warmup: "Translate.\nReply with: OK"}, "hello")
			if err != nil {
				t.Error(err)
			}
			results <- v
		}()
	}
	wg.Wait()
	close(results)
	seen := map[string]bool{}
	for v := range results {
		if !strings.HasPrefix(v, "번역-") {
			t.Errorf("not a final result: %q", v)
		}
		if seen[v] {
			t.Errorf("thread reused: %q", v)
		}
		seen[v] = true
	}
}

func TestCodexFailedTurnRejectsPartialText(t *testing.T) {
	s, _ := mockCodex(t, "failed")
	c := NewCodexLLMClient(s, 2, nil, nil)
	text, err := c.SendPrompt("k", LLMProfile{}, "hello")
	if err == nil || text != "" {
		t.Fatalf("failed turn accepted: %q %v", text, err)
	}
}

func TestCodexRejectsCommentaryOnly(t *testing.T) {
	s, _ := mockCodex(t, "commentary-only")
	text, err := NewCodexLLMClient(s, 2, nil, nil).SendPrompt("k", LLMProfile{}, "hello")
	if err == nil || text != "" {
		t.Fatalf("commentary accepted: %q %v", text, err)
	}
}

func TestCodexModelPreflightPagesAndRejectsUnavailable(t *testing.T) {
	s, _ := mockCodex(t, "completed")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.ValidateModels(ctx, "openai/gpt-6-luna", "gpt-6-sol"); err != nil {
		t.Fatal(err)
	}
	if err := s.ValidateModels(ctx, "unavailable-model"); err == nil {
		t.Fatal("unavailable model accepted")
	}
}

func TestCodexCancellationWaitsForInterrupt(t *testing.T) {
	s, methods := mockCodex(t, "wait")
	c := NewCodexLLMClient(s, 2, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	text, err := c.SendPromptContext(ctx, "k", LLMProfile{}, "hello")
	if err == nil || text != "" {
		t.Fatalf("canceled turn accepted: %q %v", text, err)
	}
	interrupted := false
	for len(methods) > 0 {
		if <-methods == "turn/interrupt" {
			interrupted = true
		}
	}
	if !interrupted {
		t.Fatal("no interrupt sent")
	}
	select {
	case <-s.done:
		t.Fatal("healthy shared server was stopped")
	default:
	}
}

func TestCodexCancellationPreservesUsageDuringInterrupt(t *testing.T) {
	s, _ := mockCodex(t, "wait")
	path := t.TempDir() + "/trace.jsonl"
	trace, err := NewJSONLTraceSink(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = NewCodexLLMClient(s, 2, nil, trace).SendPromptContext(ctx, "cancel", LLMProfile{ModelID: "test"}, "hello")
	if err == nil {
		t.Fatal("interrupted call unexpectedly succeeded")
	}
	trace.Close()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var event LLMTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Kind == "codex_usage" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("usage events during cancellation = %d, want 1", count)
	}
}

func TestCodexDisconnectUnblocksPendingCall(t *testing.T) {
	client, peer := net.Pipe()
	s := newCodexTransport(client, "")
	defer s.Close()
	go s.readLoop(client)
	go func() { var req any; _ = json.NewDecoder(peer).Decode(&req); peer.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := s.call(ctx, "test", nil); err == nil {
		t.Fatal("disconnect accepted")
	}
}

func TestCodexLiveHandshake(t *testing.T) {
	if os.Getenv("LOCALIZE_CODEX_LIVE_TEST") != "1" {
		t.Skip("opt-in local protocol/auth check; no inference")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s, err := StartCodexServer(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.CheckAuthentication(ctx); err != nil {
		t.Fatal(err)
	}
	models, err := s.call(ctx, "model/list", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Data []struct {
			Model     string `json:"model"`
			IsDefault bool   `json:"isDefault"`
		} `json:"data"`
	}
	if err = json.Unmarshal(models, &catalog); err != nil {
		t.Fatal(err)
	}
	for _, m := range catalog.Data {
		t.Logf("available model=%s default=%v", m.Model, m.IsDefault)
	}
	// Start and unload an ephemeral thread to verify the installed schema, without a model call.
	raw, err := s.call(ctx, "thread/start", map[string]any{"ephemeral": true, "modelProvider": "openai", "cwd": s.workDir, "sandbox": "read-only", "approvalPolicy": "never"})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if result.Thread.ID == "" {
		t.Fatal("no thread ID")
	}
	if _, err = s.call(ctx, "thread/unsubscribe", map[string]any{"threadId": result.Thread.ID}); err != nil {
		t.Fatal(err)
	}
}

func TestCodexLiveModels(t *testing.T) {
	if os.Getenv("LOCALIZE_CODEX_LIVE_INFERENCE") != "1" {
		t.Skip("opt-in: two small model calls using the signed-in Codex account")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	s, err := StartCodexServer(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.CheckAuthentication(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.ValidateModels(ctx, "gpt-6-luna", "gpt-6-sol"); err != nil {
		t.Fatal(err)
	}
	c := NewCodexLLMClient(s, 50, nil, nil)
	for _, model := range []string{"gpt-6-luna", "gpt-6-sol"} {
		t.Run(model, func(t *testing.T) {
			p := LLMProfile{ProviderID: "openai", ModelID: model, ResponseFormat: map[string]any{
				"type": "object", "properties": map[string]any{"translation": map[string]any{"type": "string"}},
				"required": []string{"translation"}, "additionalProperties": false,
			}}
			text, err := c.SendPromptContext(ctx, "smoke", p, "Translate this game UI text into Korean. Preserve the placeholder exactly: Hello, {player}!")
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				Translation string `json:"translation"`
			}
			if err := json.Unmarshal([]byte(text), &result); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if !strings.Contains(result.Translation, "{player}") || !strings.ContainsAny(result.Translation, "안녕반갑") {
				t.Fatalf("invalid translation: %q", result.Translation)
			}
			t.Logf("%s: %s", model, result.Translation)
		})
	}
}
