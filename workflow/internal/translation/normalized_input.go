package translation

import (
	"regexp"
	"strings"
)

type normalizedPromptInput struct {
	ID               string `json:"id"`
	EN               string `json:"en"`
	Glossary         []glossaryEntry `json:"glossary,omitempty"`
	LoreContext      string          `json:"lore_context,omitempty"`
	ContextEN        string `json:"context_en,omitempty"`
	FocusedContextEN string `json:"focused_context_en,omitempty"`
	FragmentPattern  string `json:"fragment_pattern,omitempty"`
	ActionCueEN      string `json:"action_cue_en,omitempty"`
	SpokenFragmentEN string `json:"spoken_fragment_en,omitempty"`
	StructurePattern string `json:"structure_pattern,omitempty"`
	LeadTermEN       string `json:"lead_term_en,omitempty"`
	DefinitionBodyEN string `json:"definition_body_en,omitempty"`
	StatCheck        string `json:"stat_check,omitempty"`
	CurrentKO        string `json:"current_ko,omitempty"`
	PrevEN           string `json:"prev_en,omitempty"`
	NextEN           string `json:"next_en,omitempty"`
	PrevKO           string `json:"prev_ko,omitempty"`
	NextKO           string `json:"next_ko,omitempty"`
	TextRole         string `json:"text_role,omitempty"`
	SpeakerHint      string `json:"speaker_hint,omitempty"`
	RetryReason      string `json:"retry_reason,omitempty"`
	SourceFile       string `json:"source_file,omitempty"`
	ResourceKey      string `json:"resource_key,omitempty"`
	SceneHint        string `json:"scene_hint,omitempty"`
}

type normalizedBatchPromptItem struct {
	ID          string `json:"id"`
	ContextIdx  int    `json:"ctx"`
	Line        int    `json:"line"`
	EN          string `json:"en"`
	Glossary    []glossaryEntry `json:"glossary,omitempty"`
	LoreContext string          `json:"lore_context,omitempty"`
	FragmentPattern  string `json:"fragment_pattern,omitempty"`
	ActionCueEN      string `json:"action_cue_en,omitempty"`
	SpokenFragmentEN string `json:"spoken_fragment_en,omitempty"`
	StructurePattern string `json:"structure_pattern,omitempty"`
	LeadTermEN       string `json:"lead_term_en,omitempty"`
	DefinitionBodyEN string `json:"definition_body_en,omitempty"`
	StatCheck   string `json:"stat_check,omitempty"`
	CurrentKO   string `json:"current_ko,omitempty"`
	PrevKO      string `json:"prev_ko,omitempty"`
	NextKO      string `json:"next_ko,omitempty"`
	TextRole    string `json:"text_role,omitempty"`
	SpeakerHint string `json:"speaker_hint,omitempty"`
	RetryReason string `json:"retry_reason,omitempty"`
	SourceFile  string `json:"source_file,omitempty"`
	ResourceKey string `json:"resource_key,omitempty"`
	SceneHint   string `json:"scene_hint,omitempty"`
}

type normalizedBatchPromptPayload struct {
	Contexts [][]string                  `json:"contexts"`
	Items    []normalizedBatchPromptItem `json:"items"`
}

