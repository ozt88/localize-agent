package inkparse

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// =============================================================================
// Parser error handling
// =============================================================================

func TestParse_MalformedJSON(t *testing.T) {
	_, err := Parse([]byte("not valid json"), "test")
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestParse_MissingRootKey(t *testing.T) {
	data := []byte(`{"inkVersion":21}`)
	_, err := Parse(data, "test")
	if err == nil {
		t.Fatal("expected error for missing root key, got nil")
	}
}

func TestParse_EmptyRootArray(t *testing.T) {
	data := []byte(`{"inkVersion":21,"root":[],"listDefs":{}}`)
	result, err := Parse(data, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Blocks) != 0 {
		t.Fatalf("expected 0 blocks for empty root, got %d", len(result.Blocks))
	}
}

func TestParse_UTF8BOM(t *testing.T) {
	// BOM + valid JSON
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"TestKnot": []any{
					[]any{
						"^BOM text. ", "\n",
						nil,
						map[string]any{"#f": 5, "#n": "g-0"},
					},
					map[string]any{"#f": 1},
				},
			},
		},
		"listDefs": map[string]any{},
	}
	data, _ := json.Marshal(ink)
	// Prepend BOM
	bomData := append([]byte{0xEF, 0xBB, 0xBF}, data...)
	result, err := Parse(bomData, "bom_test")
	if err != nil {
		t.Fatalf("failed to parse with BOM: %v", err)
	}
	found := false
	for _, b := range result.Blocks {
		if strings.Contains(b.Text, "BOM text") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected block with 'BOM text' after BOM stripping")
	}
}

func TestParse_RootIsNotArray(t *testing.T) {
	data := []byte(`{"inkVersion":21,"root":"not_an_array","listDefs":{}}`)
	_, err := Parse(data, "test")
	if err == nil {
		t.Fatal("expected error when root is not an array, got nil")
	}
}

// =============================================================================
// Parser empty/edge inputs
// =============================================================================

func TestParse_EmptyTextAfterCaret(t *testing.T) {
	// "^" with nothing after it
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"TestKnot": []any{
					[]any{
						"^", "\n",
						nil,
						map[string]any{"#f": 5, "#n": "g-0"},
					},
					map[string]any{"#f": 1},
				},
			},
		},
		"listDefs": map[string]any{},
	}
	data, _ := json.Marshal(ink)
	result, err := Parse(data, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should not crash; empty text after ^ should either produce empty block or skip
	_ = result
}

func TestParse_TagWithoutText(t *testing.T) {
	// Tags appear but no ^text precedes them
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"TestKnot": []any{
					[]any{
						"#", "^speaker", "/#",
						"\n",
						nil,
						map[string]any{"#f": 5, "#n": "g-0"},
					},
					map[string]any{"#f": 1},
				},
			},
		},
		"listDefs": map[string]any{},
	}
	data, _ := json.Marshal(ink)
	result, err := Parse(data, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should not crash; no text means no meaningful block
	_ = result
}

func TestParse_MultipleKnots(t *testing.T) {
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"KnotA": []any{
					[]any{
						"^From knot A. ", "\n",
						nil,
						map[string]any{"#f": 5, "#n": "g-0"},
					},
					map[string]any{"#f": 1},
				},
				"KnotB": []any{
					[]any{
						"^From knot B. ", "\n",
						nil,
						map[string]any{"#f": 5, "#n": "g-0"},
					},
					map[string]any{"#f": 1},
				},
			},
		},
		"listDefs": map[string]any{},
	}
	data, _ := json.Marshal(ink)
	result, err := Parse(data, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	knotNames := make(map[string]bool)
	for _, b := range result.Blocks {
		knotNames[b.Knot] = true
	}
	if !knotNames["KnotA"] {
		t.Error("missing blocks from KnotA")
	}
	if !knotNames["KnotB"] {
		t.Error("missing blocks from KnotB")
	}
}

// =============================================================================
// Choice flag variants (D-26)
// =============================================================================

