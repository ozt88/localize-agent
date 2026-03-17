package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"localize-agent/workflow/pkg/segmentchunk"
	"localize-agent/workflow/pkg/shared"
)

func main() {
	var inPath string
	var outPath string
	cfg := segmentchunk.DefaultConfig()

	flag.StringVar(&inPath, "in", "projects/esoteric-ebb/source/translator_package.json", "input translator package json")
	flag.StringVar(&outPath, "out", "projects/esoteric-ebb/source/translator_package_chunks.json", "output chunked translator package json")
	flag.IntVar(&cfg.MaxLines, "max-lines", cfg.MaxLines, "maximum lines per translation chunk")
	flag.IntVar(&cfg.MinLines, "min-lines", cfg.MinLines, "minimum lines before considering a split")
	flag.Parse()

	raw, err := os.ReadFile(inPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var pkg segmentchunk.TranslatorPackage
	if err := json.Unmarshal(raw, &pkg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	chunked := segmentchunk.BuildTranslatorPackageChunks(pkg, cfg)
	out, err := json.MarshalIndent(chunked, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := shared.AtomicWriteFile(outPath, out, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("input_segments=%d\n", len(pkg.Segments))
	fmt.Printf("output_chunks=%d\n", len(chunked.Chunks))
	fmt.Printf("out=%s\n", outPath)
}
