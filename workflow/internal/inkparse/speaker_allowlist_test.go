package inkparse

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSpeakerAllowList(t *testing.T) {
	// Create temp JSON file
	content := `{
		"version": 1,
		"description": "test allow-list",
		"speakers": [
			{"name": "Braxo", "frequency": 161, "verified": true},
			{"name": "wis", "frequency": 1259, "verified": true},
			{"name": "Snell", "frequency": 2663, "verified": true}
		],
		"rejected": [
			{"name": "Shocked", "frequency": 7, "reason": "animation tag"}
		]
	}`

	dir := t.TempDir()
	path := filepath.Join(dir, "allow_list.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	al, err := LoadSpeakerAllowList(path)
	if err != nil {
		t.Fatalf("LoadSpeakerAllowList: %v", err)
	}

	if al.Version != 1 {
		t.Errorf("Version = %d, want 1", al.Version)
	}
	if len(al.Speakers) != 3 {
		t.Errorf("Speakers count = %d, want 3", len(al.Speakers))
	}
	if len(al.Rejected) != 1 {
		t.Errorf("Rejected count = %d, want 1", len(al.Rejected))
	}
	if al.lookup == nil {
		t.Error("lookup map not initialized")
	}
}

func TestLoadSpeakerAllowListFileNotFound(t *testing.T) {
	_, err := LoadSpeakerAllowList("/nonexistent/path.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadSpeakerAllowListInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{invalid"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadSpeakerAllowList(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestIsAllowed(t *testing.T) {
	al := &SpeakerAllowList{
		lookup: map[string]bool{
			"braxo": true,
			"wis":   true,
			"snell": true,
		},
	}

	tests := []struct {
		name string
		want bool
	}{
		{"Braxo", true},
		{"braxo", true},
		{"BRAXO", true},
		{"wis", true},
		{"Snell", true},
		{" Snell ", true},  // trimmed
		{"Shocked", false}, // not in allow-list
		{"Unknown", false},
		{"", false},
	}

	for _, tt := range tests {
		got := al.IsAllowed(tt.name)
		if got != tt.want {
			t.Errorf("IsAllowed(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsAllowedNilList(t *testing.T) {
	var al *SpeakerAllowList

	// nil allow-list should return true (backward compat)
	if !al.IsAllowed("anything") {
		t.Error("nil SpeakerAllowList should allow all speakers")
	}
}

func TestIsAllowedEmptyLookup(t *testing.T) {
	al := &SpeakerAllowList{}

	// nil lookup map should return true (backward compat)
	if !al.IsAllowed("anything") {
		t.Error("SpeakerAllowList with nil lookup should allow all speakers")
	}
}
