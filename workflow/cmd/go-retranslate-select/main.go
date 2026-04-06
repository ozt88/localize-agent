package main

import (
	"flag"
	"fmt"
	"os"

	"localize-agent/workflow/internal/v2pipeline"
	"localize-agent/workflow/pkg/shared"
)

func main() {
	os.Exit(run())
}

func run() int {
	fs := flag.NewFlagSet("go-retranslate-select", flag.ExitOnError)

	var cfg v2pipeline.RetranslateSelectConfig

	fs.StringVar(&cfg.Project, "project", "", "project name (required)")
	fs.StringVar(&cfg.ProjectDir, "project-dir", "", "project directory containing project.json")
	fs.Float64Var(&cfg.ScoreThreshold, "score-threshold", 0, "score_final threshold (select items below this)")
	fs.BoolVar(&cfg.DryRun, "dry-run", true, "dry-run mode: show candidates without resetting state")
	fs.BoolVar(&cfg.Histogram, "histogram", false, "show score_final distribution histogram")
	fs.StringVar(&cfg.ContentType, "content-type", "", "filter by content_type (e.g., dialogue)")

	// DB overrides (usually from project.json)
	fs.StringVar(&cfg.CheckpointBackend, "backend", "", "DB backend: postgres or sqlite")
	fs.StringVar(&cfg.CheckpointDSN, "dsn", "", "PostgreSQL DSN")
	fs.StringVar(&cfg.CheckpointDB, "checkpoint-db", "", "SQLite DB path (for local dev)")

	if err := fs.Parse(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "parse flags: %v\n", err)
		return 1
	}

	if cfg.Project == "" {
		fmt.Fprintln(os.Stderr, "error: -project is required")
		return 1
	}

	// Load project config.
	projCfg, projDir, err := shared.LoadProjectConfig(cfg.Project, cfg.ProjectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load project config: %v\n", err)
		return 1
	}
	if projCfg != nil {
		cfg.ProjectDir = projDir

		// Apply project defaults only for flags not explicitly provided.
		explicit := make(map[string]bool)
		fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })

		if !explicit["backend"] {
			cfg.CheckpointBackend = projCfg.Translation.CheckpointBackend
		}
		if !explicit["dsn"] {
			cfg.CheckpointDSN = projCfg.Translation.CheckpointDSN
		}
		if !explicit["checkpoint-db"] {
			cfg.CheckpointDB = projCfg.Translation.CheckpointDB
		}
	}

	return v2pipeline.RunRetranslateSelect(cfg)
}
