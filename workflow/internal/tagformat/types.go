package tagformat

// FormatTask represents a single EN+KO pair for tag restoration by codex-spark.
type FormatTask struct {
	BlockID  string // pipeline item ID
	ENSource string // original English with tags
	KOPlain  string // Korean translation without tags
}

// FormatResult holds the output of tag restoration.
type FormatResult struct {
	BlockID     string // pipeline item ID
	KOFormatted string // Korean with tags restored
	RawOutput   string // raw LLM response
}

// TagValidationError reports a mismatch between EN and KO tag sets.
type TagValidationError struct {
	Position int
	Expected string
	Got      string
	Message  string
}

func (e *TagValidationError) Error() string {
	return e.Message
}