func TestParse_ChoiceFlg3_ConditionalWithStartContent(t *testing.T) {
	// flg:3 = 0x1|0x2 (conditional + has start content) from TS_Viira.txt
	// Should extract text from "s" sub since 0x2 bit is set
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"TestKnot": []any{
					[]any{
						[]any{
							"ev", "str", map[string]any{"->": ".^.s"}, "/str",
							map[string]any{"VAR?": "someVar"}, 1, "<",
							"/ev",
							map[string]any{"*": ".^.^.c-0", "flg": 3},
							map[string]any{
								"s": []any{"^\"Yes?\"", nil},
							},
						},
						nil,
						map[string]any{
							"#f": 5, "#n": "g-0",
							"c-0": []any{
								"^Choice body after conditional. ", "\n",
								nil,
								map[string]any{"#f": 5},
							},
						},
					},
					map[string]any{"#f": 1},
				},
			},
		},
		"listDefs": map[string]any{},
	}
	data, _ := json.Marshal(ink)
	result, err := Parse(data, "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	// flg:3 has 0x2 bit → should extract "\"Yes?\"" from s sub
	foundChoice := false
	for _, b := range result.Blocks {
		if b.Text == "\"Yes?\"" {
			foundChoice = true
			break
		}
	}
	if !foundChoice {
		texts := blockTexts(result.Blocks)
		t.Fatalf("expected choice text '\"Yes?\"' from flg:3, got: %v", texts)
	}
}

func TestParse_ChoiceFlg4_ChoiceOnlyContent(t *testing.T) {
	// flg:4 = 0x4 (choice-only content) from TestingFeatures.txt
	// Text is in ev/str (which we skip), no "s" sub-container
	// This is a KNOWN LIMITATION: flg:4 text is inside ev/str and won't be extracted
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"TestKnot": []any{
					[]any{
						[]any{
							"ev", "str", "^English", "/str", "/ev",
							map[string]any{"*": ".^.c-0", "flg": 4},
						},
						nil,
						map[string]any{
							"#f": 5, "#n": "g-0",
							"c-0": []any{
								"^You chose English. ", "\n",
								nil,
								map[string]any{"#f": 5},
							},
						},
					},
					map[string]any{"#f": 1},
				},
			},
		},
		"listDefs": map[string]any{},
	}
	data, _ := json.Marshal(ink)
	result, err := Parse(data, "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	// flg:4 has NO 0x2 bit → no "s" extraction.
	// Choice text "English" is inside ev/str → skipped.
	// The c-0 body text should still be extracted.
	foundBody := false
	for _, b := range result.Blocks {
		if strings.Contains(b.Text, "You chose English") {
			foundBody = true
		}
		// The choice label "English" should NOT appear as a block (it's in ev/str)
		if b.Text == "English" {
			t.Error("flg:4 choice text 'English' should not be extracted (it's in ev/str)")
		}
	}
	if !foundBody {
		t.Error("expected c-0 body 'You chose English.' to be extracted")
	}
}

func TestParse_ChoiceFlg18_OnceOnlyWithStartContent(t *testing.T) {
	// flg:18 = 0x10|0x2 (once-only + has start content) from CB_Kraaid.txt
	// Has "s" sub, 0x2 bit set → should extract
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"TestKnot": []any{
					[]any{
						[]any{
							"ev", "str", map[string]any{"->": ".^.s"}, "/str", "/ev",
							map[string]any{"*": ".^.^.c-0", "flg": 18},
							map[string]any{
								"s": []any{"^Someone lives here?", map[string]any{"->": "$r", "var": true}, nil},
							},
						},
						nil,
						map[string]any{
							"#f": 5, "#n": "g-0",
							"c-0": []any{
								"^You asked about the resident. ", "\n",
								nil,
								map[string]any{"#f": 5},
							},
						},
					},
					map[string]any{"#f": 1},
				},
			},
		},
		"listDefs": map[string]any{},
	}
	data, _ := json.Marshal(ink)
	result, err := Parse(data, "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	foundChoice := false
	for _, b := range result.Blocks {
		if b.Text == "Someone lives here?" {
			foundChoice = true
			break
		}
	}
	if !foundChoice {
		texts := blockTexts(result.Blocks)
		t.Fatalf("expected choice text 'Someone lives here?' from flg:18, got: %v", texts)
	}
}

