package v2pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"localize-agent/workflow/internal/contracts"
)

func TestExportBuildV3Sidecar_MixedItems(t *testing.T) {
	items := []contracts.V2PipelineItem{
		{
			ID:          "Knot1/g-0/blk-0",
			SortIndex:   0,
			SourceFile:  "TS_Intro.json",
			ContentType: "dialogue",
			Speaker:     "Braxo",
			SourceRaw:   "Hello, traveler.",
			KOFormatted: "안녕, 여행자.",
		},
		{
			ID:          "Knot1/g-0/blk-1",
			SortIndex:   1,
			SourceFile:  "TS_Intro.json",
			ContentType: "dialogue",
			Speaker:     "",
			SourceRaw:   "...",
			KOFormatted: "...", // passthrough
		},
		{
			ID:          "Knot2/g-0/blk-0",
			SortIndex:   2,
			SourceFile:  "TS_Spells.json",
			ContentType: "spell",
			Speaker:     "",
			SourceRaw:   "<b>Fireball</b>",
			KOFormatted: "<b>파이어볼</b>",
		},
	}

	sidecar := BuildV3Sidecar(items)

	if sidecar.Format != V3Format {
		t.Errorf("format: got %q, want %q", sidecar.Format, V3Format)
	}
	if sidecar.Format != "esoteric-ebb-sidecar.v3" {
		t.Errorf("V3Format constant: got %q, want %q", sidecar.Format, "esoteric-ebb-sidecar.v3")
	}
	if len(sidecar.Entries) != 3 {
		t.Fatalf("entries count: got %d, want 3", len(sidecar.Entries))
	}

	// Check first entry mapping
	e0 := sidecar.Entries[0]
	if e0.ID != "Knot1/g-0/blk-0" {
		t.Errorf("entry[0].ID: got %q", e0.ID)
	}
	if e0.Source != "Hello, traveler." {
		t.Errorf("entry[0].Source: got %q", e0.Source)
	}
	if e0.Target != "안녕, 여행자." {
		t.Errorf("entry[0].Target: got %q", e0.Target)
	}
	if e0.SourceFile != "TS_Intro.json" {
		t.Errorf("entry[0].SourceFile: got %q", e0.SourceFile)
	}
	if e0.TextRole != "dialogue" {
		t.Errorf("entry[0].TextRole: got %q", e0.TextRole)
	}
	if e0.SpeakerHint != "Braxo" {
		t.Errorf("entry[0].SpeakerHint: got %q", e0.SpeakerHint)
	}

	// Check spell entry maps ContentType->TextRole
	e2 := sidecar.Entries[2]
	if e2.TextRole != "spell" {
		t.Errorf("entry[2].TextRole: got %q, want %q", e2.TextRole, "spell")
	}
	if e2.SpeakerHint != "" {
		t.Errorf("entry[2].SpeakerHint: got %q, want empty", e2.SpeakerHint)
	}
}

func TestExportBuildV3Sidecar_DedupEntries(t *testing.T) {
	// D-01: duplicate source_raw deduped in entries[] (first-seen-wins), all in contextual_entries[]
	items := []contracts.V2PipelineItem{
		{ID: "k1/g-0/blk-0", SourceRaw: "Same text", KOFormatted: "같은 텍스트", ContentType: "dialogue"},
		{ID: "k2/g-0/blk-0", SourceRaw: "Same text", KOFormatted: "같은 텍스트", ContentType: "dialogue"},
	}

	sidecar := BuildV3Sidecar(items)
	if len(sidecar.Entries) != 1 {
		t.Fatalf("entries count: got %d, want 1 (deduped by source)", len(sidecar.Entries))
	}
	if sidecar.Entries[0].ID != "k1/g-0/blk-0" {
		t.Errorf("entries[0].ID: got %q, want %q (first-seen-wins)", sidecar.Entries[0].ID, "k1/g-0/blk-0")
	}
	if len(sidecar.ContextualEntries) != 2 {
		t.Fatalf("contextual_entries count: got %d, want 2 (all items)", len(sidecar.ContextualEntries))
	}
}

func TestExportBuildV3Sidecar_PassthroughIncluded(t *testing.T) {
	// D-03: passthrough items have source == target
	items := []contracts.V2PipelineItem{
		{ID: "k/g-0/blk-0", SourceRaw: "...", KOFormatted: "...", ContentType: "dialogue"},
	}

	sidecar := BuildV3Sidecar(items)
	if len(sidecar.Entries) != 1 {
		t.Fatalf("D-03: passthrough should be included, got %d entries", len(sidecar.Entries))
	}
	if sidecar.Entries[0].Source != sidecar.Entries[0].Target {
		t.Errorf("D-03: passthrough source=%q target=%q should be equal",
			sidecar.Entries[0].Source, sidecar.Entries[0].Target)
	}
}

