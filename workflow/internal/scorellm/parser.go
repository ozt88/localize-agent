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

// ParseBatchScoreResponse parses a JSON array of score results.
// Falls back to single-object parsing if the array parse fails (single-item batch).
func ParseBatchScoreResponse(raw string, expectedCount int) ([]*ScoreResult, error) {
	jsonStr := strings.TrimSpace(raw)

	// Try code fence extraction first.
	if m := shared.CodeFenceRe.FindStringSubmatch(raw); len(m) > 1 {
		jsonStr = strings.TrimSpace(m[1])
	}

	// Try parsing as array.
	var results []*ScoreResult
	if err := json.Unmarshal([]byte(jsonStr), &results); err != nil {
		// Fallback: single object (batch of 1).
		single, singleErr := ParseScoreResponse(raw)
		if singleErr != nil {
			return nil, fmt.Errorf("failed to parse batch score response: %w (single fallback: %v)", err, singleErr)
		}
		return []*ScoreResult{single}, nil
	}

	if len(results) != expectedCount {
		return nil, fmt.Errorf("batch score count mismatch: expected %d, got %d", expectedCount, len(results))
	}

	// Validate each result.
	for i, r := range results {
		if !validFailureTypes[r.FailureType] {
			return nil, fmt.Errorf("item %d: invalid failure_type: %q", i+1, r.FailureType)
		}
		if r.TranslationScore < 0 || r.TranslationScore > 10 {
			return nil, fmt.Errorf("item %d: translation_score %v out of range", i+1, r.TranslationScore)
		}
		if r.FormatScore < 0 || r.FormatScore > 10 {
			return nil, fmt.Errorf("item %d: format_score %v out of range", i+1, r.FormatScore)
		}
	}

	return results, nil
}
