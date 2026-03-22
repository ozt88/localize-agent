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
	TotalFiles       int                     `json:"total_files"`
	TotalBlocks      int                     `json:"total_blocks"`
	TotalTextEntries int                     `json:"total_text_entries"`
	Results          []*inkparse.ParseResult `json:"results"`
}

func main() {
	os.Exit(run())
}

func run() int {
	var (
		assetsDir string
		outputPath string
		single    string
	)

	flag.StringVar(&assetsDir, "assets-dir", "projects/esoteric-ebb/extract/1.1.3/ExportedProject/Assets/TextAsset", "path to TextAsset directory")
	flag.StringVar(&outputPath, "output", "", "output JSON file path (default: stdout)")
	flag.StringVar(&single, "single", "", "parse a single file (for debugging)")
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

	var jsonData []byte
	var err error
	if outputPath != "" {
		jsonData, err = json.MarshalIndent(out, "", "  ")
	} else {
		jsonData, err = json.MarshalIndent(out, "", "  ")
	}
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

	fmt.Fprintf(os.Stderr, "Parsed %d files, %d blocks, %d text entries", len(results), totalBlocks, totalTextEntries)
	if parseErrors > 0 {
		fmt.Fprintf(os.Stderr, " (%d errors)", parseErrors)
	}
	fmt.Fprintln(os.Stderr)

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
