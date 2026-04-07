package clustertranslate

import (
	"fmt"
	"strings"
	"unicode"

	"localize-agent/workflow/internal/inkparse"
)

// v2PromptSections organizes translation rules into 4 tiers for structured prompt assembly.
type v2PromptSections struct {
	Context     []string
	Voice       []string
	Task        []string
	Constraints []string
}

// v2Sections is the 4-tier classification of the 9 translation rules.
var v2Sections = v2PromptSections{
	Context: []string{
		"[CONTEXT] lines are for reference only -- do not translate them.",
	},
	Voice: []string{
		"Match the tone and register of the original.",
	},
	Task: []string{
		"Translate the following scene into Korean.",
		"Preserve speaker labels (e.g., 'Braxo:') in your output.",
		"Preserve [CHOICE] markers in your output.",
	},
	Constraints: []string{
		"Maintain the [NN] line numbers exactly as given.",
		"Do not add, remove, or merge lines.",
		"All proper nouns (names, places, spells, abilities) stay in English.",
		"Output only the translated lines, no commentary.",
	},
}

// sectionsToRules flattens v2PromptSections into a flat []string for backward compatibility.
func sectionsToRules(s v2PromptSections) []string {
	var rules []string
	rules = append(rules, s.Context...)
	rules = append(rules, s.Voice...)
	rules = append(rules, s.Task...)
	rules = append(rules, s.Constraints...)
	return rules
}

// BuildBaseWarmup assembles the warmup text following the v1 translateSkill.warmup() pattern.
func BuildBaseWarmup(systemPrompt, contextText, rulesText, glossaryWarmupJSON string) string {
	var parts []string

	if systemPrompt != "" {
		parts = append(parts, strings.TrimSpace(systemPrompt))
	}
	if contextText != "" {
		parts = append(parts, strings.TrimSpace(contextText))
	}

	// Translation rules section — organized by 4-tier sections
	rulesPart := "## Translation Rules\n\n"
	rulesPart += "### Context\n" + strings.Join(v2Sections.Context, "\n") + "\n\n"
	rulesPart += "### Voice\n" + strings.Join(v2Sections.Voice, "\n") + "\n\n"
	rulesPart += "### Task\n" + strings.Join(v2Sections.Task, "\n") + "\n\n"
	rulesPart += "### Constraints\n" + strings.Join(v2Sections.Constraints, "\n")
	if rulesText != "" {
		rulesPart += "\n" + strings.TrimSpace(rulesText)
	}
	parts = append(parts, rulesPart)

	// Glossary section
	if glossaryWarmupJSON != "" {
		parts = append(parts, "## Glossary (preserve these terms in English)\n"+glossaryWarmupJSON)
	}

	parts = append(parts, "Reply with exactly: OK")

	return strings.Join(parts, "\n\n")
}

// abilityScoreVoice maps ability-score speaker tags to Korean voice guide descriptions.
// Source: projects/esoteric-ebb/context/v2_base_prompt.md
var abilityScoreVoice = map[string]string{
	"wis": "침착하고 달관한 어조, 내면의 관찰자",
	"str": "직선적이고 단순한 문장, 의지와 육체",
	"int": "논리적이고 분석적인 화법, 학구적 어조",
	"cha": "설득력 있고 감정이 풍부한 말투, 사교꾼",
	"dex": "간결하고 재치 있는 표현, 반사신경",
	"con": "차분하고 인내심 있는 어조, 육체 감각",
}

// buildVoiceSection creates a per-batch voice guide reminder for ability-score speakers.
// Returns empty string if no ability-score speakers are in the batch.
func buildVoiceSection(speakers []string) string {
	seen := make(map[string]bool)
	var sb strings.Builder
	for _, s := range speakers {
		lower := strings.ToLower(strings.TrimSpace(s))
		guide, ok := abilityScoreVoice[lower]
		if ok && !seen[lower] {
			seen[lower] = true
			fmt.Fprintf(&sb, "- **%s**: %s\n", lower, guide)
		}
	}
	if sb.Len() == 0 {
		return ""
	}
	return "\n## Voice Guide (이 배치의 화자)\n" + sb.String()
}