func normalizePromptInput(task translationTask) normalizedPromptInput {
	promptEN := renderPromptEN(task)
	promptContextEN := renderPromptContextEN(task, promptEN)
	fragmentPattern, actionCueEN, spokenFragmentEN := deriveFragmentHints(task)
	structurePattern, leadTermEN, definitionBodyEN := deriveStructureHints(task)
	out := normalizedPromptInput{
		ID:               task.ID,
		EN:               promptEN,
		Glossary:         task.Glossary,
		LoreContext:      formatLoreHints(task.LoreHints),
		ContextEN:        promptContextEN,
		FocusedContextEN: buildFocusedContextEN(promptContextEN, promptEN),
		FragmentPattern:  fragmentPattern,
		ActionCueEN:      actionCueEN,
		SpokenFragmentEN: spokenFragmentEN,
		StructurePattern: structurePattern,
		LeadTermEN:       leadTermEN,
		DefinitionBodyEN: definitionBodyEN,
		StatCheck:        task.StatCheck,
		CurrentKO:        task.CurrentKO,
		PrevEN:           task.PrevEN,
		NextEN:           task.NextEN,
		PrevKO:           task.PrevKO,
		NextKO:           task.NextKO,
		TextRole:         task.TextRole,
		SpeakerHint:      task.SpeakerHint,
		RetryReason:      task.RetryReason,
	}
	if shouldIncludePromptSourceFile(task) {
		out.SourceFile = task.SourceFile
	}
	if shouldIncludePromptResourceKey(task) {
		out.ResourceKey = task.ResourceKey
	}
	if task.SceneHint != "" {
		out.SceneHint = task.SceneHint
	}
	return out
}

func normalizeBatchPrompt(tasks []translationTask) normalizedBatchPromptPayload {
	payload := normalizedBatchPromptPayload{
		Contexts: make([][]string, 0, len(tasks)),
		Items:    make([]normalizedBatchPromptItem, 0, len(tasks)),
	}
	contextIndex := map[string]int{}
	for _, task := range tasks {
		promptEN := renderPromptEN(task)
		promptContextEN := renderPromptContextEN(task, promptEN)
		contextLines, lineIndex := renderPromptContextLines(task, promptEN, promptContextEN)
		fragmentPattern, actionCueEN, spokenFragmentEN := deriveFragmentHints(task)
		structurePattern, leadTermEN, definitionBodyEN := deriveStructureHints(task)
		if len(contextLines) == 0 {
			contextLines = []string{promptEN}
			lineIndex = 0
		}
		if lineIndex < 0 || lineIndex >= len(contextLines) {
			lineIndex = 0
		}
		key := strings.Join(contextLines, "\n")
		ctxIdx, ok := contextIndex[key]
		if !ok {
			ctxIdx = len(payload.Contexts)
			contextIndex[key] = ctxIdx
			payload.Contexts = append(payload.Contexts, contextLines)
		}
		item := normalizedBatchPromptItem{
			ID:               task.ID,
			ContextIdx:       ctxIdx,
			Line:             lineIndex,
			EN:               promptEN,
			Glossary:         task.Glossary,
			LoreContext:      formatLoreHints(task.LoreHints),
			FragmentPattern:  fragmentPattern,
			ActionCueEN:      actionCueEN,
			SpokenFragmentEN: spokenFragmentEN,
			StructurePattern: structurePattern,
			LeadTermEN:       leadTermEN,
			DefinitionBodyEN: definitionBodyEN,
			StatCheck:        task.StatCheck,
			CurrentKO:        task.CurrentKO,
			PrevKO:           task.PrevKO,
			NextKO:           task.NextKO,
			TextRole:         task.TextRole,
			SpeakerHint:      task.SpeakerHint,
			RetryReason:      task.RetryReason,
		}
		if shouldIncludePromptSourceFile(task) {
			item.SourceFile = task.SourceFile
		}
		if shouldIncludePromptResourceKey(task) {
			item.ResourceKey = task.ResourceKey
		}
		if task.SceneHint != "" {
			item.SceneHint = task.SceneHint
		}
		payload.Items = append(payload.Items, item)
	}
	return payload
}

func renderPromptEN(task translationTask) string {
	return task.BodyEN
}

func renderPromptContextEN(task translationTask, promptEN string) string {
	contextEN := task.ContextEN
	if !isChoiceLikePrompt(task) || strings.TrimSpace(contextEN) == "" {
		return contextEN
	}
	lines := strings.Split(contextEN, "\n")
	replaced := false
	bodyEN := strings.TrimSpace(task.BodyEN)
	for idx, line := range lines {
		if strings.TrimSpace(line) == bodyEN {
			lines[idx] = renderChoiceContextAnchor(task, promptEN)
			replaced = true
		}
	}
	if replaced {
		return strings.Join(lines, "\n")
	}
	return contextEN
}

