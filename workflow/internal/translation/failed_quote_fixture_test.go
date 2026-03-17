package translation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

type failedQuoteFixture struct {
	Count int                `json:"count"`
	Rows  []failedQuoteEntry `json:"rows"`
}

type failedQuoteEntry struct {
	ID         string `json:"id"`
	SourceRaw  string `json:"source_raw"`
	TextRole   string `json:"text_role"`
	PrevEN     string `json:"prev_en"`
	NextEN     string `json:"next_en"`
	ContextEN  string `json:"context_en"`
	StatCheck  string `json:"stat_check"`
	ChoiceMode string `json:"choice_mode"`
	LastError  string `json:"last_error"`
}

func loadFailedQuoteFixture(t *testing.T) failedQuoteFixture {
	t.Helper()
	path := filepath.Join("testdata", "failed_quote_fixture.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fx failedQuoteFixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if fx.Count == 0 || len(fx.Rows) == 0 {
		t.Fatalf("fixture is empty")
	}
	return fx
}

func TestFailedQuoteFixture_ReportsFragmentHintCoverage(t *testing.T) {
	fx := loadFailedQuoteFixture(t)
	counts := map[string]int{}
	covered := 0
	for _, row := range fx.Rows {
		pattern, _, _ := deriveFragmentHints(translationTask{
			BodyEN:     row.SourceRaw,
			TextRole:   row.TextRole,
			PrevEN:     row.PrevEN,
			NextEN:     row.NextEN,
			ContextEN:  row.ContextEN,
			StatCheck:  row.StatCheck,
			ChoiceMode: row.ChoiceMode,
		})
		if pattern != "" {
			covered++
			counts[pattern]++
		}
	}
	t.Logf("failed fixture rows=%d covered=%d coverage=%.1f%% counts=%v", len(fx.Rows), covered, float64(covered)*100.0/float64(len(fx.Rows)), counts)
	if covered == 0 {
		t.Fatalf("expected at least some failed rows to receive fragment hints")
	}
}

func TestFailedQuoteFixture_TargetableCoverageExceedsNinetyPercent(t *testing.T) {
	fx := loadFailedQuoteFixture(t)
	targetable := 0
	covered := 0
	for _, row := range fx.Rows {
		if !isTargetableQuoteFamily(row) {
			continue
		}
		targetable++
		pattern, _, _ := deriveFragmentHints(translationTask{
			BodyEN:     row.SourceRaw,
			TextRole:   row.TextRole,
			PrevEN:     row.PrevEN,
			NextEN:     row.NextEN,
			ContextEN:  row.ContextEN,
			StatCheck:  row.StatCheck,
			ChoiceMode: row.ChoiceMode,
		})
		if pattern != "" {
			covered++
		}
	}
	if targetable == 0 {
		t.Fatalf("no targetable rows in fixture")
	}
	coverage := float64(covered) * 100.0 / float64(targetable)
	t.Logf("targetable rows=%d covered=%d coverage=%.1f%%", targetable, covered, coverage)
	if coverage < 90.0 {
		t.Fatalf("targetable quote-family coverage %.1f%% < 90%%", coverage)
	}
}

var targetableStatLikeRe = regexp.MustCompile(`^(DC|ROLL|FC)\d+\s+[A-Za-z]+`)

func isTargetableQuoteFamily(row failedQuoteEntry) bool {
	src := row.SourceRaw
	if targetableStatLikeRe.MatchString(src) {
		return true
	}
	if regexp.MustCompile(`"`).MatchString(src) {
		return true
	}
	if row.TextRole == "fragment" || row.TextRole == "choice" || row.TextRole == "dialogue" || row.TextRole == "narration" {
		if len(src) > 0 && src[0] == '(' {
			return true
		}
		if regexp.MustCompile(`\.\.\.|-$`).MatchString(src) {
			return true
		}
	}
	return false
}
