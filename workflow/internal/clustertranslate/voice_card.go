package clustertranslate

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// VoiceCard describes a named character's speaking style for Korean translation.
type VoiceCard struct {
	SpeechStyle string `json:"speech_style"` // 말투
	Honorific   string `json:"honorific"`    // 반말/평어/존대
	Personality string `json:"personality"`  // 성격 키워드
}

// LoadVoiceCards loads voice card data from a JSON file.
// Returns nil, nil if path is empty. The JSON structure is:
//
//	{"CharName": {"speech_style":"...", "honorific":"...", "personality":"..."}, ...}
func LoadVoiceCards(path string) (map[string]VoiceCard, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]VoiceCard
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// BuildNamedVoiceSection creates a voice guide section for named characters
// that appear in the current batch's speakers list.
// Returns empty string if no named characters match.
func BuildNamedVoiceSection(speakers []string, cards map[string]VoiceCard) string {
	if len(cards) == 0 {
		return ""
	}

	seen := make(map[string]bool)
	var sb strings.Builder

	for _, s := range speakers {
		name := strings.TrimSpace(s)
		if name == "" {
			continue
		}
		// Skip if already seen (dedup)
		if seen[name] {
			continue
		}
		card, ok := cards[name]
		if !ok {
			continue
		}
		seen[name] = true
		fmt.Fprintf(&sb, "- **%s**: %s, %s, %s\n", name, card.SpeechStyle, card.Honorific, card.Personality)
	}

	if sb.Len() == 0 {
		return ""
	}
	return "\n## Named Character Voice Guide\n" + sb.String()
}
