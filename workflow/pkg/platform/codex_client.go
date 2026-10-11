package platform

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"localize-agent/workflow/pkg/shared"
)

// PromptClient is the model boundary used by the V2 workers.
type PromptClient interface {
	EnsureContext(string, LLMProfile) error
	SendPromptContext(context.Context, string, LLMProfile, string) (string, error)
}

type rpcMessage struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// CodexServer owns only the child process it starts. It never touches desktop
// sessions or other Codex/OpenCode processes. A failed transport fails closed;
// the caller must restart the pipeline rather than replay an ambiguous turn.
type CodexServer struct {
	cmd     *exec.Cmd
	in      io.WriteCloser
	workDir string
	writeMu sync.Mutex
	mu      sync.Mutex
	nextID  int64
	pending map[string]chan rpcMessage
	threads map[string]chan rpcMessage
	done    chan struct{}
	waited  chan struct{}
	err     error
	once    sync.Once
}

// StartCodexServer uses saved Codex authentication; it does not read or copy tokens.
// executable may name a native binary. Empty resolves Codex from PATH, including
// the npm Windows shim without invoking cmd.exe or interpolating shell commands.
func StartCodexServer(ctx context.Context, executable string) (*CodexServer, error) {
	return StartCodexServerWithOptions(ctx, executable, CodexServerOptions{})
}

// CodexServerOptions applies only to the owned translation subprocess. It never
// edits the user's configuration or credentials.
type CodexServerOptions struct {
	MinimalContext bool
}

func StartCodexServerWithOptions(ctx context.Context, executable string, options CodexServerOptions) (*CodexServer, error) {
	bin, args, err := codexCommand(executable)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "localize-codex-")
	if err != nil {
		return nil, err
	}
	args = append(args, "app-server", "--listen", "stdio://", "-c", "features.shell_tool=false", "-c", "web_search=\"disabled\"")
	if options.MinimalContext {
		args = append(args, "-c", "skills.include_instructions=false", "-c", "project_doc_max_bytes=0")
		// A text-only localization worker has no reason to advertise app,
		// browser, skill-discovery or delegation tools to the model.
		for _, feature := range []string{"apps", "plugins", "multi_agent", "skill_search", "browser_use", "computer_use", "view_image", "goals", "sleep_tool", "tool_suggest"} {
			args = append(args, "-c", "features."+feature+"=false")
		}
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	// Do not retain raw stderr, which may contain provider configuration.
	cmd.Stderr = io.Discard
	in, err := cmd.StdinPipe()
	if err != nil {
		os.Remove(dir)
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		os.Remove(dir)
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		in.Close()
		out.Close()
		os.Remove(dir)
		return nil, fmt.Errorf("start codex app-server: %w", err)
	}
	s := newCodexTransport(in, dir)
	s.cmd = cmd
	go s.readLoop(out)
	go func() {
		err := cmd.Wait()
		if err == nil {
			err = errors.New("codex app-server exited")
		}
		s.fail(err)
		close(s.waited)
	}()
	initCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	_, err = s.call(initCtx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "localize_agent", "version": "1.0.0"}, "capabilities": map[string]any{"experimentalApi": true}})
	if err == nil {
		err = s.write(map[string]any{"method": "initialized", "params": map[string]any{}})
	}
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("initialize codex: %w", err)
	}
	return s, nil
}

func codexCommand(executable string) (string, []string, error) {
	if executable != "" {
		return executable, nil, nil
	}
	path, err := exec.LookPath("codex")
	if err != nil {
		return "", nil, fmt.Errorf("Codex CLI not found; install it and sign in with codex login: %w", err)
	}
	if runtime.GOOS == "windows" && (strings.EqualFold(filepath.Ext(path), ".cmd") || strings.EqualFold(filepath.Ext(path), ".ps1")) {
		arch := "x64"
		if runtime.GOARCH == "arm64" {
			arch = "arm64"
		}
		root := filepath.Join(filepath.Dir(path), "node_modules", "@openai")
		for _, pattern := range []string{
			filepath.Join(root, "codex", "node_modules", "@openai", "codex-win32-"+arch, "vendor", "*", "bin", "codex.exe"),
			filepath.Join(root, "codex-win32-"+arch, "vendor", "*", "bin", "codex.exe"),
		} {
			matches, _ := filepath.Glob(pattern)
			if len(matches) == 1 {
				return matches[0], nil, nil
			}
		}
		return "", nil, fmt.Errorf("cannot resolve native npm Codex binary; set --codex-bin to native codex.exe")
	}
	return path, nil, nil
}

