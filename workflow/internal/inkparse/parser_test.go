package inkparse

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

// --- hash tests ---

func TestSourceHash_Known(t *testing.T) {
	h := sha256.Sum256([]byte("hello"))
	want := hex.EncodeToString(h[:])
	got := SourceHash("hello")
	if got != want {
		t.Fatalf("SourceHash(hello) = %s, want %s", got, want)
	}
}

func TestSourceHash_Empty(t *testing.T) {
	h := sha256.Sum256([]byte(""))
	want := hex.EncodeToString(h[:])
	got := SourceHash("")
	if got != want {
		t.Fatalf("SourceHash('') = %s, want %s", got, want)
	}
}

func TestSourceHash_Deterministic(t *testing.T) {
	a := SourceHash("test input")
	b := SourceHash("test input")
	if a != b {
		t.Fatalf("SourceHash not deterministic: %s != %s", a, b)
	}
}

// --- block merging tests (PARSE-01) ---

func TestParse_ConsecutiveTextMerge(t *testing.T) {
	// Two ^text entries in the same container merge into one block
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"TestKnot": []any{
					[]any{
						"^text1 ", "^text2 ",
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
	result, err := Parse(data, "test_file")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	// Should have at least one block with merged text
	found := false
	for _, b := range result.Blocks {
		if b.Text == "text1 text2 \n" || b.Text == "text1 text2 " {
			found = true
			break
		}
	}
	if !found {
		texts := make([]string, len(result.Blocks))
		for i, b := range result.Blocks {
			texts[i] = b.Text
		}
		t.Fatalf("expected merged text 'text1 text2 ', got blocks: %v", texts)
	}
}

func TestParse_NewlineWithinBlock(t *testing.T) {
	// \n between ^text entries does NOT break block
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"TestKnot": []any{
					[]any{
						"^Hello ", "\n", "^World ",
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
	result, err := Parse(data, "test_file")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	found := false
	for _, b := range result.Blocks {
		if b.Text == "Hello \nWorld \n" || b.Text == "Hello \nWorld " {
			found = true
			break
		}
	}
	if !found {
		texts := make([]string, len(result.Blocks))
		for i, b := range result.Blocks {
			texts[i] = b.Text
		}
		t.Fatalf("expected newline within block, got blocks: %v", texts)
	}
}

func TestParse_DivertEndsBlock(t *testing.T) {
	// divert {"->":"target"} ends current block
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"TestKnot": []any{
					[]any{
						"^Before divert. ",
						"\n",
						map[string]any{"->": "somewhere"},
						"^After divert. ",
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
	result, err := Parse(data, "test_file")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(result.Blocks) < 2 {
		t.Fatalf("expected at least 2 blocks (divert should split), got %d", len(result.Blocks))
	}
}

func TestParse_EvStrSkipped(t *testing.T) {
	// ev/str sections are skipped entirely
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"TestKnot": []any{
					[]any{
						"^Visible text. ",
						"ev", "str", "^hidden", "/str", "/ev",
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
	result, err := Parse(data, "test_file")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	for _, b := range result.Blocks {
		if b.Text != "" && contains(b.Text, "hidden") {
			t.Fatalf("ev/str content should be skipped, but block has text: %q", b.Text)
		}
	}
}

// --- branch structure tests (PARSE-02) ---

func TestParse_GateStructure(t *testing.T) {
	// gate container g-N produces blocks with Gate="g-0"
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"TestKnot": []any{
					[]any{
						[]any{
							"^Gate text. ", "\n",
							nil,
							map[string]any{"#f": 5, "#n": "g-0"},
						},
						map[string]any{"#f": 5, "#n": "g-0"},
					},
					map[string]any{"#f": 1},
				},
			},
		},
		"listDefs": map[string]any{},
	}
	data, _ := json.Marshal(ink)
	result, err := Parse(data, "test_file")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	found := false
	for _, b := range result.Blocks {
		if b.Gate == "g-0" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected block with Gate='g-0', got none")
	}
}

func TestParse_ChoiceStructure(t *testing.T) {
	// choice container c-N produces blocks with Choice="c-N"
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"TestKnot": []any{
					[]any{
						[]any{
							"^Gate text. ", "\n",
							nil,
							map[string]any{
								"#f": 5, "#n": "g-0",
								"c-0": []any{
									"^Choice text. ", "\n",
									nil,
									map[string]any{"#f": 5},
								},
							},
						},
						map[string]any{"#f": 5, "#n": "g-0"},
					},
					map[string]any{"#f": 1},
				},
			},
		},
		"listDefs": map[string]any{},
	}
	data, _ := json.Marshal(ink)
	result, err := Parse(data, "test_file")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	found := false
	for _, b := range result.Blocks {
		if b.Choice == "c-0" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected block with Choice='c-0', got none")
	}
}

func TestParse_BlockIDPattern(t *testing.T) {
	// block ID follows pattern "KnotName/g-N/c-N/blk-M"
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"MyKnot": []any{
					[]any{
						[]any{
							"^Text. ", "\n",
							nil,
							map[string]any{
								"#f": 5, "#n": "g-0",
								"c-0": []any{
									"^Choice. ", "\n",
									nil,
									map[string]any{"#f": 5},
								},
							},
						},
						map[string]any{"#f": 5, "#n": "g-0"},
					},
					map[string]any{"#f": 1},
				},
			},
		},
		"listDefs": map[string]any{},
	}
	data, _ := json.Marshal(ink)
	result, err := Parse(data, "test_file")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	foundGateBlock := false
	foundChoiceBlock := false
	for _, b := range result.Blocks {
		if b.ID == "MyKnot/g-0/blk-0" {
			foundGateBlock = true
		}
		if b.ID == "MyKnot/g-0/c-0/blk-0" {
			foundChoiceBlock = true
		}
	}
	if !foundGateBlock {
		ids := make([]string, len(result.Blocks))
		for i, b := range result.Blocks {
			ids[i] = b.ID
		}
		t.Fatalf("expected block ID 'MyKnot/g-0/blk-0', got: %v", ids)
	}
	if !foundChoiceBlock {
		ids := make([]string, len(result.Blocks))
		for i, b := range result.Blocks {
			ids[i] = b.ID
		}
		t.Fatalf("expected block ID 'MyKnot/g-0/c-0/blk-0', got: %v", ids)
	}
}

