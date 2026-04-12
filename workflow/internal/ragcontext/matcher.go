package ragcontext

import "strings"

// FormatHints formats RAG hints into a compact string for prompt injection.
// Format matches lore.go formatLoreHints pattern: "Term: Description | Term2: Description2"
func FormatHints(hints []RAGHint) string {
	if len(hints) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, h := range hints {
		if i > 0 {
			sb.WriteString(" | ")
		}
		sb.WriteString(h.Term)
		sb.WriteString(": ")
		sb.WriteString(h.Description)
	}
	return sb.String()
}
