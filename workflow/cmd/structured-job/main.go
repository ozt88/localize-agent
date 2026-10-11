// structured-job executes one isolated structured request, with no pipeline writes.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"localize-agent/workflow/pkg/platform"
	"os"
	"path/filepath"
	"time"
)

func run() error {
	jobPath := flag.String("job", "", "Frozen model job")
	out := flag.String("out", "", "New isolated output directory")
	bin := flag.String("codex-bin", "", "Native Codex binary")
	flag.Parse()
	raw, err := os.ReadFile(*jobPath)
	if err != nil {
		return err
	}
	var job struct {
		Prompt string `json:"prompt"`
		Warmup string `json:"warmup"`
		Schema any    `json:"schema"`
	}
	if err = json.Unmarshal(raw, &job); err != nil {
		return err
	}
	if job.Prompt == "" || job.Schema == nil || *out == "" {
		return fmt.Errorf("job and output required")
	}
	if err = os.Mkdir(*out, 0700); err != nil {
		return err
	}
	trace, err := platform.NewJSONLTraceSink(filepath.Join(*out, "trace.jsonl"))
	if err != nil {
		return err
	}
	defer trace.Close()
	ctx := context.Background()
	server, err := platform.StartCodexServerWithOptions(ctx, *bin, platform.CodexServerOptions{MinimalContext: true})
	if err != nil {
		return err
	}
	defer server.Close()
	if err = server.CheckAuthentication(ctx); err != nil {
		return err
	}
	if err = server.ValidateModels(ctx, "gpt-6-sol"); err != nil {
		return err
	}
	client := platform.NewCodexLLMClient(server, 600, nil, trace)
	defer client.CloseSessions()
	started := time.Now()
	response, callErr := client.SendPromptContext(ctx, "structured-translation-job", platform.LLMProfile{
		ProviderID: "openai", ModelID: "gpt-6-sol", ReasoningEffort: "medium", Warmup: job.Warmup, ResponseFormat: job.Schema}, job.Prompt)
	result := map[string]any{"success": callErr == nil, "model": "gpt-6-sol", "effort": "medium", "seconds": time.Since(started).Seconds()}
	if callErr != nil {
		result["error"] = callErr.Error()
	}
	b, _ := json.MarshalIndent(result, "", "  ")
	if err = os.WriteFile(filepath.Join(*out, "run.json"), b, 0600); err != nil {
		return err
	}
	if callErr != nil {
		return callErr
	}
	return os.WriteFile(filepath.Join(*out, "response.json"), []byte(response), 0600)
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
