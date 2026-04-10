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

func TestBuildVoiceSection_WisSpeaker(t *testing.T) {
	section := buildVoiceSection([]string{"wis"})
	if !strings.Contains(section, "침착하고 달관한") {
		t.Errorf("wis voice guide missing, got: %s", section)
	}
}

func TestBuildVoiceSection_StrSpeaker(t *testing.T) {
	section := buildVoiceSection([]string{"str"})
	if !strings.Contains(section, "직선적이고 단순한") {
		t.Errorf("str voice guide missing, got: %s", section)
	}
}

func TestBuildVoiceSection_NonAbilitySpeaker(t *testing.T) {
	section := buildVoiceSection([]string{"Braxo"})
	if section != "" {
		t.Errorf("non-ability speaker should produce empty section, got: %s", section)
	}
}

func TestBuildVoiceSection_MultipleAbilities(t *testing.T) {
	section := buildVoiceSection([]string{"wis", "cha", "wis"})
	if !strings.Contains(section, "침착하고 달관한") {
		t.Error("missing wis guide")
	}
	if !strings.Contains(section, "설득력 있고 감정이 풍부한") {
		t.Error("missing cha guide")
	}
	// No duplicates
	if strings.Count(section, "wis") != 1 {
		t.Error("wis should appear only once (dedup)")
	}
}

func TestBuildScriptPrompt_VoiceInjection(t *testing.T) {
	task := ClusterTask{
		Batch: inkparse.Batch{
			ID:          "test/batch",
			ContentType: inkparse.ContentDialogue,
			Format:      inkparse.FormatScript,
			Blocks: []inkparse.DialogueBlock{
				{ID: "blk-0", Text: "You sense something.", Speaker: "wis"},
			},
		},
	}
	prompt, _ := BuildScriptPrompt(task)
	if !strings.Contains(prompt, "침착하고 달관한") {
		t.Errorf("prompt should contain wis voice guide, got:\n%s", prompt)
	}
}

func TestEstimateTokens(t *testing.T) {
	// "hello" = 5 ASCII chars, 5/4 = 1
	if got := estimateTokens("hello"); got != 1 {
		t.Errorf("estimateTokens(\"hello\") = %d, want 1", got)
	}
	// "안녕하세요" = 5 Korean runes, 5/2 = 2
	if got := estimateTokens("안녕하세요"); got != 2 {
		t.Errorf("estimateTokens(\"안녕하세요\") = %d, want 2", got)
	}
}

func TestBuildScriptPrompt_EstimatedTokens(t *testing.T) {
	task := ClusterTask{
		Batch: inkparse.Batch{
			ID:          "test/batch",
			ContentType: inkparse.ContentDialogue,
			Format:      inkparse.FormatScript,
			Blocks: []inkparse.DialogueBlock{
				{ID: "blk-0", Text: "Hello there."},
			},
		},
	}
	_, meta := BuildScriptPrompt(task)
	if meta.EstimatedTokens <= 0 {
		t.Errorf("meta.EstimatedTokens should be > 0, got %d", meta.EstimatedTokens)
	}
}

// --- Phase 07 Plan 03: Context enrichment tests ---