func renderPromptContextLines(task translationTask, promptEN, promptContextEN string) ([]string, int) {
	if task.ContextLine >= 0 && len(task.ContextLines) > 0 {
		lines := append([]string(nil), task.ContextLines...)
		if isChoiceLikePrompt(task) && task.ContextLine < len(lines) {
			lines[task.ContextLine] = renderChoiceContextAnchor(task, promptEN)
		}
		return lines, task.ContextLine
	}
	if strings.TrimSpace(promptContextEN) != "" {
		lines := strings.Split(promptContextEN, "\n")
		if idx := findContextLineIndex(lines, promptEN, task.BodyEN); idx >= 0 {
			return lines, idx
		}
	}
	lines := make([]string, 0, 3)
	lineIndex := 0
	if prev := strings.TrimSpace(task.PrevEN); prev != "" {
		lines = append(lines, prev)
		lineIndex = 1
	}
	lines = append(lines, promptEN)
	if next := strings.TrimSpace(task.NextEN); next != "" {
		lines = append(lines, next)
	}
	return lines, lineIndex
}

func buildFocusedContextEN(contextEN, bodyEN string) string {
	contextEN = strings.TrimSpace(contextEN)
	bodyEN = strings.TrimSpace(bodyEN)
	if contextEN == "" || bodyEN == "" {
		return ""
	}
	lines := strings.Split(contextEN, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == bodyEN || trimmed == "[PLAYER OPTION] "+bodyEN {
			lines[i] = "[[BODY_EN]] " + line + " [[/BODY_EN]]"
			return strings.Join(lines, "\n")
		}
	}
	return ""
}

func findContextLineIndex(lines []string, promptEN, bodyEN string) int {
	promptEN = strings.TrimSpace(promptEN)
	bodyEN = strings.TrimSpace(bodyEN)
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == promptEN || trimmed == "[PLAYER OPTION] "+promptEN {
			return idx
		}
	}
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == bodyEN || trimmed == "[PLAYER OPTION] "+bodyEN {
			return idx
		}
	}
	return -1
}

func renderChoiceContextAnchor(task translationTask, promptEN string) string {
	if !isChoiceLikePrompt(task) {
		return promptEN
	}
	return "[PLAYER OPTION] " + promptEN
}

func shouldIncludePromptSourceFile(task translationTask) bool {
	if task.SourceFile == "" {
		return false
	}
	if task.SourceType == "resource" {
		return false
	}
	return task.ContextEN == "" || len([]rune(task.BodyEN)) <= 24
}

func shouldIncludePromptResourceKey(task translationTask) bool {
	if task.ResourceKey == "" {
		return false
	}
	return task.SourceType == "resource"
}

func isChoiceLikePrompt(task translationTask) bool {
	if task.TextRole == "choice" {
		return true
	}
	return task.ChoiceMode == "choice_stat_check" || task.ChoiceMode == "stat_check_action"
}

