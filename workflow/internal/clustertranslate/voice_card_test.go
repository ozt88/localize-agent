package clustertranslate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testVoiceCardJSON = `{
  "Snell": {
    "speech_style": "조용하고 신중한 어조, 짧은 문장 선호",
    "honorific": "평어",
    "personality": "내성적, 관찰력 있음"
  },
  "Viira": {
    "speech_style": "직설적이고 거친 말투",
    "honorific": "반말",
    "personality": "공격적, 솔직함"
  }
}`

func TestLoadVoiceCards_EmptyPath(t *testing.T) {
	cards, err := LoadVoiceCards("")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if cards != nil {
		t.Fatalf("expected nil map, got %v", cards)
	}
}

func TestLoadVoiceCards_ValidPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "voice_cards.json")
	if err := os.WriteFile(path, []byte(testVoiceCardJSON), 0644); err != nil {
		t.Fatal(err)
	}

	cards, err := LoadVoiceCards(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	snell, ok := cards["Snell"]
	if !ok {
		t.Fatal("expected 'Snell' key in map")
	}
	if snell.SpeechStyle == "" {
		t.Error("SpeechStyle should not be empty")
	}
	if snell.Honorific == "" {
		t.Error("Honorific should not be empty")
	}
	if snell.Personality == "" {
		t.Error("Personality should not be empty")
	}
}

func TestLoadVoiceCards_InvalidPath(t *testing.T) {
	_, err := LoadVoiceCards("/nonexistent/path/voice_cards.json")
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestBuildNamedVoiceSection_Matching(t *testing.T) {
	cards := map[string]VoiceCard{
		"Snell": {SpeechStyle: "조용하고 신중한 어조", Honorific: "평어", Personality: "내성적"},
	}
	result := BuildNamedVoiceSection([]string{"Snell"}, cards)
	if !strings.Contains(result, "## Named Character Voice Guide") {
		t.Error("expected header '## Named Character Voice Guide'")
	}
	if !strings.Contains(result, "**Snell**") {
		t.Error("expected Snell entry")
	}
}

func TestBuildNamedVoiceSection_NoMatch(t *testing.T) {
	cards := map[string]VoiceCard{
		"Snell": {SpeechStyle: "조용하고 신중한 어조", Honorific: "평어", Personality: "내성적"},
	}
	result := BuildNamedVoiceSection([]string{"UnknownChar"}, cards)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestBuildNamedVoiceSection_Dedup(t *testing.T) {
	cards := map[string]VoiceCard{
		"Snell": {SpeechStyle: "조용하고 신중한 어조", Honorific: "평어", Personality: "내성적"},
	}
	result := BuildNamedVoiceSection([]string{"Snell", "Snell", "Snell"}, cards)
	count := strings.Count(result, "**Snell**")
	if count != 1 {
		t.Errorf("expected Snell to appear once, appeared %d times", count)
	}
}

func TestBuildNamedVoiceSection_IgnoresAbilityScore(t *testing.T) {
	cards := map[string]VoiceCard{
		"Snell": {SpeechStyle: "조용하고 신중한 어조", Honorific: "평어", Personality: "내성적"},
	}
	// ability-score speakers should not match (they're not in cards anyway)
	result := BuildNamedVoiceSection([]string{"wis", "str", "int", "cha", "dex", "con"}, cards)
	if result != "" {
		t.Errorf("expected empty string for ability-score speakers, got %q", result)
	}
}
