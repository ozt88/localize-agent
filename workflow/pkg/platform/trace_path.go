package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var tracePathUnsafeRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func BuildLLMTracePath(traceDir string, fallbackDir string, role string, workerID string) string {
	baseDir := strings.TrimSpace(traceDir)
	if baseDir == "" {
		baseDir = strings.TrimSpace(fallbackDir)
	}
	if baseDir == "" {
		return ""
	}
	rolePart := sanitizeTracePathPart(role)
	if rolePart == "" {
		rolePart = "llm"
	}
	workerPart := sanitizeTracePathPart(workerID)
	if workerPart == "" {
		workerPart = "worker"
	}
	fileName := fmt.Sprintf("%s__%s__%d.jsonl", rolePart, workerPart, os.Getpid())
	return filepath.Join(baseDir, fileName)
}

func sanitizeTracePathPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = tracePathUnsafeRe.ReplaceAllString(value, "_")
	value = strings.Trim(value, "._-")
	return value
}
