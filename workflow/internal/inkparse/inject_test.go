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
