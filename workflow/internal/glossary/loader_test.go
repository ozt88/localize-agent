package glossary

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadGlossaryTerms(t *testing.T) {
	// Create a temp CSV matching GlossaryTerms.txt format: ID,ResponseAS,Tags,DC,ENGLISH,GERMAN
	content := "ID,ResponseAS,Tags,DC,ENGLISH,GERMAN\n" +
		"1,INT,Spells,10,\"Speak with Dead - a third level necromantic spell.\",\n" +
		"2,INT,Spells,8,Mage Hand - a common arcane cantrip.,\n" +
		"3,WIS,Esoterics,8,\"Cleric - a servant of a god, deeply devoted.\",\n" +
		"4,INT,City,12,\"Citizenry - a privileged political status.\",\n" +
		"5,STR,Folk,6,Urthfolk - the people of Urth.,\n"

	dir := t.TempDir()
	path := filepath.Join(dir, "GlossaryTerms.txt")
	os.WriteFile(path, []byte(content), 0644)

	terms, err := LoadGlossaryTerms(path)
	if err != nil {
		t.Fatalf("LoadGlossaryTerms error: %v", err)
	}
	if len(terms) != 5 {
		t.Fatalf("expected 5 terms, got %d", len(terms))
	}

	// Check extracted term names (before " - ")
	expected := []string{"Speak with Dead", "Mage Hand", "Cleric", "Citizenry", "Urthfolk"}
	for i, exp := range expected {
		if terms[i].Source != exp {
			t.Errorf("term[%d].Source = %q, want %q", i, terms[i].Source, exp)
		}
		if terms[i].Target != exp {
			t.Errorf("term[%d].Target = %q, want %q (preserve mode)", i, terms[i].Target, exp)
		}
		if terms[i].Mode != "preserve" {
			t.Errorf("term[%d].Mode = %q, want %q", i, terms[i].Mode, "preserve")
		}
	}
}

func TestLoadGlossaryTerms_SkipsBlankRows(t *testing.T) {
	content := "ID,ResponseAS,Tags,DC,ENGLISH,GERMAN\n" +
		"1,INT,Spells,10,Fireball - a spell.,\n" +
		"\n" +
		",,,,  ,\n" +
		"2,INT,Spells,10,Mage Hand - a cantrip.,\n"

	dir := t.TempDir()
	path := filepath.Join(dir, "GlossaryTerms.txt")
	os.WriteFile(path, []byte(content), 0644)

	terms, err := LoadGlossaryTerms(path)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(terms) != 2 {
		t.Fatalf("expected 2 terms, got %d", len(terms))
	}
}

func TestLoadLocalizationTexts(t *testing.T) {
	dir := t.TempDir()

	// Feats.txt format: ID,ENGLISH,KOREAN
	feats := "ID,ENGLISH,KOREAN\n" +
		"FEAT_1,\"Lone Cleric - At the start of your Short Rest, heal.\",\n" +
		"FEAT_2,Master Memorizer - During your Short Rest.,\n"
	os.WriteFile(filepath.Join(dir, "Feats.txt"), []byte(feats), 0644)

	// SpellTexts.txt
	spells := "ID,ENGLISH,KOREAN\n" +
		"Spell_1_1,SPELLNAME,\n" +
		"Spell_1_2,SPELL DESC,\n"
	os.WriteFile(filepath.Join(dir, "SpellTexts.txt"), []byte(spells), 0644)

	terms, err := LoadLocalizationTexts(dir)
	if err != nil {
		t.Fatalf("LoadLocalizationTexts error: %v", err)
	}
	// Should extract unique terms from ENGLISH column
	if len(terms) < 2 {
		t.Fatalf("expected at least 2 terms, got %d", len(terms))
	}
	for _, term := range terms {
		if term.Mode != "preserve" {
			t.Errorf("term %q mode = %q, want preserve", term.Source, term.Mode)
		}
	}
}

func TestLoadSpeakers(t *testing.T) {
	names := []string{"Braxo", "Snell", "", "Braxo", "Narrator"}
	terms := LoadSpeakers(names)

	// Should deduplicate and skip empty
	if len(terms) != 3 {
		t.Fatalf("expected 3 terms, got %d", len(terms))
	}

	sources := map[string]bool{}
	for _, term := range terms {
		sources[term.Source] = true
		if term.Mode != "preserve" {
			t.Errorf("speaker %q mode = %q, want preserve", term.Source, term.Mode)
		}
		if term.Source != term.Target {
			t.Errorf("speaker %q target = %q, want same as source", term.Source, term.Target)
		}
	}
	for _, name := range []string{"Braxo", "Snell", "Narrator"} {
		if !sources[name] {
			t.Errorf("missing speaker: %s", name)
		}
	}
}

