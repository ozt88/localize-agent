package glossary

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadGlossaryTerms parses GlossaryTerms.txt CSV and extracts term names.
// Format: ID,ResponseAS,Tags,DC,ENGLISH,GERMAN
// Term name = text before " - " or " – " in the ENGLISH column.
// All terms use Mode="preserve" per D-10.
func LoadGlossaryTerms(path string) ([]Term, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open glossary terms: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(stripBOM(f))
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	// Skip header
	if _, err := r.Read(); err != nil {
		return nil, fmt.Errorf("read glossary header: %w", err)
	}

	var terms []Term
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // skip malformed rows
		}

		// Need at least 5 columns (ENGLISH is column index 4)
		if len(record) < 5 {
			continue
		}

		english := strings.TrimSpace(record[4])
		if english == "" {
			continue
		}

		name := extractTermName(english)
		if name == "" {
			continue
		}

		terms = append(terms, Term{
			Source: name,
			Target: name,
			Mode:   "preserve",
		})
	}

	return terms, nil
}

// extractTermName extracts the term name before " - " or " – " separator.
func extractTermName(english string) string {
	for _, sep := range []string{" - ", " – "} {
		if idx := strings.Index(english, sep); idx > 0 {
			return strings.TrimSpace(english[:idx])
		}
	}
	// If no separator, use the whole text as the term name
	return strings.TrimSpace(english)
}

// LoadLocalizationTexts reads all .txt files in a directory as CSV (ID,ENGLISH,KOREAN)
// and extracts entity names. Mode="preserve".
func LoadLocalizationTexts(dir string) ([]Term, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read localization dir: %w", err)
	}

	seen := make(map[string]bool)
	var terms []Term

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".txt") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		fileTerms, err := loadLocalizationFile(path)
		if err != nil {
			continue // skip files that fail to parse
		}

		for _, t := range fileTerms {
			lower := strings.ToLower(t.Source)
			if !seen[lower] {
				seen[lower] = true
				terms = append(terms, t)
			}
		}
	}

	return terms, nil
}

// loadLocalizationFile parses a single localization CSV file (ID,ENGLISH,KOREAN).
func loadLocalizationFile(path string) ([]Term, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(stripBOM(f))
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	// Skip header
	if _, err := r.Read(); err != nil {
		return nil, err
	}

	var terms []Term
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		// Need at least 2 columns (ENGLISH is index 1)
		if len(record) < 2 {
			continue
		}

		english := strings.TrimSpace(record[1])
		if english == "" {
			continue
		}

		// Extract term name (before " - " if present)
		name := extractTermName(english)
		if name == "" {
			continue
		}

		terms = append(terms, Term{
			Source: name,
			Target: name,
			Mode:   "preserve",
		})
	}

	return terms, nil
}

// LoadSpeakers creates preserve-mode terms from speaker names.
// Deduplicates and skips empty names.
func LoadSpeakers(names []string) []Term {
	seen := make(map[string]bool)
	var terms []Term
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		lower := strings.ToLower(name)
		if seen[lower] {
			continue
		}
		seen[lower] = true
		terms = append(terms, Term{
			Source: name,
			Target: name,
			Mode:   "preserve",
		})
	}
	return terms
}

// LoadGlossary loads from all 3 sources, deduplicates by case-insensitive Source,
// and builds the term index.
func LoadGlossary(glossaryPath, locTextsDir string, speakers []string) (*GlossarySet, error) {
	var allTerms []Term

	// 1. Glossary terms
	if glossaryPath != "" {
		terms, err := LoadGlossaryTerms(glossaryPath)
		if err != nil {
			return nil, fmt.Errorf("glossary terms: %w", err)
		}
		allTerms = append(allTerms, terms...)
	}

	// 2. Localization texts
	if locTextsDir != "" {
		terms, err := LoadLocalizationTexts(locTextsDir)
		if err != nil {
			return nil, fmt.Errorf("localization texts: %w", err)
		}
		allTerms = append(allTerms, terms...)
	}

	// 3. Speaker names
	speakerTerms := LoadSpeakers(speakers)
	allTerms = append(allTerms, speakerTerms...)

	// Deduplicate by case-insensitive Source
	gs := &GlossarySet{
		termIndex: make(map[string]int),
	}
	for _, term := range allTerms {
		lower := strings.ToLower(term.Source)
		if _, exists := gs.termIndex[lower]; exists {
			continue
		}
		gs.termIndex[lower] = len(gs.Terms)
		gs.Terms = append(gs.Terms, term)
	}

	return gs, nil
}

// WarmupTerms returns the first n terms sorted alphabetically for determinism.
// Per D-11: top 50 for warmup.
func (gs *GlossarySet) WarmupTerms(n int) []Term {
	if len(gs.Terms) == 0 {
		return nil
	}

	sorted := make([]Term, len(gs.Terms))
	copy(sorted, gs.Terms)
	sort.Slice(sorted, func(i, j int) bool {
		return strings.ToLower(sorted[i].Source) < strings.ToLower(sorted[j].Source)
	})

	if n > len(sorted) {
		n = len(sorted)
	}
	return sorted[:n]
}

// FilterForBatch returns terms whose Source appears in batchText (case-insensitive
// substring match), excluding any in the warmup set. Per D-11.
func (gs *GlossarySet) FilterForBatch(batchText string, excludeWarmup []Term) []Term {
	excludeSet := make(map[string]bool)
	for _, t := range excludeWarmup {
		excludeSet[strings.ToLower(t.Source)] = true
	}

	lowerText := strings.ToLower(batchText)
	var result []Term
	for _, term := range gs.Terms {
		lower := strings.ToLower(term.Source)
		if excludeSet[lower] {
			continue
		}
		if strings.Contains(lowerText, lower) {
			result = append(result, term)
		}
	}
	return result
}

// FormatJSON formats terms as a JSON array per D-12.
func (gs *GlossarySet) FormatJSON(terms []Term) string {
	if len(terms) == 0 {
		return "[]"
	}
	data, err := json.Marshal(terms)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// stripBOM wraps a reader to skip a UTF-8 BOM if present.
func stripBOM(r io.Reader) io.Reader {
	buf := make([]byte, 3)
	n, err := r.Read(buf)
	if err != nil || n < 3 {
		// Return what was read plus the rest
		return io.MultiReader(strings.NewReader(string(buf[:n])), r)
	}
	// UTF-8 BOM: EF BB BF
	if buf[0] == 0xEF && buf[1] == 0xBB && buf[2] == 0xBF {
		return r // skip BOM
	}
	// Not a BOM, put it back
	return io.MultiReader(strings.NewReader(string(buf[:n])), r)
}
