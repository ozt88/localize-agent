package platform

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type LLMTraceEvent struct {
	Timestamp  string `json:"timestamp"`
	Kind       string `json:"kind"`
	SessionKey string `json:"session_key"`
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"`
	Agent      string `json:"agent"`
	Prompt     string `json:"prompt,omitempty"`
	Response   string `json:"response,omitempty"`
	Error      string `json:"error,omitempty"`
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
