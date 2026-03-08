package semanticreview

import (
	"strings"

	"localize-agent/workflow/internal/shared"
)

func semanticReviewContext(cfg Config) string {
	return shared.LoadContext(cfg.ContextFiles)
}

func backtranslationWarmup(cfg Config) string {
	return strings.TrimSpace(strings.Join([]string{
		semanticReviewContext(cfg),
		"Reply to this warmup with exactly: OK",
		"You are performing Korean-to-English backtranslation for semantic review.",
		"Follow the runtime prompt format exactly and return only the requested output.",
	}, "\n"))
}

func directScoreWarmup(cfg Config) string {
	return strings.TrimSpace(strings.Join([]string{
		semanticReviewContext(cfg),
		"Reply to this warmup with exactly: OK",
		"You are assigning semantic oddness scores to Korean translations against English source text.",
		"Follow the runtime prompt format exactly and return only the requested output.",
	}, "\n"))
}
