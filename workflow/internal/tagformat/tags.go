package tagformat

import "regexp"

// tagRe matches all 7 rich-text tag types: i, b, shake, wiggle, u, size=N, s.
// Handles opening tags (with optional attributes like size=50) and closing tags.
var tagRe = regexp.MustCompile(`</?[a-zA-Z][a-zA-Z0-9]*(?:=[^>]*)?>`)

// ExtractTags returns an ordered list of all tags found in s.
func ExtractTags(s string) []string {
	return tagRe.FindAllString(s, -1)
}

// HasRichTags returns true if s contains any rich-text tags.
func HasRichTags(s string) bool {
	return tagRe.MatchString(s)
}

// StripTags removes all rich-text tags from s.
func StripTags(s string) string {
	return tagRe.ReplaceAllString(s, "")
}

// CountTags returns the number of tags in s.
func CountTags(s string) int {
	return len(ExtractTags(s))
}
