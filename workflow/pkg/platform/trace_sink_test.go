package platform

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSONLTraceSinkAutoEncodesRequestAndResponseBase64(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.jsonl")

	sink, err := NewJSONLTraceSink(path)
	if err != nil {
		t.Fatalf("NewJSONLTraceSink error: %v", err)
	}
	defer sink.Close()

	event := LLMTraceEvent{
		Kind:        "response_parse_error",
		Request:     "{\"a\":1}",
		ResponseRaw: "{\"b\":2}",
	}
	if err := sink.Write(event); err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	line := strings.TrimSpace(string(raw))
	var got LLMTraceEvent
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if got.RequestBase64 != base64.StdEncoding.EncodeToString([]byte(event.Request)) {
		t.Fatalf("RequestBase64=%q", got.RequestBase64)
	}
	if got.ResponseBase64 != base64.StdEncoding.EncodeToString([]byte(event.ResponseRaw)) {
		t.Fatalf("ResponseBase64=%q", got.ResponseBase64)
	}
}
