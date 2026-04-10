package ragcontext

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadBatchContext_ValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rag_batch_context.json")
	data := `{
		"knot_Alfoz/gate_001": [
			{"term": "Alfoz", "description": "A small town in the countryside", "category": "location"},
			{"term": "Norvik", "description": "Your home city", "category": "location"}
		],
		"knot_Braxo/gate_002": [
			{"term": "Braxo", "description": "A mysterious sorcerer", "category": "character"}
		]
	}`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	bc, err := LoadBatchContext(path)
	if err != nil {
		t.Fatalf("LoadBatchContext: %v", err)
	}

	hints := bc.HintsForBatch("knot_Alfoz/gate_001")
	if len(hints) != 2 {
		t.Fatalf("expected 2 hints, got %d", len(hints))
	}
	if hints[0].Term != "Alfoz" {
		t.Errorf("expected term Alfoz, got %s", hints[0].Term)
	}
	if hints[0].Category != "location" {
		t.Errorf("expected category location, got %s", hints[0].Category)
	}
}

func TestLoadBatchContext_MissingBatchID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rag_batch_context.json")
	data := `{"knot_Alfoz/gate_001": [{"term": "Alfoz", "description": "A town", "category": "location"}]}`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	bc, err := LoadBatchContext(path)
	if err != nil {
		t.Fatalf("LoadBatchContext: %v", err)
	}

	hints := bc.HintsForBatch("nonexistent_batch")
	if len(hints) != 0 {
		t.Fatalf("expected 0 hints for missing batch, got %d", len(hints))
	}
}

func TestLoadBatchContext_EmptyPath(t *testing.T) {
	bc, err := LoadBatchContext("")
	if err != nil {
		t.Fatalf("LoadBatchContext empty path: %v", err)
	}
	hints := bc.HintsForBatch("anything")
	if len(hints) != 0 {
		t.Fatalf("expected 0 hints from empty context, got %d", len(hints))
	}
}

func TestFormatHints_MultipleHints(t *testing.T) {
	hints := []RAGHint{
		{Term: "Alfoz", Description: "A small town", Category: "location"},
		{Term: "Norvik", Description: "Your home city", Category: "location"},
	}
	got := FormatHints(hints)
	want := "Alfoz: A small town | Norvik: Your home city"
	if got != want {
		t.Errorf("FormatHints = %q, want %q", got, want)
	}
}

func TestFormatHints_Empty(t *testing.T) {
	got := FormatHints(nil)
	if got != "" {
		t.Errorf("FormatHints(nil) = %q, want empty", got)
	}
	got = FormatHints([]RAGHint{})
	if got != "" {
		t.Errorf("FormatHints([]) = %q, want empty", got)
	}
}

func TestHintsForBatch_NilContext(t *testing.T) {
	var bc *BatchContext
	hints := bc.HintsForBatch("anything")
	if len(hints) != 0 {
		t.Fatalf("expected 0 hints from nil context, got %d", len(hints))
	}
}
