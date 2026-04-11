package scorellm

import (
	"fmt"
	"strings"
)

// BuildBatchScorePrompt builds a numbered batch scoring prompt.
// Returns the prompt and the ordered list of block IDs for result mapping.
// ragContext is optional world-building context for evaluation (D-19).
func BuildBatchScorePrompt(tasks []ScoreTask, ragContext string) (string, []string) {
	var b strings.Builder
	ids := make([]string, len(tasks))

	// RAG context header (D-19, Phase 07.1)
	if ragContext != "" {
		fmt.Fprintf(&b, "(세계관 정보 -- 평가 참고용)\n%s\n\n", ragContext)
	}

	fmt.Fprintf(&b, "Score these %d translations. Return a JSON array with %d results in order.\n\n", len(tasks), len(tasks))
	for i, t := range tasks {
		ids[i] = t.BlockID
		fmt.Fprintf(&b, "[%d] EN: %s\nKO: %s\nTags: %v\n\n", i+1, t.ENSource, t.KOFormatted, t.HasTags)
	}

	return b.String(), ids
}
