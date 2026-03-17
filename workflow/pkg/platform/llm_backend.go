package platform

import (
	"fmt"
	"strings"
)

const (
	LLMBackendOpencode = "opencode"
	LLMBackendOllama   = "ollama"
)

func NormalizeLLMBackend(raw string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return LLMBackendOpencode, nil
	}
	switch v {
	case LLMBackendOpencode, LLMBackendOllama:
		return v, nil
	default:
		return "", fmt.Errorf("invalid llm backend: %s (expected opencode or ollama)", raw)
	}
}