// estimateTokens provides a rough token count approximation.
// English: ~4 chars per token. Korean: ~2 runes per token.
func estimateTokens(text string) int {
	runes := []rune(text)
	koreanCount := 0
	for _, r := range runes {
		if r >= 0xAC00 && r <= 0xD7AF {
			koreanCount++
		}
	}
	englishChars := len(runes) - koreanCount
	return englishChars/4 + koreanCount/2
}

// contextBudgetTokens is the token budget limit for the full prompt.
// gpt-5.4 context window (~8k), prompt target ~50%, base prompt ~2000 + max ~900 context = ~3000, with margin = 4000.
const contextBudgetTokens = 4000

// BuildScriptPrompt builds the batch prompt for cluster translation.
// Returns the prompt string and metadata for validation.
// Applies token budget trimming per D-08 priority: continuity -> branch -> voice card.
func BuildScriptPrompt(task ClusterTask) (string, PromptMeta) {
	// Apply token budget trimming before building prompt
	task = trimContextForBudget(task, contextBudgetTokens)
	return buildScriptPromptCore(task)
}

// buildScriptPromptCore is the core prompt builder without budget trimming.
func buildScriptPromptCore(task ClusterTask) (string, PromptMeta) {
	var prompt strings.Builder
	meta := PromptMeta{}

	// Filter out punctuation-only blocks (D-13)
	var translatableBlocks []inkparse.DialogueBlock
	for _, block := range task.Batch.Blocks {
		if isPunctuationOnly(block.Text) {
			meta.ExcludedBlockIDs = append(meta.ExcludedBlockIDs, block.ID)
			continue
		}
		translatableBlocks = append(translatableBlocks, block)
	}

	// --- [CONTEXT] block: PrevGateLines + ParentChoiceText + NextLines + PrevKO + NextKO ---
	hasContext := len(task.PrevGateLines) > 0 || task.ParentChoiceText != "" ||
		len(task.NextLines) > 0 || len(task.PrevKO) > 0 || len(task.NextKO) > 0

	if hasContext {
		// PrevGateLines (D-03)
		if len(task.PrevGateLines) > 0 {
			prompt.WriteString("[CONTEXT]\n")
			prompt.WriteString("(이전 게이트 -- 번역하지 마세요)\n")
			for i, line := range task.PrevGateLines {
				fmt.Fprintf(&prompt, "[C%d] %q\n", i+1, line)
			}
		}

		// Branch context: ParentChoiceText (D-04, BRANCH-02)
		if task.ParentChoiceText != "" {
			fmt.Fprintf(&prompt, "[CONTEXT] Player chose: %q\n", task.ParentChoiceText)
		}

		// Next lines context (CONT-01)
		if len(task.NextLines) > 0 {
			prompt.WriteString("[CONTEXT]\n")
			prompt.WriteString("(다음 게이트 -- 번역하지 마세요)\n")
			for i, line := range task.NextLines {
				fmt.Fprintf(&prompt, "[N%d] %q\n", i+1, line)
			}
		}

		// PrevKO context (CONT-02, retranslation)
		if len(task.PrevKO) > 0 {
			prompt.WriteString("[CONTEXT]\n")
			prompt.WriteString("(이전 한국어 번역 -- 참고용)\n")
			for i, ko := range task.PrevKO {
				fmt.Fprintf(&prompt, "[K%d] %q\n", i+1, ko)
			}
		}

		// NextKO context (CONT-02, retranslation)
		if len(task.NextKO) > 0 {
			prompt.WriteString("[CONTEXT]\n")
			prompt.WriteString("(다음 한국어 번역 -- 참고용)\n")
			for i, ko := range task.NextKO {
				fmt.Fprintf(&prompt, "[NK%d] %q\n", i+1, ko)
			}
		}

		prompt.WriteString("\n---\n\n")
	}

	// Format each translatable block as numbered lines
	padWidth := 2
	if len(translatableBlocks) >= 100 {
		padWidth = 3
	}

	for i, block := range translatableBlocks {
		num := fmt.Sprintf("%0*d", padWidth, i+1)
		meta.BlockIDOrder = append(meta.BlockIDOrder, block.ID)

		if block.Choice != "" {
			// Choice marker (D-02)
			fmt.Fprintf(&prompt, "[%s] [CHOICE] %q\n", num, block.Text)
		} else if block.Speaker != "" {
			// Speaker label (D-01)
			fmt.Fprintf(&prompt, "[%s] %s: %q\n", num, block.Speaker, block.Text)
		} else {
			// Plain line
			fmt.Fprintf(&prompt, "[%s] %q\n", num, block.Text)
		}
	}

	meta.LineCount = len(translatableBlocks)

	// Inject per-batch voice guide for ability-score speakers
	var speakers []string
	for _, block := range translatableBlocks {
		if block.Speaker != "" {
			speakers = append(speakers, block.Speaker)
		}
	}
	voiceSection := buildVoiceSection(speakers)
	if voiceSection != "" {
		prompt.WriteString(voiceSection)
	}

	// Named character voice cards (TONE-02)
	if len(task.VoiceCards) > 0 {
		seen := make(map[string]bool)
		var namedVoice strings.Builder
		for _, block := range translatableBlocks {
			if block.Speaker != "" {
				if guide, ok := task.VoiceCards[block.Speaker]; ok && !seen[block.Speaker] {
					seen[block.Speaker] = true
					fmt.Fprintf(&namedVoice, "- **%s**: %s\n", block.Speaker, guide)
				}
			}
		}
		if namedVoice.Len() > 0 {
			prompt.WriteString("\n## Named Character Voice Guide\n")
			prompt.WriteString(namedVoice.String())
		}
	}

	// Append per-batch glossary if non-empty (D-11)
	if task.GlossaryJSON != "" {
		prompt.WriteString("\n## Batch Glossary\n")
		prompt.WriteString(task.GlossaryJSON)
		prompt.WriteString("\n")
	}

	// Append content-type suffix rules (D-04)
	suffix := BuildContentSuffix(task.Batch.ContentType)
	if suffix != "" {
		prompt.WriteString("\n")
		prompt.WriteString(suffix)
		prompt.WriteString("\n")
	}

	promptStr := prompt.String()
	meta.EstimatedTokens = estimateTokens(promptStr)
	return promptStr, meta
}