func TestParse_ChoiceFlg8_InvisibleDefault(t *testing.T) {
	// flg:8 = 0x8 (invisible default) — no visible text expected
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"TestKnot": []any{
					[]any{
						[]any{
							"ev", "/ev",
							map[string]any{"*": ".^.^.c-0", "flg": 8},
						},
						nil,
						map[string]any{
							"#f": 5, "#n": "g-0",
							"c-0": []any{
								"^Default path. ", "\n",
								nil,
								map[string]any{"#f": 5},
							},
						},
					},
					map[string]any{"#f": 1},
				},
			},
		},
		"listDefs": map[string]any{},
	}
	data, _ := json.Marshal(ink)
	result, err := Parse(data, "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	// flg:8 invisible default: no "s" sub, no 0x2 bit → no choice text
	// But c-0 body should still extract
	foundBody := false
	for _, b := range result.Blocks {
		if strings.Contains(b.Text, "Default path") {
			foundBody = true
		}
	}
	if !foundBody {
		t.Error("expected c-0 body 'Default path.' even for invisible default choice")
	}
}

// =============================================================================
// Cross-divert glue (D-19) — KNOWN LIMITATION
// =============================================================================

func TestParse_GlueAcrossDivert_KnownLimitation(t *testing.T) {
	// Pattern from TS_Braxo.txt:
	//   "^\"Now me, ", "<>", "\n", {"->":".^.^.g-0"}, ...
	//   "g-0": ["<>", "^I do scribin' most days...", ...]
	// The game renders: "Now me, I do scribin' most days..."
	// Current parser does NOT follow diverts → produces 2 separate blocks.
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"TestKnot": []any{
					[]any{
						[]any{
							"^\"Now me, ", "<>", "\n",
							map[string]any{"->": ".^.^.g-0"},
							nil,
							map[string]any{"#f": 5, "#n": "c-0"},
						},
						nil,
						map[string]any{
							"#f": 5, "#n": "g-0",
							"g-0": []any{
								"<>", "^I do scribin' most days.\" ",
								"#", "^Braxo", "/#",
								"\n",
								nil,
								map[string]any{"#f": 5},
							},
						},
					},
					map[string]any{"#f": 1},
				},
			},
		},
		"listDefs": map[string]any{},
	}
	data, _ := json.Marshal(ink)
	result, err := Parse(data, "test")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// KNOWN LIMITATION: cross-divert glue is NOT joined (D-19, D-23 conflict).
	// Parser produces two separate blocks instead of one merged block.
	// This test documents the current behavior. If cross-divert glue is
	// implemented later, update this test to expect merged text.
	foundNowMe := false
	foundScribin := false
	for _, b := range result.Blocks {
		if strings.Contains(b.Text, "Now me") {
			foundNowMe = true
		}
		if strings.Contains(b.Text, "scribin") {
			foundScribin = true
		}
	}
	if !foundNowMe {
		t.Error("expected block containing 'Now me'")
	}
	if !foundScribin {
		t.Error("expected block containing 'scribin'")
	}

	// Verify they are NOT merged (documenting current limitation)
	for _, b := range result.Blocks {
		if strings.Contains(b.Text, "Now me") && strings.Contains(b.Text, "scribin") {
			t.Log("NOTE: cross-divert glue IS working — update test expectations")
			return
		}
	}
	t.Log("KNOWN LIMITATION: cross-divert glue not joined (10 files, ~34 markers affected)")
}

// =============================================================================
// Real file integration tests for glue and flg variants
// =============================================================================

func TestParse_RealFile_TS_Braxo_HasBlocks(t *testing.T) {
	data, err := readTextAsset("TS_Braxo.txt")
	if err != nil {
		t.Skipf("real file not available: %v", err)
	}
	result, err := Parse(data, "TS_Braxo")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(result.Blocks) == 0 {
		t.Fatal("expected blocks from TS_Braxo, got 0")
	}
	t.Logf("TS_Braxo: %d blocks, %d text entries", len(result.Blocks), result.TotalTextEntries)

	// NOTE: "Braxo" is classified as a tag, not Speaker, because isSpeakerTag
	// only recognizes literal "speaker" and ability scores (wis, str, etc.).
	// Character names (Braxo, Snell, Meek) go to Tags[]. This is a known
	// metadata quality gap — does not affect block extraction correctness.
	foundBraxoTag := false
	for _, b := range result.Blocks {
		for _, tag := range b.Tags {
			if tag == "Braxo" {
				foundBraxoTag = true
				break
			}
		}
	}
	if !foundBraxoTag {
		t.Error("expected at least one block with 'Braxo' in Tags")
	}
}