func TestLoadGlossary_Dedup(t *testing.T) {
	dir := t.TempDir()

	// GlossaryTerms.txt with "Fireball"
	glossary := "ID,ResponseAS,Tags,DC,ENGLISH,GERMAN\n" +
		"1,INT,Spells,10,Fireball - a spell.,\n"
	os.WriteFile(filepath.Join(dir, "GlossaryTerms.txt"), []byte(glossary), 0644)

	// localizationtexts with "fireball" (case-insensitive duplicate)
	locDir := filepath.Join(dir, "loc")
	os.MkdirAll(locDir, 0755)
	loc := "ID,ENGLISH,KOREAN\n" +
		"Spell_1,fireball,\n" +
		"Spell_2,Mage Hand,\n"
	os.WriteFile(filepath.Join(locDir, "SpellTexts.txt"), []byte(loc), 0644)

	// speakers with "Fireball" (another dupe)
	speakers := []string{"Braxo"}

	gs, err := LoadGlossary(filepath.Join(dir, "GlossaryTerms.txt"), locDir, speakers)
	if err != nil {
		t.Fatalf("LoadGlossary error: %v", err)
	}

	// Should have: Fireball, Mage Hand, Braxo = 3 (fireball deduped)
	if len(gs.Terms) != 3 {
		t.Errorf("expected 3 unique terms, got %d: %v", len(gs.Terms), gs.Terms)
	}
}

func TestWarmupTerms(t *testing.T) {
	gs := &GlossarySet{
		Terms: []Term{
			{Source: "Zebra", Target: "Zebra", Mode: "preserve"},
			{Source: "Alpha", Target: "Alpha", Mode: "preserve"},
			{Source: "Mage", Target: "Mage", Mode: "preserve"},
		},
		termIndex: map[string]int{"zebra": 0, "alpha": 1, "mage": 2},
	}

	warmup := gs.WarmupTerms(2)
	if len(warmup) != 2 {
		t.Fatalf("expected 2 warmup terms, got %d", len(warmup))
	}
	// Should be alphabetically sorted
	if warmup[0].Source != "Alpha" {
		t.Errorf("first warmup term = %q, want Alpha", warmup[0].Source)
	}
	if warmup[1].Source != "Mage" {
		t.Errorf("second warmup term = %q, want Mage", warmup[1].Source)
	}
}

func TestWarmupTerms_MoreThanAvailable(t *testing.T) {
	gs := &GlossarySet{
		Terms: []Term{
			{Source: "Alpha", Target: "Alpha", Mode: "preserve"},
		},
		termIndex: map[string]int{"alpha": 0},
	}
	warmup := gs.WarmupTerms(50)
	if len(warmup) != 1 {
		t.Fatalf("expected 1 warmup term, got %d", len(warmup))
	}
}

func TestFilterForBatch(t *testing.T) {
	gs := &GlossarySet{
		Terms: []Term{
			{Source: "Braxo", Target: "Braxo", Mode: "preserve"},
			{Source: "Fireball", Target: "Fireball", Mode: "preserve"},
			{Source: "Norvik", Target: "Norvik", Mode: "preserve"},
		},
		termIndex: map[string]int{"braxo": 0, "fireball": 1, "norvik": 2},
	}

	batchText := "Braxo cast Fireball at the enemy."
	warmup := []Term{{Source: "Braxo", Target: "Braxo", Mode: "preserve"}}

	filtered := gs.FilterForBatch(batchText, warmup)

	// Should include Fireball (in text, not in warmup) but not Braxo (in warmup) or Norvik (not in text)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered term, got %d: %v", len(filtered), filtered)
	}
	if filtered[0].Source != "Fireball" {
		t.Errorf("filtered term = %q, want Fireball", filtered[0].Source)
	}
}

func TestFormatJSON(t *testing.T) {
	gs := &GlossarySet{}
	terms := []Term{
		{Source: "Braxo", Target: "Braxo", Mode: "preserve"},
		{Source: "Norvik", Target: "Norvik", Mode: "preserve"},
	}

	result := gs.FormatJSON(terms)

	// Should be valid JSON array per D-12
	if !strings.HasPrefix(result, "[") || !strings.HasSuffix(result, "]") {
		t.Errorf("FormatJSON should produce JSON array, got: %s", result)
	}
	if !strings.Contains(result, `"source":"Braxo"`) {
		t.Errorf("FormatJSON missing Braxo: %s", result)
	}
	if !strings.Contains(result, `"mode":"preserve"`) {
		t.Errorf("FormatJSON missing mode: %s", result)
	}
}
