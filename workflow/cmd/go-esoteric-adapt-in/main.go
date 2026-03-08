package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"localize-agent/workflow/internal/shared"
)

type record struct {
	ID       string
	Source   string
	Target   string
	Status   string
	Category string
}

type translatorPackage struct {
	Format   string                   `json:"format"`
	Segments []translatorPackageEntry `json:"segments"`
}

type translatorPackageEntry struct {
	Lines []translatorPackageLine `json:"lines"`
}

type translatorPackageLine struct {
	LineID     string `json:"line_id"`
	SourceText string `json:"source_text"`
	TextRole   string `json:"text_role"`
}

func main() {
	var inPath string
	var outDir string
	var includeNonEmptyTarget bool
	var skipNoise bool

	flag.StringVar(&inPath, "in", "projects/esoteric-ebb/source/translator_package.json", "input esoteric translation json or translator package json")
	flag.StringVar(&outDir, "out-dir", "projects/esoteric-ebb/output/batches/translation_assetripper_textasset_unique", "output directory")
	flag.BoolVar(&includeNonEmptyTarget, "include-non-empty-target", false, "include rows where target is already non-empty")
	flag.BoolVar(&skipNoise, "skip-noise", true, "skip low-value noise entries (asset/event/key-like)")
	flag.Parse()

	root, entries, err := loadEntries(inPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = root

	recs := make([]record, 0, len(entries))
	for _, e := range entries {
		id, _ := e["id"].(string)
		src, _ := e["source"].(string)
		tgt, _ := e["target"].(string)
		st, _ := e["status"].(string)
		cat, _ := e["category"].(string)
		if strings.TrimSpace(id) == "" || strings.TrimSpace(src) == "" {
			continue
		}
		if skipNoise && isNoiseLike(src) {
			continue
		}
		if !includeNonEmptyTarget && strings.TrimSpace(tgt) != "" {
			continue
		}
		if !isTranslatableStatus(st) {
			continue
		}
		recs = append(recs, record{
			ID:       id,
			Source:   src,
			Target:   tgt,
			Status:   st,
			Category: cat,
		})
	}

	sort.SliceStable(recs, func(i, j int) bool {
		pi := priority(recs[i])
		pj := priority(recs[j])
		if pi == pj {
			return recs[i].ID < recs[j].ID
		}
		return pi < pj
	})

	sourceJSON := map[string]any{"strings": map[string]any{}}
	currentJSON := map[string]any{"strings": map[string]any{}}
	sStrings := sourceJSON["strings"].(map[string]any)
	cStrings := currentJSON["strings"].(map[string]any)
	ids := make([]string, 0, len(recs))
	for _, r := range recs {
		sStrings[r.ID] = map[string]any{"Text": r.Source}
		cStrings[r.ID] = map[string]any{"Text": r.Target}
		ids = append(ids, r.ID)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sourceOut := filepath.Join(outDir, "source_esoteric.json")
	currentOut := filepath.Join(outDir, "current_esoteric.json")
	idsOut := filepath.Join(outDir, "ids_esoteric.txt")

	if err := writeJSON(sourceOut, sourceJSON); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := writeJSON(currentOut, currentJSON); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := shared.AtomicWriteFile(idsOut, []byte(strings.Join(ids, "\n")+"\n"), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("input: %s\n", inPath)
	fmt.Printf("selected: %d\n", len(ids))
	fmt.Printf("source : %s\n", sourceOut)
	fmt.Printf("current: %s\n", currentOut)
	fmt.Printf("ids    : %s\n", idsOut)
}

func priority(r record) int {
	c := strings.ToLower(strings.TrimSpace(r.Category))
	switch c {
	case "quest":
		return 0
	case "dialog":
		if looksSentence(r.Source) {
			return 1
		}
		return 2
	default:
		return 3
	}
}

func looksSentence(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if len([]rune(s)) >= 24 {
		return true
	}
	if strings.ContainsAny(s, ".?!") {
		return true
	}
	return false
}

func isNoiseLike(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" {
		return true
	}
	lt := strings.ToLower(t)
	if strings.HasPrefix(lt, "event:/") {
		return true
	}
	if strings.HasPrefix(t, "FEAT_") {
		return true
	}
	if isAllCapsUnderscoreNumber(t) {
		return true
	}
	return false
}

func isAllCapsUnderscoreNumber(s string) bool {
	if len(s) < 6 {
		return false
	}
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

func isTranslatableStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "new", "skip":
		return true
	default:
		return false
	}
}

func loadEntries(path string) (any, []map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, nil, err
	}
	if pkgEntries, ok, err := extractTranslatorPackageEntries(raw); err != nil {
		return nil, nil, err
	} else if ok {
		return root, pkgEntries, nil
	}
	entries, err := extractEntries(root)
	if err != nil {
		return nil, nil, err
	}
	return root, entries, nil
}

func extractTranslatorPackageEntries(raw []byte) ([]map[string]any, bool, error) {
	var pkg translatorPackage
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return nil, false, nil
	}
	if !strings.Contains(pkg.Format, "translator-package") || len(pkg.Segments) == 0 {
		return nil, false, nil
	}
	out := make([]map[string]any, 0)
	for _, seg := range pkg.Segments {
		for _, line := range seg.Lines {
			if strings.TrimSpace(line.LineID) == "" || strings.TrimSpace(line.SourceText) == "" {
				continue
			}
			out = append(out, map[string]any{
				"id":       line.LineID,
				"source":   line.SourceText,
				"target":   "",
				"status":   "new",
				"category": line.TextRole,
			})
		}
	}
	return out, true, nil
}

func extractEntries(root any) ([]map[string]any, error) {
	switch v := root.(type) {
	case []any:
		return castEntryArray(v)
	case map[string]any:
		for _, key := range []string{"items", "entries", "translations"} {
			if arr, ok := v[key].([]any); ok {
				return castEntryArray(arr)
			}
		}
		return nil, fmt.Errorf("unsupported object schema: expected one of keys [items, entries, translations]")
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

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return shared.AtomicWriteFile(path, b, 0o644)
}