func TestParse_RealFile_TestingFeatures_Flg4(t *testing.T) {
	data, err := readTextAsset("TestingFeatures.txt")
	if err != nil {
		t.Skipf("real file not available: %v", err)
	}
	result, err := Parse(data, "TestingFeatures")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(result.Blocks) == 0 {
		t.Fatal("expected blocks from TestingFeatures, got 0")
	}
	t.Logf("TestingFeatures: %d blocks, %d text entries", len(result.Blocks), result.TotalTextEntries)
}

func TestParse_RealFile_CB_Kraaid_Flg18(t *testing.T) {
	data, err := readTextAsset("CB_Kraaid.txt")
	if err != nil {
		t.Skipf("real file not available: %v", err)
	}
	result, err := Parse(data, "CB_Kraaid")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(result.Blocks) == 0 {
		t.Fatal("expected blocks from CB_Kraaid, got 0")
	}
	t.Logf("CB_Kraaid: %d blocks, %d text entries", len(result.Blocks), result.TotalTextEntries)

	// Should have choice blocks with "Someone lives down in the sewers?" etc.
	foundSewer := false
	for _, b := range result.Blocks {
		if strings.Contains(b.Text, "sewers") {
			foundSewer = true
			break
		}
	}
	if !foundSewer {
		t.Error("expected block mentioning 'sewers' from flg:18 choices in CB_Kraaid")
	}
}

// =============================================================================
// Classifier boundary cases
// =============================================================================

func TestClassifyBoundary_Exactly50Chars(t *testing.T) {
	// 50 runes exactly — should NOT be UI (condition is < 50)
	text := strings.Repeat("A", 50)
	block := &DialogueBlock{SourceFile: "UnknownFile", Text: text}
	got := Classify(block)
	if got == ContentUI {
		t.Errorf("50-char text should not be UI (threshold is < 50), got %q", got)
	}
}

func TestClassifyBoundary_49Chars(t *testing.T) {
	text := strings.Repeat("A", 49)
	block := &DialogueBlock{SourceFile: "UnknownFile", Text: text}
	got := Classify(block)
	if got != ContentUI {
		t.Errorf("49-char text without speaker should be UI, got %q", got)
	}
}

func TestClassifyTagConflict_SpellAndItem(t *testing.T) {
	// Both spell and item tags — spell takes priority (checked first)
	block := &DialogueBlock{
		SourceFile: "Enc_Test",
		Tags:       []string{"spell", "OBJ"},
	}
	got := Classify(block)
	if got != ContentSpell {
		t.Errorf("spell tag should take priority over item tag, got %q", got)
	}
}

func TestClassifyEmptySourceFile(t *testing.T) {
	block := &DialogueBlock{SourceFile: "", Text: "Some long dialogue text that is over fifty chars for sure."}
	got := Classify(block)
	// Should not panic; defaults to dialogue
	if got != ContentDialogue {
		t.Errorf("empty source file with long text should default to dialogue, got %q", got)
	}
}

// =============================================================================
// Passthrough edge cases
// =============================================================================

func TestPassthroughNumbersOnly(t *testing.T) {
	// "12345" — has digits but no letters. isPunctOnly returns false for digits.
	// This should NOT be passthrough (could be a game stat or label)
	if IsPassthrough("12345") {
		t.Error("numbers-only '12345' should NOT be passthrough")
	}
}

func TestPassthroughSingleLetter(t *testing.T) {
	// "A" — single letter, too short to be meaningful
	// Current implementation: not passthrough (has a letter)
	// This documents current behavior
	result := IsPassthrough("A")
	t.Logf("IsPassthrough('A') = %v (documenting behavior)", result)
}

func TestPassthroughUnicodeEllipsis(t *testing.T) {
	// "…" (unicode ellipsis) — should be passthrough (punctuation only)
	if !IsPassthrough("…") {
		t.Error("unicode ellipsis '…' should be passthrough")
	}
}

func TestPassthroughEmDash(t *testing.T) {
	// "—" (em dash) — should be passthrough (punctuation only)
	if !IsPassthrough("—") {
		t.Error("em dash '—' should be passthrough")
	}
}

func TestPassthroughMixedVariableWithText(t *testing.T) {
	// "$name said hello" — has variable syntax but also translatable text
	if IsPassthrough("$name said hello") {
		t.Error("mixed variable with text should NOT be passthrough")
	}
}

// =============================================================================
// Batcher edge cases
// =============================================================================

