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
		if isLiteralPassthroughSource(enTrim) {
			return ""
		}
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

var placeholderStripRe = regexp.MustCompile(`\[\[/?[ET]\d+\]\]`)

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
	// Strip both HTML tags and LLM placeholders for content analysis
	stripped := stripSimpleTags(s)
	strippedPlaceholders := strings.TrimSpace(placeholderStripRe.ReplaceAllString(stripped, ""))
	if stripped == "" && strippedPlaceholders == "" {
		return false
	}
	content := stripped
	if strippedPlaceholders != "" {
		content = strippedPlaceholders
	}
	// Short uppercase tokens (existing): "OK", "DC", "STR 14"
	if len([]rune(content)) <= 8 && isUpperishToken(content) {
		return true
	}
	// Short text (≤3 words) that looks like a proper noun or label:
	// all words start uppercase, no common English sentence words
	words := strings.Fields(content)
	if len(words) >= 1 && len(words) <= 3 && len([]rune(content)) <= 30 && isProperNounPhrase(words) {
		return true
	}
	// Numeric/stat-like: "+1 X", "5", "DC 5 10 15 20 25 30"
	if isNumericOrStatLike(content) {
		return true
	}
	// Foreign-script text: Latin, Nordic characters (non-ASCII non-Korean)
	if isForeignScriptText(content) {
		return true
	}
	// Entire text is wrapped in emphasis tags (italic/bold) or placeholders wrapping foreign text
	if isFullyEmphasisWrapped(s) || isPlaceholderWrappedForeign(s) {
		return true
	}
	// Credit/attribution: "by Name", "Design by Name"
	if isCreditLine(content) {
		return true
	}
	// Placeholder text
	if strings.HasPrefix(strings.ToLower(content), "lorem ipsum") {
		return true
	}
	// Multiline proper noun lists (credits, name lists)
	if isMultilineProperNounList(s) {
		return true
	}
	// Repeating placeholder pattern: "XX | XX | XX"
	if isRepeatingPlaceholderPattern(content) {
		return true
	}
	return false
}

// isPlaceholderWrappedForeign checks for text wrapped in [[E0]]...[[/E0]] placeholders
// where the inner content is foreign-script text (Latin with diacritics, etc.)
func isPlaceholderWrappedForeign(s string) bool {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[[E") {
		return false
	}
	inner := placeholderStripRe.ReplaceAllString(s, "")
	inner = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(inner), ".!?,;:"))
	if inner == "" {
		return false
	}
	return isForeignScriptText(inner) || !containsCommonEnglishWord(inner)
}

func containsCommonEnglishWord(s string) bool {
	for _, w := range strings.Fields(s) {
		clean := strings.TrimFunc(w, func(r rune) bool { return !unicode.IsLetter(r) })
		if commonEnglishWords[strings.ToLower(clean)] {
			return true
		}
	}
	return false
}

// isMultilineProperNounList checks for multiline text where each line is proper nouns (credits)
func isMultilineProperNounList(s string) bool {
	lines := strings.Split(s, "\n")
	if len(lines) < 2 {
		return false
	}
	for _, line := range lines {
		line = strings.TrimSpace(placeholderStripRe.ReplaceAllString(stripSimpleTags(line), ""))
		if line == "" {
			continue
		}
		words := strings.Fields(line)
		if !isProperNounPhrase(words) {
			return false
		}
	}
	return true
}

// isRepeatingPlaceholderPattern checks for "XX | XX | XX" style patterns
func isRepeatingPlaceholderPattern(s string) bool {
	parts := strings.Split(s, "|")
	if len(parts) < 2 {
		return false
	}
	first := strings.TrimSpace(parts[0])
	if first == "" {
		return false
	}
	for _, p := range parts[1:] {
		if strings.TrimSpace(p) != first {
			return false
		}
	}
	return true
}

var commonEnglishWords = map[string]bool{
	"hello": true, "world": true, "the": true, "a": true, "an": true,
	"is": true, "are": true, "was": true, "were": true, "be": true,
	"have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"can": true, "may": true, "might": true, "shall": true, "must": true,
	"not": true, "no": true, "yes": true, "and": true, "or": true, "but": true,
	"if": true, "then": true, "else": true, "when": true, "where": true,
	"what": true, "who": true, "how": true, "why": true, "which": true,
	"this": true, "that": true, "these": true, "those": true,
	"i": true, "you": true, "he": true, "she": true, "it": true,
	"we": true, "they": true, "me": true, "him": true, "her": true,
	"us": true, "them": true, "my": true, "your": true, "his": true,
	"its": true, "our": true, "their": true,
	"in": true, "on": true, "at": true, "to": true, "for": true,
	"with": true, "from": true, "of": true, "about": true,
	"new": true, "old": true, "good": true, "bad": true,
	"all": true, "some": true, "any": true, "each": true, "every": true,
	"go": true, "get": true, "got": true, "take": true, "make": true,
	"come": true, "see": true, "look": true, "find": true, "give": true,
	"tell": true, "say": true, "said": true, "know": true, "think": true,
}

func isProperNounPhrase(words []string) bool {
	for _, w := range words {
		clean := strings.TrimFunc(w, func(r rune) bool { return !unicode.IsLetter(r) })
		if clean == "" {
			continue
		}
		r := []rune(clean)
		// Each word must start with uppercase
		if unicode.IsLetter(r[0]) && unicode.IsLower(r[0]) {
			return false
		}
		// Reject common English words even if capitalized
		if commonEnglishWords[strings.ToLower(clean)] {
			return false
		}
	}
	return true
}

func isNumericOrStatLike(s string) bool {
	letters := 0
	digits := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
		}
		if unicode.IsDigit(r) {
			digits++
		}
	}
	return digits > 0 && digits >= letters
}

func isForeignScriptText(s string) bool {
	// Text that contains diacritics or non-ASCII Latin characters
	// indicating Finnish, Swedish, Latin, etc. — not meant to be translated
	nonASCIILatin := 0
	asciiLetters := 0
	for _, r := range s {
		if !unicode.IsLetter(r) {
			continue
		}
		if r <= unicode.MaxASCII {
			asciiLetters++
		} else if unicode.In(r, unicode.Latin) {
			nonASCIILatin++
		}
	}
	return nonASCIILatin >= 2 || (nonASCIILatin >= 1 && asciiLetters+nonASCIILatin <= 20)
}

var creditLineRe = regexp.MustCompile(`(?i)\b(by|design|art|music|sound|written|directed|produced|created)\b`)
func isFullyEmphasisWrapped(s string) bool {
	s = strings.TrimSpace(s)
	for _, tag := range []string{"i", "b", "em", "strong"} {
		open := "<" + tag + ">"
		close := "</" + tag + ">"
		if strings.HasPrefix(s, open) {
			trimmed := strings.TrimSuffix(strings.TrimSuffix(s, "."), close)
			if trimmed != s && !strings.Contains(trimmed[len(open):], "<") {
				return true
			}
		}
	}
	return false
}

func isCreditLine(s string) bool {
	return creditLineRe.MatchString(s)
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
