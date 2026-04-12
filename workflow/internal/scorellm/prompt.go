package scorellm

import (
	"fmt"
	"strings"
)

// BuildScoreWarmup returns the system prompt for Score LLM.
func BuildScoreWarmup() string {
	return `You evaluate Korean translations of English game dialogue.

For each item, assess:
1. Translation quality (1-10): accuracy, naturalness, tone
2. Format quality (1-10): tag preservation, structure

When given multiple items, return a JSON array with one result per item in order.
When given a single item, return a single JSON object.

Each result:
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

// BuildBatchScorePrompt builds a numbered batch scoring prompt.
// ragContext is optional world-building context injected as a [CONTEXT] section.
// Returns the prompt and the ordered list of block IDs for result mapping.
func BuildBatchScorePrompt(tasks []ScoreTask, ragContext string) (string, []string) {
	var b strings.Builder
	ids := make([]string, len(tasks))

	if ragContext != "" {
		fmt.Fprintf(&b, "[CONTEXT]\n%s\n\n---\n\n", ragContext)
	}

	fmt.Fprintf(&b, "Score these %d translations. Return a JSON array with %d results in order.\n\n", len(tasks), len(tasks))
	for i, t := range tasks {
		ids[i] = t.BlockID
		fmt.Fprintf(&b, "[%d] EN: %s\nKO: %s\nTags: %v\n\n", i+1, t.ENSource, t.KOFormatted, t.HasTags)
	}

	return b.String(), ids
}
