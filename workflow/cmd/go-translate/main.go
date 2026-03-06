package main

import (
	"flag"
	"fmt"
	"os"

	"localize-agent/workflow/internal/shared"
	"localize-agent/workflow/internal/translation"
)

func main() {
	def := translation.DefaultConfig()
	cfg := def
	var project string
	var projectDir string
	fs := flag.NewFlagSet("go-translate", flag.ContinueOnError)
	fs.StringVar(&project, "project", "", "project name under workflow/projects/<name>")
	fs.StringVar(&projectDir, "project-dir", "", "project directory containing project.json")
	fs.StringVar(&cfg.Source, "source", cfg.Source, "")
	fs.StringVar(&cfg.Current, "current", cfg.Current, "")
	fs.StringVar(&cfg.IDsFile, "ids-file", "", "")
	fs.StringVar(&cfg.LLMBackend, "llm-backend", cfg.LLMBackend, "llm backend: opencode or ollama")
	fs.StringVar(&cfg.ServerURL, "server-url", cfg.ServerURL, "")
	fs.StringVar(&cfg.Model, "model", cfg.Model, "model (opencode: provider/model, ollama: model)")
	fs.StringVar(&cfg.Agent, "agent", cfg.Agent, "")
	fs.IntVar(&cfg.Concurrency, "concurrency", cfg.Concurrency, "")
	fs.IntVar(&cfg.BatchSize, "batch-size", cfg.BatchSize, "")
	fs.IntVar(&cfg.MaxBatchChars, "max-batch-chars", cfg.MaxBatchChars, "max total EN chars per batch (0=disabled)")
	fs.IntVar(&cfg.TimeoutSec, "timeout-sec", cfg.TimeoutSec, "")
	fs.IntVar(&cfg.MaxAttempts, "max-attempts", cfg.MaxAttempts, "")
	fs.Float64Var(&cfg.BackoffSec, "backoff-sec", cfg.BackoffSec, "")
	fs.IntVar(&cfg.MaxPlainLen, "max-plain-len", cfg.MaxPlainLen, "")
	fs.BoolVar(&cfg.SkipInvalid, "skip-invalid", cfg.SkipInvalid, "")
	fs.BoolVar(&cfg.SkipTimeout, "skip-timeout", cfg.SkipTimeout, "")
	fs.IntVar(&cfg.PlaceholderRecoveryAttempts, "placeholder-recovery-attempts", cfg.PlaceholderRecoveryAttempts, "")
	fs.Var(&cfg.ContextFiles, "context-file", "context file(s) to load; prefer single workflow/context/agent_context.md")
	fs.StringVar(&cfg.RulesFile, "rules-file", "", "optional: external static rules file (default: built-in rules)")
	fs.StringVar(&cfg.CheckpointDB, "checkpoint-db", cfg.CheckpointDB, "")
	fs.StringVar(&cfg.TraceOut, "trace-out", "", "optional JSONL trace output path for prompt/response tuning")
	fs.StringVar(&cfg.ReviewExportOut, "review-export-out", "", "export translated results from checkpoint DB as JSONL")
	fs.StringVar(&cfg.ReviewStatuses, "review-statuses", cfg.ReviewStatuses, "statuses to include in review export (comma-separated)")
	fs.BoolVar(&cfg.Resume, "resume", false, "")
	fs.BoolVar(&cfg.OllamaStructuredOutput, "ollama-structured-output", cfg.OllamaStructuredOutput, "use Ollama JSON schema responses for translation prompts")
	fs.BoolVar(&cfg.OllamaResetHistory, "ollama-reset-history", cfg.OllamaResetHistory, "reset Ollama chat history after each prompt while keeping warmup rules")
	fs.StringVar(&cfg.OllamaKeepAlive, "ollama-keep-alive", cfg.OllamaKeepAlive, "optional Ollama keep_alive value, e.g. 12h")
	fs.IntVar(&cfg.OllamaNumCtx, "ollama-num-ctx", cfg.OllamaNumCtx, "optional Ollama num_ctx override")
	fs.Float64Var(&cfg.OllamaTemperature, "ollama-temperature", cfg.OllamaTemperature, "optional Ollama temperature override (-1 keeps model default)")

	if err := fs.Parse(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if project != "" || projectDir != "" {
		pc, baseDir, err := shared.LoadProjectConfig(project, projectDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading project config: %v\n", err)
			os.Exit(2)
		}
		if pc != nil {
			t := pc.Translation
			if cfg.Source == def.Source && t.Source != "" {
				cfg.Source = t.Source
			}
			if cfg.Current == def.Current && t.Current != "" {
				cfg.Current = t.Current
			}
			if cfg.IDsFile == "" && t.IDsFile != "" {
				cfg.IDsFile = t.IDsFile
			}
			if cfg.CheckpointDB == def.CheckpointDB && t.CheckpointDB != "" {
				cfg.CheckpointDB = t.CheckpointDB
			}
			if cfg.RulesFile == "" && t.RulesFile != "" {
				cfg.RulesFile = t.RulesFile
			}
			if len(cfg.ContextFiles) == 0 && len(t.ContextFiles) > 0 {
				cfg.ContextFiles = append(cfg.ContextFiles, t.ContextFiles...)
			}
			if cfg.ServerURL == def.ServerURL && t.ServerURL != "" {
				cfg.ServerURL = t.ServerURL
			}
			if cfg.Model == def.Model && t.Model != "" {
				cfg.Model = t.Model
			}
			if cfg.LLMBackend == def.LLMBackend && t.LLMBackend != "" {
				cfg.LLMBackend = t.LLMBackend
			}
			if !cfg.OllamaStructuredOutput && t.OllamaStructuredOutput {
				cfg.OllamaStructuredOutput = true
			}
			if !cfg.OllamaResetHistory && t.OllamaResetHistory {
				cfg.OllamaResetHistory = true
			}
			if cfg.OllamaKeepAlive == def.OllamaKeepAlive && t.OllamaKeepAlive != "" {
				cfg.OllamaKeepAlive = t.OllamaKeepAlive
			}
			if cfg.OllamaNumCtx == def.OllamaNumCtx && t.OllamaNumCtx > 0 {
				cfg.OllamaNumCtx = t.OllamaNumCtx
			}
			if cfg.OllamaTemperature == def.OllamaTemperature && t.OllamaTemperature >= 0 {
				cfg.OllamaTemperature = t.OllamaTemperature
			}
			fmt.Printf("Project loaded: %s\n", baseDir)
		}
	}
	if cfg.IDsFile == "" {
		fmt.Fprintln(os.Stderr, "error: --ids-file is required")
		os.Exit(2)
	}

	os.Exit(translation.Run(cfg))
}