func TestBuildBatchesNilInput(t *testing.T) {
	batches := BuildBatches(nil)
	if len(batches) != 0 {
		t.Errorf("nil input: expected 0 batches, got %d", len(batches))
	}
}

func TestBuildBatchesEmptyResults(t *testing.T) {
	batches := BuildBatches([]ParseResult{})
	if len(batches) != 0 {
		t.Errorf("empty results: expected 0 batches, got %d", len(batches))
	}
}

func TestBuildBatchesSpellSplitAt10(t *testing.T) {
	blocks := makeBlocks(15, "GT_Test", "knot1", "g-0", ContentSpell)
	results := []ParseResult{{SourceFile: "GT_Test", Blocks: blocks}}
	batches := BuildBatches(results)

	spellBatches := filterBatches(batches, ContentSpell)
	if len(spellBatches) != 2 {
		t.Fatalf("15 spell blocks: want 2 batches (10+5), got %d", len(spellBatches))
	}
	if len(spellBatches[0].Blocks) != 10 {
		t.Errorf("first spell batch: want 10, got %d", len(spellBatches[0].Blocks))
	}
	if len(spellBatches[1].Blocks) != 5 {
		t.Errorf("second spell batch: want 5, got %d", len(spellBatches[1].Blocks))
	}
}

func TestBuildBatchesUISplitAt100(t *testing.T) {
	blocks := makeBlocks(150, "UI_Test", "knot1", "g-0", ContentUI)
	results := []ParseResult{{SourceFile: "UI_Test", Blocks: blocks}}
	batches := BuildBatches(results)

	uiBatches := filterBatches(batches, ContentUI)
	if len(uiBatches) != 2 {
		t.Fatalf("150 UI blocks: want 2 batches (100+50), got %d", len(uiBatches))
	}
	if len(uiBatches[0].Blocks) != 100 {
		t.Errorf("first UI batch: want 100, got %d", len(uiBatches[0].Blocks))
	}
}

func TestBuildBatchesGateMergeOverflow(t *testing.T) {
	// 25 + 25 = 50 > maxBatch(30) → should NOT merge, separate batches
	blocks1 := makeBlocks(25, "TS_Test", "knot1", "g-0", ContentDialogue)
	blocks2 := makeBlocks(25, "TS_Test", "knot1", "g-1", ContentDialogue)
	allBlocks := append(blocks1, blocks2...)
	results := []ParseResult{{SourceFile: "TS_Test", Blocks: allBlocks}}
	batches := BuildBatches(results)

	dialogueBatches := filterBatches(batches, ContentDialogue)
	if len(dialogueBatches) < 2 {
		t.Fatalf("25+25 same knot: should NOT merge (>30), want >=2 batches, got %d", len(dialogueBatches))
	}
}

func TestBuildBatchesSingleBlock(t *testing.T) {
	blocks := makeBlocks(1, "TS_Test", "knot1", "g-0", ContentDialogue)
	results := []ParseResult{{SourceFile: "TS_Test", Blocks: blocks}}
	batches := BuildBatches(results)

	if len(batches) != 1 {
		t.Fatalf("single block: want 1 batch, got %d", len(batches))
	}
	if len(batches[0].Blocks) != 1 {
		t.Errorf("single block batch: want 1 block, got %d", len(batches[0].Blocks))
	}
}

func TestBuildBatchesMixedContentTypes(t *testing.T) {
	blocks := []DialogueBlock{
		{SourceFile: "TS_Test", Knot: "k1", Gate: "g-0", ContentType: ContentDialogue, Text: "Dialogue text here."},
		{SourceFile: "TS_Test", Knot: "k1", Gate: "g-0", ContentType: ContentSpell, Text: "Spell description text."},
		{SourceFile: "TS_Test", Knot: "k1", Gate: "g-0", ContentType: ContentUI, Text: "OK"},
	}
	results := []ParseResult{{SourceFile: "TS_Test", Blocks: blocks}}
	batches := BuildBatches(results)

	types := make(map[string]bool)
	for _, b := range batches {
		types[b.ContentType] = true
	}
	if !types[ContentDialogue] || !types[ContentSpell] || !types[ContentUI] {
		t.Errorf("expected all 3 content types in batches, got: %v", types)
	}
}

// =============================================================================
// Validate edge cases
// =============================================================================

func TestNormalizeBlockText_StripEPrefix(t *testing.T) {
	got := normalizeBlockText("E-The guard walks away.")
	want := "The guard walks away."
	if got != want {
		t.Errorf("normalizeBlockText E- prefix: got %q, want %q", got, want)
	}
}

