package ragcontext

// RAGHint represents a single world-building context hint for prompt injection.
type RAGHint struct {
	Term        string `json:"term"`
	Description string `json:"description"`
	Category    string `json:"category"`
}
