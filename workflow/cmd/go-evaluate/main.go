package main

import (
	"flag"
	"fmt"
	"os"

	"localize-agent/workflow/internal/evaluation"
	"localize-agent/workflow/internal/shared"
)

func main() {
	def := evaluation.DefaultConfig()
	cfg := def
	var project string
	var projectDir string
	fs := flag.NewFlagSet("go-evaluate", flag.ContinueOnError)
	fs.StringVar(&project, "project", "", "project name under projects/<name>")
	fs.StringVar(&projectDir, "project-dir", "", "project directory containing project.json")

	fs.BoolVar(&cfg.Resume, "resume", false, "resume: skip already-completed items, recover 'evaluating' to pending")
	fs.BoolVar(&cfg.StatusOnly, "status", false, "print DB status summary and exit")
	fs.BoolVar(&cfg.Export, "export", false, "export DB contents to JSON files and exit")
	fs.StringVar(&cfg.ResetStatus, "reset-status", "", "comma-separated statuses to reset to pending (e.g. 'revise,reject')")
	fs.StringVar(&cfg.ReevalIDs, "reeval-ids", "", "comma-separated item IDs to reset to pending and re-evaluate")

	fs.StringVar(&cfg.PackIn, "pack-in", "", "pack.json from translation run (required for first run)")
	fs.StringVar(&cfg.DB, "db", cfg.DB, "SQLite DB path for status tracking and resume")
	fs.StringVar(&cfg.RunName, "run-name", cfg.RunName, "logical run name inside unified DB")

	fs.StringVar(&cfg.LLMBackend, "llm-backend", cfg.LLMBackend, "llm backend: opencode or ollama")
	fs.StringVar(&cfg.ServerURL, "server-url", cfg.ServerURL, "")
	fs.StringVar(&cfg.TransModel, "trans-model", cfg.TransModel, "model for revision (opencode: provider/model, ollama: model)")
	fs.StringVar(&cfg.EvalModel, "eval-model", cfg.EvalModel, "model for evaluation (opencode: provider/model, ollama: model)")
	fs.StringVar(&cfg.TransAgent, "trans-agent", cfg.TransAgent, "")
	fs.StringVar(&cfg.EvalAgent, "eval-agent", cfg.EvalAgent, "")

	fs.IntVar(&cfg.Concurrency, "concurrency", cfg.Concurrency, "")
	fs.IntVar(&cfg.TimeoutSec, "timeout-sec", cfg.TimeoutSec, "")
	fs.IntVar(&cfg.MaxAttempts, "max-attempts", cfg.MaxAttempts, "")
	fs.Float64Var(&cfg.BackoffSec, "backoff-sec", cfg.BackoffSec, "")
	fs.IntVar(&cfg.MaxRetry, "max-retry", cfg.MaxRetry, "max revise/retranslate cycles per item")

	fs.Var(&cfg.ContextFiles, "context-file", "context file(s); prefer workflow/context/agent_context.md")
	fs.StringVar(&cfg.RulesFile, "rules-file", "", "external translate rules (default: built-in)")
	fs.StringVar(&cfg.EvalRulesFile, "eval-rules-file", "", "external eval rules (default: built-in)")
	fs.StringVar(&cfg.TraceOut, "trace-out", "", "optional JSONL trace output path for prompt/response tuning")

	fs.StringVar(&cfg.ReportOut, "report-out", cfg.ReportOut, "full eval report")
	fs.StringVar(&cfg.RejectedOut, "rejected-out", cfg.RejectedOut, "reject items for human review")
	fs.StringVar(&cfg.RevisedOut, "revised-out", cfg.RevisedOut, "revised items (pass+revised)")
	fs.StringVar(&cfg.ReviewExportOut, "review-export-out", "", "export compact review JSONL for manual validation")
	fs.StringVar(&cfg.ReviewStatuses, "review-statuses", cfg.ReviewStatuses, "statuses to include in review export (comma-separated)")

	if err := fs.Parse(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	explicit := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		explicit[f.Name] = true
	})
	if project != "" || projectDir != "" {
		pc, baseDir, err := shared.LoadProjectConfig(project, projectDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading project config: %v\n", err)
			os.Exit(2)
		}
		if pc != nil {
			e := pc.Evaluation
			if !explicit["pack-in"] && cfg.PackIn == "" && e.PackIn != "" {
				cfg.PackIn = e.PackIn
			}
			if !explicit["db"] && cfg.DB == def.DB && e.DB != "" {
				cfg.DB = e.DB
			}
			if !explicit["run-name"] && cfg.RunName == def.RunName && e.RunName != "" {
				cfg.RunName = e.RunName
			}
			if !explicit["context-file"] && len(cfg.ContextFiles) == 0 && len(e.ContextFiles) > 0 {
				cfg.ContextFiles = append(cfg.ContextFiles, e.ContextFiles...)
			}
			if !explicit["rules-file"] && cfg.RulesFile == "" && e.RulesFile != "" {
				cfg.RulesFile = e.RulesFile
			}
			if !explicit["eval-rules-file"] && cfg.EvalRulesFile == "" && e.EvalRulesFile != "" {
				cfg.EvalRulesFile = e.EvalRulesFile
			}
			if !explicit["server-url"] && cfg.ServerURL == def.ServerURL && e.ServerURL != "" {
				cfg.ServerURL = e.ServerURL
			}
			if !explicit["trans-model"] && cfg.TransModel == def.TransModel && e.TransModel != "" {
				cfg.TransModel = e.TransModel
			}
			if !explicit["eval-model"] && cfg.EvalModel == def.EvalModel && e.EvalModel != "" {
				cfg.EvalModel = e.EvalModel
			}
			if !explicit["llm-backend"] && cfg.LLMBackend == def.LLMBackend && e.LLMBackend != "" {
				cfg.LLMBackend = e.LLMBackend
			}
			fmt.Printf("Project loaded: %s\n", baseDir)
		}
	}

	os.Exit(evaluation.Run(cfg))
}
