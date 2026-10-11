package platform

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestCodexFirstDeltaTimingDoesNotOverflowBeforeTurnAck(t *testing.T) {
	s, _ := mockCodex(t, "stream")
	path := t.TempDir() + "/trace.jsonl"
	trace, err := NewJSONLTraceSink(path)
	if err != nil {
		t.Fatal(err)
	}
	c := NewCodexLLMClient(s, 2, nil, trace)
	if _, err = c.SendPrompt("stream", LLMProfile{ModelID: "test"}, "hello"); err != nil {
		t.Fatal(err)
	}
	trace.Close()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var event LLMTraceEvent
		if err = json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Kind == "codex_timing" {
			var timing struct {
				Elapsed float64  `json:"elapsed_seconds"`
				First   *float64 `json:"first_assistant_delta_seconds"`
			}
			if err = json.Unmarshal([]byte(event.ResponseRaw), &timing); err != nil {
				t.Fatal(err)
			}
			if timing.First == nil || *timing.First < 0 || *timing.First > timing.Elapsed {
				t.Fatal("invalid first delta timing")
			}
			found = true
		}
	}
	if !found {
		t.Fatal("missing timing")
	}
}
