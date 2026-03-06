package translation

import "strings"

type translateSkill struct {
	contextText string
	staticRules string
}

func newTranslateSkill(contextText, rulesText string) *translateSkill {
	rules := rulesText
	if strings.TrimSpace(rules) == "" {
		rules = defaultStaticRules()
	}
	return &translateSkill{contextText: strings.TrimSpace(contextText), staticRules: strings.TrimSpace(rules)}
}

func defaultStaticRules() string {
	return strings.Join([]string{
		"1. Return strictly JSON/JSONL in the requested shape only.",
		"2. Do not output markdown, explanation, code fences, headings, or extra lines.",
		"3. Reply to this warmup with exactly: OK",
		"4. Priority order: fidelity to EN meaning > natural Korean flow > brevity.",
		"5. When fluency and fidelity conflict, prefer fidelity and flag the tension in notes.",
		"6. Use Korean. Keep EN meaning as source-of-truth.",
		"7. Prefer concise natural phrasing; avoid duplicated modifiers and awkward comma insertion.",
		"8. Allow light rewrite for natural Korean flow; avoid literal English word order.",
		"9. In dialogues, keep character voice natural; avoid overly bureaucratic or stiff formal phrasing.",
		"10. Use current_ko as reference only.",
		"11. Do not inherit mistranslations, unnatural phrasing, or literal word-for-word renderings from current_ko.",
		"12. Proper nouns (character names, faction names, item names, lore terms) keep EN spelling by default.",
		"13. Use a canonical KR term only if it is explicitly provided in the session context.",
		"14. Preserve every [Tn] tag exactly: no rename, no reorder, no add, no delete.",
		"15. For every output line, risk is mandatory and must be one of: low | med | high.",
		"16. risk=high when: meaning shifts, a lore term is ambiguous, or natural Korean requires significant structural change.",
		"17. risk=med when: phrasing is debatable but meaning is preserved.",
		"18. risk=low when: translation is straightforward and confident.",
		"19. For every output line, notes must be a string; use empty string only when truly nothing to flag.",
	}, "\n")
}

func (s *translateSkill) warmup() string {
	return strings.Join([]string{
		"You are a Korean localization translator for the current project in this repository.",
		"Follow the supplied project context for genre, tone, terminology, and output constraints.",
		"Apply the following fixed rules and context for all translations in this session.\n",
		s.contextText,
		"\n" + s.staticRules,
	}, "\n")
}

func (s *translateSkill) shapeHint() string {
	return `{"id":"...","proposed_ko":"...","risk":"low|med|high","notes":""}`
}
