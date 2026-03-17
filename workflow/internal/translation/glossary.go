package translation

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

type glossaryFilePayload struct {
	TranslateTerms []glossaryEntry `json:"translate_terms"`
	PreserveTerms  []glossaryEntry `json:"preserve_terms"`
}

func loadGlossaryEntries(path string) ([]glossaryEntry, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var payload glossaryFilePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	out := make([]glossaryEntry, 0, len(payload.TranslateTerms)+len(payload.PreserveTerms))
	for _, entry := range append(payload.TranslateTerms, payload.PreserveTerms...) {
		if strings.TrimSpace(entry.Source) == "" || strings.TrimSpace(entry.Target) == "" {
			continue
		}
		out = append(out, glossaryEntry{
			Source: strings.TrimSpace(entry.Source),
			Target: strings.TrimSpace(entry.Target),
			Mode:   strings.TrimSpace(entry.Mode),
		})
	}
	return out, nil
}

func matchedGlossaryEntries(entries []glossaryEntry, en string) []glossaryEntry {
	if len(entries) == 0 {
		return nil
	}
	en = strings.TrimSpace(en)
	if en == "" {
		return nil
	}
	out := make([]glossaryEntry, 0, 4)
	seen := map[string]bool{}
	for _, entry := range entries {
		if containsGlossaryTerm(en, entry.Source) {
			key := entry.Source + "=>" + entry.Target + "/" + entry.Mode
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, entry)
		}
	}
	return out
}

func containsGlossaryTerm(en, term string) bool {
	en = strings.TrimSpace(en)
	term = strings.TrimSpace(term)
	if en == "" || term == "" {
		return false
	}
	if strings.EqualFold(en, term) {
		return true
	}
	pattern := `(?i)(^|[^A-Za-z0-9])` + regexp.QuoteMeta(term) + `([^A-Za-z0-9]|$)`
	re := regexp.MustCompile(pattern)
	return re.FindStringIndex(en) != nil
}
