package v2pipeline

import (
	"fmt"
	"math"
	"os"
	"strings"

	"localize-agent/workflow/internal/contracts"
	"localize-agent/workflow/pkg/shared"
)

// RetranslateSelectConfig holds configuration for the retranslation candidate selection CLI.
type RetranslateSelectConfig struct {
	ScoreThreshold    float64
	DryRun            bool
	Histogram         bool
	ContentType       string
	Project           string
	ProjectDir        string
	CheckpointBackend string
	CheckpointDSN     string
	CheckpointDB      string
}

// RunRetranslateSelect selects retranslation candidates by score_final threshold,
// optionally displays a histogram, and resets selected batches for re-translation.
// Returns 0 on success, 1 on error, 2 on config error.
func RunRetranslateSelect(cfg RetranslateSelectConfig) int {
	// Validate: at least one mode must be specified.
	if !cfg.Histogram && cfg.ScoreThreshold <= 0 {
		fmt.Fprintln(os.Stderr, "error: -score-threshold (> 0) or -histogram required")
		return 2
	}

	// Load project config if not already loaded (backend/dsn may come from project.json).
	if cfg.CheckpointBackend == "" || cfg.CheckpointDSN == "" {
		projCfg, _, err := shared.LoadProjectConfig(cfg.Project, cfg.ProjectDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "retranslate-select: load project config: %v\n", err)
			return 2
		}
		if projCfg != nil {
			if cfg.CheckpointBackend == "" {
				cfg.CheckpointBackend = projCfg.Translation.CheckpointBackend
			}
			if cfg.CheckpointDSN == "" {
				cfg.CheckpointDSN = projCfg.Translation.CheckpointDSN
			}
		}
	}

	// Open store.
	store, err := OpenStore(cfg.CheckpointBackend, cfg.CheckpointDB, cfg.CheckpointDSN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "retranslate-select: open store: %v\n", err)
		return 1
	}
	defer store.Close()

	return runRetranslateSelectWithStore(cfg, store)
}

// runRetranslateSelectWithStore is the testable core that operates on a store.
func runRetranslateSelectWithStore(cfg RetranslateSelectConfig, store *Store) int {
	// Histogram mode.
	if cfg.Histogram {
		return printHistogram(store)
	}

	// Select candidates.
	candidates, err := store.SelectRetranslationBatches(cfg.ScoreThreshold, cfg.ContentType)
	if err != nil {
		fmt.Fprintf(os.Stderr, "retranslate-select: query candidates: %v\n", err)
		return 1
	}

	if len(candidates) == 0 {
		fmt.Println("No batches found with score_final below threshold.")
		return 0
	}

	// Print candidate summary.
	totalItems := 0
	for _, c := range candidates {
		totalItems += c.ItemCount
	}
	fmt.Printf("Found %d candidate batches (%d total items) with score_final < %.1f\n",
		len(candidates), totalItems, cfg.ScoreThreshold)
	fmt.Println()
	printCandidateTable(candidates)

	if cfg.DryRun {
		fmt.Println()
		fmt.Println("dry-run mode: no changes made")
		return 0
	}

	// Execute reset.
	return executeReset(store, candidates)
}

// printHistogram displays an ASCII histogram of score_final distribution.
func printHistogram(store *Store) int {
	buckets, err := store.ScoreHistogram(0.5)
	if err != nil {
		fmt.Fprintf(os.Stderr, "retranslate-select: histogram: %v\n", err)
		return 1
	}

	if len(buckets) == 0 {
		fmt.Println("No scored items found.")
		return 0
	}

	// Find max count for scaling.
	maxCount := 0
	for _, b := range buckets {
		if b.Count > maxCount {
			maxCount = b.Count
		}
	}

	maxBarWidth := 50
	fmt.Println("Score Distribution (bucket=0.5):")
	for _, b := range buckets {
		upper := b.LowerBound + 0.5
		barLen := 0
		if maxCount > 0 {
			barLen = int(math.Round(float64(b.Count) / float64(maxCount) * float64(maxBarWidth)))
		}
		if barLen == 0 && b.Count > 0 {
			barLen = 1
		}
		bar := strings.Repeat("|", barLen)
		fmt.Printf("[%4.1f-%4.1f) %s %d\n", b.LowerBound, upper, bar, b.Count)
	}
	return 0
}

// printCandidateTable prints a table of retranslation candidates.
func printCandidateTable(candidates []contracts.RetranslationCandidate) {
	fmt.Printf("%-30s %8s %8s %8s\n", "BATCH_ID", "ITEMS", "MIN", "AVG")
	fmt.Println(strings.Repeat("-", 58))
	for _, c := range candidates {
		batchDisplay := c.BatchID
		if len(batchDisplay) > 30 {
			batchDisplay = batchDisplay[:27] + "..."
		}
		fmt.Printf("%-30s %8d %8.1f %8.1f\n", batchDisplay, c.ItemCount, c.MinScore, c.AvgScore)
	}
}

// executeReset resets all candidate batches for retranslation.
func executeReset(store *Store, candidates []contracts.RetranslationCandidate) int {
	totalReset := 0
	for _, c := range candidates {
		// Determine next gen: query current max retranslation_gen for the batch.
		var currentGen int
		err := store.db.QueryRow(store.rebind(`SELECT COALESCE(MAX(retranslation_gen), 0) FROM pipeline_items_v2 WHERE batch_id = ?`), c.BatchID).Scan(&currentGen)
		if err != nil {
			fmt.Fprintf(os.Stderr, "retranslate-select: query gen for %s: %v\n", c.BatchID, err)
			return 1
		}
		nextGen := currentGen + 1

		count, err := store.ResetForRetranslation(c.BatchID, nextGen)
		if err != nil {
			fmt.Fprintf(os.Stderr, "retranslate-select: reset %s: %v\n", c.BatchID, err)
			return 1
		}
		totalReset += count
		fmt.Printf("  reset batch %s: %d items -> pending_translate (gen=%d)\n", c.BatchID, count, nextGen)
	}
	fmt.Printf("\nTotal: %d batches, %d items reset to pending_translate\n", len(candidates), totalReset)
	return 0
}
