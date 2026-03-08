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
		"3. Return only the contract defined by the project-local translator system prompt.",
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
