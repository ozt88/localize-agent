package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"

	"localize-agent/workflow/internal/contracts"
	"localize-agent/workflow/internal/inkparse"
	"localize-agent/workflow/internal/v2pipeline"
)

// tagPattern detects rich-text tags (e.g., <b>, <color=#FFF>, <i>, </b>).
var tagPattern = regexp.MustCompile(`<[a-zA-Z][^>]*>`)

func main() {
	os.Exit(run())
}

func run() int {
	var (
		inputPath string
		dsn       string
		backend   string
		dbPath    string
	)

	flag.StringVar(&inputPath, "input", "", "path to Phase 1 parser JSON output (required)")
	flag.StringVar(&dsn, "dsn", "", "PostgreSQL DSN for pipeline_items_v2")
	flag.StringVar(&backend, "backend", "postgres", "DB backend: postgres or sqlite")
	flag.StringVar(&dbPath, "db", "", "SQLite DB path (for local dev)")
	flag.Parse()

	if inputPath == "" {
		fmt.Fprintf(os.Stderr, "v2-ingest: -input is required\n")
		return 1
	}

	// Read and parse input JSON
	data, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "v2-ingest: read input: %v\n", err)
		return 1
	}

	// The parser output is wrapped in an envelope with "results" field
	var envelope struct {
		Results []inkparse.ParseResult `json:"results"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		fmt.Fprintf(os.Stderr, "v2-ingest: parse JSON: %v\n", err)
		return 1
	}

	results := envelope.Results
	if len(results) == 0 {
		fmt.Fprintf(os.Stderr, "v2-ingest: no results in input\n")
		return 1
	}

	// Build batches to get batch_id assignments
	batches := inkparse.BuildBatches(results)
	blockToBatch := make(map[string]string)
	for _, batch := range batches {
		for _, block := range batch.Blocks {
			blockToBatch[block.ID] = batch.ID
		}
	}

	// Build pipeline items from all blocks
	var items []contracts.V2PipelineItem
	sortIndex := 0
	passthroughCount := 0

	for _, result := range results {
		for _, block := range result.Blocks {
			state := v2pipeline.StatePendingTranslate
			koFormatted := ""
			if block.IsPassthrough {
				state = v2pipeline.StateDone
				koFormatted = block.Text
				passthroughCount++
			}

			hasTags := tagPattern.MatchString(block.Text)

			items = append(items, contracts.V2PipelineItem{
				ID:          block.ID,
				SortIndex:   sortIndex,
				SourceFile:  block.SourceFile,
				Knot:        block.Knot,
				ContentType: block.ContentType,
				SourceRaw:   block.Text,
				SourceHash:  block.SourceHash,
				HasTags:     hasTags,
				State:       state,
				KOFormatted: koFormatted,
				BatchID:     blockToBatch[block.ID],
			})
			sortIndex++
		}
	}

	// Open store
	store, err := v2pipeline.OpenStore(backend, dbPath, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "v2-ingest: open store: %v\n", err)
		return 1
	}
	defer store.Close()

	// Seed in chunks of 500
	const chunkSize = 500
	totalInserted := 0
	totalSkipped := 0

	for i := 0; i < len(items); i += chunkSize {
		end := i + chunkSize
		if end > len(items) {
			end = len(items)
		}
		inserted, skipped, err := store.Seed(items[i:end])
		if err != nil {
			fmt.Fprintf(os.Stderr, "v2-ingest: seed chunk %d-%d: %v\n", i, end, err)
			return 1
		}
		totalInserted += inserted
		totalSkipped += skipped
	}

	fmt.Fprintf(os.Stderr, "v2-ingest: %d blocks, %d inserted (%d passthrough=done), %d skipped (dedup)\n",
		len(items), totalInserted, passthroughCount, totalSkipped)

	return 0
}
