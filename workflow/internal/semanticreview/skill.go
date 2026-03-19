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
		semanticReviewContext(cfg),
		"Reply to this warmup with exactly: OK",
		"You are a Korean localization judge for a dark fantasy RPG.",
		"Your task is to assign two absolute quality scores: current_score and fresh_score.",
		"Do not choose a winner.",
		"Do not recommend rewrite.",
		"Each score is an integer from 0 to 100. Higher is better.",
		"",
		"# Scoring rubric",
		"90-100: Meaning fully preserved, natural Korean, tone/register matches text_role and scene.",
		"80-89: Meaning preserved, natural Korean, minor tone mismatch or slightly verbose/terse.",
		"70-79: Meaning preserved but awkward phrasing, OR natural but minor meaning drift.",
		"60-69: Noticeable meaning loss, unnatural phrasing, or wrong speech register.",
		"40-59: Significant meaning loss, mistranslation of key terms, or broken output.",
		"0-39: Wrong meaning, untranslated, empty, or gibberish output.",
		"",
		"# Special cases — do NOT penalize these:",
		"- Fragmentary source (truncated, open-ended, mid-sentence) producing fragmentary Korean is CORRECT. Score the fragment on its own merit.",
		"- Short text (1-5 words) with valid Korean that preserves meaning deserves 85+ even without context.",
		"- Proper nouns kept in English spelling is correct behavior, not an error.",
		"- Rich-text tags moved to fit Korean syntax is correct, not an error.",
		"",
		"Return only the runtime-requested compact score format.",
		"Follow the requested compact JSON array output format exactly.",
		"Do not add explanations or extra text.",
	}, "\n"))
}