// trimContextForBudget removes context elements when estimated tokens exceed budget.
// Priority (D-08): voice card (last removed) > branch > continuity (first removed).
// maxTokens is the budget limit. Returns modified task.
func trimContextForBudget(task ClusterTask, maxTokens int) ClusterTask {
	// Estimate current prompt tokens
	testPrompt, _ := buildScriptPromptCore(task)
	tokens := estimateTokens(testPrompt)
	if tokens <= maxTokens {
		return task
	}

	// Phase 1: Remove continuity (lowest priority)
	task.NextLines = nil
	task.NextKO = nil
	task.PrevKO = nil
	testPrompt, _ = buildScriptPromptCore(task)
	if estimateTokens(testPrompt) <= maxTokens {
		return task
	}

	// Phase 2: Remove branch context
	task.ParentChoiceText = ""
	testPrompt, _ = buildScriptPromptCore(task)
	if estimateTokens(testPrompt) <= maxTokens {
		return task
	}

	// Phase 3: Remove voice cards (last resort)
	task.VoiceCards = nil
	return task
}

// BuildContentSuffix returns type-specific translation instructions per D-04.
func BuildContentSuffix(contentType string) string {
	switch contentType {
	case inkparse.ContentDialogue:
		return "이 씬은 대화입니다. 자연스러운 구어체를 사용하세요."
	case inkparse.ContentSpell:
		return "주문/능력 이름은 영문 유지. 설명만 번역하세요."
	case inkparse.ContentItem:
		return "아이템 이름은 영문 유지. 설명만 번역하세요."
	case inkparse.ContentUI:
		return "UI 텍스트입니다. 간결하고 명확하게 번역하세요."
	case inkparse.ContentSystem:
		return "시스템 텍스트입니다. 정확하게 번역하세요."
	default:
		return ""
	}
}

// isPunctuationOnly checks if text contains only punctuation and whitespace (D-13).
func isPunctuationOnly(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return false
		}
	}
	return true
}
