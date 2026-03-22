package scorellm

// ScoreTask represents a single item to be evaluated by the Score LLM.
type ScoreTask struct {
	BlockID     string
	ENSource    string // original English
	KOFormatted string // final Korean (with tags if applicable)
	HasTags     bool
}

// ScoreResult holds the parsed Score LLM response.
type ScoreResult struct {
	TranslationScore float64 `json:"translation_score"`
	FormatScore      float64 `json:"format_score"`
	FailureType      string  `json:"failure_type"` // "pass", "translation", "format", "both"
	Reason           string  `json:"reason"`
}

// ScoreFinal returns the averaged score for pipeline storage.
func (r *ScoreResult) ScoreFinal() float64 {
	return (r.TranslationScore + r.FormatScore) / 2.0
}

// TargetState returns the pipeline state this score routes to per D-14.
// Uses string literals matching v2pipeline state constants to avoid import cycle.
func (r *ScoreResult) TargetState() string {
	switch r.FailureType {
	case "pass":
		return "done"
	case "translation", "both":
		return "pending_translate"
	case "format":
		return "pending_format"
	default:
		return "failed" // unknown failure_type
	}
}
