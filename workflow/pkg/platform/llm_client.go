package platform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"localize-agent/workflow/pkg/shared"
)

type LLMProfile struct {
	ProviderID     string
	ModelID        string
	Agent          string
	Warmup         string
	KeepAlive      string
	ResponseFormat any
	Options        map[string]any
	ResetHistory   bool
}

type SessionLLMClient struct {
	serverURL string
	directory string
	http      *http.Client
	metrics   *shared.MetricCollector
	traceSink LLMTraceSink

	mu           sync.Mutex
	sessionIDs   map[string]string
	contextReady map[string]bool
}

func NewSessionLLMClient(serverURL string, timeoutSec int, metrics *shared.MetricCollector, traceSink LLMTraceSink) *SessionLLMClient {
	// NOTE: directory left empty — passing cwd causes OpenCode to scan the entire
	// project tree on every session/message (skills, file indexing), adding minutes
	// of latency per request. Translation pipeline does not need project context.
	return &SessionLLMClient{
		serverURL:    strings.TrimRight(serverURL, "/"),
		directory:    "",
		http:         newHTTPClient(timeoutSec),
		metrics:      metrics,
		traceSink:    traceSink,
		sessionIDs:   map[string]string{},
		contextReady: map[string]bool{},
	}
}

func ParseModel(model string) (string, string, error) {
	parts := strings.SplitN(model, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid model format: %s (expected provider/model)", model)
	}
	return parts[0], parts[1], nil
}

func (c *SessionLLMClient) EnsureContext(key string, profile LLMProfile) error {
	c.mu.Lock()
	ready := c.contextReady[key]
	c.mu.Unlock()
	if ready {
		return nil
	}

	if strings.TrimSpace(profile.Warmup) == "" {
		c.mu.Lock()
		c.contextReady[key] = true
		c.mu.Unlock()
		return nil
	}

	sid, err := c.getSessionID(key)
	if err != nil {
		return err
	}
	body := map[string]any{
		"model": map[string]any{"providerID": profile.ProviderID, "modelID": profile.ModelID},
		"parts": []map[string]string{{"type": "text", "text": profile.Warmup}},
	}
	// NOTE: "agent" field omitted — OpenCode 1.2.26 returns empty response when agent is set.
	var out map[string]any
	if err := c.postJSON("/session/"+sid+"/message", body, &out); err != nil {
		c.writeTrace(LLMTraceEvent{
			Kind:       "warmup_error",
			SessionKey: key,
			ProviderID: profile.ProviderID,
			ModelID:    profile.ModelID,
			Agent:      profile.Agent,
			Prompt:     profile.Warmup,
			Error:      err.Error(),
		})
		return err
	}
	if respText, err := joinTextParts(out); err == nil {
		c.writeTrace(LLMTraceEvent{
			Kind:       "warmup",
			SessionKey: key,
			ProviderID: profile.ProviderID,
			ModelID:    profile.ModelID,
			Agent:      profile.Agent,
			Prompt:     profile.Warmup,
			Response:   respText,
		})
	}
	c.mu.Lock()
	c.contextReady[key] = true
	c.mu.Unlock()
	return nil
}

// ResetAllSessions clears all cached session IDs and warmup state.
// Called after OpenCode restart to force new sessions.
func (c *SessionLLMClient) ResetAllSessions() {
	c.mu.Lock()
	c.sessionIDs = map[string]string{}
	c.contextReady = map[string]bool{}
	c.mu.Unlock()
}

func (c *SessionLLMClient) SendPrompt(key string, profile LLMProfile, prompt string) (string, error) {
	if profile.ResetHistory {
		c.mu.Lock()
		delete(c.sessionIDs, key)
		delete(c.contextReady, key)
		c.mu.Unlock()
	}
	if err := c.EnsureContext(key, profile); err != nil {
		return "", err
	}
	sid, err := c.getSessionID(key)
	if err != nil {
		return "", err
	}
	body := map[string]any{
		"model": map[string]any{"providerID": profile.ProviderID, "modelID": profile.ModelID},
		"parts": []map[string]string{{"type": "text", "text": prompt}},
	}
	// NOTE: "agent" field omitted — OpenCode 1.2.26 returns empty response when agent is set.
	var out map[string]any
	if err := c.postJSON("/session/"+sid+"/message", body, &out); err != nil {
		c.writeTrace(LLMTraceEvent{
			Kind:       "prompt_error",
			SessionKey: key,
			ProviderID: profile.ProviderID,
			ModelID:    profile.ModelID,
			Agent:      profile.Agent,
			Prompt:     prompt,
			Error:      err.Error(),
		})
		return "", err
	}
	respText, err := joinTextParts(out)
	if err != nil {
		c.writeTrace(LLMTraceEvent{
			Kind:       "prompt_error",
			SessionKey: key,
			ProviderID: profile.ProviderID,
			ModelID:    profile.ModelID,
			Agent:      profile.Agent,
			Prompt:     prompt,
			Error:      err.Error(),
		})
		return "", err
	}
	c.writeTrace(LLMTraceEvent{
		Kind:       "prompt",
		SessionKey: key,
		ProviderID: profile.ProviderID,
		ModelID:    profile.ModelID,
		Agent:      profile.Agent,
		Prompt:     prompt,
		Response:   respText,
	})
	return respText, nil
}

