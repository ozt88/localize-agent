package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"localize-agent/workflow/pkg/shared"

	_ "modernc.org/sqlite"
)

type record struct {
	ID       string
	Source   string
	Target   string
	Status   string
	Category string
	Extra    map[string]any
}

type translatorPackage struct {
	Format   string                   `json:"format"`
	Segments []translatorPackageEntry `json:"segments"`
}

type translatorPackageEntry struct {
	SegmentID   string                  `json:"segment_id"`
	SourceFile  string                  `json:"source_file"`
	SceneHint   string                  `json:"scene_hint"`
	SourceText  string                  `json:"source_text"`
	Lines       []translatorPackageLine `json:"lines"`
}

type translatorPackageLine struct {
	LineID      string  `json:"line_id"`
	SegmentPos  *int    `json:"segment_pos"`
	SourceText  string  `json:"source_text"`
	TextRole    string  `json:"text_role"`
	SpeakerHint *string `json:"speaker_hint"`
	PrevLineID  *string `json:"prev_line_id"`
	NextLineID  *string `json:"next_line_id"`
}

type retryPackage struct {
	Format   string               `json:"format"`
	Datasets retryPackageDatasets `json:"datasets"`
}

type retryPackageDatasets struct {
	TextAssetRetry retryDataset `json:"textasset_retry"`
	ResourceRetry  retryDataset `json:"resource_retry"`
}

type retryDataset struct {
	Count int                `json:"count"`
	Items []retryPackageItem `json:"items"`
}

type retryPackageItem struct {
	ID            string               `json:"id"`
	SourceType    string               `json:"source_type"`
	RetryLane     string               `json:"retry_lane"`
	RetryReason   string               `json:"retry_reason"`
	SourceText    string               `json:"source_text"`
	TextRole      string               `json:"text_role"`
	ExistingTarget string              `json:"existing_target"`
	ContextEN     string               `json:"context_en"`
	SpeakerHint   string               `json:"speaker_hint"`
	ChoicePrefix  string               `json:"choice_prefix"`
	TranslationLane string             `json:"translation_lane"`
	Risk          string               `json:"risk"`
	SourceFile    string               `json:"source_file"`
	ResourceKey   string               `json:"resource_key"`
	MetaPathLabel string               `json:"meta_path_label"`
	SceneHint     string               `json:"scene_hint"`
	SegmentID     string               `json:"segment_id"`
	SegmentPos    *int                 `json:"segment_pos"`
	Tags          []string             `json:"tags"`
	TopCandidates []retryTopCandidate  `json:"top_candidates"`
}

type retryTopCandidate struct {
	SourceFile    string `json:"source_file"`
	MetaPathLabel string `json:"meta_path_label"`
	SpeakerHint   string `json:"speaker_hint"`
	TextRole      string `json:"text_role"`
	SegmentID     string `json:"segment_id"`
	SegmentPos    *int   `json:"segment_pos"`
}