func TestParse_KnotEntryBeforeGate(t *testing.T) {
	// knot entry section (before first gate) produces blocks with empty Gate
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"TestKnot": []any{
					"^Entry text. ", "\n",
					[]any{
						"^Gate text. ", "\n",
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
	result, err := Parse(data, "test_file")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	found := false
	for _, b := range result.Blocks {
		if b.Gate == "" && b.Knot == "TestKnot" && b.Text != "" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected entry block with empty Gate in TestKnot")
	}
}

// --- metadata tests (PARSE-03) ---

func TestParse_SpeakerTag(t *testing.T) {
	// "#" + "^speaker " + "/#" sets Speaker
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"TestKnot": []any{
					[]any{
						"^Hello world. ",
						"#", "^speaker ", "/#",
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
	result, err := Parse(data, "test_file")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	found := false
	for _, b := range result.Blocks {
		if b.Speaker == "speaker" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected block with Speaker='speaker'")
	}
}

func TestParse_DCCheckTag(t *testing.T) {
	// "#" + "^DC10" + "/#" adds "DC10" to Tags
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"TestKnot": []any{
					[]any{
						"^Some text. ",
						"#", "^DC10", "/#",
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
	result, err := Parse(data, "test_file")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	found := false
	for _, b := range result.Blocks {
		for _, tag := range b.Tags {
			if tag == "DC10" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("expected block with tag 'DC10'")
	}
}

func TestParse_MultipleTags(t *testing.T) {
	// Multiple tags on same block all captured
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"TestKnot": []any{
					[]any{
						"^Tagged text. ",
						"#", "^OBJ ", "/#",
						"#", "^XPGain ", "/#",
						"#", "^Minor", "/#",
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
	result, err := Parse(data, "test_file")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	for _, b := range result.Blocks {
		if b.Text != "" {
			tagSet := map[string]bool{}
			for _, tag := range b.Tags {
				tagSet[tag] = true
			}
			if !tagSet["OBJ"] || !tagSet["XPGain"] || !tagSet["Minor"] {
				t.Fatalf("expected tags [OBJ, XPGain, Minor], got: %v", b.Tags)
			}
			return
		}
	}
	t.Fatalf("no blocks with text found")
}

func TestParse_OBJTag(t *testing.T) {
	// "#" + "^OBJ" + "/#" sets tag "OBJ"
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"TestKnot": []any{
					[]any{
						"^Object description. ",
						"#", "^OBJ", "/#",
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
	result, err := Parse(data, "test_file")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	found := false
	for _, b := range result.Blocks {
		for _, tag := range b.Tags {
			if tag == "OBJ" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected block with tag 'OBJ'")
	}
}

// --- glue tests ---

func TestParse_GlueJoinsText(t *testing.T) {
	// "<>" marker between text entries joins them without extra separator
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"TestKnot": []any{
					[]any{
						"^First part. ", "<>", "^Second part.",
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
	result, err := Parse(data, "test_file")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	found := false
	for _, b := range result.Blocks {
		if b.Text == "First part. Second part.\n" || b.Text == "First part. Second part." {
			found = true
			break
		}
	}
	if !found {
		texts := make([]string, len(result.Blocks))
		for i, b := range result.Blocks {
			texts[i] = b.Text
		}
		t.Fatalf("expected glue-joined text 'First part. Second part.', got: %v", texts)
	}
}

// --- choice flag tests (D-26) ---

func TestParse_ChoiceFlg2_StartContent(t *testing.T) {
	// flg=2 (has start content) -> choice text from "s" sub-container
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
							map[string]any{"*": ".^.^.c-0", "flg": 2},
							map[string]any{
								"s": []any{"^(Choose me.)", nil},
							},
						},
						nil,
						map[string]any{
							"#f": 5, "#n": "g-0",
							"c-0": []any{
								"^Choice body. ", "\n",
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
	result, err := Parse(data, "test_file")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	// Should find a choice block with text "(Choose me.)" stripped of ^ prefix
	foundChoice := false
	for _, b := range result.Blocks {
		if b.Text == "(Choose me.)" {
			foundChoice = true
			break
		}
	}
	if !foundChoice {
		texts := make([]string, len(result.Blocks))
		for i, b := range result.Blocks {
			texts[i] = b.Text
		}
		t.Fatalf("expected choice text '(Choose me.)', got: %v", texts)
	}
}

func TestParse_SourceHash_SHA256(t *testing.T) {
	// Blocks should have SHA-256 hashes
	ink := map[string]any{
		"inkVersion": 21,
		"root": []any{
			[]any{"done", map[string]any{"#f": 5, "#n": "g-0"}},
			nil,
			map[string]any{
				"TestKnot": []any{
					[]any{
						"^Test text. ", "\n",
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
	result, err := Parse(data, "test_file")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	for _, b := range result.Blocks {
		if b.Text != "" {
			if b.SourceHash == "" {
				t.Fatalf("block has empty SourceHash")
			}
			if len(b.SourceHash) != 64 {
				t.Fatalf("expected 64-char SHA-256 hex, got %d chars: %s", len(b.SourceHash), b.SourceHash)
			}
			expected := SourceHash(b.Text)
			if b.SourceHash != expected {
				t.Fatalf("SourceHash mismatch: got %s, want %s", b.SourceHash, expected)
			}
			return
		}
	}
	t.Fatalf("no blocks with text found")
}

// --- integration test with real file ---

func TestParse_RealFile_AR_CoastMap(t *testing.T) {
	data, err := os.ReadFile("../../../projects/esoteric-ebb/extract/1.1.3/ExportedProject/Assets/TextAsset/AR_CoastMap.txt")
	if err != nil {
		t.Skipf("real file not available: %v", err)
	}
	result, err := Parse(data, "AR_CoastMap")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(result.Blocks) == 0 {
		t.Fatalf("expected blocks from AR_CoastMap, got 0")
	}
	t.Logf("AR_CoastMap: %d blocks, %d text entries", len(result.Blocks), result.TotalTextEntries)
	// Verify some known content exists
	foundCoastMap := false
	for _, b := range result.Blocks {
		if b.Knot == "CoastMap" {
			foundCoastMap = true
		}
		// Every block must have non-empty fields
		if b.Text == "" {
			t.Errorf("block %s has empty text", b.ID)
		}
		if b.SourceHash == "" {
			t.Errorf("block %s has empty hash", b.ID)
		}
		if b.ID == "" {
			t.Errorf("block has empty ID")
		}
		if len(b.SourceHash) != 64 {
			t.Errorf("block %s hash length %d != 64", b.ID, len(b.SourceHash))
		}
	}
	if !foundCoastMap {
		t.Fatalf("expected CoastMap knot")
	}
}

// helper
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
