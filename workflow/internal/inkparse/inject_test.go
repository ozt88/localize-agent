package inkparse

import (
	"encoding/json"
	"strings"
	"testing"
)

// buildMinimalInkJSON creates a minimal ink JSON with a single knot containing text nodes.
func buildMinimalInkJSON(knots map[string][]any) []byte {
	knotDict := map[string]any{}
	for name, arr := range knots {
		knotDict[name] = arr
	}
	root := map[string]any{
		"inkVersion": 21,
		"root": []any{
			knotDict,
			nil,
		},
	}
	data, _ := json.Marshal(root)
	return data
}

func TestInjectSingleBlock(t *testing.T) {
	// One knot with one "^text" node
	knots := map[string][]any{
		"TestKnot": {
			"^Hello world\n",
			nil,
			map[string]any{"#n": "g-0", "#f": 5},
		},
	}
	data := buildMinimalInkJSON(knots)

	// Compute expected hash
	hash := SourceHash("Hello world\n")
	translations := map[string]string{
		hash: "안녕하세요 세계\n",
	}

	out, report, err := InjectTranslations(data, "test", translations)
	if err != nil {
		t.Fatalf("InjectTranslations error: %v", err)
	}
	if report.Total != 1 {
		t.Errorf("Total: got %d, want 1", report.Total)
	}
	if report.Replaced != 1 {
		t.Errorf("Replaced: got %d, want 1", report.Replaced)
	}
	if report.Missing != 0 {
		t.Errorf("Missing: got %d, want 0", report.Missing)
	}

	// Verify the output contains Korean text
	outStr := string(out)
	if !strings.Contains(outStr, "안녕하세요 세계") {
		t.Errorf("output does not contain Korean text: %s", outStr)
	}
}

func TestInjectMultiNodeBlock(t *testing.T) {
	// Two consecutive "^text" nodes that form a single block
	knots := map[string][]any{
		"TestKnot": {
			"^Hello ",
			"^world\n",
			nil,
			map[string]any{"#n": "g-0", "#f": 5},
		},
	}
	data := buildMinimalInkJSON(knots)

	// Merged text is "Hello world\n"
	hash := SourceHash("Hello world\n")
	translations := map[string]string{
		hash: "안녕하세요 세계\n",
	}

	out, report, err := InjectTranslations(data, "test", translations)
	if err != nil {
		t.Fatalf("InjectTranslations error: %v", err)
	}
	if report.Total != 1 {
		t.Errorf("Total: got %d, want 1", report.Total)
	}
	if report.Replaced != 1 {
		t.Errorf("Replaced: got %d, want 1", report.Replaced)
	}

	// Parse output to verify structure
	var root map[string]any
	trimmed := out[3:] // skip BOM
	if err := json.Unmarshal(trimmed, &root); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	rootArr := root["root"].([]any)
	knotDict := rootArr[0].(map[string]any)
	knotArr := knotDict["TestKnot"].([]any)

	// First node should have full Korean text, second should be "^"
	first, ok := knotArr[0].(string)
	if !ok || first != "^안녕하세요 세계\n" {
		t.Errorf("first node: got %q, want %q", first, "^안녕하세요 세계\n")
	}
	second, ok := knotArr[1].(string)
	if !ok || second != "^" {
		t.Errorf("second node: got %q, want %q", second, "^")
	}
}

func TestInjectMissingTranslation(t *testing.T) {
	knots := map[string][]any{
		"TestKnot": {
			"^Some text\n",
			nil,
			map[string]any{"#n": "g-0", "#f": 5},
		},
	}
	data := buildMinimalInkJSON(knots)

	// Empty translations map
	translations := map[string]string{}

	out, report, err := InjectTranslations(data, "test", translations)
	if err != nil {
		t.Fatalf("InjectTranslations error: %v", err)
	}
	if report.Total != 1 {
		t.Errorf("Total: got %d, want 1", report.Total)
	}
	if report.Missing != 1 {
		t.Errorf("Missing: got %d, want 1", report.Missing)
	}
	if report.Replaced != 0 {
		t.Errorf("Replaced: got %d, want 0", report.Replaced)
	}

	// Original text should be preserved
	outStr := string(out)
	if !strings.Contains(outStr, "Some text") {
		t.Errorf("output should preserve original text")
	}
}

func TestInjectMixedResults(t *testing.T) {
	// Two blocks: one with translation, one without
	knots := map[string][]any{
		"TestKnot": {
			"^First block\n",
			map[string]any{"->": "somewhere"}, // divert flushes first block
			"^Second block\n",
			nil,
			map[string]any{"#n": "g-0", "#f": 5},
		},
	}
	data := buildMinimalInkJSON(knots)

	hash1 := SourceHash("First block\n")
	translations := map[string]string{
		hash1: "첫 번째 블록\n",
	}

	_, report, err := InjectTranslations(data, "test", translations)
	if err != nil {
		t.Fatalf("InjectTranslations error: %v", err)
	}
	if report.Total != 2 {
		t.Errorf("Total: got %d, want 2", report.Total)
	}
	if report.Replaced != 1 {
		t.Errorf("Replaced: got %d, want 1", report.Replaced)
	}
	if report.Missing != 1 {
		t.Errorf("Missing: got %d, want 1", report.Missing)
	}
}