func TestNormalizeBlockText_StripConditionalPrefix(t *testing.T) {
	got := normalizeBlockText(".DrummerIntro==1-You hear drums in the distance.")
	want := "You hear drums in the distance."
	if got != want {
		t.Errorf("normalizeBlockText conditional: got %q, want %q", got, want)
	}
}

func TestNormalizeBlockText_StripInkCommand(t *testing.T) {
	got := normalizeBlockText("The battle ends. UpdateEntities")
	want := "The battle ends."
	if got != want {
		t.Errorf("normalizeBlockText ink command: got %q, want %q", got, want)
	}
}

func TestNormalizeBlockText_PlainTextUnchanged(t *testing.T) {
	input := "Hello, adventurer."
	got := normalizeBlockText(input)
	if got != input {
		t.Errorf("normalizeBlockText plain text changed: got %q, want %q", got, input)
	}
}

func TestExtractCaptureDialogueText_InkDialogueWithSpeaker(t *testing.T) {
	// Multi-line: first line is speaker, rest is dialogue
	text := "Braxo\nNow me, I do scribin' most days."
	lines := extractCaptureDialogueText(text, "ink_dialogue")
	if len(lines) == 0 {
		t.Fatal("expected dialogue lines, got none")
	}
	// Speaker "Braxo" should be stripped (short, no punctuation)
	for _, line := range lines {
		if line == "Braxo" {
			t.Error("speaker header 'Braxo' should be stripped")
		}
	}
	found := false
	for _, line := range lines {
		if strings.Contains(line, "scribin") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected dialogue text containing 'scribin', got: %v", lines)
	}
}

func TestExtractCaptureDialogueText_InkChoice(t *testing.T) {
	// Choice with number prefix
	text := "1.   Ask about the sewers"
	lines := extractCaptureDialogueText(text, "ink_choice")
	if len(lines) == 0 {
		t.Fatal("expected lines for choice, got none")
	}
	if lines[0] != "Ask about the sewers" {
		t.Errorf("expected stripped choice text, got %q", lines[0])
	}
}

func TestExtractCaptureDialogueText_DCCheck(t *testing.T) {
	text := "Intelligence dc 8: Success\nYou decipher the runes."
	lines := extractCaptureDialogueText(text, "ink_dialogue")
	if len(lines) == 0 {
		t.Fatal("expected lines, got none")
	}
	for _, line := range lines {
		if strings.Contains(line, "dc 8") {
			t.Error("DC check header should be stripped")
		}
	}
}

func TestExtractCaptureDialogueText_Empty(t *testing.T) {
	lines := extractCaptureDialogueText("", "ink_dialogue")
	if len(lines) != 0 {
		t.Errorf("expected no lines for empty text, got %d", len(lines))
	}
}

func TestLoadCaptureData_FileNotFound(t *testing.T) {
	_, err := LoadCaptureData("/nonexistent/path/capture.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestValidateAgainstCapture_DuplicateEntries(t *testing.T) {
	blocks := []DialogueBlock{{Text: "Hello world"}}
	capture := CaptureData{
		Count: 3,
		Entries: []CaptureEntry{
			{Text: "Hello world", Origin: "ink_dialogue"},
			{Text: "Hello world", Origin: "ink_dialogue"}, // duplicate
			{Text: "Missing", Origin: "ink_dialogue"},
		},
	}
	report := ValidateAgainstCapture(blocks, capture)
	if report.TotalCapture != 3 {
		t.Errorf("TotalCapture = %d, want 3", report.TotalCapture)
	}
	// Both duplicate entries should match
	if report.Matched != 2 {
		t.Errorf("Matched = %d, want 2 (duplicates both match)", report.Matched)
	}
	if report.Unmatched != 1 {
		t.Errorf("Unmatched = %d, want 1", report.Unmatched)
	}
}

// =============================================================================
// Helpers
// =============================================================================

func blockTexts(blocks []DialogueBlock) []string {
	texts := make([]string, len(blocks))
	for i, b := range blocks {
		texts[i] = b.Text
	}
	return texts
}

func readTextAsset(name string) ([]byte, error) {
	return os.ReadFile("../../../projects/esoteric-ebb/extract/1.1.3/ExportedProject/Assets/TextAsset/" + name)
}
