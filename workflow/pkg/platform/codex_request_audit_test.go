package platform

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Explicitly opt-in: identical real job, alternating environment settings,
// persisted traces, no operational pipeline state.
func TestCodexLiveRequestAudit(t *testing.T) {
	jobPath := os.Getenv("LOCALIZE_CODEX_AUDIT_JOB")
	if jobPath == "" {
		t.Skip("opt-in inference audit")
	}
	var job struct {
		Warmup string
		Prompt string
		Schema any
	}
	b, err := os.ReadFile(jobPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &job); err != nil {
		t.Fatal(err)
	}
	out := os.Getenv("LOCALIZE_CODEX_AUDIT_OUT")
	if err = os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	p := LLMProfile{ModelID: "gpt-6-sol", ReasoningEffort: "medium", Warmup: job.Warmup, ResponseFormat: job.Schema}
	var rows []map[string]any
	if os.Getenv("LOCALIZE_CODEX_AUDIT_FORK") == "1" {
		s, e := StartCodexServerWithOptions(ctx, os.Getenv("LOCALIZE_CODEX_BIN"), CodexServerOptions{MinimalContext: true})
		if e != nil {
			t.Fatal(e)
		}
		defer s.Close()
		trace, e := NewJSONLTraceSink(filepath.Join(out, "fork.jsonl"))
		if e != nil {
			t.Fatal(e)
		}
		defer trace.Close()
		c := NewCodexLLMClient(s, 180, nil, trace)
		defer c.CloseSessions()
		for i := 0; i < 2; i++ {
			start := time.Now()
			_, e = c.SendDocumentPromptContext(ctx, "audit", p, job.Prompt, "")
			if e != nil {
				t.Fatal(e)
			}
			t.Logf("minimal fork %d: %.3fs", i, time.Since(start).Seconds())
		}
		return
	}
	for i, minimal := range []bool{false, true, true, false} {
		s, e := StartCodexServerWithOptions(ctx, os.Getenv("LOCALIZE_CODEX_BIN"), CodexServerOptions{MinimalContext: minimal})
		if e != nil {
			t.Fatal(e)
		}
		name := []string{"original-1", "minimal-1", "minimal-2", "original-2"}[i]
		trace, e := NewJSONLTraceSink(filepath.Join(out, name+".jsonl"))
		if e != nil {
			t.Fatal(e)
		}
		c := NewCodexLLMClient(s, 180, nil, trace)
		start := time.Now()
		response, e := c.SendPromptContext(ctx, "audit", p, job.Prompt)
		elapsed := time.Since(start).Seconds()
		c.CloseSessions()
		s.Close()
		trace.Close()
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(out, name+"-response.json"), []byte(response), 0600); e != nil {
			t.Fatal(e)
		}
		rows = append(rows, map[string]any{"name": name, "minimal": minimal, "seconds": elapsed})
		t.Logf("%s %.3fs", name, elapsed)
	}
	b, _ = json.MarshalIndent(rows, "", "  ")
	if err = os.WriteFile(filepath.Join(out, "timings.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
}
