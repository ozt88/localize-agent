package shared

import (
	"encoding/json"
	"os"
	"strings"
)

func LoadContext(paths []string) string {
	parts := []string{}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err == nil {
			parts = append(parts, strings.TrimSpace(string(b)))
		}
	}
	return strings.Join(parts, "\n\n")
}

func LoadRules(path string) string {
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func WriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWriteFile(path, b, 0o644)
}
