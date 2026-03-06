package platform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"localize-agent/workflow/internal/shared"
)

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OllamaLLMClient struct {
	serverURL string
	http      *http.Client
	metrics   *shared.MetricCollector
	traceSink LLMTraceSink

	mu        sync.Mutex
	histories map[string][]ollamaMessage
}

func NewOllamaLLMClient(serverURL string, timeoutSec int, metrics *shared.MetricCollector, traceSink LLMTraceSink) *OllamaLLMClient {
	return &OllamaLLMClient{
		serverURL: strings.TrimRight(serverURL, "/"),
		http:      newHTTPClient(timeoutSec),
		metrics:   metrics,
		traceSink: traceSink,
		histories: map[string][]ollamaMessage{},
	}
}

func (c *OllamaLLMClient) EnsureContext(key string, profile LLMProfile) error {
	if strings.TrimSpace(profile.Warmup) == "" {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	history := c.histories[key]
	if len(history) > 0 {
		return nil
	}
	c.histories[key] = []ollamaMessage{{Role: "system", Content: profile.Warmup}}
	c.writeTrace(LLMTraceEvent{
		Kind:       "warmup",
		SessionKey: key,
		ProviderID: profile.ProviderID,
		ModelID:    profile.ModelID,
		Agent:      profile.Agent,
		Prompt:     profile.Warmup,
		Response:   "(ollama system warmup applied)",
	})
	return nil
}

func (c *OllamaLLMClient) SendPrompt(key string, profile LLMProfile, prompt string) (string, error) {
	if err := c.EnsureContext(key, profile); err != nil {
		return "", err
	}

	c.mu.Lock()
	history := append([]ollamaMessage(nil), c.histories[key]...)
	if profile.ResetHistory {
		history = history[:0]
		if strings.TrimSpace(profile.Warmup) != "" {
			history = append(history, ollamaMessage{Role: "system", Content: profile.Warmup})
		}
	}
	history = append(history, ollamaMessage{Role: "user", Content: prompt})
	c.mu.Unlock()

	body := map[string]any{
		"model":    profile.ModelID,
		"messages": history,
		"stream":   false,
	}
	if strings.TrimSpace(profile.KeepAlive) != "" {
		body["keep_alive"] = profile.KeepAlive
	}
	if profile.ResponseFormat != nil {
		body["format"] = profile.ResponseFormat
	}
	if len(profile.Options) > 0 {
		body["options"] = profile.Options
	}
	var out struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error"`
	}
	if err := c.postJSON("/api/chat", body, &out); err != nil {
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
	if strings.TrimSpace(out.Error) != "" {
		err := fmt.Errorf("ollama error: %s", out.Error)
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
	respText := strings.TrimSpace(out.Message.Content)
	if respText == "" {
		err := fmt.Errorf("no text in response")
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

	if !profile.ResetHistory {
		c.mu.Lock()
		next := append([]ollamaMessage(nil), history...)
		next = append(next, ollamaMessage{Role: "assistant", Content: respText})
		c.histories[key] = next
		c.mu.Unlock()
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

func (c *OllamaLLMClient) postJSON(path string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, c.serverURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	started := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		c.metrics.Add(float64(time.Since(started).Milliseconds()), false)
		return err
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.metrics.Add(float64(time.Since(started).Milliseconds()), false)
		return fmt.Errorf("http %d: %s", resp.StatusCode, string(payload))
	}
	c.metrics.Add(float64(time.Since(started).Milliseconds()), true)
	return json.Unmarshal(payload, out)
}

func (c *OllamaLLMClient) writeTrace(event LLMTraceEvent) {
	if c.traceSink == nil {
		return
	}
	_ = c.traceSink.Write(event)
}