func main() {
	var inPath string
	var outDir string
	var translatorPackagePath string
	var includeNonEmptyTarget bool
	var skipNoise bool

	flag.StringVar(&inPath, "in", "projects/esoteric-ebb/source/translator_package.json", "input esoteric translation json or translator package json")
	flag.StringVar(&outDir, "out-dir", "projects/esoteric-ebb/output/batches/translation_assetripper_textasset_unique", "output directory")
	flag.StringVar(&translatorPackagePath, "translator-package", "projects/esoteric-ebb/source/translator_package.json", "translator package json for context enrichment")
	flag.BoolVar(&includeNonEmptyTarget, "include-non-empty-target", false, "include rows where target is already non-empty")
	flag.BoolVar(&skipNoise, "skip-noise", true, "skip low-value noise entries (asset/event/key-like)")
	flag.Parse()

	root, entries, err := loadEntries(inPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = root
	segIndex, err := loadTranslatorSegmentIndex(translatorPackagePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

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
		enrichEntry(e, segIndex)
		recs = append(recs, record{
			ID:       id,
			Source:   src,
			Target:   tgt,
			Status:   st,
			Category: cat,
			Extra:    e,
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
	checkpointOut := filepath.Join(outDir, "translation_checkpoint.db")

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
	if err := seedCheckpoint(checkpointOut, recs); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("input: %s\n", inPath)
	fmt.Printf("selected: %d\n", len(ids))
	fmt.Printf("source : %s\n", sourceOut)
	fmt.Printf("current: %s\n", currentOut)
	fmt.Printf("ids    : %s\n", idsOut)
	fmt.Printf("db     : %s\n", checkpointOut)
}

type translatorSegmentIndex struct {
	bySegmentID map[string]translatorPackageEntry
	byLineID    map[string]translatorPackageLine
}

func loadTranslatorSegmentIndex(path string) (translatorSegmentIndex, error) {
	out := translatorSegmentIndex{
		bySegmentID: map[string]translatorPackageEntry{},
		byLineID:    map[string]translatorPackageLine{},
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return out, err
	}
	var pkg translatorPackage
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return out, err
	}
	for _, seg := range pkg.Segments {
		if strings.TrimSpace(seg.SegmentID) != "" {
			out.bySegmentID[seg.SegmentID] = seg
		}
		for _, line := range seg.Lines {
			if strings.TrimSpace(line.LineID) != "" {
				out.byLineID[line.LineID] = line
			}
		}
	}
	return out, nil
}

func enrichEntry(entry map[string]any, idx translatorSegmentIndex) {
	segmentID := stringValue(entry, "segment_id")
	lineID := stringValue(entry, "id")
	seg, ok := idx.bySegmentID[segmentID]
	if !ok {
		return
	}
	line := idx.byLineID[lineID]
	if stringValue(entry, "context_en") == "" && strings.TrimSpace(seg.SourceText) != "" {
		entry["context_en"] = seg.SourceText
	}
	if stringValue(entry, "source_file") == "" && strings.TrimSpace(seg.SourceFile) != "" {
		entry["source_file"] = seg.SourceFile
	}
	if stringValue(entry, "scene_hint") == "" && strings.TrimSpace(seg.SceneHint) != "" {
		entry["scene_hint"] = seg.SceneHint
	}
	if stringValue(entry, "text_role") == "" && strings.TrimSpace(line.TextRole) != "" {
		entry["text_role"] = line.TextRole
	}
	if stringValue(entry, "speaker_hint") == "" && line.SpeakerHint != nil {
		entry["speaker_hint"] = *line.SpeakerHint
	}
	if _, ok := entry["segment_pos"]; !ok && line.SegmentPos != nil {
		entry["segment_pos"] = *line.SegmentPos
	}
	if stringValue(entry, "prev_en") == "" && line.PrevLineID != nil {
		if prev, ok := idx.byLineID[*line.PrevLineID]; ok {
			entry["prev_en"] = prev.SourceText
		}
	}
	if stringValue(entry, "next_en") == "" && line.NextLineID != nil {
		if next, ok := idx.byLineID[*line.NextLineID]; ok {
			entry["next_en"] = next.SourceText
		}
	}
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
	if retryEntries, ok, err := extractRetryPackageEntries(raw); err != nil {
		return nil, nil, err
	} else if ok {
		return root, retryEntries, nil
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

func extractRetryPackageEntries(raw []byte) ([]map[string]any, bool, error) {
	var pkg retryPackage
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return nil, false, nil
	}
	if !strings.Contains(pkg.Format, "retry-package") {
		return nil, false, nil
	}

	out := make([]map[string]any, 0, pkg.Datasets.TextAssetRetry.Count+pkg.Datasets.ResourceRetry.Count)
	appendItems := func(items []retryPackageItem) {
		for _, item := range items {
			if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.SourceText) == "" {
				continue
			}
			category := strings.TrimSpace(item.TextRole)
			if category == "" {
				category = strings.TrimSpace(item.SourceType)
			}
			out = append(out, map[string]any{
				"id":               item.ID,
				"source":           item.SourceText,
				"target":           item.ExistingTarget,
				"status":           "new",
				"category":         category,
				"lane":             item.RetryLane,
				"reason":           item.RetryReason,
				"kind":             item.SourceType,
				"context_en":       item.ContextEN,
				"speaker_hint":     item.SpeakerHint,
				"choice_prefix":    item.ChoicePrefix,
				"translation_lane": item.TranslationLane,
				"risk":             item.Risk,
				"source_file":      firstNonEmpty(item.SourceFile, firstCandidateSourceFile(item.TopCandidates)),
				"resource_key":     item.ResourceKey,
				"meta_path_label":  firstNonEmpty(item.MetaPathLabel, firstCandidateMetaPath(item.TopCandidates)),
				"scene_hint":       item.SceneHint,
				"segment_id":       firstNonEmpty(item.SegmentID, firstCandidateSegmentID(item.TopCandidates)),
				"segment_pos":      firstNonNilInt(item.SegmentPos, firstCandidateSegmentPos(item.TopCandidates)),
				"tags":             item.Tags,
			})
		}
	}

	appendItems(pkg.Datasets.TextAssetRetry.Items)
	appendItems(pkg.Datasets.ResourceRetry.Items)
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

func seedCheckpoint(path string, recs []record) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS jobs (
		  run_id TEXT PRIMARY KEY,
		  created_at TEXT NOT NULL,
		  total_ids INTEGER NOT NULL,
		  config_json TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS items (
		  id TEXT PRIMARY KEY,
		  status TEXT NOT NULL,
		  ko_json TEXT,
		  pack_json TEXT,
		  attempts INTEGER NOT NULL DEFAULT 0,
		  last_error TEXT NOT NULL DEFAULT '',
		  updated_at TEXT NOT NULL,
		  latency_ms REAL NOT NULL DEFAULT 0,
		  source_hash TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_items_status ON items(status);
		DELETE FROM items;
	`); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO items(id, status, ko_json, pack_json, attempts, last_error, updated_at, latency_ms, source_hash) VALUES(?, ?, ?, ?, 0, '', ?, 0, '')`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, r := range recs {
		koJSON, _ := json.Marshal(map[string]any{"Text": r.Target})
		packJSON, _ := json.Marshal(buildPackObject(r))
		if _, err := stmt.Exec(r.ID, "new", string(koJSON), string(packJSON), now); err != nil {
			tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_, err = db.Exec(`INSERT OR REPLACE INTO jobs(run_id, created_at, total_ids, config_json) VALUES('checkpoint', ?, ?, '{}')`, now, len(recs))
	return err
}

func buildPackObject(r record) map[string]any {
	pack := map[string]any{
		"id":               r.ID,
		"en":               r.Source,
		"source_raw":       r.Source,
		"current_ko":       r.Target,
		"context_en":       stringValue(r.Extra, "context_en"),
		"prev_en":          stringValue(r.Extra, "prev_en"),
		"next_en":          stringValue(r.Extra, "next_en"),
		"text_role":        firstNonEmpty(r.Category, stringValue(r.Extra, "category")),
		"speaker_hint":     stringValue(r.Extra, "speaker_hint"),
		"retry_reason":     stringValue(r.Extra, "reason"),
		"source_type":      stringValue(r.Extra, "kind"),
		"source_file":      stringValue(r.Extra, "source_file"),
		"resource_key":     stringValue(r.Extra, "resource_key"),
		"meta_path_label":  stringValue(r.Extra, "meta_path_label"),
		"scene_hint":       stringValue(r.Extra, "scene_hint"),
		"segment_id":       stringValue(r.Extra, "segment_id"),
		"choice_prefix":    stringValue(r.Extra, "choice_prefix"),
		"translation_lane": stringValue(r.Extra, "translation_lane"),
		"risk":             stringValue(r.Extra, "risk"),
		"pipeline_version": "chunkctx-v1",
	}
	if pos, ok := intValue(r.Extra, "segment_pos"); ok {
		pack["segment_pos"] = pos
	}
	if tags, ok := r.Extra["tags"]; ok {
		pack["tags"] = tags
	}
	return pack
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func firstCandidateSourceFile(items []retryTopCandidate) string {
	if len(items) == 0 {
		return ""
	}
	return items[0].SourceFile
}

func firstCandidateMetaPath(items []retryTopCandidate) string {
	if len(items) == 0 {
		return ""
	}
	return items[0].MetaPathLabel
}

func firstCandidateSegmentID(items []retryTopCandidate) string {
	if len(items) == 0 {
		return ""
	}
	return items[0].SegmentID
}

func firstCandidateSegmentPos(items []retryTopCandidate) *int {
	if len(items) == 0 {
		return nil
	}
	return items[0].SegmentPos
}

func firstNonNilInt(values ...*int) *int {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

func stringValue(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}

func intValue(m map[string]any, key string) (int, bool) {
	if m == nil {
		return 0, false
	}
	switch v := m[key].(type) {
	case int:
		return v, true
	case float64:
		return int(v), true
	}
	return 0, false
}
