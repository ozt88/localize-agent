package translation

import (
	"fmt"
	"strings"

	"localize-agent/workflow/internal/contracts"
)

func readStrings(files contracts.FileStore, path string) (map[string]map[string]any, error) {
	var root map[string]any
	if err := files.ReadJSON(path, &root); err != nil {
		return nil, err
	}
	s, ok := root["strings"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("no 'strings' key in %s", path)
	}
	out := map[string]map[string]any{}
	for k, v := range s {
		if m, ok := v.(map[string]any); ok {
			out[k] = m
		}
	}
	return out, nil
}

func readIDs(files contracts.FileStore, path string) ([]string, error) {
	lines, err := files.ReadLines(path)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		ids = append(ids, t)
	}
	return ids, nil
}
