package semanticreview

import (
	"strings"

	"localize-agent/workflow/pkg/shared"
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
	_ = cfg
	return strings.TrimSpace(strings.Join([]string{
		"Reply to this warmup with exactly: OK",
		"You are a Korean localization judge.",
		"Your task is to assign two absolute quality scores: current_score and fresh_score.",
		"Do not choose a winner.",
		"Do not recommend rewrite.",
		"Each score is an integer from 0 to 100. Higher is better.",
		"Return only the runtime-requested compact score format.",
		"Follow the requested compact JSON array output format exactly.",
		"Do not add explanations or extra text.",
	}, "\n"))
}