func newCodexTransport(in io.WriteCloser, dir string) *CodexServer {
	return &CodexServer{in: in, workDir: dir, pending: make(map[string]chan rpcMessage), threads: make(map[string]chan rpcMessage), done: make(chan struct{}), waited: make(chan struct{})}
}

func (s *CodexServer) fail(err error) {
	s.once.Do(func() { s.mu.Lock(); s.err = err; s.mu.Unlock(); close(s.done) })
}

func (s *CodexServer) failure() error { s.mu.Lock(); defer s.mu.Unlock(); return s.err }

func (s *CodexServer) Done() <-chan struct{} { return s.done }
func (s *CodexServer) Err() error            { return s.failure() }

func (s *CodexServer) write(v any) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	select {
	case <-s.done:
		return s.failure()
	default:
	}
	return json.NewEncoder(s.in).Encode(v)
}

func (s *CodexServer) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.nextID++
	id := fmt.Sprint(s.nextID)
	ch := make(chan rpcMessage, 1)
	s.pending[id] = ch
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.pending, id); s.mu.Unlock() }()
	if err := s.write(map[string]any{"id": json.RawMessage(id), "method": method, "params": params}); err != nil {
		s.fail(err)
		return nil, err
	}
	select {
	case m := <-ch:
		if m.Error != nil {
			return nil, fmt.Errorf("codex %s (%d): %s", method, m.Error.Code, m.Error.Message)
		}
		return m.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.done:
		return nil, s.failure()
	}
}

func (s *CodexServer) readLoop(r io.Reader) {
	firstDelta := map[string]string{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var m rpcMessage
		if err := json.Unmarshal(scanner.Bytes(), &m); err != nil {
			s.fail(fmt.Errorf("invalid codex protocol: %w", err))
			return
		}
		if len(m.ID) > 0 {
			if m.Method != "" {
				// Translation never requires interactive approvals or client tools.
				if err := s.write(map[string]any{"id": m.ID, "error": map[string]any{"code": -32601, "message": "interactive tools are unavailable in localization worker"}}); err != nil {
					s.fail(err)
					return
				}
				continue
			}
			s.mu.Lock()
			ch := s.pending[string(m.ID)]
			s.mu.Unlock()
			if ch != nil {
				select {
				case ch <- m:
				default:
				}
			}
			continue
		}
		if m.Method != "item/completed" && m.Method != "turn/completed" && m.Method != "turn/started" && m.Method != "thread/tokenUsage/updated" && m.Method != "item/agentMessage/delta" {
			continue
		}
		var p struct {
			ThreadID string `json:"threadId"`
			TurnID   string `json:"turnId"`
			Delta    string `json:"delta"`
		}
		if json.Unmarshal(m.Params, &p) != nil {
			continue
		}
		if m.Method == "item/agentMessage/delta" {
			if p.Delta == "" || firstDelta[p.ThreadID] == p.TurnID {
				continue
			}
			firstDelta[p.ThreadID] = p.TurnID
		}
		if m.Method == "turn/completed" {
			delete(firstDelta, p.ThreadID)
		}
		s.mu.Lock()
		ch := s.threads[p.ThreadID]
		s.mu.Unlock()
		if ch != nil {
			select {
			case ch <- m:
			default:
				s.fail(errors.New("codex event buffer overflow"))
				return
			}
		}
	}
	err := scanner.Err()
	if err == nil {
		err = io.EOF
	}
	s.fail(fmt.Errorf("codex transport closed: %w", err))
}