func TestExportBuildV3Sidecar_EmptyItems(t *testing.T) {
	sidecar := BuildV3Sidecar(nil)
	if sidecar.Entries == nil {
		t.Fatal("entries should not be nil for empty input")
	}
	if len(sidecar.Entries) != 0 {
		t.Errorf("entries count: got %d, want 0", len(sidecar.Entries))
	}
	if sidecar.ContextualEntries == nil {
		t.Fatal("contextual_entries should not be nil for empty input")
	}
	if len(sidecar.ContextualEntries) != 0 {
		t.Errorf("contextual_entries count: got %d, want 0", len(sidecar.ContextualEntries))
	}
	if sidecar.Format != V3Format {
		t.Errorf("format: got %q, want %q", sidecar.Format, V3Format)
	}
}

func TestExportBuildV3Sidecar_ContextualEntries(t *testing.T) {
	// 3 items: A and B share same SourceRaw "Hello", C has unique "Goodbye"
	items := []contracts.V2PipelineItem{
		{ID: "k1/g-0/blk-0", SourceRaw: "Hello", KOFormatted: "안녕", SourceFile: "TS_A.json", ContentType: "dialogue", Speaker: "Braxo"},
		{ID: "k2/g-0/blk-0", SourceRaw: "Hello", KOFormatted: "안녕", SourceFile: "TS_B.json", ContentType: "dialogue", Speaker: "Xan"},
		{ID: "k1/g-0/blk-1", SourceRaw: "Goodbye", KOFormatted: "잘가", SourceFile: "TS_A.json", ContentType: "narration", Speaker: ""},
	}

	sidecar := BuildV3Sidecar(items)

	// entries[] should be deduped: "Hello" once + "Goodbye" once = 2
	if len(sidecar.Entries) != 2 {
		t.Fatalf("entries count: got %d, want 2 (deduped by source)", len(sidecar.Entries))
	}
	// First-seen-wins: "Hello" entry should have item A's ID
	if sidecar.Entries[0].ID != "k1/g-0/blk-0" {
		t.Errorf("entries[0].ID: got %q, want %q (first-seen-wins)", sidecar.Entries[0].ID, "k1/g-0/blk-0")
	}

	// contextual_entries[] should have ALL 3 items
	if len(sidecar.ContextualEntries) != 3 {
		t.Fatalf("contextual_entries count: got %d, want 3 (all items)", len(sidecar.ContextualEntries))
	}
	// Metadata preserved for item B
	if sidecar.ContextualEntries[1].SourceFile != "TS_B.json" {
		t.Errorf("contextual_entries[1].SourceFile: got %q, want %q", sidecar.ContextualEntries[1].SourceFile, "TS_B.json")
	}
	if sidecar.ContextualEntries[1].SpeakerHint != "Xan" {
		t.Errorf("contextual_entries[1].SpeakerHint: got %q, want %q", sidecar.ContextualEntries[1].SpeakerHint, "Xan")
	}
}

func TestExportBuildV3Sidecar_ContextualEntriesEmpty(t *testing.T) {
	sidecar := BuildV3Sidecar(nil)
	if sidecar.ContextualEntries == nil {
		t.Fatal("contextual_entries should not be nil for empty input")
	}
	if len(sidecar.ContextualEntries) != 0 {
		t.Errorf("contextual_entries count: got %d, want 0", len(sidecar.ContextualEntries))
	}
}

func TestExportWriteTranslationsJSON_HasContextualEntries(t *testing.T) {
	items := []contracts.V2PipelineItem{
		{ID: "k/g-0/blk-0", SourceRaw: "Test", KOFormatted: "테스트", SourceFile: "TS_X.json", ContentType: "dialogue", Speaker: "NPC"},
	}
	sidecar := BuildV3Sidecar(items)

	dir := t.TempDir()
	path := filepath.Join(dir, "test_contextual.json")
	if err := WriteTranslationsJSON(path, sidecar); err != nil {
		t.Fatalf("WriteTranslationsJSON: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	var parsed V3Sidecar
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(parsed.ContextualEntries) != 1 {
		t.Fatalf("parsed contextual_entries: got %d, want 1", len(parsed.ContextualEntries))
	}
	if parsed.ContextualEntries[0].ID != "k/g-0/blk-0" {
		t.Errorf("parsed contextual_entries[0].ID: got %q", parsed.ContextualEntries[0].ID)
	}
}

func TestExportWriteTranslationsJSON(t *testing.T) {
	items := []contracts.V2PipelineItem{
		{
			ID:          "k/g-0/blk-0",
			SourceFile:  "TS_Test.json",
			ContentType: "dialogue",
			Speaker:     "NPC",
			SourceRaw:   "Hi",
			KOFormatted: "안녕",
		},
	}

	sidecar := BuildV3Sidecar(items)

	dir := t.TempDir()
	path := filepath.Join(dir, "translations.json")

	if err := WriteTranslationsJSON(path, sidecar); err != nil {
		t.Fatalf("WriteTranslationsJSON: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	// Verify valid JSON
	var parsed V3Sidecar
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed.Format != "esoteric-ebb-sidecar.v3" {
		t.Errorf("parsed format: got %q", parsed.Format)
	}
	if len(parsed.Entries) != 1 {
		t.Fatalf("parsed entries: got %d", len(parsed.Entries))
	}
	if parsed.Entries[0].Target != "안녕" {
		t.Errorf("parsed target: got %q", parsed.Entries[0].Target)
	}
}
