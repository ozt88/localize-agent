package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"localize-agent/workflow/internal/inkparse"
)

type output struct {
	TotalFiles       int                          `json:"total_files"`
	TotalBlocks      int                          `json:"total_blocks"`
	TotalTextEntries int                          `json:"total_text_entries"`
	Results          []*inkparse.ParseResult      `json:"results"`
	Validation       *inkparse.ValidationReport   `json:"validation,omitempty"`
}

func main() {
	os.Exit(run())
}

func run() int {
	var (
		assetsDir   string
		outputPath  string
		single      string
		validate    bool
		captureFile string
	)

	flag.StringVar(&assetsDir, "assets-dir", "projects/esoteric-ebb/extract/1.1.3/ExportedProject/Assets/TextAsset", "path to TextAsset directory")
	flag.StringVar(&outputPath, "output", "", "output JSON file path (default: stdout)")
	flag.StringVar(&single, "single", "", "parse a single file (for debugging)")
	flag.BoolVar(&validate, "validate", false, "run validation against capture data after parsing")
	flag.StringVar(&captureFile, "capture-file", "projects/esoteric-ebb/source/full_text_capture_clean.json", "path to capture JSON for validation")
	flag.Parse()

	var results []*inkparse.ParseResult
	var totalBlocks, totalTextEntries int
	var parseErrors int

	if single != "" {
		// Single file mode
		result, err := parseFile(single)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error parsing %s: %v\n", single, err)
			return 1
		}
		results = append(results, result)
		totalBlocks += len(result.Blocks)
		totalTextEntries += result.TotalTextEntries
	} else {
		// Batch mode: glob all .txt files in assets dir
		pattern := filepath.Join(assetsDir, "*.txt")
		matches, err := filepath.Glob(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "glob error: %v\n", err)
			return 1
		}
		if len(matches) == 0 {
			fmt.Fprintf(os.Stderr, "no .txt files found in %s\n", assetsDir)
			return 1
		}

		for _, path := range matches {
			// Skip .meta files if any sneak in
			if strings.HasSuffix(path, ".meta") {
				continue
			}
			result, err := parseFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error parsing %s: %v\n", path, err)
				parseErrors++
				continue
			}
			results = append(results, result)
			totalBlocks += len(result.Blocks)
			totalTextEntries += result.TotalTextEntries
		}
	}

	out := output{
		TotalFiles:       len(results),
		TotalBlocks:      totalBlocks,
		TotalTextEntries: totalTextEntries,
		Results:          results,
	}

	fmt.Fprintf(os.Stderr, "Parsed %d files, %d blocks, %d text entries", len(results), totalBlocks, totalTextEntries)
	if parseErrors > 0 {
		fmt.Fprintf(os.Stderr, " (%d errors)", parseErrors)
	}
	fmt.Fprintln(os.Stderr)

	// Run validation if requested
	if validate {
		captureData, err := inkparse.LoadCaptureData(captureFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading capture data from %s: %v\n", captureFile, err)
			return 1
		}

		// Collect all blocks from all results
		var allBlocks []inkparse.DialogueBlock
		for _, r := range results {
			allBlocks = append(allBlocks, r.Blocks...)
		}

		report := inkparse.ValidateAgainstCapture(allBlocks, captureData)
		out.Validation = &report

		// Print validation report to stderr
		fmt.Fprintf(os.Stderr, "\n=== Validation Report ===\n")
		fmt.Fprintf(os.Stderr, "Capture entries (ink_dialogue + ink_choice): %d\n", report.TotalCapture)
		fmt.Fprintf(os.Stderr, "Matched: %d (%.1f%%)\n", report.Matched, report.MatchRate*100)
		fmt.Fprintf(os.Stderr, "Unmatched: %d\n", report.Unmatched)

		// Print skipped origins
		if len(report.SkippedOrigins) > 0 {
			fmt.Fprintf(os.Stderr, "Skipped origins:")
			for origin, count := range report.SkippedOrigins {
				fmt.Fprintf(os.Stderr, " %s=%d", origin, count)
			}
			fmt.Fprintln(os.Stderr)
		}

		// Print top 20 unmatched entries
		if len(report.UnmatchedItems) > 0 {
			fmt.Fprintf(os.Stderr, "\nTop 20 unmatched entries:\n")
			limit := len(report.UnmatchedItems)
			if limit > 20 {
				limit = 20
			}
			for i := 0; i < limit; i++ {
				item := report.UnmatchedItems[i]
				text := item.Text
				if len(text) > 80 {
					text = text[:80] + "..."
				}
				fmt.Fprintf(os.Stderr, "[%d] %q (%s)\n", i+1, text, item.Origin)
			}
		}

	}

	// Write JSON output (before match rate check so output is always available)
	var jsonData []byte
	var err error
	jsonData, err = json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "json marshal error: %v\n", err)
		return 1
	}

	if outputPath != "" {
		if err := os.WriteFile(outputPath, jsonData, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write error: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "Output written to %s\n", outputPath)
	} else {
		fmt.Println(string(jsonData))
	}

	// Exit 1 if validation was run and match rate below 95%
	if validate && out.Validation != nil && out.Validation.MatchRate < 0.95 {
		fmt.Fprintf(os.Stderr, "\nWARNING: Match rate %.1f%% is below 95%% target\n", out.Validation.MatchRate*100)
		return 1
	}

	return 0
}

func parseFile(path string) (*inkparse.ParseResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return inkparse.Parse(data, base)
}