func (s *CodexServer) Close() error {
	s.fail(errors.New("codex app-server closed"))
	s.in.Close()
	if s.cmd != nil && s.cmd.Process != nil {
		select {
		case <-s.waited:
		case <-time.After(time.Second):
			_ = s.cmd.Process.Kill()
			<-s.waited
		}
	}
	if s.workDir != "" {
		_ = os.Remove(s.workDir)
	} // only remove an empty owned directory
	return nil
}

// CheckAuthentication never logs account details or initiates a login flow.
func (s *CodexServer) CheckAuthentication(ctx context.Context) error {
	raw, err := s.call(ctx, "account/read", map[string]any{"refreshToken": false})
	if err != nil {
		return err
	}
	var result struct {
		Account  json.RawMessage `json:"account"`
		Requires bool            `json:"requiresOpenaiAuth"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return err
	}
	if result.Requires && (len(result.Account) == 0 || string(result.Account) == "null") {
		return errors.New("Codex is not signed in; run codex login before starting the pipeline")
	}
	return nil
}

// ValidateModels checks the account's advertised catalog before claiming work.
// It never silently substitutes a different model.
func (s *CodexServer) ValidateModels(ctx context.Context, models ...string) error {
	available := map[string]bool{}
	seenCursors := map[string]bool{}
	var cursor *string
	for {
		raw, err := s.call(ctx, "model/list", map[string]any{"includeHidden": true, "cursor": cursor})
		if err != nil {
			return err
		}
		var page struct {
			Data []struct {
				Model string `json:"model"`
			} `json:"data"`
			NextCursor *string `json:"nextCursor"`
		}
		if err = json.Unmarshal(raw, &page); err != nil {
			return err
		}
		for _, m := range page.Data {
			available[m.Model] = true
		}
		if page.NextCursor == nil {
			break
		}
		if seenCursors[*page.NextCursor] {
			return errors.New("codex model catalog repeated a cursor")
		}
		seenCursors[*page.NextCursor] = true
		cursor = page.NextCursor
	}
	for _, m := range models {
		if m == "" {
			continue
		}
		model := strings.TrimPrefix(strings.TrimPrefix(m, "openai/"), "codex/")
		if !available[model] {
			return fmt.Errorf("model %q is not advertised by the signed-in Codex CLI; select a supported model explicitly", m)
		}
	}
	return nil
}

type CodexLLMClient struct {
	server     *CodexServer
	timeout    time.Duration
	metrics    *shared.MetricCollector
	trace      LLMTraceSink
	sessionsMu sync.Mutex
	sessions   map[string]*codexSession
}

type codexSession struct {
	mu            sync.Mutex
	id            string
	baseID        string
	signature     string
	turns         int
	contextTokens int
	events        chan rpcMessage
}

func NewCodexLLMClient(server *CodexServer, timeoutSec int, metrics *shared.MetricCollector, trace LLMTraceSink) *CodexLLMClient {
	if timeoutSec <= 0 {
		timeoutSec = 120
	}
	return &CodexLLMClient{server: server, timeout: time.Duration(timeoutSec) * time.Second, metrics: metrics, trace: trace, sessions: map[string]*codexSession{}}
}

func (c *CodexLLMClient) EnsureContext(_ string, p LLMProfile) error {
	if p.ProviderID != "" && p.ProviderID != "openai" && p.ProviderID != "codex" {
		return fmt.Errorf("Codex subscription client does not support provider %q", p.ProviderID)
	}
	select {
	case <-c.server.done:
		return c.server.failure()
	default:
		return nil
	}
}

func (c *CodexLLMClient) SendPrompt(key string, p LLMProfile, prompt string) (string, error) {
	return c.SendPromptContext(context.Background(), key, p, prompt)
}

func (c *CodexLLMClient) SendPromptContext(parent context.Context, key string, p LLMProfile, prompt string) (text string, err error) {
	return c.sendPrompt(parent, key, p, prompt, "", false, false, false, false)
}

// SendSessionPromptContext sends full on a new/rotated session and continuation
// only when the same role and configuration still have a healthy live history.
// Callers must provide a self-contained full request for safe rotation.
func (c *CodexLLMClient) SendSessionPromptContext(parent context.Context, key string, p LLMProfile, full, continuation string) (string, error) {
	return c.sendPrompt(parent, key, p, full, continuation, true, false, false, false)
}

// SendDocumentPromptContext forks a clean general-instructions template for each
// full document. Only nonempty continuations reuse that document's child thread.
func (c *CodexLLMClient) SendDocumentPromptContext(parent context.Context, key string, p LLMProfile, full, continuation string) (string, error) {
	return c.sendPrompt(parent, key, p, full, continuation, true, true, false, true)
}

// SendIsolatedDocumentPromptContext starts each document without a template turn.
// Retries reuse only this document; stable rules remain thread instructions.
func (c *CodexLLMClient) SendIsolatedDocumentPromptContext(parent context.Context, key string, p LLMProfile, full, continuation string) (string, error) {
	return c.sendPrompt(parent, key, p, full, continuation, true, false, false, true)
}

func (c *CodexLLMClient) unloadTemplate(s *codexSession) {
	if s.baseID != "" {
		id := s.baseID
		c.unloadSession(&codexSession{id: id})
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, _ = c.server.call(ctx, "thread/archive", map[string]any{"threadId": id})
		cancel()
		s.baseID = ""
	}
}

func (c *CodexLLMClient) unloadSession(s *codexSession) {
	if s.id == "" {
		return
	}
	c.server.mu.Lock()
	delete(c.server.threads, s.id)
	c.server.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = c.server.call(ctx, "thread/unsubscribe", map[string]any{"threadId": s.id})
	s.id = ""
	s.turns = 0
	s.contextTokens = 0
	s.events = nil
}

func (c *CodexLLMClient) CloseSessions() {
	c.sessionsMu.Lock()
	defer c.sessionsMu.Unlock()
	for _, s := range c.sessions {
		s.mu.Lock()
		c.unloadSession(s)
		c.unloadTemplate(s)
		s.mu.Unlock()
	}
}

func (c *CodexLLMClient) sendPrompt(parent context.Context, key string, p LLMProfile, prompt, continuation string, reuse, forkDocuments, persistTemplate, freshDocuments bool) (text string, err error) {
	var session *codexSession
	if reuse {
		c.sessionsMu.Lock()
		session = c.sessions[key]
		if session == nil {
			session = &codexSession{}
			c.sessions[key] = session
		}
		c.sessionsMu.Unlock()
		session.mu.Lock()
		defer session.mu.Unlock()
		signatureBytes, e := json.Marshal([]any{p.ProviderID, p.ModelID, p.Warmup, p.ReasoningEffort, forkDocuments, freshDocuments})
		if e != nil {
			return "", e
		}
		signature := fmt.Sprintf("%x", sha256.Sum256(signatureBytes))
		nextBytes := len(prompt)
		if continuation != "" {
			nextBytes = len(continuation)
		}
		// UTF-8 bytes are a conservative upper estimate for new text tokens.
		// Rotate before appending another large document, not only after it.
		if session.signature != signature {
			c.unloadTemplate(session)
		}
		if freshDocuments && continuation == "" {
			c.unloadSession(session)
		}
		if p.ResetHistory || session.signature != signature || session.turns >= 8 || session.contextTokens+nextBytes >= 180000 {
			c.unloadSession(session)
		}
		session.signature = signature
		if session.id != "" && continuation != "" {
			prompt = continuation
		}
		defer func() {
			if err != nil {
				if persistTemplate {
					// A failed initializer must not leave an owned persistent template active.
					c.unloadTemplate(&codexSession{baseID: session.id})
				}
				c.unloadSession(session)
			} else {
				session.turns++
			}
		}()
	}
	started := time.Now()
	var firstAssistantDelta *float64
	defer func() {
		if c.metrics != nil {
			c.metrics.Add(float64(time.Since(started).Milliseconds()), err == nil)
		}
		if c.trace != nil {
			timing, _ := json.Marshal(map[string]any{"elapsed_seconds": time.Since(started).Seconds(), "first_assistant_delta_seconds": firstAssistantDelta, "success": err == nil})
			_ = c.trace.Write(LLMTraceEvent{Kind: "codex_timing", SessionKey: key, ModelID: p.ModelID, ResponseRaw: string(timing)})
			e := LLMTraceEvent{Kind: "codex_prompt", SessionKey: key, ModelID: p.ModelID, Prompt: prompt, Response: text}
			if err != nil {
				e.Error = err.Error()
			}
			_ = c.trace.Write(e)
		}
	}()
	if err = c.EnsureContext(key, p); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(parent, c.timeout)
	defer cancel()
	params := map[string]any{"cwd": c.server.workDir, "ephemeral": !persistTemplate, "modelProvider": "openai", "approvalPolicy": "never", "sandbox": "read-only", "baseInstructions": "You are a localization worker. Return only the requested result. Do not use tools, access files, or execute commands.", "developerInstructions": strings.TrimSuffix(strings.TrimSpace(p.Warmup), "Reply with: OK")}
	if p.ModelID != "" {
		params["model"] = strings.TrimPrefix(p.ModelID, "openai/")
	}
	var raw json.RawMessage
	id := ""
	var events chan rpcMessage
	if session != nil {
		id = session.id
		events = session.events
	}
	reused := id != ""
	if id == "" {
		if forkDocuments {
			if session.baseID == "" {
				// The installed server forks persisted rollouts, so initialize a
				// document-free template once. Child documents remain ephemeral.
				initializer := NewCodexLLMClient(c.server, int(c.timeout.Seconds()), nil, c.trace)
				templateProfile := p
				templateProfile.ResponseFormat = nil
				templateProfile.ResetHistory = false
				_, err = initializer.sendPrompt(ctx, "general-template", templateProfile,
					"Initialize these general localization instructions. No document is supplied. Reply only READY.", "", true, false, true, false)
				if err == nil {
					session.baseID = initializer.sessions["general-template"].id
				}
			}
			if err == nil {
				raw, err = c.server.call(ctx, "thread/fork", map[string]any{"threadId": session.baseID, "ephemeral": true, "excludeTurns": true})
			}
		} else {
			raw, err = c.server.call(ctx, "thread/start", params)
		}
		if err != nil {
			if ctx.Err() != nil {
				c.server.Close()
			}
			return "", err
		}
		var thread struct {
			Thread struct {
				ID string `json:"id"`
			} `json:"thread"`
		}
		if err = json.Unmarshal(raw, &thread); err != nil {
			return "", err
		}
		id = thread.Thread.ID
		if id == "" {
			return "", errors.New("codex thread/start returned no ID")
		}
		events = make(chan rpcMessage, 256)
		c.server.mu.Lock()
		c.server.threads[id] = events
		c.server.mu.Unlock()
		if session != nil {
			session.id = id
			session.events = events
		}
	}
	if session == nil {
		defer c.unloadSession(&codexSession{id: id, events: events})
	}
	if c.trace != nil {
		metadata, _ := json.Marshal(map[string]any{"thread_id": id, "reused": reused, "prompt_bytes": len(prompt), "warmup_bytes": len(p.Warmup), "template_thread_id": func() string {
			if forkDocuments {
				return session.baseID
			}
			return ""
		}(), "document_fork": forkDocuments})
		_ = c.trace.Write(LLMTraceEvent{Kind: "codex_session", SessionKey: key, ModelID: p.ModelID, ResponseRaw: string(metadata)})
	}
	turnParams := map[string]any{"threadId": id, "input": []any{map[string]any{"type": "text", "text": prompt, "text_elements": []any{}}}}
	if p.ReasoningEffort != "" {
		turnParams["effort"] = p.ReasoningEffort
	}
	if p.ResponseFormat != nil {
		turnParams["outputSchema"] = p.ResponseFormat
	}
	raw, err = c.server.call(ctx, "turn/start", turnParams)
	if err != nil {
		if ctx.Err() != nil {
			c.server.Close()
		}
		return "", err
	}
	var startedTurn struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err = json.Unmarshal(raw, &startedTurn); err != nil {
		return "", err
	}
	turnID := startedTurn.Turn.ID
	if turnID == "" {
		c.server.Close()
		return "", errors.New("codex turn/start returned no ID")
	}
	var lastMessage, finalMessage string
	recordUsage := func(event rpcMessage) {
		if event.Method != "thread/tokenUsage/updated" {
			return
		}
		if session != nil {
			var usage struct {
				TokenUsage struct {
					Last struct {
						InputTokens int `json:"totalTokens"`
					} `json:"last"`
				} `json:"tokenUsage"`
			}
			if json.Unmarshal(event.Params, &usage) == nil {
				session.contextTokens = usage.TokenUsage.Last.InputTokens
			}
		}
		if c.trace != nil {
			_ = c.trace.Write(LLMTraceEvent{Kind: "codex_usage", SessionKey: key, ModelID: p.ModelID, ResponseRaw: string(event.Params)})
		}
	}
	for {
		select {
		case <-ctx.Done():
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
			_, stopErr := c.server.call(stopCtx, "turn/interrupt", map[string]any{"threadId": id, "turnId": turnID})
			if stopErr == nil {
				stopErr = waitCodexStopped(stopCtx, events, turnID, c.server.done, recordUsage)
			}
			stopCancel()
			if stopErr != nil {
				c.server.Close()
			}
			return "", ctx.Err()
		case <-c.server.done:
			return "", c.server.failure()
		case event := <-events:
			var e struct {
				TurnID string `json:"turnId"`
				Item   struct {
					Type  string `json:"type"`
					Text  string `json:"text"`
					Phase string `json:"phase"`
				} `json:"item"`
				Turn struct {
					ID     string          `json:"id"`
					Status string          `json:"status"`
					Error  json.RawMessage `json:"error"`
				} `json:"turn"`
			}
			if err = json.Unmarshal(event.Params, &e); err != nil {
				return "", err
			}
			switch event.Method {
			case "item/agentMessage/delta":
				if e.TurnID == turnID && firstAssistantDelta == nil {
					seconds := time.Since(started).Seconds()
					firstAssistantDelta = &seconds
				}
			case "item/completed":
				if e.TurnID == turnID && e.Item.Type == "agentMessage" {
					if e.Item.Phase == "" {
						lastMessage = e.Item.Text
					}
					if e.Item.Phase == "final_answer" {
						finalMessage = e.Item.Text
					}
				}
			case "thread/tokenUsage/updated":
				recordUsage(event)
			case "turn/completed":
				if e.Turn.ID != turnID {
					continue
				}
				if e.Turn.Status != "completed" {
					return "", fmt.Errorf("codex turn %s: %s", e.Turn.Status, string(e.Turn.Error))
				}
				if finalMessage != "" {
					return finalMessage, nil
				}
				if lastMessage != "" {
					return lastMessage, nil
				}
				return "", errors.New("codex completed without an assistant response")
			}
		}
	}
}

func waitCodexStopped(ctx context.Context, events <-chan rpcMessage, turnID string, done <-chan struct{}, observers ...func(rpcMessage)) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			return errors.New("codex exited during cancellation")
		case event := <-events:
			for _, observe := range observers {
				observe(event)
			}
			if event.Method != "turn/completed" {
				continue
			}
			var e struct {
				Turn struct {
					ID string `json:"id"`
				} `json:"turn"`
			}
			if json.Unmarshal(event.Params, &e) == nil && e.Turn.ID == turnID {
				return nil
			}
		}
	}
}
