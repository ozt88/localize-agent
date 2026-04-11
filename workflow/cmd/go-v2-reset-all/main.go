package main

import (
	"flag"
	"fmt"
	"os"

	"localize-agent/workflow/internal/v2pipeline"
	"localize-agent/workflow/pkg/shared"
)

func main() { os.Exit(run()) }

func run() int {
	fs := flag.NewFlagSet("go-v2-reset-all", flag.ContinueOnError)
	var (
		project    string
		projectDir string
		backend    string
		dsn        string
		dbPath     string
		dryRun     bool
		cleanup    bool
	)
	fs.StringVar(&project, "project", "", "project name (required)")
	fs.StringVar(&projectDir, "project-dir", "", "project directory")
	fs.StringVar(&backend, "backend", "", "DB backend: postgres or sqlite")
	fs.StringVar(&dsn, "dsn", "", "PostgreSQL DSN")
	fs.StringVar(&dbPath, "checkpoint-db", "", "SQLite DB path")
	fs.BoolVar(&dryRun, "dry-run", true, "show what would happen without executing")
	fs.BoolVar(&cleanup, "cleanup-stale-claims", true, "reclaim stale working items before reset")

	if err := fs.Parse(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "parse flags: %v\n", err)
		return 1
	}
	if project == "" {
		fmt.Fprintln(os.Stderr, "error: -project is required")
		return 1
	}

	// Load project config with explicit flag tracking
	explicit := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })

	projCfg, _, err := shared.LoadProjectConfig(project, projectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load project config: %v\n", err)
		return 1
	}
	if projCfg != nil {
		if !explicit["backend"] && projCfg.Translation.CheckpointBackend != "" {
			backend = projCfg.Translation.CheckpointBackend
		}
		if !explicit["dsn"] && projCfg.Translation.CheckpointDSN != "" {
			dsn = projCfg.Translation.CheckpointDSN
		}
		if !explicit["checkpoint-db"] && projCfg.Translation.CheckpointDB != "" {
			dbPath = projCfg.Translation.CheckpointDB
		}
	}

	if backend == "" {
		backend = "postgres"
	}

	store, err := v2pipeline.OpenStore(backend, dbPath, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open store: %v\n", err)
		return 1
	}
	defer store.Close()

	// Pre-reset state
	before, err := store.CountByState()
	if err != nil {
		fmt.Fprintf(os.Stderr, "count by state: %v\n", err)
		return 1
	}
	fmt.Printf("Before reset: %v\n", before)

	if dryRun {
		total := before["done"] + before["failed"]
		fmt.Printf("Would reset %d items (done=%d, failed=%d) to pending_translate with retranslation_gen=1\n",
			total, before["done"], before["failed"])
		return 0
	}

	// Cleanup stale claims first
	if cleanup {
		reclaimed, err := store.CleanupStaleClaims(0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cleanup stale claims: %v\n", err)
			return 1
		}
		if reclaimed > 0 {
			fmt.Printf("Reclaimed %d stale claims\n", reclaimed)
		}
	}

	// Execute reset
	affected, err := store.ResetAllForRetranslation(1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reset: %v\n", err)
		return 1
	}

	// Post-reset state
	after, err := store.CountByState()
	if err != nil {
		fmt.Fprintf(os.Stderr, "count by state (after): %v\n", err)
		return 1
	}
	fmt.Printf("After reset: %v\n", after)
	fmt.Printf("Reset %d items to pending_translate (retranslation_gen=1)\n", affected)
	return 0
}
