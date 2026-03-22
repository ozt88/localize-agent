package scorellm

import "fmt"

// BuildScoreWarmup returns the system prompt for Score LLM.
func BuildScoreWarmup() string {
	return `You evaluate Korean translations of English game dialogue.

For each item, assess:
1. Translation quality (1-10): accuracy, naturalness, tone
2. Format quality (1-10): tag preservation, structure

Return JSON only:
{"translation_score": N, "format_score": N, "failure_type": "pass|translation|format|both", "reason": "brief explanation if not pass"}

Rules:
- "pass" if both scores >= 7
- "translation" if translation_score < 7 and format_score >= 7
- "format" if format_score < 7 and translation_score >= 7
- "both" if both < 7
- Keep reason under 100 characters

Reply with: OK`
}

// BuildScorePrompt builds a single-item scoring prompt.
func BuildScorePrompt(task ScoreTask) string {
	return fmt.Sprintf("EN: %s\nKO: %s\nHas tags: %v", task.ENSource, task.KOFormatted, task.HasTags)
}