func deriveFragmentHints(task translationTask) (string, string, string) {
	en := strings.TrimSpace(task.BodyEN)
	if en == "" {
		return "", "", ""
	}
	actionQuoteOpenRe := regexp.MustCompile(`^(\([^)]*\))\s*"(.*)$`)
	speechThenActionQuoteRe := regexp.MustCompile(`^(.*?)"\s*(\([^)]*\).*)$`)
	speechThenNarrationQuoteRe := regexp.MustCompile(`^(.*?)"\s*((?:[Hh]e|[Ss]he|[Tt]hey|[Ii]t|[Yy]ou|[Tt]he\s+[A-Za-z]|[A-Z][a-z]+).*)$`)
	if task.StatCheck != "" {
		dcQuoteOpenRe := regexp.MustCompile(`^[A-Za-z0-9]+\s+[A-Za-z]{3}-"(.*)$`)
		if m := dcQuoteOpenRe.FindStringSubmatch(en); len(m) == 2 {
			return "dc_open_quote", "", strings.TrimSpace(m[1])
		}
	}
	if m := actionQuoteOpenRe.FindStringSubmatch(en); len(m) == 3 {
		pattern := "action_open_quote"
		if task.StatCheck != "" || task.ChoiceMode == "stat_check_action" {
			pattern = "dc_open_quote"
		}
		return pattern, strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
	}
	if strings.Count(en, "\"") == 1 {
		if m := speechThenActionQuoteRe.FindStringSubmatch(en); len(m) == 3 {
			return "speech_then_action_quote", strings.TrimSpace(m[2]), strings.TrimSpace(m[1])
		}
		if m := speechThenNarrationQuoteRe.FindStringSubmatch(en); len(m) == 3 {
			return "speech_then_narration_quote", strings.TrimSpace(m[2]), strings.TrimSpace(m[1])
		}
	}
	if m := speechThenActionQuoteRe.FindStringSubmatch(en); len(m) == 3 && strings.Count(en, "\"")%2 == 1 {
		return "speech_then_action_quote", strings.TrimSpace(m[2]), strings.TrimSpace(m[1])
	}
	if m := speechThenNarrationQuoteRe.FindStringSubmatch(en); len(m) == 3 && strings.Count(en, "\"")%2 == 1 {
		return "speech_then_narration_quote", strings.TrimSpace(m[2]), strings.TrimSpace(m[1])
	}
	if strings.Count(en, "\"")%2 == 1 {
		firstQuote := strings.Index(en, "\"")
		if firstQuote >= 0 {
			leading := strings.TrimSpace(en[:firstQuote])
			spoken := strings.TrimSpace(en[firstQuote+1:])
			if spoken != "" {
				if leading != "" {
					return "narration_open_quote", leading, spoken
				}
				return "open_quote", "", spoken
			}
		}
	}
	return "", "", ""
}

func deriveStructureHints(task translationTask) (string, string, string) {
	en := strings.TrimSpace(task.BodyEN)
	if en == "" {
		return "", "", ""
	}
	if !(task.TextRole == "glossary" || task.TextRole == "quest" || task.TextRole == "system" || task.TextRole == "narration") {
		if (task.TextRole == "dialogue" || task.TextRole == "narration") && len([]rune(en)) >= 220 {
			return "long_discourse", "", en
		}
		return "", "", ""
	}
	dashIdx := strings.Index(en, " - ")
	if dashIdx <= 0 {
		if len([]rune(en)) < 140 {
			return "", "", ""
		}
		return "expository_entry", "", en
	}
	left := strings.TrimSpace(en[:dashIdx])
	right := strings.TrimSpace(en[dashIdx+3:])
	if left == "" || right == "" {
		return "", "", ""
	}
	if len([]rune(right)) < 40 {
		return "", "", ""
	}
	if task.TextRole == "system" && !looksLikeDefinitionHead(left) {
		if len([]rune(en)) >= 140 {
			return "expository_entry", "", en
		}
		return "", "", ""
	}
	if task.TextRole == "glossary" || task.TextRole == "quest" || looksLikeDefinitionHead(left) {
		return "definition_dash", left, right
	}
	if len([]rune(en)) >= 140 {
		return "expository_entry", "", en
	}
	if (task.TextRole == "dialogue" || task.TextRole == "narration") && len([]rune(en)) >= 220 {
		return "long_discourse", "", en
	}
	return "", "", ""
}

func looksLikeDefinitionHead(head string) bool {
	head = strings.TrimSpace(head)
	if head == "" {
		return false
	}
	if len([]rune(head)) > 40 {
		return false
	}
	lower := strings.ToLower(head)
	for _, bad := range []string{" to ", " either ", " or ", " and ", ","} {
		if strings.Contains(lower, bad) {
			return false
		}
	}
	return true
}
