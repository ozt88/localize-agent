package clustertranslate

import (
	"localize-agent/workflow/internal/inkparse"
	"strings"
	"testing"
)

func TestBuildScriptPrompt_Basic(t *testing.T) {
	task := ClusterTask{
		Batch: inkparse.Batch{
			ID:          "test/knot/g-0/batch-0",
			ContentType: inkparse.ContentDialogue,
			Format:      inkparse.FormatScript,
			Blocks: []inkparse.DialogueBlock{
				{ID: "knot/g-0/blk-0", Text: "Hello there.", Speaker: "Braxo"},
				{ID: "knot/g-0/blk-1", Text: "Good to see you."},
				{ID: "knot/g-0/blk-2", Text: "Let's go.", Choice: "c-0"},
			},
		},
	}

	prompt, meta := BuildScriptPrompt(task)

	// Check numbered line format per D-01/D-02
	if !strings.Contains(prompt, `[01] Braxo: "Hello there."`) {
		t.Errorf("prompt missing speaker format, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, `[02] "Good to see you."`) {
		t.Errorf("prompt missing plain format, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, `[03] [CHOICE] "Let's go."`) {
		t.Errorf("prompt missing choice format, got:\n%s", prompt)
	}

	if meta.LineCount != 3 {
		t.Errorf("meta.LineCount = %d, want 3", meta.LineCount)
	}
	if len(meta.BlockIDOrder) != 3 {
		t.Errorf("meta.BlockIDOrder length = %d, want 3", len(meta.BlockIDOrder))
	}
	if meta.BlockIDOrder[0] != "knot/g-0/blk-0" {
		t.Errorf("meta.BlockIDOrder[0] = %q, want knot/g-0/blk-0", meta.BlockIDOrder[0])
	}
}

func TestBuildScriptPrompt_WithContext(t *testing.T) {
	task := ClusterTask{
		Batch: inkparse.Batch{
			ID:          "test/knot/g-1/batch-0",
			ContentType: inkparse.ContentDialogue,
			Format:      inkparse.FormatScript,
			Blocks: []inkparse.DialogueBlock{
				{ID: "knot/g-1/blk-0", Text: "What happened?"},
			},
		},
		PrevGateLines: []string{"The door opened.", "A gust of wind.", "Silence fell."},
	}

	prompt, _ := BuildScriptPrompt(task)

	// Check [CONTEXT] block per D-03
	if !strings.Contains(prompt, "[CONTEXT]") {
		t.Errorf("prompt missing [CONTEXT] block")
	}
	if !strings.Contains(prompt, "이전 게이트") {
		t.Errorf("prompt missing Korean context instruction")
	}
	if !strings.Contains(prompt, `[C1] "The door opened."`) {
		t.Errorf("prompt missing context line C1")
	}
	if !strings.Contains(prompt, `[C3] "Silence fell."`) {
		t.Errorf("prompt missing context line C3")
	}
	// Context should appear before the numbered lines
	contextIdx := strings.Index(prompt, "[CONTEXT]")
	lineIdx := strings.Index(prompt, "[01]")
	if contextIdx > lineIdx {
		t.Errorf("context should appear before numbered lines")
	}
}

func TestBuildScriptPrompt_Numbering(t *testing.T) {
	blocks := make([]inkparse.DialogueBlock, 12)
	for i := range blocks {
		blocks[i] = inkparse.DialogueBlock{
			ID:   "knot/g-0/blk-" + strings.Repeat("0", 1),
			Text: "Line text.",
		}
	}

	task := ClusterTask{
		Batch: inkparse.Batch{
			ID:          "test/batch",
			ContentType: inkparse.ContentDialogue,
			Format:      inkparse.FormatScript,
			Blocks:      blocks,
		},
	}

	prompt, _ := BuildScriptPrompt(task)

	// Numbers should be zero-padded to 2 digits
	if !strings.Contains(prompt, "[01]") {
		t.Errorf("prompt missing [01] zero-padded")
	}
	if !strings.Contains(prompt, "[12]") {
		t.Errorf("prompt missing [12]")
	}
}

func TestBuildBaseWarmup(t *testing.T) {
	result := BuildBaseWarmup("System prompt here.", "World context.", "Extra rules.", `[{"source":"X","target":"X","mode":"preserve"}]`)

	if !strings.Contains(result, "System prompt here.") {
		t.Error("warmup missing system prompt")
	}
	if !strings.Contains(result, "World context.") {
		t.Error("warmup missing context text")
	}
	if !strings.Contains(result, "## Translation Rules") {
		t.Error("warmup missing rules section")
	}
	if !strings.Contains(result, "Maintain the [NN] line numbers exactly as given") {
		t.Error("warmup missing v2 static rule about line numbers")
	}
	if !strings.Contains(result, "## Glossary") {
		t.Error("warmup missing glossary section")
	}
	if !strings.Contains(result, "Reply with exactly: OK") {
		t.Error("warmup missing OK instruction")
	}
}

func TestV2SectionsContext(t *testing.T) {
	found := false
	for _, r := range v2Sections.Context {
		if strings.Contains(r, "[CONTEXT] lines are for reference only") {
			found = true
		}
	}
	if !found {
		t.Error("v2Sections.Context should contain '[CONTEXT] lines are for reference only' rule")
	}
}

func TestV2SectionsVoice(t *testing.T) {
	found := false
	for _, r := range v2Sections.Voice {
		if strings.Contains(r, "Match the tone and register") {
			found = true
		}
	}
	if !found {
		t.Error("v2Sections.Voice should contain 'Match the tone and register' rule")
	}
}

func TestV2SectionsTask(t *testing.T) {
	expected := []string{
		"Translate the following scene",
		"Preserve speaker labels",
		"Preserve [CHOICE] markers",
	}
	for _, exp := range expected {
		found := false
		for _, r := range v2Sections.Task {
			if strings.Contains(r, exp) {
				found = true
			}
		}
		if !found {
			t.Errorf("v2Sections.Task should contain rule with %q", exp)
		}
	}
	if len(v2Sections.Task) != 3 {
		t.Errorf("v2Sections.Task should have 3 rules, got %d", len(v2Sections.Task))
	}
}

func TestV2SectionsConstraints(t *testing.T) {
	expected := []string{
		"Maintain the [NN] line numbers",
		"Do not add, remove, or merge",
		"All proper nouns",
		"Output only",
	}
	for _, exp := range expected {
		found := false
		for _, r := range v2Sections.Constraints {
			if strings.Contains(r, exp) {
				found = true
			}
		}
		if !found {
			t.Errorf("v2Sections.Constraints should contain rule with %q", exp)
		}
	}
	if len(v2Sections.Constraints) != 4 {
		t.Errorf("v2Sections.Constraints should have 4 rules, got %d", len(v2Sections.Constraints))
	}
}

func TestBuildBaseWarmupSectionHeaders(t *testing.T) {
	output := BuildBaseWarmup("system", "context", "", "")
	headers := []string{"### Context", "### Voice", "### Task", "### Constraints"}
	prevIdx := -1
	for _, h := range headers {
		idx := strings.Index(output, h)
		if idx == -1 {
			t.Errorf("BuildBaseWarmup output missing section header %q", h)
			continue
		}
		if idx <= prevIdx {
			t.Errorf("section header %q not in order (idx=%d, prev=%d)", h, idx, prevIdx)
		}
		prevIdx = idx
	}
}

func TestBuildBaseWarmupContainsAllRules(t *testing.T) {
	output := BuildBaseWarmup("system", "context", "", "")
	allRules := []string{
		"[CONTEXT] lines are for reference only",
		"Match the tone and register",
		"Translate the following scene into Korean",
		"Preserve speaker labels",
		"Preserve [CHOICE] markers",
		"Maintain the [NN] line numbers",
		"Do not add, remove, or merge",
		"All proper nouns",
		"Output only the translated lines",
	}
	for _, rule := range allRules {
		if !strings.Contains(output, rule) {
			t.Errorf("BuildBaseWarmup output missing rule: %q", rule)
		}
	}
}

func TestSectionsToRules(t *testing.T) {
	rules := sectionsToRules(v2Sections)
	if len(rules) != 9 {
		t.Fatalf("sectionsToRules should return 9 rules, got %d", len(rules))
	}
	// Verify order: Context(1) + Voice(1) + Task(3) + Constraints(4)
	if !strings.Contains(rules[0], "[CONTEXT]") {
		t.Error("first rule should be from Context section")
	}
	if !strings.Contains(rules[1], "tone and register") {
		t.Error("second rule should be from Voice section")
	}
	if !strings.Contains(rules[2], "Translate the following") {
		t.Error("third rule should be from Task section")
	}
}

func TestBuildContentSuffix_Dialogue(t *testing.T) {
	suffix := BuildContentSuffix(inkparse.ContentDialogue)
	if !strings.Contains(suffix, "대화") {
		t.Errorf("dialogue suffix missing Korean dialogue instruction: %s", suffix)
	}
}

func TestBuildContentSuffix_Spell(t *testing.T) {
	suffix := BuildContentSuffix(inkparse.ContentSpell)
	if !strings.Contains(suffix, "주문") || !strings.Contains(suffix, "영문 유지") {
		t.Errorf("spell suffix incorrect: %s", suffix)
	}
}

func TestBuildContentSuffix_Item(t *testing.T) {
	suffix := BuildContentSuffix(inkparse.ContentItem)
	if !strings.Contains(suffix, "아이템") {
		t.Errorf("item suffix incorrect: %s", suffix)
	}
}

func TestBuildContentSuffix_UI(t *testing.T) {
	suffix := BuildContentSuffix(inkparse.ContentUI)
	if !strings.Contains(suffix, "UI") {
		t.Errorf("UI suffix incorrect: %s", suffix)
	}
}

func TestBuildContentSuffix_System(t *testing.T) {
	suffix := BuildContentSuffix(inkparse.ContentSystem)
	if !strings.Contains(suffix, "시스템") {
		t.Errorf("system suffix incorrect: %s", suffix)
	}
}

func TestBuildScriptPrompt_PunctuationOnlyExcluded(t *testing.T) {
	task := ClusterTask{
		Batch: inkparse.Batch{
			ID:          "test/batch",
			ContentType: inkparse.ContentDialogue,
			Format:      inkparse.FormatScript,
			Blocks: []inkparse.DialogueBlock{
				{ID: "blk-0", Text: "Hello there."},
				{ID: "blk-1", Text: "..."},  // punctuation-only -> excluded
				{ID: "blk-2", Text: "---"},  // punctuation-only -> excluded
				{ID: "blk-3", Text: "Goodbye."},
			},
		},
	}

	prompt, meta := BuildScriptPrompt(task)

	// Should only have 2 numbered lines (punctuation-only excluded)
	if meta.LineCount != 2 {
		t.Errorf("meta.LineCount = %d, want 2", meta.LineCount)
	}
	if len(meta.ExcludedBlockIDs) != 2 {
		t.Errorf("excluded count = %d, want 2", len(meta.ExcludedBlockIDs))
	}
	if !strings.Contains(prompt, `[01] "Hello there."`) {
		t.Errorf("prompt missing [01] for Hello")
	}
	if !strings.Contains(prompt, `[02] "Goodbye."`) {
		t.Errorf("prompt missing [02] for Goodbye")
	}
	// Should NOT contain the punctuation-only text
	if strings.Contains(prompt, `"..."`) {
		t.Errorf("prompt should not contain punctuation-only block")
	}
}

func TestBuildScriptPrompt_WithGlossary(t *testing.T) {
	task := ClusterTask{
		Batch: inkparse.Batch{
			ID:          "test/batch",
			ContentType: inkparse.ContentDialogue,
			Format:      inkparse.FormatScript,
			Blocks: []inkparse.DialogueBlock{
				{ID: "blk-0", Text: "Hello."},
			},
		},
		GlossaryJSON: `[{"source":"Braxo","target":"Braxo","mode":"preserve"}]`,
	}

	prompt, _ := BuildScriptPrompt(task)

	if !strings.Contains(prompt, "## Batch Glossary") {
		t.Error("prompt missing batch glossary section")
	}
	if !strings.Contains(prompt, "Braxo") {
		t.Error("prompt missing glossary term")
	}
}
