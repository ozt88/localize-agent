package platform

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"

	"localize-agent/workflow/internal/contracts"
	"localize-agent/workflow/internal/shared"
)

type osFileStore struct{}

func NewOSFileStore() contracts.FileStore {
	return &osFileStore{}
}

func (osFileStore) ReadJSON(path string, out any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func (osFileStore) WriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return shared.AtomicWriteFile(path, b, 0o644)
}

func (osFileStore) ReadLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := []string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out, sc.Err()
}

func (osFileStore) WriteLines(path string, lines []string) error {
	var b strings.Builder
	for _, ln := range lines {
		b.WriteString(ln)
		b.WriteByte('\n')
	}
	return shared.AtomicWriteFile(path, []byte(b.String()), 0o644)
}
