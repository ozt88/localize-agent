package inkparse

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// SpeakerEntry represents a single speaker in the allow-list.
type SpeakerEntry struct {
	Name      string `json:"name"`
	Frequency int    `json:"frequency"`
	Verified  bool   `json:"verified"`
}

// RejectedEntry represents a rejected (non-speaker) tag.
type RejectedEntry struct {
	Name      string `json:"name"`
	Frequency int    `json:"frequency"`
	Reason    string `json:"reason"`
}

// SpeakerAllowList holds the verified speaker names and rejected tags.
type SpeakerAllowList struct {
	Version     int             `json:"version"`
	Description string          `json:"description"`
	Speakers    []SpeakerEntry  `json:"speakers"`
	Rejected    []RejectedEntry `json:"rejected"`
	lookup      map[string]bool
}

// LoadSpeakerAllowList reads and parses a speaker allow-list JSON file.
func LoadSpeakerAllowList(path string) (*SpeakerAllowList, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load speaker allow-list: %w", err)
	}
	var al SpeakerAllowList
	if err := json.Unmarshal(data, &al); err != nil {
		return nil, fmt.Errorf("parse speaker allow-list: %w", err)
	}
	al.lookup = make(map[string]bool, len(al.Speakers))
	for _, s := range al.Speakers {
		al.lookup[strings.ToLower(s.Name)] = true
	}
	return &al, nil
}

// globalAllowList is the package-level allow-list used by isSpeakerTag.
// Set via SetSpeakerAllowList at initialization time.
var globalAllowList *SpeakerAllowList

// SetSpeakerAllowList sets (or clears) the global speaker allow-list
// used by isSpeakerTag as a priority-1 check. Pass nil to disable.
func SetSpeakerAllowList(al *SpeakerAllowList) {
	globalAllowList = al
}

// IsAllowed returns true if the speaker name is in the allow-list.
// If the allow-list is nil or not loaded, returns true for backward
// compatibility (no filtering applied).
func (al *SpeakerAllowList) IsAllowed(speaker string) bool {
	if al == nil || al.lookup == nil {
		return true // no allow-list loaded = allow all (backward compat)
	}
	return al.lookup[strings.ToLower(strings.TrimSpace(speaker))]
}