func (c *SessionLLMClient) postJSON(path string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	var providerID, modelID, agent string
	if bodyMap, ok := body.(map[string]any); ok {
		if modelMap, ok := bodyMap["model"].(map[string]any); ok {
			providerID, _ = modelMap["providerID"].(string)
			modelID, _ = modelMap["modelID"].(string)
		}
		agent, _ = bodyMap["agent"].(string)
	}
	c.writeTrace(LLMTraceEvent{
		Kind:       "request",
		Path:       path,
		ProviderID: providerID,
		ModelID:    modelID,
		Agent:      agent,
		Request:    string(raw),
	})
	req, err := http.NewRequest(http.MethodPost, c.endpointURL(path), bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	started := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		c.metrics.Add(float64(time.Since(started).Milliseconds()), false)
		c.writeTrace(LLMTraceEvent{
			Kind:       "request_error",
			Path:       path,
			ProviderID: providerID,
			ModelID:    modelID,
			Agent:      agent,
			Request:    string(raw),
			Error:      err.Error(),
		})
		return err
	}
	defer resp.Body.Close()
	payload, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		c.metrics.Add(float64(time.Since(started).Milliseconds()), false)
		c.writeTrace(LLMTraceEvent{
			Kind:        "response_read_error",
			Path:        path,
			ProviderID:  providerID,
			ModelID:     modelID,
			Agent:       agent,
			Request:     string(raw),
			ResponseRaw: string(payload),
			ResponseStatus: resp.StatusCode,
			ResponseBytes:  len(payload),
			Error:       readErr.Error(),
		})
		return fmt.Errorf("read response body: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.metrics.Add(float64(time.Since(started).Milliseconds()), false)
		c.writeTrace(LLMTraceEvent{
			Kind:        "response_error",
			Path:        path,
			ProviderID:  providerID,
			ModelID:     modelID,
			Agent:       agent,
			Request:     string(raw),
			ResponseRaw: string(payload),
			ResponseStatus: resp.StatusCode,
			ResponseBytes:  len(payload),
			Error:       fmt.Sprintf("http %d", resp.StatusCode),
		})
		return fmt.Errorf("http %d: %s", resp.StatusCode, string(payload))
	}
	c.metrics.Add(float64(time.Since(started).Milliseconds()), true)
	if len(payload) == 0 {
		c.writeTrace(LLMTraceEvent{
			Kind:           "response_empty",
			Path:           path,
			ProviderID:     providerID,
			ModelID:        modelID,
			Agent:          agent,
			Request:        string(raw),
			ResponseRaw:    "",
			ResponseStatus: resp.StatusCode,
			ResponseBytes:  0,
			ResponseEmpty:  true,
			Error:          "empty response body",
		})
		return fmt.Errorf("empty response body")
	}
	if err := json.Unmarshal(payload, out); err != nil {
		c.writeTrace(LLMTraceEvent{
			Kind:        "response_parse_error",
			Path:        path,
			ProviderID:  providerID,
			ModelID:     modelID,
			Agent:       agent,
			Request:     string(raw),
			ResponseRaw: string(payload),
			ResponseStatus: resp.StatusCode,
			ResponseBytes:  len(payload),
			Error:       err.Error(),
		})
		return err
	}
	return nil
}

func (c *SessionLLMClient) endpointURL(path string) string {
	if strings.TrimSpace(c.directory) == "" {
		return c.serverURL + path
	}
	u, err := url.Parse(c.serverURL + path)
	if err != nil {
		return c.serverURL + path
	}
	q := u.Query()
	q.Set("directory", c.directory)
	u.RawQuery = q.Encode()
	return u.String()
}

func (c *SessionLLMClient) getSessionID(key string) (string, error) {
	c.mu.Lock()
	if id, ok := c.sessionIDs[key]; ok {
		c.mu.Unlock()
		return id, nil
	}
	c.mu.Unlock()

	var resp map[string]any
	if err := c.postJSON("/session", map[string]any{}, &resp); err != nil {
		c.writeTrace(LLMTraceEvent{
			Kind:       "session_create_error",
			SessionKey: key,
			Path:       "/session",
			Error:      err.Error(),
		})
		return "", err
	}
	id, _ := resp["id"].(string)
	if id == "" {
		c.writeTrace(LLMTraceEvent{
			Kind:       "session_create_error",
			SessionKey: key,
			Path:       "/session",
			Error:      "session id missing in response",
		})
		return "", fmt.Errorf("session id missing in response")
	}
	c.mu.Lock()
	c.sessionIDs[key] = id
	c.mu.Unlock()
	return id, nil
}

func joinTextParts(out map[string]any) (string, error) {
	parts, _ := out["parts"].([]any)
	texts := make([]string, 0, len(parts))
	for _, p := range parts {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		if pm["type"] == "text" {
			if t, ok := pm["text"].(string); ok {
				texts = append(texts, t)
			}
		}
	}
	if len(texts) == 0 {
		return "", fmt.Errorf("no text in response")
	}
	return strings.Join(texts, "\n"), nil
}

func (c *SessionLLMClient) writeTrace(event LLMTraceEvent) {
	if c.traceSink == nil {
		return
	}
	_ = c.traceSink.Write(event)
}
