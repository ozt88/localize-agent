package inkparse

import "testing"

func makeBlocks(n int, sourceFile, knot, gate, contentType string) []DialogueBlock {
	blocks := make([]DialogueBlock, n)
	for i := range blocks {
		blocks[i] = DialogueBlock{
			SourceFile:  sourceFile,
			Knot:        knot,
			Gate:        gate,
			ContentType: contentType,
			Text:        "Some dialogue text here.",
		}
	}
	return blocks
}

func TestBuildBatchesDialogueSameGate(t *testing.T) {
	blocks := makeBlocks(20, "TS_Test", "knot1", "g-0", ContentDialogue)
	results := []ParseResult{{SourceFile: "TS_Test", Blocks: blocks}}
	batches := BuildBatches(results)

	dialogueBatches := filterBatches(batches, ContentDialogue)
	if len(dialogueBatches) != 1 {
		t.Fatalf("20 blocks same gate: want 1 batch, got %d", len(dialogueBatches))
	}
	if dialogueBatches[0].Format != FormatScript {
		t.Errorf("want format %q, got %q", FormatScript, dialogueBatches[0].Format)
	}
	if len(dialogueBatches[0].Blocks) != 20 {
		t.Errorf("want 20 blocks, got %d", len(dialogueBatches[0].Blocks))
	}
}

func TestBuildBatchesDialogueSplitLargeGate(t *testing.T) {
	blocks := makeBlocks(40, "TS_Test", "knot1", "g-0", ContentDialogue)
	results := []ParseResult{{SourceFile: "TS_Test", Blocks: blocks}}
	batches := BuildBatches(results)

	dialogueBatches := filterBatches(batches, ContentDialogue)
	if len(dialogueBatches) != 2 {
		t.Fatalf("40 blocks same gate: want 2 batches, got %d", len(dialogueBatches))
	}
	if len(dialogueBatches[0].Blocks) != 30 {
		t.Errorf("first batch: want 30 blocks, got %d", len(dialogueBatches[0].Blocks))
	}
	if len(dialogueBatches[1].Blocks) != 10 {
		t.Errorf("second batch: want 10 blocks, got %d", len(dialogueBatches[1].Blocks))
	}
}

func TestBuildBatchesSpellCardFormat(t *testing.T) {
	blocks := makeBlocks(5, "GT_Test", "knot1", "g-0", ContentSpell)
	results := []ParseResult{{SourceFile: "GT_Test", Blocks: blocks}}
	batches := BuildBatches(results)

	spellBatches := filterBatches(batches, ContentSpell)
	if len(spellBatches) != 1 {
		t.Fatalf("5 spell blocks: want 1 batch, got %d", len(spellBatches))
	}
	if spellBatches[0].Format != FormatCard {
		t.Errorf("want format %q, got %q", FormatCard, spellBatches[0].Format)
	}
}

func TestBuildBatchesUIDictionaryFormat(t *testing.T) {
	blocks := makeBlocks(80, "UI_Test", "knot1", "g-0", ContentUI)
	results := []ParseResult{{SourceFile: "UI_Test", Blocks: blocks}}
	batches := BuildBatches(results)

	uiBatches := filterBatches(batches, ContentUI)
	if len(uiBatches) != 1 {
		t.Fatalf("80 UI blocks: want 1 batch, got %d", len(uiBatches))
	}
	if uiBatches[0].Format != FormatDictionary {
		t.Errorf("want format %q, got %q", FormatDictionary, uiBatches[0].Format)
	}
	if len(uiBatches[0].Blocks) != 80 {
		t.Errorf("want 80 blocks, got %d", len(uiBatches[0].Blocks))
	}
}

func TestBuildBatchesExcludesPassthrough(t *testing.T) {
	blocks := []DialogueBlock{
		{SourceFile: "TS_Test", Knot: "k1", Gate: "g-0", ContentType: ContentDialogue, Text: "Hello", IsPassthrough: false},
		{SourceFile: "TS_Test", Knot: "k1", Gate: "g-0", ContentType: ContentDialogue, Text: "$var", IsPassthrough: true},
		{SourceFile: "TS_Test", Knot: "k1", Gate: "g-0", ContentType: ContentDialogue, Text: "World", IsPassthrough: false},
	}
	results := []ParseResult{{SourceFile: "TS_Test", Blocks: blocks}}
	batches := BuildBatches(results)

	total := 0
	for _, b := range batches {
		total += len(b.Blocks)
	}
	if total != 2 {
		t.Errorf("want 2 non-passthrough blocks total, got %d", total)
	}
}

func TestBuildBatchesDifferentGatesSeparateBatches(t *testing.T) {
	blocks1 := makeBlocks(15, "TS_Test", "knot1", "g-0", ContentDialogue)
	blocks2 := makeBlocks(15, "TS_Test", "knot1", "g-1", ContentDialogue)
	allBlocks := append(blocks1, blocks2...)
	results := []ParseResult{{SourceFile: "TS_Test", Blocks: allBlocks}}
	batches := BuildBatches(results)

	dialogueBatches := filterBatches(batches, ContentDialogue)
	// 15 + 15 = 30 which fits in one merged batch (same knot)
	if len(dialogueBatches) != 1 {
		t.Fatalf("15+15 same knot different gates: want 1 merged batch, got %d", len(dialogueBatches))
	}
	if len(dialogueBatches[0].Blocks) != 30 {
		t.Errorf("merged batch: want 30 blocks, got %d", len(dialogueBatches[0].Blocks))
	}
}

func TestBuildBatchesDifferentKnotsSeparateBatches(t *testing.T) {
	blocks1 := makeBlocks(15, "TS_Test", "knot1", "g-0", ContentDialogue)
	blocks2 := makeBlocks(15, "TS_Test", "knot2", "g-0", ContentDialogue)
	allBlocks := append(blocks1, blocks2...)
	results := []ParseResult{{SourceFile: "TS_Test", Blocks: allBlocks}}
	batches := BuildBatches(results)

	dialogueBatches := filterBatches(batches, ContentDialogue)
	if len(dialogueBatches) != 2 {
		t.Fatalf("different knots: want 2 batches, got %d", len(dialogueBatches))
	}
}

func TestBuildBatchesSystemDocumentFormat(t *testing.T) {
	blocks := makeBlocks(50, "TU_Combat", "knot1", "g-0", ContentSystem)
	results := []ParseResult{{SourceFile: "TU_Combat", Blocks: blocks}}
	batches := BuildBatches(results)

	sysBatches := filterBatches(batches, ContentSystem)
	if len(sysBatches) != 1 {
		t.Fatalf("system blocks: want 1 batch, got %d", len(sysBatches))
	}
	if sysBatches[0].Format != FormatDocument {
		t.Errorf("want format %q, got %q", FormatDocument, sysBatches[0].Format)
	}
	if len(sysBatches[0].Blocks) != 50 {
		t.Errorf("want 50 blocks in system batch, got %d", len(sysBatches[0].Blocks))
	}
}

func filterBatches(batches []Batch, contentType string) []Batch {
	var filtered []Batch
	for _, b := range batches {
		if b.ContentType == contentType {
			filtered = append(filtered, b)
		}
	}
	return filtered
}
