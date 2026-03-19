package translation

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

// loreEntry represents a single lore entry from the wiki-derived termbank.
type loreEntry struct {
	Term     string   // the dictionary key (e.g. "The Cleric")
	Lore     string   `json:"lore"`
	Category string   `json:"category"`
	Aliases  []string `json:"aliases,omitempty"`
	Related  []string `json:"related,omitempty"`
}

// loadLoreEntries loads the lore termbank JSON file.
// The file is a flat map: { "Term": { "lore": "...", "category": "...", ... } }
func loadLoreEntries(path string) ([]loreEntry, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string]loreEntry
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	out := make([]loreEntry, 0, len(m))
	for term, entry := range m {
		entry.Term = term
		if strings.TrimSpace(entry.Lore) == "" {
			continue
		}
		out = append(out, entry)
	}
	return out, nil
}

// matchedLoreEntries returns lore entries whose term (or alias) appears in the
// English source text. Up to maxHints entries are returned, prioritising longer
// term matches (more specific) first.
func matchedLoreEntries(entries []loreEntry, en string, maxHints int) []loreEntry {
	if len(entries) == 0 || strings.TrimSpace(en) == "" {
		return nil
	}
	if maxHints <= 0 {
		maxHints = 3
	}

	type match struct {
		entry   loreEntry
		termLen int
	}
	var hits []match
	seen := map[string]bool{}

	for _, entry := range entries {
		terms := append([]string{entry.Term}, entry.Aliases...)
		for _, term := range terms {
			term = strings.TrimSpace(term)
			if term == "" || len(term) < 3 {
				continue
			}
			if containsLoreTerm(en, term) {
				key := entry.Term
				if seen[key] {
					break
				}
				seen[key] = true
				hits = append(hits, match{entry: entry, termLen: len(entry.Term)})
				break
			}
		}
	}

	// Sort by term length descending (more specific terms first).
	for i := 0; i < len(hits); i++ {
		for j := i + 1; j < len(hits); j++ {
			if hits[j].termLen > hits[i].termLen {
				hits[i], hits[j] = hits[j], hits[i]
			}
		}
	}

	out := make([]loreEntry, 0, maxHints)
	for i, h := range hits {
		if i >= maxHints {
			break
		}
		out = append(out, h.entry)
	}
	return out
}

// containsLoreTerm checks whether en contains the given term with word boundaries.
func containsLoreTerm(en, term string) bool {
	en = strings.TrimSpace(en)
	term = strings.TrimSpace(term)
	if en == "" || term == "" {
		return false
	}
	if strings.EqualFold(en, term) {
		return true
	}
	// Word-boundary matching with common English suffixes.
	suffixPattern := `(?:'s|s|ed|ing)?`
	pattern := `(?i)(?:^|[^A-Za-z0-9])` + regexp.QuoteMeta(term) + suffixPattern + `(?:[^A-Za-z0-9]|$)`
	re, err := regexp.Compile(pattern)
	if err != nil {
		return strings.Contains(strings.ToLower(en), strings.ToLower(term))
	}
	return re.FindStringIndex(en) != nil
}

// formatLoreHints formats matched lore entries into a compact string for prompt injection.
func formatLoreHints(entries []loreEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, e := range entries {
		if i > 0 {
			sb.WriteString(" | ")
		}
		sb.WriteString(e.Term)
		sb.WriteString(": ")
		sb.WriteString(e.Lore)
	}
	return sb.String()
}