func TestInjectEmptyTranslationsMap(t *testing.T) {
	knots := map[string][]any{
		"TestKnot": {
			"^Hello\n",
			"^World\n",
			nil,
			map[string]any{"#n": "g-0", "#f": 5},
		},
	}
	data := buildMinimalInkJSON(knots)

	_, report, err := InjectTranslations(data, "test", map[string]string{})
	if err != nil {
		t.Fatalf("InjectTranslations error: %v", err)
	}
	if report.Replaced != 0 {
		t.Errorf("Replaced: got %d, want 0", report.Replaced)
	}
}

func TestInjectBOMHandling(t *testing.T) {
	knots := map[string][]any{
		"TestKnot": {
			"^Hello\n",
			nil,
			map[string]any{"#n": "g-0", "#f": 5},
		},
	}
	data := buildMinimalInkJSON(knots)

	// Add BOM to input
	bomData := append([]byte{0xEF, 0xBB, 0xBF}, data...)

	out, _, err := InjectTranslations(bomData, "test", map[string]string{})
	if err != nil {
		t.Fatalf("InjectTranslations error: %v", err)
	}

	// Output should have BOM
	if len(out) < 3 || out[0] != 0xEF || out[1] != 0xBB || out[2] != 0xBF {
		t.Errorf("output should start with BOM")
	}

	// Should be valid JSON after stripping BOM
	var root map[string]any
	if err := json.Unmarshal(out[3:], &root); err != nil {
		t.Fatalf("output after BOM is not valid JSON: %v", err)
	}
}

func TestInjectStructurePreservation(t *testing.T) {
	knots := map[string][]any{
		"TestKnot": {
			"^Hello\n",
			nil,
			map[string]any{"#n": "g-0", "#f": 5},
		},
	}
	data := buildMinimalInkJSON(knots)

	hash := SourceHash("Hello\n")
	translations := map[string]string{
		hash: "안녕\n",
	}

	out, _, err := InjectTranslations(data, "test", translations)
	if err != nil {
		t.Fatalf("InjectTranslations error: %v", err)
	}

	// Parse output and verify structure
	var outRoot map[string]any
	if err := json.Unmarshal(out[3:], &outRoot); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	// inkVersion should be preserved
	if v, ok := outRoot["inkVersion"]; !ok || v != float64(21) {
		t.Errorf("inkVersion not preserved")
	}

	// root array should exist
	rootArr, ok := outRoot["root"].([]any)
	if !ok {
		t.Fatalf("root array missing")
	}

	// Knot dict should have TestKnot
	knotDict, ok := rootArr[0].(map[string]any)
	if !ok {
		t.Fatalf("knot dict missing")
	}
	if _, ok := knotDict["TestKnot"]; !ok {
		t.Errorf("TestKnot missing from output")
	}
}

func TestInjectGateChoiceContainers(t *testing.T) {
	// Knot with a gate containing a choice
	knots := map[string][]any{
		"TestKnot": {
			"^Gate text\n",
			nil,
			map[string]any{
				"#n": "g-0",
				"#f": 5,
				"c-0": []any{
					"^Choice text\n",
					nil,
					map[string]any{"#n": "c-0", "#f": 5},
				},
			},
		},
	}
	data := buildMinimalInkJSON(knots)

	hashGate := SourceHash("Gate text\n")
	hashChoice := SourceHash("Choice text\n")
	translations := map[string]string{
		hashGate:   "게이트 텍스트\n",
		hashChoice: "선택 텍스트\n",
	}

	out, report, err := InjectTranslations(data, "test", translations)
	if err != nil {
		t.Fatalf("InjectTranslations error: %v", err)
	}
	if report.Total != 2 {
		t.Errorf("Total: got %d, want 2", report.Total)
	}
	if report.Replaced != 2 {
		t.Errorf("Replaced: got %d, want 2", report.Replaced)
	}

	outStr := string(out)
	if !strings.Contains(outStr, "게이트 텍스트") {
		t.Errorf("gate text not replaced")
	}
	if !strings.Contains(outStr, "선택 텍스트") {
		t.Errorf("choice text not replaced")
	}
}

// --- Integration Tests ---

