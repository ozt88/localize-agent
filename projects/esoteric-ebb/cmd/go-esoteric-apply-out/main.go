package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"localize-agent/workflow/pkg/platform"
	"localize-agent/workflow/pkg/shared"
)

var tokenRE = regexp.MustCompile(`(\$[A-Za-z0-9_]+|<[^>]+>|\{[^{}]+\})`)

func main() {
	var inPath string
	var outPath string
	var checkpointDB string
	var appliedStatus string
	var inPlace bool
	var allowTokenDrift bool
	var pipelineVersion string

	flag.StringVar(&inPath, "in", "projects/esoteric-ebb/source/translation_assetripper_textasset_unique.json", "input esoteric translation json")
	flag.StringVar(&outPath, "out", "projects/esoteric-ebb/output/batches/translation_assetripper_textasset_unique/translation_assetripper_textasset_unique.translated.json", "output json path")
	flag.StringVar(&checkpointDB, "checkpoint-db", "projects/esoteric-ebb/output/batches/translation_assetripper_textasset_unique/translation_checkpoint.db", "translator checkpoint DB")
	flag.StringVar(&appliedStatus, "applied-status", "translated", "status value for applied rows")
	flag.BoolVar(&inPlace, "in-place", false, "write output in-place to --in")
	flag.BoolVar(&allowTokenDrift, "allow-token-drift", false, "allow placeholder/tag drift")
	flag.StringVar(&pipelineVersion, "pipeline-version", "chunkctx-v1", "apply only rows written by this pipeline version")
	flag.Parse()

	items, err := platform.LoadDonePackItems(checkpointDB, pipelineVersion)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(items) == 0 {
		fmt.Fprintln(os.Stderr, "no done pack rows in checkpoint db")
		os.Exit(1)
	}
	translated := map[string]string{}
	for _, it := range items {
		if strings.TrimSpace(it.ID) == "" || strings.TrimSpace(it.ProposedKORestored) == "" {
			continue
		}
		translated[it.ID] = it.ProposedKORestored
	}

	raw, err := os.ReadFile(inPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	entries, err := extractEntries(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	applied := 0
	skippedToken := 0
	notFound := 0
	for _, e := range entries {
		id, _ := e["id"].(string)
		src, _ := e["source"].(string)
		ko, ok := translated[id]
		if !ok {
			notFound++
			continue
		}
		if !allowTokenDrift && !tokenCompatible(src, ko) {
			skippedToken++
			continue
		}
		e["target"] = ko
		e["status"] = appliedStatus
		applied++
	}

	writePath := outPath
	if inPlace {
		writePath = inPath
	}
	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := shared.AtomicWriteFile(writePath, b, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("input       : %s\n", inPath)
	fmt.Printf("checkpoint  : %s\n", checkpointDB)
	fmt.Printf("applied     : %d\n", applied)
	fmt.Printf("skipped_tag : %d\n", skippedToken)
	fmt.Printf("not_found   : %d\n", notFound)
	fmt.Printf("output      : %s\n", writePath)
}

func tokenCompatible(src, ko string) bool {
	srcTokens := tokenRE.FindAllString(src, -1)
	koTokens := tokenRE.FindAllString(ko, -1)
	if len(srcTokens) != len(koTokens) {
		return false
	}
	for i := range srcTokens {
		if srcTokens[i] != koTokens[i] {
			return false
		}
	}
	if strings.Count(src, "\n") != strings.Count(ko, "\n") {
		return false
	}
	return true
}

func extractEntries(root any) ([]map[string]any, error) {
	switch v := root.(type) {
	case []any:
		return castEntryArray(v)
	case map[string]any:
		if stringsMap, ok := v["strings"].(map[string]any); ok {
			return castStringMap(stringsMap)
		}
		for _, key := range []string{"items", "entries", "translations"} {
			if arr, ok := v[key].([]any); ok {
				return castEntryArray(arr)
			}
		}
		return nil, fmt.Errorf("unsupported object schema: expected one of keys [strings, items, entries, translations]")
	default:
		return nil, fmt.Errorf("unsupported json root type")
	}
}

func castEntryArray(arr []any) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("entry is not object")
		}
		out = append(out, m)
	}
	return out, nil
}

func castStringMap(m map[string]any) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(m))
	for id, raw := range m {
		entry, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("strings entry is not object")
		}
		source, _ := entry["Text"].(string)
		out = append(out, map[string]any{
			"id":     id,
			"source": source,
			"target": source,
			"status": "",
		})
	}
	return out, nil
}
