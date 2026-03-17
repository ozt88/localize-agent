package main

import (
	"flag"
	"os"
	"strings"

	"localize-agent/workflow/internal/semanticreview"
)

func main() {
	cfg := semanticreview.Config{
		CheckpointDB:            "projects/esoteric-ebb/output/batches/translation_assetripper_textasset_unique/translation_checkpoint.db",
		SourcePath:              "projects/esoteric-ebb/source/prepared/source_esoteric.json",
		CurrentPath:             "projects/esoteric-ebb/source/prepared/current_esoteric.json",
		IDsFile:                 "projects/esoteric-ebb/source/prepared/ids_esoteric.txt",
		TranslatorPackageChunks: "projects/esoteric-ebb/source/translator_package_chunks.json",
		Mode:                    "backtranslate",
		ScoreOnly:               false,
		LLMBackend:              "opencode",
		ServerURL:               "http://127.0.0.1:4112",
		Model:                   "openai/gpt-5.4",
		Agent:                   "",
		ContextFiles:            []string{"projects/esoteric-ebb/context/esoteric_ebb_semantic_review_system.md"},
		Concurrency:             2,
		BatchSize:               20,
		TimeoutSec:              120,
		OutputDir:               "projects/esoteric-ebb/output/semantic_review",
	}

	fs := flag.NewFlagSet("go-semantic-review", flag.ContinueOnError)
	fs.StringVar(&cfg.CheckpointDB, "checkpoint-db", cfg.CheckpointDB, "checkpoint DB path")
	fs.StringVar(&cfg.SourcePath, "source", cfg.SourcePath, "source JSON path")
	fs.StringVar(&cfg.CurrentPath, "current", cfg.CurrentPath, "current JSON path")
	fs.StringVar(&cfg.IDsFile, "ids-file", cfg.IDsFile, "ids file path")
	fs.StringVar(&cfg.TranslatorPackageChunks, "translator-package-chunks", cfg.TranslatorPackageChunks, "chunked translator package JSON path")
	fs.StringVar(&cfg.Mode, "mode", cfg.Mode, "review mode: backtranslate or direct")
	fs.BoolVar(&cfg.ScoreOnly, "score-only", cfg.ScoreOnly, "direct mode: request score only without reasons")
	fs.StringVar(&cfg.LLMBackend, "llm-backend", cfg.LLMBackend, "llm backend: opencode or ollama")
	fs.StringVar(&cfg.ServerURL, "server-url", cfg.ServerURL, "LLM server URL")
	fs.StringVar(&cfg.Model, "model", cfg.Model, "model for back-translation")
	fs.StringVar(&cfg.Agent, "agent", cfg.Agent, "optional agent name for opencode")
	contextFiles := strings.Join(cfg.ContextFiles, ",")
	fs.StringVar(&contextFiles, "context-files", contextFiles, "comma-separated context files")
	fs.IntVar(&cfg.Concurrency, "concurrency", cfg.Concurrency, "analysis worker concurrency")
	fs.IntVar(&cfg.BatchSize, "batch-size", cfg.BatchSize, "batch size for ko->en translation")
	fs.IntVar(&cfg.TimeoutSec, "timeout-sec", cfg.TimeoutSec, "timeout in seconds")
	fs.IntVar(&cfg.Limit, "limit", 0, "optional row limit")
	fs.StringVar(&cfg.OutputDir, "output-dir", cfg.OutputDir, "output directory for reports")
	fs.StringVar(&cfg.TraceOut, "trace-out", "", "optional JSONL trace output")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	cfg.ContextFiles = splitList(contextFiles)

	os.Exit(semanticreview.Run(cfg))
}

func splitList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
