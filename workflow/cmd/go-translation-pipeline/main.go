package main

import (
	"flag"
	"os"

	"localize-agent/workflow/internal/translationpipeline"
)

func main() {
	cfg := translationpipeline.Config{
		Project:        "esoteric-ebb",
		StageBatchSize: 100,
		Threshold:      0.7,
		MaxRetries:     3,
	}

	fs := flag.NewFlagSet("go-translation-pipeline", flag.ContinueOnError)
	fs.StringVar(&cfg.Project, "project", cfg.Project, "project name under projects/<name>")
	fs.StringVar(&cfg.ProjectDir, "project-dir", cfg.ProjectDir, "project directory containing project.json")
	fs.StringVar(&cfg.CheckpointDB, "checkpoint-db", cfg.CheckpointDB, "override checkpoint DB path")
	fs.BoolVar(&cfg.InitOnly, "init-only", cfg.InitOnly, "reset/seed pipeline state then exit without running workers")
	fs.BoolVar(&cfg.RequeueFailedNoRow, "requeue-failed-no-row", cfg.RequeueFailedNoRow, "move failed rows with 'translator produced no done row' back to pending_translate and exit")
	fs.IntVar(&cfg.RequeueLimit, "requeue-limit", cfg.RequeueLimit, "optional max number of failed rows to requeue")
	fs.BoolVar(&cfg.Reset, "reset", cfg.Reset, "reset pipeline_items before seeding")
	fs.IntVar(&cfg.StageBatchSize, "stage-batch-size", cfg.StageBatchSize, "number of ids to process per stage loop")
	fs.IntVar(&cfg.SeedLimit, "seed-limit", cfg.SeedLimit, "optional number of ids to seed from the project ids file")
	fs.Float64Var(&cfg.Threshold, "threshold", cfg.Threshold, "semantic weirdness score threshold for retranslation")
	fs.IntVar(&cfg.MaxRetries, "max-retries", cfg.MaxRetries, "maximum GPT-5.4 retranslation attempts per item")
	fs.StringVar(&cfg.LowBackend, "low-llm-backend", cfg.LowBackend, "backend for translation stage")
	fs.StringVar(&cfg.LowServerURL, "low-server-url", cfg.LowServerURL, "server URL for translation stage")
	fs.StringVar(&cfg.LowModel, "low-model", cfg.LowModel, "model for translation stage")
	fs.StringVar(&cfg.LowAgent, "low-agent", cfg.LowAgent, "agent for translation stage")
	fs.IntVar(&cfg.LowConcurrency, "low-concurrency", cfg.LowConcurrency, "concurrency for translation stage")
	fs.IntVar(&cfg.LowBatchSize, "low-batch-size", cfg.LowBatchSize, "batch size for translation stage")
	fs.IntVar(&cfg.LowTimeoutSec, "low-timeout-sec", cfg.LowTimeoutSec, "timeout seconds for translation stage")
	fs.StringVar(&cfg.RetranslateBackend, "retranslate-llm-backend", cfg.RetranslateBackend, "backend for retranslation stage")
	fs.StringVar(&cfg.RetranslateServerURL, "retranslate-server-url", cfg.RetranslateServerURL, "server URL for retranslation stage")
	fs.StringVar(&cfg.RetranslateModel, "retranslate-model", cfg.RetranslateModel, "model for retranslation stage")
	fs.StringVar(&cfg.RetranslateAgent, "retranslate-agent", cfg.RetranslateAgent, "agent for retranslation stage")
	fs.StringVar(&cfg.ScoreBackend, "score-llm-backend", cfg.ScoreBackend, "backend for score stage")
	fs.StringVar(&cfg.ScoreServerURL, "score-server-url", cfg.ScoreServerURL, "server URL for score stage")
	fs.StringVar(&cfg.ScoreModel, "score-model", cfg.ScoreModel, "model for score stage")
	fs.StringVar(&cfg.ScoreAgent, "score-agent", cfg.ScoreAgent, "agent for score stage")
	fs.IntVar(&cfg.ScoreConcurrency, "score-concurrency", cfg.ScoreConcurrency, "concurrency for score stage")
	fs.IntVar(&cfg.ScoreBatchSize, "score-batch-size", cfg.ScoreBatchSize, "batch size for score stage")
	fs.IntVar(&cfg.ScoreTimeoutSec, "score-timeout-sec", cfg.ScoreTimeoutSec, "timeout seconds for score stage")
	fs.StringVar(&cfg.WorkerRole, "worker-role", cfg.WorkerRole, "worker role: all|translate|score|retranslate")
	fs.StringVar(&cfg.WorkerID, "worker-id", cfg.WorkerID, "optional stable worker ID")
	fs.IntVar(&cfg.LeaseSec, "lease-sec", cfg.LeaseSec, "claim lease duration in seconds")
	fs.IntVar(&cfg.IdleSleepSec, "idle-sleep-sec", cfg.IdleSleepSec, "idle sleep seconds for dedicated worker mode")
	fs.BoolVar(&cfg.Once, "once", cfg.Once, "process at most one claimed batch then exit in dedicated worker mode")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}

	os.Exit(translationpipeline.Run(cfg))
}
