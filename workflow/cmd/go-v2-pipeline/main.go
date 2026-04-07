package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"localize-agent/workflow/internal/v2pipeline"
)

func main() {
	cfg := v2pipeline.Config{
		Project: "esoteric-ebb",
	}

	fs := flag.NewFlagSet("go-v2-pipeline", flag.ContinueOnError)
	fs.StringVar(&cfg.Project, "project", cfg.Project, "project name under projects/<name>")
	fs.StringVar(&cfg.ProjectDir, "project-dir", cfg.ProjectDir, "project directory containing project.json")

	// DB
	fs.StringVar(&cfg.CheckpointBackend, "backend", cfg.CheckpointBackend, "DB backend: postgres or sqlite")
	fs.StringVar(&cfg.CheckpointDSN, "dsn", cfg.CheckpointDSN, "PostgreSQL DSN")
	fs.StringVar(&cfg.CheckpointDB, "checkpoint-db", cfg.CheckpointDB, "SQLite DB path (for local dev)")

	// Translate stage
	fs.StringVar(&cfg.TranslateBackend, "translate-backend", cfg.TranslateBackend, "backend for translate stage")
	fs.StringVar(&cfg.TranslateServerURL, "translate-server-url", cfg.TranslateServerURL, "server URL for translate stage")
	fs.StringVar(&cfg.TranslateModel, "translate-model", cfg.TranslateModel, "model for translate stage (provider/model)")
	fs.IntVar(&cfg.TranslateConcurrency, "translate-concurrency", cfg.TranslateConcurrency, "concurrent translate workers")
	fs.IntVar(&cfg.TranslateBatchSize, "translate-batch-size", cfg.TranslateBatchSize, "items per translate claim")
	fs.IntVar(&cfg.TranslateTimeoutSec, "translate-timeout-sec", cfg.TranslateTimeoutSec, "timeout for translate LLM calls")

	// Format stage
	fs.StringVar(&cfg.FormatBackend, "format-backend", cfg.FormatBackend, "backend for format stage")
	fs.StringVar(&cfg.FormatServerURL, "format-server-url", cfg.FormatServerURL, "server URL for format stage")
	fs.StringVar(&cfg.FormatModel, "format-model", cfg.FormatModel, "model for format stage (provider/model)")
	fs.IntVar(&cfg.FormatConcurrency, "format-concurrency", cfg.FormatConcurrency, "concurrent format workers")
	fs.IntVar(&cfg.FormatBatchSize, "format-batch-size", cfg.FormatBatchSize, "items per format claim")
	fs.IntVar(&cfg.FormatTimeoutSec, "format-timeout-sec", cfg.FormatTimeoutSec, "timeout for format LLM calls")

	// Score stage
	fs.StringVar(&cfg.ScoreBackend, "score-backend", cfg.ScoreBackend, "backend for score stage")
	fs.StringVar(&cfg.ScoreServerURL, "score-server-url", cfg.ScoreServerURL, "server URL for score stage")
	fs.StringVar(&cfg.ScoreModel, "score-model", cfg.ScoreModel, "model for score stage (provider/model)")
	fs.IntVar(&cfg.ScoreConcurrency, "score-concurrency", cfg.ScoreConcurrency, "concurrent score workers")
	fs.IntVar(&cfg.ScoreBatchSize, "score-batch-size", cfg.ScoreBatchSize, "items per score claim")
	fs.IntVar(&cfg.ScoreTimeoutSec, "score-timeout-sec", cfg.ScoreTimeoutSec, "timeout for score LLM calls")

	// Orchestrator
	fs.StringVar(&cfg.WorkerRole, "role", cfg.WorkerRole, "worker role: all, translate, format, score")
	fs.StringVar(&cfg.WorkerID, "worker-id", cfg.WorkerID, "worker identifier (auto-generated if empty)")
	fs.IntVar(&cfg.LeaseSec, "lease-sec", cfg.LeaseSec, "claim lease duration in seconds")
	fs.IntVar(&cfg.IdleSleepSec, "idle-sleep-sec", cfg.IdleSleepSec, "idle sleep seconds when no work")
	fs.IntVar(&cfg.MaxRetries, "max-retries", cfg.MaxRetries, "max retry attempts per item per stage")
	fs.StringVar(&cfg.TraceOutDir, "trace-dir", cfg.TraceOutDir, "directory for LLM trace output")
	fs.BoolVar(&cfg.CleanupStaleClaims, "cleanup-stale-claims", cfg.CleanupStaleClaims, "reclaim expired leases before starting")
	fs.BoolVar(&cfg.Once, "once", cfg.Once, "run one batch per role then exit (for testing)")

	// Context enrichment (Phase 07)
	fs.StringVar(&cfg.VoiceCardsPath, "voice-cards", cfg.VoiceCardsPath, "path to voice_cards.json for named character voice cards")

	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}

	// Auto-generate worker ID if not provided.
	if cfg.WorkerID == "" {
		cfg.WorkerID = fmt.Sprintf("v2-%d", time.Now().UnixNano()%100000)
	}

	os.Exit(v2pipeline.Run(cfg))
}
