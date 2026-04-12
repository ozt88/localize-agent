package clustertranslate

import (
	"fmt"
	"strings"
	"unicode"

	"localize-agent/workflow/internal/inkparse"
)

// contextBudgetTokens is the approximate token budget for all injected context blocks (D-18).
// Priority order (high to low): continuity > RAG > glossary > branch > voice.
const contextBudgetTokens = 4000

// v2StaticRules are the translation rules for v2 numbered-line script format.
var v2StaticRules = []string{
	"1. Translate the following scene into Korean.",
	"2. Maintain the [NN] line numbers exactly as given.",
	"3. Do not add, remove, or merge lines.",
	"4. Preserve speaker labels (e.g., 'Braxo:') in your output.",
	"5. Preserve [CHOICE] markers in your output.",
	"6. [CONTEXT] lines are for reference only -- do not translate them.",
	"7. All proper nouns (names, places, spells, abilities) stay in English.",
	"8. Match the tone and register of the original.",
	"9. Output only the translated lines, no commentary.",
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

	// Translation rules section
	rulesPart := "## Translation Rules\n" + strings.Join(v2StaticRules, "\n")
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

// BuildScriptPrompt builds the batch prompt for cluster translation.
// Returns the prompt string and metadata for validation.
func BuildScriptPrompt(task ClusterTask) (string, PromptMeta) {
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

	// Prepend [CONTEXT] block if previous gate lines exist (D-03)
	if len(task.PrevGateLines) > 0 {
		prompt.WriteString("[CONTEXT]\n")
		prompt.WriteString("(이전 게이트 -- 번역하지 마세요)\n")
		for i, line := range task.PrevGateLines {
			fmt.Fprintf(&prompt, "[C%d] %q\n", i+1, line)
		}
		prompt.WriteString("\n---\n\n")
	}

	// CONT-01: look-ahead next lines context
	if len(task.NextLines) > 0 {
		prompt.WriteString("[NEXT LINES]\n")
		prompt.WriteString("(다음 게이트 미리보기 -- 번역하지 마세요)\n")
		for i, line := range task.NextLines {
			fmt.Fprintf(&prompt, "[N%d] %q\n", i+1, line)
		}
		prompt.WriteString("\n---\n\n")
	}

	// CONT-02: previous/next Korean translations for continuity
	if len(task.PrevKO) > 0 {
		prompt.WriteString("[PREV KO]\n")
		prompt.WriteString("(이전 한국어 번역 -- 어조 참고용, 번역하지 마세요)\n")
		for i, ko := range task.PrevKO {
			fmt.Fprintf(&prompt, "[P%d] %s\n", i+1, ko)
		}
		prompt.WriteString("\n---\n\n")
	}
	if len(task.NextKO) > 0 {
		prompt.WriteString("[NEXT KO]\n")
		prompt.WriteString("(다음 한국어 번역 -- 어조 참고용, 번역하지 마세요)\n")
		for i, ko := range task.NextKO {
			fmt.Fprintf(&prompt, "[FK%d] %s\n", i+1, ko)
		}
		prompt.WriteString("\n---\n\n")
	}

	// BRANCH-01: parent choice text
	if task.ParentChoiceText != "" {
		fmt.Fprintf(&prompt, "[BRANCH]\n(이 씬은 선택지 %q 이후 분기입니다 -- 참고용)\n\n---\n\n", task.ParentChoiceText)
	}

	// TONE-02: voice cards for speakers in this batch
	if len(task.VoiceCards) > 0 {
		prompt.WriteString("[VOICE CARDS]\n")
		prompt.WriteString("(화자별 한국어 말투 가이드)\n")
		// Collect unique speakers from batch
		seenSpeakers := make(map[string]bool)
		for _, block := range translatableBlocks {
			if block.Speaker != "" && !seenSpeakers[block.Speaker] {
				seenSpeakers[block.Speaker] = true
				if guide, ok := task.VoiceCards[block.Speaker]; ok {
					fmt.Fprintf(&prompt, "- %s: %s\n", block.Speaker, guide)
				}
			}
		}
		prompt.WriteString("\n---\n\n")
	}

	// D-17: RAG world-building context hints
	if task.RAGHints != "" {
		prompt.WriteString("[WORLD CONTEXT]\n")
		prompt.WriteString("(세계관 맥락 -- 번역 참고용)\n")
		prompt.WriteString(task.RAGHints)
		prompt.WriteString("\n\n---\n\n")
	}

	// Format each translatable block as numbered lines
	padWidth := 2
	if len(translatableBlocks) >= 100 {
		padWidth = 3
	}

	for i, block := range translatableBlocks {
		num := fmt.Sprintf("%0*d", padWidth, i+1)
		meta.BlockIDOrder = append(meta.BlockIDOrder, block.ID)

		// Use quoteForPrompt instead of %q to preserve real newlines in multiline blocks.
		// %q escapes \n to \\n, which causes LLM responses to contain literal backslash-n
		// that the parser would store verbatim rather than as real newlines.
		quoted := quoteForPrompt(block.Text)
		if block.Choice != "" {
			// Choice marker (D-02)
			fmt.Fprintf(&prompt, "[%s] [CHOICE] %s\n", num, quoted)
		} else if block.Speaker != "" {
			// Speaker label (D-01)
			fmt.Fprintf(&prompt, "[%s] %s: %s\n", num, block.Speaker, quoted)
		} else {
			// Plain line
			fmt.Fprintf(&prompt, "[%s] %s\n", num, quoted)
		}
	}

	meta.LineCount = len(translatableBlocks)

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

	return prompt.String(), meta
}

// trimContextForBudget trims context fields from a ClusterTask to stay within the token budget.
// Priority order (D-18): continuity (PrevKO/NextKO/NextLines) > RAG > glossary > branch > voice.
// Approximation: 1 token ≈ 4 characters.
func trimContextForBudget(task *ClusterTask) {
	budget := contextBudgetTokens * 4 // convert to characters

	estimate := func(t *ClusterTask) int {
		total := 0
		for _, s := range t.PrevKO {
			total += len(s)
		}
		for _, s := range t.NextKO {
			total += len(s)
		}
		for _, s := range t.NextLines {
			total += len(s)
		}
		total += len(t.RAGHints)
		total += len(t.GlossaryJSON)
		total += len(t.ParentChoiceText)
		for _, v := range t.VoiceCards {
			total += len(v)
		}
		return total
	}

	if estimate(task) <= budget {
		return
	}

	// Remove voice cards first (lowest priority)
	task.VoiceCards = nil
	if estimate(task) <= budget {
		return
	}

	// Remove branch context
	task.ParentChoiceText = ""
	if estimate(task) <= budget {
		return
	}

	// Remove glossary
	task.GlossaryJSON = ""
	if estimate(task) <= budget {
		return
	}

	// Remove RAG hints
	task.RAGHints = ""
	if estimate(task) <= budget {
		return
	}

	// Remove continuity (last resort)
	task.PrevKO = nil
	task.NextKO = nil
	task.NextLines = nil
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

// quoteForPrompt wraps text in double quotes without escaping internal newlines.
// Unlike %q (strconv.Quote), this preserves real newlines so multiline dialogue
// blocks are shown as-is to the LLM rather than as literal \n sequences.
// Internal double quotes are escaped as \" to avoid breaking the line format.
func quoteForPrompt(s string) string {
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
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
