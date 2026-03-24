package inkparse

import (
	"regexp"
	"strings"
	"unicode"
)

// passthroughControlRe matches ink control patterns from v1.
// Patterns: ".Variable>=10-", "Var==val-", "SPELL FireBolt-"
var passthroughControlRe = regexp.MustCompile(`(?i)^\.[A-Za-z0-9_'\-]+(?:[<>]=?\d+|==[^\s]+)?-$|^[A-Za-z0-9_'\-]+==[^\s]+-$|^SPELL [A-Za-z0-9_'\-]+-$`)

// variableRefRe matches pure variable references like "$var_name".
var variableRefRe = regexp.MustCompile(`^\$[A-Za-z_][A-Za-z0-9_]*$`)

// templateStringRe matches template strings with no translatable content like "{expr}".
var templateStringRe = regexp.MustCompile(`^\{[^}]+\}$`)

// inkControlWords are ink runtime control words that should not be translated.
var inkControlWords = map[string]bool{
	"end":  true,
	"done": true,
	"DONE": true,
}

// IsPassthrough determines whether a text string should skip translation.
// Returns true for: ink control patterns, variable references, template strings,
// ink control words, punctuation-only, whitespace-only, and empty strings.
// Returns false for: dialogue text, tagged text with translatable content,
// choice text with E- prefix.
func IsPassthrough(text string) bool {
	trimmed := strings.TrimSpace(text)

	// Empty or whitespace-only
	if trimmed == "" {
		return true
	}

	// Ink control words
	if inkControlWords[trimmed] {
		return true
	}

	// v1 control patterns
	if passthroughControlRe.MatchString(trimmed) {
		return true
	}

	// Pure variable references
	if variableRefRe.MatchString(trimmed) {
		return true
	}

	// Template strings with no translatable content
	if templateStringRe.MatchString(trimmed) {
		return true
	}

	// Single ASCII letter — skill check result markers (S=Success, F=Fail) and similar
	// game UI labels that are not translatable dialogue.
	if len(trimmed) == 1 && trimmed[0] >= 'A' && trimmed[0] <= 'Z' {
		return true
	}

	// Punctuation-only
	if isPunctOnly(trimmed) {
		return true
	}

	// Do NOT mark as passthrough:
	// - Text with HTML tags wrapping real words (e.g., "<b>COLLECTION</b>")
	// - Text with "E-" prefix (choice exit text, translatable)

	return false
}

// isPunctOnly returns true if s contains only punctuation and symbols (no letters or digits).
func isPunctOnly(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
