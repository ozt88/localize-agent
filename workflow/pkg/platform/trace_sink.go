package platform

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type LLMTraceEvent struct {
	Timestamp      string `json:"timestamp"`
	Kind           string `json:"kind"`
	SessionKey     string `json:"session_key"`
	ProviderID     string `json:"provider_id"`
	ModelID        string `json:"model_id"`
	Agent          string `json:"agent"`
	Path           string `json:"path,omitempty"`
	Request        string `json:"request,omitempty"`
	RequestBase64  string `json:"request_base64,omitempty"`
	Prompt         string `json:"prompt,omitempty"`
	Response       string `json:"response,omitempty"`
	ResponseRaw    string `json:"response_raw,omitempty"`
	ResponseBase64 string `json:"response_base64,omitempty"`
	ResponseStatus int    `json:"response_status,omitempty"`
	ResponseBytes  int    `json:"response_bytes"`
	ResponseEmpty  bool   `json:"response_empty,omitempty"`
	Error          string `json:"error,omitempty"`
}

type LLMTraceSink interface {
	Write(event LLMTraceEvent) error
	Close() error
}

type jsonlTraceSink struct {
	mu sync.Mutex
	f  *os.File
}

func NewJSONLTraceSink(path string) (LLMTraceSink, error) {
	if path == "" {
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &jsonlTraceSink{f: f}, nil
}

func (s *jsonlTraceSink) Write(event LLMTraceEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if event.Request != "" && event.RequestBase64 == "" {
		event.RequestBase64 = base64.StdEncoding.EncodeToString([]byte(event.Request))
	}
	if event.ResponseRaw != "" && event.ResponseBase64 == "" {
		event.ResponseBase64 = base64.StdEncoding.EncodeToString([]byte(event.ResponseRaw))
	}
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := s.f.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *jsonlTraceSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return nil
	}
	return s.f.Close()
}
