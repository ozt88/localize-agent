package scorellm

import "localize-agent/workflow/internal/v2pipeline"

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

// TargetState returns the pipeline state this score routes to per D-14.
func (r *ScoreResult) TargetState() string {
	switch r.FailureType {
	case "pass":
		return v2pipeline.StateDone
	case "translation", "both":
		return v2pipeline.StatePendingTranslate
	case "format":
		return v2pipeline.StatePendingFormat
	default:
		return v2pipeline.StateFailed // unknown failure_type
	}
}
