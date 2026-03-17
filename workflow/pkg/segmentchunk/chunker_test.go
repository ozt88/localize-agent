package segmentchunk

import "testing"

func TestBuildChunks_ChoiceBlockStaysWhole(t *testing.T) {
	choiceID := "choice-1"
	seg := segment{
		SegmentID:     "seg-a",
		BlockKind:     "choice_block",
		ChoiceBlockID: &choiceID,
		LineIDs:       []string{"l1"},
		SourceLines:   []string{"(Look at Askan.)"},
		TextRoles:     []string{"choice"},
	}
	chunks := buildChunks([]segment{seg}, DefaultConfig())
	if len(chunks) != 1 {
		t.Fatalf("len=%d want 1", len(chunks))
	}
	if len(chunks[0].LineIDs) != 1 || chunks[0].LineIDs[0] != "l1" {
		t.Fatalf("chunk=%+v", chunks[0])
	}
}

func TestBuildChunks_SplitsLongSegmentOnRoleBoundary(t *testing.T) {
	seg := segment{
		SegmentID: "seg-b",
		LineIDs:   []string{"l1", "l2", "l3", "l4", "l5", "l6"},
		SourceLines: []string{
			"You approach.",
			"It is dark.",
			"A foolish way to view the world.",
			"Silence, egghead.",
			"FATE ITSELF TIES YOU TO THE LAND.",
			"APPLES. Yes. MORE APPLES.",
		},
		TextRoles: []string{"narration", "narration", "reaction", "dialogue", "reaction", "fragment"},
	}
	cfg := DefaultConfig()
	cfg.MaxLines = 4
	chunks := buildChunks([]segment{seg}, cfg)
	if len(chunks) < 2 {
		t.Fatalf("expected split, got %d chunk(s)", len(chunks))
	}
	if chunks[0].LineIDs[0] != "l1" || chunks[1].LineIDs[0] == "l1" {
		t.Fatalf("chunks=%+v", chunks)
	}
}

func TestBuildChunks_KeepsShortFragmentWithPrevious(t *testing.T) {
	seg := segment{
		SegmentID:   "seg-c",
		LineIDs:     []string{"l1", "l2", "l3"},
		SourceLines: []string{"You do so, seeing the four bands closest to you.", "Emotionally, at least.", "Another sentence."},
		TextRoles:   []string{"narration", "fragment", "narration"},
	}
	cfg := DefaultConfig()
	cfg.MaxLines = 2
	chunks := buildChunks([]segment{seg}, cfg)
	if len(chunks) != 2 {
		t.Fatalf("len=%d want 2", len(chunks))
	}
	if len(chunks[0].LineIDs) != 2 || chunks[0].LineIDs[1] != "l2" {
		t.Fatalf("first chunk=%+v", chunks[0])
	}
}

func TestBuildTranslatorPackageChunks_PreservesLineAlignment(t *testing.T) {
	pkg := TranslatorPackage{
		Format: "esoteric-ebb-translator-package.v1",
		Instructions: packageInstructions{
			TranslateUnit: "segment",
			ReturnUnit:    "line",
		},
		Segments: []packageSegment{
			{
				SegmentID:   "seg-pkg",
				SourceFile:  "AR_Test",
				SceneHint:   "AR_Test",
				BlockKind:   "script_block",
				SegmentSize: 6,
				SourceText:  "A.\nB.\nC.\nD.\nE.\nF.",
				Lines: []packageLine{
					{LineID: "l1", SegmentPos: 0, SourceText: "A.", TextRole: "narration"},
					{LineID: "l2", SegmentPos: 1, SourceText: "B.", TextRole: "narration"},
					{LineID: "l3", SegmentPos: 2, SourceText: "C.", TextRole: "reaction"},
					{LineID: "l4", SegmentPos: 3, SourceText: "D.", TextRole: "dialogue"},
					{LineID: "l5", SegmentPos: 4, SourceText: "E.", TextRole: "reaction"},
					{LineID: "l6", SegmentPos: 5, SourceText: "F.", TextRole: "fragment"},
				},
			},
		},
	}
	cfg := DefaultConfig()
	cfg.MaxLines = 4
	out := BuildTranslatorPackageChunks(pkg, cfg)
	if out.Instructions.TranslateUnit != "chunk" {
		t.Fatalf("translate unit=%q want chunk", out.Instructions.TranslateUnit)
	}
	if len(out.Chunks) < 2 {
		t.Fatalf("expected split into chunks, got %d", len(out.Chunks))
	}
	if out.Chunks[0].ParentSegmentID != "seg-pkg" {
		t.Fatalf("parent segment=%q", out.Chunks[0].ParentSegmentID)
	}
	if out.Chunks[0].Lines[0].LineID != "l1" {
		t.Fatalf("first line=%q", out.Chunks[0].Lines[0].LineID)
	}
	if out.Chunks[1].Lines[0].SegmentPos != 2 {
		t.Fatalf("preserved segment_pos=%d want 2", out.Chunks[1].Lines[0].SegmentPos)
	}
}
