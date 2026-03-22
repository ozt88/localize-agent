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

func TestExportBuildV3Sidecar_NoDedup(t *testing.T) {
	// D-02: duplicate source_raw with different IDs produce separate entries
	items := []contracts.V2PipelineItem{
		{ID: "k1/g-0/blk-0", SourceRaw: "Same text", KOFormatted: "같은 텍스트", ContentType: "dialogue"},
		{ID: "k2/g-0/blk-0", SourceRaw: "Same text", KOFormatted: "같은 텍스트", ContentType: "dialogue"},
	}

	sidecar := BuildV3Sidecar(items)
	if len(sidecar.Entries) != 2 {
		t.Fatalf("D-02 violation: entries count: got %d, want 2 (no dedup)", len(sidecar.Entries))
	}
	if sidecar.Entries[0].ID == sidecar.Entries[1].ID {
		t.Error("D-02: entries should have different IDs")
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
	if sidecar.Format != V3Format {
		t.Errorf("format: got %q, want %q", sidecar.Format, V3Format)
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
