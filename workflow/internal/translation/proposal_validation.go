package translation

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	passthroughControlRe = regexp.MustCompile(`(?i)^\.[A-Za-z0-9_'\-]+(?:[<>]=?\d+|==[^\s]+)?-$|^[A-Za-z0-9_'\-]+==[^\s]+-$|^SPELL [A-Za-z0-9_'\-]+-$`)
)

func isDegenerateProposal(en, ko string) bool {
	return degenerateProposalReason(en, ko) != ""
}

func degenerateProposalReason(en, ko string) string {
	enTrim := strings.TrimSpace(en)
	koTrim := strings.TrimSpace(ko)
	if koTrim == "" {
		return "empty"
	}
	if enTrim == "" {
		return ""
	}
	if isPunctuationOnly(koTrim) {
		if isLiteralPassthroughSource(enTrim) {
			return ""
		}
		return "punctuation_only"
	}
	if normalizedComparable(enTrim) == normalizedComparable(koTrim) {
		if isLiteralPassthroughSource(enTrim) {
			return ""
		}
		return "exact_source_copy"
	}
	if isASCIIHeavyEnglishLike(koTrim) {
		return "ascii_heavy"
	}
	return ""
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

func normalizedComparable(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isASCIIHeavyEnglishLike(s string) bool {
	letters := 0
	asciiLetters := 0
	hangul := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if r <= unicode.MaxASCII {
				asciiLetters++
			}
			if unicode.In(r, unicode.Hangul) {
				hangul++
			}
		}
	}
	if letters == 0 {
		return false
	}
	if hangul > 0 {
		return false
	}
	return asciiLetters*100/letters >= 80
}

func isLiteralPassthroughSource(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if passthroughControlRe.MatchString(s) {
		return true
	}
	if strings.Contains(s, "<wiggle>") && isPunctuationOnly(stripSimpleTags(s)) {
		return true
	}
	stripped := stripSimpleTags(s)
	if stripped != "" && len([]rune(stripped)) <= 8 && isUpperishToken(stripped) {
		return true
	}
	return false
}

func stripSimpleTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch r {
		case '<':
			inTag = true
			continue
		case '>':
			inTag = false
			continue
		}
		if !inTag {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func isUpperishToken(s string) bool {
	hasLetter := false
	for _, r := range s {
		if unicode.IsLetter(r) {
			hasLetter = true
			if unicode.IsLower(r) {
				return false
			}
			continue
		}
		if unicode.IsNumber(r) || unicode.IsSpace(r) || unicode.IsPunct(r) {
			continue
		}
		return false
	}
	return hasLetter
}
