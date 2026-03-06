package translation

import (
	"strings"
	"unicode"
)

func isDegenerateProposal(en, ko string) bool {
	enTrim := strings.TrimSpace(en)
	koTrim := strings.TrimSpace(ko)
	if koTrim == "" {
		return true
	}
	if enTrim == "" {
		return false
	}
	if isPunctuationOnly(koTrim) {
		return true
	}
	return false
}

func isPunctuationOnly(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return false
		}
	}
	return true
}
