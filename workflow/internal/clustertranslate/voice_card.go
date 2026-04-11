package clustertranslate

import (
	"encoding/json"
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

