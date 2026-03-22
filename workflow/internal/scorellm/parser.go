package scorellm

import (
	"encoding/json"
	"fmt"
	"strings"

	"localize-agent/workflow/pkg/shared"
)

// validFailureTypes lists the accepted failure_type values per D-14.
var validFailureTypes = map[string]bool{
	"pass":        true,
	"translation": true,
	"format":      true,
	"both":        true,
}

// ParseScoreResponse parses the Score LLM JSON response.
// Handles direct JSON and JSON wrapped in markdown code fences (Pitfall 5).
// Validates failure_type is one of: pass, translation, format, both.
// Validates scores are in 0-10 range.
func ParseScoreResponse(raw string) (*ScoreResult, error) {
	jsonStr := strings.TrimSpace(raw)

	var result ScoreResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		// Try extracting from code fence.
		if m := shared.CodeFenceRe.FindStringSubmatch(raw); len(m) > 1 {
			jsonStr = strings.TrimSpace(m[1])
			if err2 := json.Unmarshal([]byte(jsonStr), &result); err2 != nil {
				return nil, fmt.Errorf("failed to parse score response: %w", err2)
			}
		} else {
			return nil, fmt.Errorf("failed to parse score response: %w", err)
		}
	}

	// Validate failure_type.
	if !validFailureTypes[result.FailureType] {
		return nil, fmt.Errorf("invalid failure_type: %q (must be pass/translation/format/both)", result.FailureType)
	}

	// Validate score ranges.
	if result.TranslationScore < 0 || result.TranslationScore > 10 {
		return nil, fmt.Errorf("translation_score %v out of range 0-10", result.TranslationScore)
	}
	if result.FormatScore < 0 || result.FormatScore > 10 {
		return nil, fmt.Errorf("format_score %v out of range 0-10", result.FormatScore)
	}

	return &result, nil
}
