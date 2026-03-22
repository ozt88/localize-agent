package tagformat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"localize-agent/workflow/pkg/shared"
)

// BuildFormatWarmup returns the system prompt for gpt-5.3-codex-spark tag restoration.
func BuildFormatWarmup() string {
	return `You are a tag restoration assistant. Given pairs of:
- EN: English text with rich-text tags (<b>, <i>, <shake>, <wiggle>, <u>, <size=N>, <s>)
- KO: Korean translation without tags

Your job:
1. Find where each EN tag's content maps to in the KO text
2. Insert the exact same tags at the corresponding positions in KO
3. Tags must be identical to EN (same case, same attributes, same order)
4. Do NOT translate or modify the text content
5. Return JSON: {"results": ["tagged KO line 1", "tagged KO line 2", ...]}

Reply with: OK`
}

// formatPair is the JSON structure for each EN+KO pair sent to the formatter.
type formatPair struct {
	EN string `json:"en"`
	KO string `json:"ko"`
}

// formatPrompt is the top-level JSON structure sent to the formatter.
type formatPrompt struct {
	Pairs []formatPair `json:"pairs"`
}

// BuildFormatPrompt builds a JSON prompt with EN+KO pairs for tag restoration per D-05.
// Caller is responsible for batching (D-06 recommends 3-5 pairs).
func BuildFormatPrompt(tasks []FormatTask) string {
	pairs := make([]formatPair, len(tasks))
	for i, t := range tasks {
		pairs[i] = formatPair{EN: t.ENSource, KO: t.KOPlain}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(formatPrompt{Pairs: pairs})
	return strings.TrimSpace(buf.String())
}

// formatResponse is the expected JSON structure from the formatter LLM.
type formatResponse struct {
	Results []string `json:"results"`
}

// ParseFormatResponse parses the LLM response containing tagged KO strings.
// Validates that the number of results matches expectedCount.
func ParseFormatResponse(raw string, expectedCount int) ([]string, error) {
	jsonStr := raw

	// Try extracting from code fence if direct parse fails.
	var resp formatResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		if m := shared.CodeFenceRe.FindStringSubmatch(raw); len(m) > 1 {
			jsonStr = strings.TrimSpace(m[1])
			if err2 := json.Unmarshal([]byte(jsonStr), &resp); err2 != nil {
				return nil, fmt.Errorf("failed to parse format response: %w", err2)
			}
		} else {
			return nil, fmt.Errorf("failed to parse format response: %w", err)
		}
	}

	if len(resp.Results) != expectedCount {
		return nil, fmt.Errorf("result count mismatch: got %d, expected %d", len(resp.Results), expectedCount)
	}

	return resp.Results, nil
}
