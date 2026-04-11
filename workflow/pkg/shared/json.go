package shared

import "regexp"

// CodeFenceRe matches markdown code fences wrapping content (typically JSON).
// Used by tagformat and scorellm to strip LLM response formatting.
var CodeFenceRe = regexp.MustCompile("(?s)```(?:json)?\\s*(.+?)\\s*```")

// StripCodeFence extracts content from markdown code fences if present.
// Returns the original string if no code fence is found.
func StripCodeFence(s string) string {
	if m := CodeFenceRe.FindStringSubmatch(s); len(m) > 1 {
		return m[1]
	}
	return s
}

func ExtractJSONObjectChunks(raw string) []string {
	out := []string{}
	depth := 0
	inStr := false
	esc := false
	start := -1
	for i, r := range raw {
		if inStr {
			if esc {
				esc = false
			} else if r == '\\' {
				esc = true
			} else if r == '"' {
				inStr = false
			}
			continue
		}
		if r == '"' {
			inStr = true
			continue
		}
		if r == '{' {
			if depth == 0 {
				start = i
			}
			depth++
			continue
		}
		if r == '}' {
			depth--
			if depth == 0 && start >= 0 {
				out = append(out, raw[start:i+1])
				start = -1
			}
		}
	}
	return out
}
