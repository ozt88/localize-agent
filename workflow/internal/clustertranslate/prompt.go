package clustertranslate

import (
	"fmt"
	"strings"
	"unicode"

	"localize-agent/workflow/internal/inkparse"
)

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
