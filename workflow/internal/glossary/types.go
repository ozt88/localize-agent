package glossary

// Term represents a single glossary entry with source/target pair and translation mode.
type Term struct {
	Source string `json:"source"` // English term
	Target string `json:"target"` // Korean (or same as Source for preserve)
	Mode   string `json:"mode"`   // "preserve" or "translate"
}

// GlossarySet holds a deduplicated set of glossary terms with fast lookup.
type GlossarySet struct {
	Terms     []Term
	termIndex map[string]int // lowercase source -> index in Terms
}