func TestBuildScriptPrompt_ParentChoiceText(t *testing.T) {
	task := ClusterTask{
		Batch: inkparse.Batch{
			ID:          "test/batch",
			ContentType: inkparse.ContentDialogue,
			Format:      inkparse.FormatScript,
			Blocks: []inkparse.DialogueBlock{
				{ID: "blk-0", Text: "Hello there.", Speaker: "Braxo"},
			},
		},
		ParentChoiceText: "Go to the tavern",
	}

	prompt, _ := BuildScriptPrompt(task)
	if !strings.Contains(prompt, `Player chose:`) {
		t.Errorf("prompt should contain 'Player chose:' when ParentChoiceText is set, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Go to the tavern") {
		t.Errorf("prompt should contain the choice text, got:\n%s", prompt)
	}
}

func TestBuildScriptPrompt_EmptyParentChoiceText(t *testing.T) {
	task := ClusterTask{
		Batch: inkparse.Batch{
			ID:          "test/batch",
			ContentType: inkparse.ContentDialogue,
			Format:      inkparse.FormatScript,
			Blocks: []inkparse.DialogueBlock{
				{ID: "blk-0", Text: "Hello there."},
			},
		},
		ParentChoiceText: "",
	}

	prompt, _ := BuildScriptPrompt(task)
	if strings.Contains(prompt, "Player chose") {
		t.Errorf("prompt should NOT contain 'Player chose' when ParentChoiceText is empty, got:\n%s", prompt)
	}
}

func TestBuildScriptPrompt_NextLines(t *testing.T) {
	task := ClusterTask{
		Batch: inkparse.Batch{
			ID:          "test/batch",
			ContentType: inkparse.ContentDialogue,
			Format:      inkparse.FormatScript,
			Blocks: []inkparse.DialogueBlock{
				{ID: "blk-0", Text: "Hello there."},
			},
		},
		NextLines: []string{"Next line 1", "Next line 2"},
	}

	prompt, _ := BuildScriptPrompt(task)
	if !strings.Contains(prompt, `[N1]`) {
		t.Errorf("prompt should contain [N1] for next lines, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, `[N2]`) {
		t.Errorf("prompt should contain [N2] for next lines, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "다음 게이트") {
		t.Errorf("prompt should contain next gate instruction, got:\n%s", prompt)
	}
}

func TestBuildScriptPrompt_PrevKO(t *testing.T) {
	task := ClusterTask{
		Batch: inkparse.Batch{
			ID:          "test/batch",
			ContentType: inkparse.ContentDialogue,
			Format:      inkparse.FormatScript,
			Blocks: []inkparse.DialogueBlock{
				{ID: "blk-0", Text: "Hello there."},
			},
		},
		PrevKO: []string{"이전 번역 1", "이전 번역 2"},
	}

	prompt, _ := BuildScriptPrompt(task)
	if !strings.Contains(prompt, `[K1]`) {
		t.Errorf("prompt should contain [K1] for prev KO, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, `[K2]`) {
		t.Errorf("prompt should contain [K2] for prev KO, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "이전 한국어 번역") {
		t.Errorf("prompt should contain prev KO instruction, got:\n%s", prompt)
	}
}

func TestBuildScriptPrompt_NextKO(t *testing.T) {
	task := ClusterTask{
		Batch: inkparse.Batch{
			ID:          "test/batch",
			ContentType: inkparse.ContentDialogue,
			Format:      inkparse.FormatScript,
			Blocks: []inkparse.DialogueBlock{
				{ID: "blk-0", Text: "Hello there."},
			},
		},
		NextKO: []string{"다음 번역 1"},
	}

	prompt, _ := BuildScriptPrompt(task)
	if !strings.Contains(prompt, `[NK1]`) {
		t.Errorf("prompt should contain [NK1] for next KO, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "다음 한국어 번역") {
		t.Errorf("prompt should contain next KO instruction, got:\n%s", prompt)
	}
}

func TestBuildScriptPrompt_NamedVoiceCards(t *testing.T) {
	task := ClusterTask{
		Batch: inkparse.Batch{
			ID:          "test/batch",
			ContentType: inkparse.ContentDialogue,
			Format:      inkparse.FormatScript,
			Blocks: []inkparse.DialogueBlock{
				{ID: "blk-0", Text: "Hello there.", Speaker: "Snell"},
			},
		},
		VoiceCards: map[string]string{
			"Snell": "거친 말투, 반말, 전사",
		},
	}

	prompt, _ := BuildScriptPrompt(task)
	if !strings.Contains(prompt, "Named Character Voice Guide") {
		t.Errorf("prompt should contain Named Character Voice Guide section, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Snell") {
		t.Errorf("prompt should contain Snell voice card, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "거친 말투") {
		t.Errorf("prompt should contain Snell's voice guide, got:\n%s", prompt)
	}
}

func TestTrimContextForBudget_Priority(t *testing.T) {
	// Create a task with all context types
	task := ClusterTask{
		Batch: inkparse.Batch{
			ID:          "test/batch",
			ContentType: inkparse.ContentDialogue,
			Format:      inkparse.FormatScript,
			Blocks: []inkparse.DialogueBlock{
				{ID: "blk-0", Text: "Hello there.", Speaker: "Snell"},
			},
		},
		NextLines:        []string{"Next 1", "Next 2", "Next 3"},
		PrevKO:           []string{"이전 1", "이전 2"},
		NextKO:           []string{"다음 1"},
		ParentChoiceText: "Go to the tavern",
		VoiceCards:       map[string]string{"Snell": "거친 말투, 반말, 전사"},
	}

	// With very low budget, continuity should be removed first
	trimmed := trimContextForBudget(task, 50)

	// NextLines (continuity) should be removed first
	if len(trimmed.NextLines) > 0 {
		t.Error("NextLines should be removed when budget is low (priority: continuity removed first)")
	}
	if len(trimmed.PrevKO) > 0 {
		t.Error("PrevKO should be removed when budget is low")
	}
	if len(trimmed.NextKO) > 0 {
		t.Error("NextKO should be removed when budget is low")
	}

	// With more generous budget, only continuity removed
	trimmed2 := trimContextForBudget(task, 200)
	// At least voice cards should survive (last to be removed)
	// This depends on actual token count -- we just verify the function runs
	_ = trimmed2
}

func TestBuildScriptPrompt_FullContextTokens(t *testing.T) {
	task := ClusterTask{
		Batch: inkparse.Batch{
			ID:          "test/batch",
			ContentType: inkparse.ContentDialogue,
			Format:      inkparse.FormatScript,
			Blocks: []inkparse.DialogueBlock{
				{ID: "blk-0", Text: "Hello there.", Speaker: "Snell"},
				{ID: "blk-1", Text: "Good to see you."},
			},
		},
		PrevGateLines:    []string{"Previous gate line."},
		NextLines:        []string{"Next line 1", "Next line 2"},
		PrevKO:           []string{"이전 번역"},
		NextKO:           []string{"다음 번역"},
		ParentChoiceText: "Go to the tavern",
		VoiceCards:       map[string]string{"Snell": "거친 말투, 반말, 전사"},
	}

	_, meta := BuildScriptPrompt(task)
	if meta.EstimatedTokens <= 0 {
		t.Errorf("EstimatedTokens should be > 0 for full context prompt, got %d", meta.EstimatedTokens)
	}
}

func TestBuildScriptPrompt_RAGHints(t *testing.T) {
	task := ClusterTask{
		Batch: inkparse.Batch{
			ID:          "test/batch",
			ContentType: inkparse.ContentDialogue,
			Format:      inkparse.FormatScript,
			Blocks: []inkparse.DialogueBlock{
				{ID: "blk-0", Text: "Welcome to Alfoz.", Speaker: "Braxo"},
			},
		},
		RAGHints: "Alfoz: A small town in the countryside | Norvik: Your home city",
	}

	prompt, _ := BuildScriptPrompt(task)
	if !strings.Contains(prompt, "세계관 정보") {
		t.Errorf("prompt should contain '세계관 정보' when RAGHints is set, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Alfoz: A small town") {
		t.Errorf("prompt should contain RAG hint text, got:\n%s", prompt)
	}
	// RAG should appear in [CONTEXT] block before the --- separator
	ragIdx := strings.Index(prompt, "세계관 정보")
	sepIdx := strings.Index(prompt, "---")
	if ragIdx > sepIdx {
		t.Error("RAG hints should appear before --- separator")
	}
}

func TestBuildScriptPrompt_EmptyRAGHints(t *testing.T) {
	task := ClusterTask{
		Batch: inkparse.Batch{
			ID:          "test/batch",
			ContentType: inkparse.ContentDialogue,
			Format:      inkparse.FormatScript,
			Blocks: []inkparse.DialogueBlock{
				{ID: "blk-0", Text: "Hello."},
			},
		},
		RAGHints: "",
	}

	prompt, _ := BuildScriptPrompt(task)
	if strings.Contains(prompt, "세계관 정보") {
		t.Errorf("prompt should NOT contain '세계관 정보' when RAGHints is empty")
	}
}

func TestTrimContextForBudget_RAGBeforeGlossary(t *testing.T) {
	// Create a task with RAG, glossary, branch, and voice — all context types
	task := ClusterTask{
		Batch: inkparse.Batch{
			ID:          "test/batch",
			ContentType: inkparse.ContentDialogue,
			Format:      inkparse.FormatScript,
			Blocks: []inkparse.DialogueBlock{
				{ID: "blk-0", Text: "Hello there.", Speaker: "Snell"},
			},
		},
		NextLines:        []string{"Next 1", "Next 2", "Next 3"},
		PrevKO:           []string{"이전 1", "이전 2"},
		NextKO:           []string{"다음 1"},
		RAGHints:         "Alfoz: A small town | Norvik: Your home city",
		GlossaryJSON:     `[{"source":"Braxo","target":"Braxo","mode":"preserve"}]`,
		ParentChoiceText: "Go to the tavern",
		VoiceCards:       map[string]string{"Snell": "거친 말투, 반말, 전사"},
	}

	// With very low budget (50 tokens), everything should be stripped
	trimmed := trimContextForBudget(task, 50)

	// RAG should be removed (Phase 2, before glossary)
	if trimmed.RAGHints != "" {
		t.Error("RAGHints should be removed when budget is very low")
	}
	// Glossary should also be removed (Phase 3)
	if trimmed.GlossaryJSON != "" {
		t.Error("GlossaryJSON should be removed when budget is very low")
	}

	// With a moderate budget that allows voice+branch+glossary but not RAG+continuity,
	// verify RAG is removed before glossary
	taskNoCont := task
	taskNoCont.NextLines = nil
	taskNoCont.NextKO = nil
	taskNoCont.PrevKO = nil
	promptNoCont, _ := buildScriptPromptCore(taskNoCont)
	tokensNoCont := estimateTokens(promptNoCont)

	taskNoRAG := taskNoCont
	taskNoRAG.RAGHints = ""
	promptNoRAG, _ := buildScriptPromptCore(taskNoRAG)
	tokensNoRAG := estimateTokens(promptNoRAG)

	// Budget between tokensNoRAG and tokensNoCont means RAG gets trimmed but glossary stays
	if tokensNoRAG < tokensNoCont {
		budget := (tokensNoRAG + tokensNoCont) / 2
		trimmed2 := trimContextForBudget(task, budget)
		if trimmed2.RAGHints != "" {
			t.Error("RAGHints should be removed before GlossaryJSON (D-18 priority)")
		}
		if trimmed2.GlossaryJSON == "" {
			t.Error("GlossaryJSON should survive when budget allows (removed after RAG)")
		}
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
