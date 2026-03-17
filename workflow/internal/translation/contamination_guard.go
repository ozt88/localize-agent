package translation

import "unicode"

type scriptGroup string

const (
	scriptArabic     scriptGroup = "arabic"
	scriptBengali    scriptGroup = "bengali"
	scriptCJK        scriptGroup = "cjk"
	scriptCyrillic   scriptGroup = "cyrillic"
	scriptDevanagari scriptGroup = "devanagari"
	scriptGeorgian   scriptGroup = "georgian"
	scriptGreek      scriptGroup = "greek"
	scriptHebrew     scriptGroup = "hebrew"
	scriptLatinExtra scriptGroup = "latin_extra"
)

func sanitizePromptKoreanReference(sourceEN, ko string) string {
	if !hasUnexpectedScriptGroup(ko, sourceEN) {
		return ko
	}
	return ""
}

func hasUnexpectedScriptGroup(text, source string) bool {
	if text == "" {
		return false
	}
	allowed := scriptGroupsIn(source)
	for group := range scriptGroupsIn(text) {
		if _, ok := allowed[group]; !ok {
			return true
		}
	}
	return false
}

func scriptGroupsIn(s string) map[scriptGroup]struct{} {
	out := map[scriptGroup]struct{}{}
	for _, r := range s {
		if g, ok := classifyScriptGroup(r); ok {
			out[g] = struct{}{}
		}
	}
	return out
}

func classifyScriptGroup(r rune) (scriptGroup, bool) {
	switch {
	case r < 128:
		return "", false
	case unicode.In(r, unicode.Hangul):
		return "", false
	case 0x1100 <= r && r <= 0x11FF:
		return "", false
	case 0x3130 <= r && r <= 0x318F:
		return "", false
	case 0x2000 <= r && r <= 0x206F:
		return "", false
	case 0x3000 <= r && r <= 0x303F:
		return "", false
	case 0xFF00 <= r && r <= 0xFFEF:
		return "", false
	case unicode.In(r, unicode.Arabic):
		return scriptArabic, true
	case unicode.In(r, unicode.Bengali):
		return scriptBengali, true
	case unicode.In(r, unicode.Han):
		return scriptCJK, true
	case unicode.In(r, unicode.Cyrillic):
		return scriptCyrillic, true
	case unicode.In(r, unicode.Devanagari):
		return scriptDevanagari, true
	case unicode.In(r, unicode.Georgian):
		return scriptGeorgian, true
	case unicode.In(r, unicode.Greek):
		return scriptGreek, true
	case unicode.In(r, unicode.Hebrew):
		return scriptHebrew, true
	case unicode.In(r, unicode.Latin) && r > 127:
		return scriptLatinExtra, true
	default:
		return "", false
	}
}
