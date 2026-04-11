package tagformat

import "regexp"

// tagRe matches all 7 rich-text tag types: i, b, shake, wiggle, u, size=N, s.
// Handles opening tags (with optional attributes like size=50) and closing tags.
var tagRe = regexp.MustCompile(`</?[a-zA-Z][a-zA-Z0-9]*(?:=[^>]*)?>`)

// ExtractTags returns an ordered list of all tags found in s.
func ExtractTags(s string) []string {
	return tagRe.FindAllString(s, -1)
}

