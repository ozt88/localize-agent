package evaluation

import "strings"

type evaluateSkill struct {
	contextText string
	staticRules string
}

type translateSkill struct {
	contextText string
	staticRules string
}

func newEvaluateSkill(contextText, rulesText string) *evaluateSkill {
	rules := mergeRules(defaultEvalRules(), rulesText)
	return &evaluateSkill{contextText: strings.TrimSpace(contextText), staticRules: strings.TrimSpace(rules)}
}

func newTranslateSkill(contextText, rulesText string) *translateSkill {
	rules := mergeRules(defaultTranslateRules(), rulesText)
	return &translateSkill{contextText: strings.TrimSpace(contextText), staticRules: strings.TrimSpace(rules)}
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

func defaultEvalRules() string {
	return strings.Join([]string{
		"1. Return strictly JSON/JSONL in the requested shape only.",
		"2. Do not output markdown, explanation, code fences, headings, or extra lines.",
		"3. Reply to this warmup with exactly: OK",
		"4. You are a strict quality evaluator, NOT a translator.",
		"5. Do not suggest alternative translations unless explicitly asked.",
		"6. Evaluate only what is given; do not infer intent.",
		"7. For each unit, output integer scores 1-5 per dimension:",
		"   fidelity   : EN meaning preserved (1=lost, 5=exact)",
		"   fluency    : natural Korean grammar and flow (1=broken, 5=native-natural)",
		"   tone       : matches Warhammer 40K register and character voice (1=wrong, 5=perfect)",
		"   tags       : all [Tn] preserved correctly - 1 (violated) or 5 (intact) ONLY",
		"   consistency: proper nouns and lore terms match session context (1=inconsistent, 5=consistent)",
		"8. verdict must be one of: pass | revise | reject",
		"   pass   : all scores >= 4, no critical issues",
		"   revise : any score == 3, or a non-blocking issue noted",
		"   reject : any score <= 2, or tags==1, or meaning is lost",
		"9. issues: list of strings - each must state dimension + problem + KR fragment (quoted).",
		"   Use [] only when verdict=pass and nothing is noteworthy.",
		"10. Be strict. Awkward Korean must not score fluency >= 4.",
		"11. Do not reward literal translations with high fidelity if they produce unnatural Korean.",
		"12. Tone: Imperial officials=formal, voidsmen=rough, daemons=archaic.",
		"13. Do not penalize creative phrasing if EN meaning is preserved and tone is appropriate.",
		"14. Penalize unnecessary honorifics, bureaucratic phrasing, and English word-order calques in dialogue or choices.",
		"15. For tagged emphasis, judge whether Korean emphasis is natural, not whether the English tag stayed in the same position.",
	}, "\n")
}

func defaultTranslateRules() string {
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
		"10. Use current_ko as reference only; do not inherit mistranslations or unnatural phrasing.",
		"11. Proper nouns keep EN spelling by default; use canonical KR term only if in session context.",
		"12. Preserve every [Tn] tag exactly: no rename, no reorder, no add, no delete.",
		"13. risk is mandatory: low | med | high.",
		"14. notes must be a string; empty string only when truly nothing to flag.",
		"15. Keep dialogue and choices in natural Korean register; avoid unnecessary honorifics unless context clearly requires them.",
		"16. Preserve emphasis tags exactly, but reposition the emphasized Korean phrase if needed for natural word order.",
	}, "\n")
}

func (s *evaluateSkill) warmup() string {
	return strings.Join([]string{
		"You are a Korean localization quality evaluator for the current project in this repository.",
		"You will receive translation units: source EN, translated KR, and translator metadata.",
		"Evaluate each unit independently and return a structured quality report.\n",
		s.contextText,
		"\n" + s.staticRules,
	}, "\n")
}

func (s *evaluateSkill) shapeHint() string {
	return `{"id":"...","fidelity":5,"fluency":5,"tone":5,"tags":5,"consistency":5,"verdict":"pass|revise|reject","issues":[]}`
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
