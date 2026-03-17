package translation

import "strings"

type translateSkill struct {
	contextText string
	staticRules string
}

func newTranslateSkill(contextText, rulesText string) *translateSkill {
	return &translateSkill{
		contextText: strings.TrimSpace(contextText),
		staticRules: strings.TrimSpace(mergeRules(defaultStaticRules(), rulesText)),
	}
}

func mergeRules(base, extra string) string {
	base = strings.TrimSpace(base)
	extra = strings.TrimSpace(extra)
	switch {
	case base == "":
		return extra
	case extra == "":
		return base
	default:
		return base + "\n" + extra
	}
}

func defaultStaticRules() string {
	return strings.Join([]string{
		"1. Reply to this warmup with exactly: OK",
		"2. Use Korean. Keep EN meaning as source-of-truth.",
		"3. Translate only the `en` field.",
		"4. `context_en` or `contexts` are reference-only scene context. Do not translate, summarize, or copy them unless a line inside them is also exactly the same as `en`.",
		"5. If `focused_context_en` is present, the line wrapped by [[BODY_EN]]...[[/BODY_EN]] is the exact line to translate.",
		"6. For batch inputs, each item uses `items[*].en` as the source text. `items[*].ctx` and `items[*].line` point to the matching reference line in `contexts`.",
		"7. Speaker labels inside context such as `Name: line` are reference-only. Do not copy the label unless it is also part of `en`.",
		"8. `current_ko`, `prev_ko`, and `next_ko` are continuity references only. Use them only to keep tone, speech level, and terminology consistent unless English clearly requires a change.",
		"9. For batch inputs, keep output order exactly the same as input order.",
		"10. Never reuse one item's translation for another item unless their `en` values are identical.",
		"11. Choice lines represent player actions, not narration. Translate them as short actionable options.",
		"12. If `choice_mode` is present or `text_role` is `choice`, the line is a player-selectable option. Translate it as a short clickable option, not as narration or explanation.",
		"13. `[PLAYER OPTION]` inside `context_en` or `contexts` is a reference-only anchor. Do not copy `[PLAYER OPTION]` into the output.",
		"14. Prefer concise action phrasing for choice lines.",
		"15. If `stat_check` is present, preserve it as a bracketed gameplay prefix in Korean.",
		"16. If `text_role` is `ui_label` or `button`, translate it as a concise UI label, not as dialogue or prose.",
		"17. If `text_role` is `tooltip` or `ui_description`, translate it as explanatory UI/help text with clear, natural Korean.",
		"18. For UI rows, treat noisy surrounding UI text as weak reference only. Prefer the exact `en` line over unrelated menu copy.",
		"19. Source English may be intentionally truncated, broken, or open-ended. Do not repair, complete, or reinterpret missing source text.",
		"20. Do not invent missing quotation closure or missing continuation. Translate only the visible fragment.",
		"21. If the source begins mid-utterance or ends abruptly, Korean may also remain fragmentary.",
		"22. Preserve gameplay prefixes, action markers, and mixed action-plus-dialogue structure as meaningful source content.",
		"23. Output must be valid JSON only. No markdown, no code fences, no commentary, no escaped outer brackets.",
		"24. Return only the contract defined by the project-local translator system prompt.",
	}, "\n")
}

func (s *translateSkill) warmup() string {
	parts := []string{}
	if s.contextText != "" {
		parts = append(parts, s.contextText)
	}
	if s.staticRules != "" {
		parts = append(parts, s.staticRules)
	}
	return strings.Join(parts, "\n")
}

func (s *translateSkill) shapeHint() string {
	return `{"id":"...","proposed_ko":"..."}`
}