func TestInjectRoundTrip(t *testing.T) {
	// Build a realistic ink JSON with multiple knots, gates, choices
	knots := map[string][]any{
		"SceneOne": {
			"^Welcome to the tavern.\n",
			map[string]any{"->": "SceneOne.g-0"},
			nil,
			map[string]any{
				"#n": "g-0",
				"#f": 5,
				"c-0": []any{
					"^Order a drink.\n",
					nil,
					map[string]any{"#n": "c-0", "#f": 5},
				},
			},
		},
		"SceneTwo": {
			"^The night grows dark.\n",
			"^You feel uneasy.\n",
			nil,
			map[string]any{"#n": "g-0", "#f": 5},
		},
	}
	data := buildMinimalInkJSON(knots)

	// Parse original to get blocks
	origResult, err := Parse(data, "test")
	if err != nil {
		t.Fatalf("Parse original: %v", err)
	}
	if len(origResult.Blocks) == 0 {
		t.Fatalf("Parse returned 0 blocks")
	}

	// Build translations map: source_hash -> "KO_" + original text
	translations := map[string]string{}
	for _, block := range origResult.Blocks {
		translations[block.SourceHash] = "KO_" + block.Text
	}

	// Inject translations
	out, report, err := InjectTranslations(data, "test", translations)
	if err != nil {
		t.Fatalf("InjectTranslations: %v", err)
	}

	// Verify report
	if report.Replaced != len(origResult.Blocks) {
		t.Errorf("Replaced: got %d, want %d", report.Replaced, len(origResult.Blocks))
	}
	if report.Missing != 0 {
		t.Errorf("Missing: got %d, want 0", report.Missing)
	}

	// Re-parse injected output
	injResult, err := Parse(out, "test")
	if err != nil {
		t.Fatalf("Parse injected: %v", err)
	}

	// Same number of blocks
	if len(injResult.Blocks) != len(origResult.Blocks) {
		t.Fatalf("block count: got %d, want %d", len(injResult.Blocks), len(origResult.Blocks))
	}

	// Build sets of IDs from both parses
	origIDs := map[string]bool{}
	for _, block := range origResult.Blocks {
		origIDs[block.ID] = true
	}
	injIDs := map[string]bool{}
	for _, block := range injResult.Blocks {
		injIDs[block.ID] = true
		// Each output block's text should start with "KO_"
		if !strings.HasPrefix(block.Text, "KO_") {
			t.Errorf("block %s: text %q does not start with KO_", block.ID, block.Text)
		}
	}

	// All original IDs should exist in injected output
	for id := range origIDs {
		if !injIDs[id] {
			t.Errorf("original block ID %q missing from injected output", id)
		}
	}
	for id := range injIDs {
		if !origIDs[id] {
			t.Errorf("injected output has unexpected block ID %q", id)
		}
	}
}

func TestInjectPreservesNonTextStructure(t *testing.T) {
	// Create ink JSON with diverts, eval blocks, and tags
	knots := map[string][]any{
		"TestKnot": {
			"ev",
			"^some eval content",
			"/ev",
			"^Visible text\n",
			"#", "^speaker:Braxo", "/#",
			map[string]any{"->": "OtherKnot"},
			nil,
			map[string]any{"#n": "g-0", "#f": 5},
		},
	}
	data := buildMinimalInkJSON(knots)

	hash := SourceHash("Visible text\n")
	translations := map[string]string{
		hash: "보이는 텍스트\n",
	}

	out, _, err := InjectTranslations(data, "test", translations)
	if err != nil {
		t.Fatalf("InjectTranslations error: %v", err)
	}

	// Parse output and verify non-text elements preserved
	var outRoot map[string]any
	if err := json.Unmarshal(out[3:], &outRoot); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	rootArr := outRoot["root"].([]any)
	knotDict := rootArr[0].(map[string]any)
	knotArr := knotDict["TestKnot"].([]any)

	// Check that "ev", "/ev" still exist
	foundEv := false
	foundEvEnd := false
	foundDivert := false
	foundTag := false
	for _, elem := range knotArr {
		switch v := elem.(type) {
		case string:
			if v == "ev" {
				foundEv = true
			}
			if v == "/ev" {
				foundEvEnd = true
			}
			if v == "#" {
				foundTag = true
			}
		case map[string]any:
			if _, ok := v["->"]; ok {
				foundDivert = true
			}
		}
	}

	if !foundEv {
		t.Errorf("'ev' not preserved")
	}
	if !foundEvEnd {
		t.Errorf("'/ev' not preserved")
	}
	if !foundDivert {
		t.Errorf("divert not preserved")
	}
	if !foundTag {
		t.Errorf("tag marker '#' not preserved")
	}
}

func TestInjectCountByState(t *testing.T) {
	// 3 blocks, translations for only 2
	knots := map[string][]any{
		"KnotA": {
			"^Block one.\n",
			map[string]any{"->": "KnotA.g-0"},
			"^Block two.\n",
			map[string]any{"->": "KnotA.g-1"},
			"^Block three.\n",
			nil,
			map[string]any{"#n": "g-0", "#f": 5},
		},
	}
	data := buildMinimalInkJSON(knots)

	hash1 := SourceHash("Block one.\n")
	hash3 := SourceHash("Block three.\n")
	translations := map[string]string{
		hash1: "블록 1.\n",
		hash3: "블록 3.\n",
	}

	_, report, err := InjectTranslations(data, "test", translations)
	if err != nil {
		t.Fatalf("InjectTranslations error: %v", err)
	}
	if report.Total != 3 {
		t.Errorf("Total: got %d, want 3", report.Total)
	}
	if report.Replaced != 2 {
		t.Errorf("Replaced: got %d, want 2", report.Replaced)
	}
	if report.Missing != 1 {
		t.Errorf("Missing: got %d, want 1", report.Missing)
	}
}
