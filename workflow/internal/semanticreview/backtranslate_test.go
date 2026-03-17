package semanticreview

import (
	"strings"
	"testing"
)

func TestExtractBacktranslationObjects_Array(t *testing.T) {
	raw := `[{"id":"a","backtranslated_en":"Hello."},{"id":"b","backtranslated_en":"Goodbye."}]`
	got := extractBacktranslationObjects(raw)
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].ID != "a" || got[0].BacktranslatedEN != "Hello." {
		t.Fatalf("unexpected first row: %+v", got[0])
	}
}

func TestBuildBatchBacktranslationPrompt(t *testing.T) {
	got := buildBatchBacktranslationPrompt([]ReviewItem{{ID: "x", TranslatedKO: "sample"}})
	if got == "" {
		t.Fatal("empty prompt")
	}
	if want := `"id":"x"`; !strings.Contains(got, want) {
		t.Fatalf("prompt missing %s: %s", want, got)
	}
}

func TestExtractDirectScoreObjects_Array(t *testing.T) {
	raw := `[{"id":"a","current_score":73,"fresh_score":81,"reason_tags":["semantic_drift"],"short_reason":"meaning changed"}]`
	got := extractDirectScoreObjects(raw, nil, false)
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].ID != "a" || got[0].CurrentScore != 73 || got[0].FreshScore != 81 {
		t.Fatalf("unexpected row: %+v", got[0])
	}
}

func TestExtractDirectScoreObjects_NDJSON(t *testing.T) {
	raw := "{\"current_score\":73,\"fresh_score\":81}\n{\"current_score\":68,\"fresh_score\":66}"
	items := []ReviewItem{{ID: "a"}, {ID: "b"}}
	got := extractDirectScoreObjects(raw, items, false)
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].ID != "a" || got[0].CurrentScore != 73 || got[0].FreshScore != 81 {
		t.Fatalf("unexpected first row: %+v", got[0])
	}
	if got[1].ID != "b" || got[1].CurrentScore != 68 || got[1].FreshScore != 66 {
		t.Fatalf("unexpected second row: %+v", got[1])
	}
}

func TestExtractDirectScoreObjects_SingleTupleArray(t *testing.T) {
	raw := `[73,81]`
	items := []ReviewItem{{ID: "a"}}
	got := extractDirectScoreObjects(raw, items, false)
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].ID != "a" || got[0].CurrentScore != 73 || got[0].FreshScore != 81 {
		t.Fatalf("unexpected row: %+v", got[0])
	}
}

func TestExtractDirectScoreObjects_ScoreOnlyArray(t *testing.T) {
	raw := `[[73,81]]`
	got := extractDirectScoreObjects(raw, []ReviewItem{{ID: "a"}}, true)
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].ID != "a" || got[0].CurrentScore != 73 || got[0].FreshScore != 81 {
		t.Fatalf("unexpected row: %+v", got[0])
	}
}

func TestExtractDirectScoreObjects_ScoreOnlyPlain(t *testing.T) {
	raw := "0\t73\t81\n1\t68\t66"
	items := []ReviewItem{{ID: "a"}, {ID: "b"}}
	got := extractDirectScoreObjects(raw, items, true)
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].ID != "a" || got[0].CurrentScore != 73 || got[0].FreshScore != 81 {
		t.Fatalf("unexpected first row: %+v", got[0])
	}
	if got[1].ID != "b" || got[1].CurrentScore != 68 || got[1].FreshScore != 66 {
		t.Fatalf("unexpected second row: %+v", got[1])
	}
}

func TestExtractDirectScoreObjects_ArrayTupleNormalizesFractionalScores(t *testing.T) {
	raw := `[[0.73,0.81]]`
	got := extractDirectScoreObjects(raw, []ReviewItem{{ID: "a"}}, true)
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].ID != "a" || got[0].CurrentScore != 73 || got[0].FreshScore != 81 {
		t.Fatalf("unexpected row: %+v", got[0])
	}
}

func TestExtractDirectScoreObjects_ScoreOnlyPlainNormalizesPercentStyleScores(t *testing.T) {
	raw := "0\t0.12\t0.87\n1\t12\t87"
	items := []ReviewItem{{ID: "a"}, {ID: "b"}}
	got := extractDirectScoreObjects(raw, items, true)
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].CurrentScore != 12 || got[0].FreshScore != 87 {
		t.Fatalf("unexpected first score: %+v", got[0])
	}
	if got[1].CurrentScore != 12 || got[1].FreshScore != 87 {
		t.Fatalf("unexpected second score: %+v", got[1])
	}
}

func TestBuildMinimalDirectScorePrompt_UsesCompactArrayContract(t *testing.T) {
	got := buildMinimalDirectScorePrompt([]ReviewItem{{
		ID:        "x",
		SourceEN:  "Hello.",
		CurrentKO: "안녕.",
		FreshKO:   "안녕하세요.",
		TextRole:  "choice",
	}}, "compact")
	for _, want := range []string{
		"Return exactly one JSON array.",
		"[current_score, fresh_score]",
		"[[91,84],[70,88]]",
		"prefer concise actionable option wording",
		"Input items:",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "retry_reason") {
		t.Fatalf("prompt should not include retry_reason payload: %s", got)
	}
}

func TestBuildMinimalDirectScorePrompt_UltraOmitsTextRoleRule(t *testing.T) {
	got := buildMinimalDirectScorePrompt([]ReviewItem{{
		ID:        "x",
		SourceEN:  "Hello.",
		CurrentKO: "안녕.",
		FreshKO:   "안녕하세요.",
		TextRole:  "choice",
	}}, "ultra")
	if strings.Contains(got, "text_role") {
		t.Fatalf("ultra prompt should not include text_role payload/rule: %s", got)
	}
	if strings.Contains(got, "prefer concise actionable option wording") {
		t.Fatalf("ultra prompt should omit choice-specific rule: %s", got)
	}
}

func TestDirectScoreWarmup_UsesDualScoreSemantics(t *testing.T) {
	got := directScoreWarmup(Config{})
	if strings.Contains(strings.ToLower(got), "oddness") {
		t.Fatalf("warmup should not mention oddness scoring: %s", got)
	}
	for _, want := range []string{
		"current_score and fresh_score",
		"Do not choose a winner.",
		"Do not recommend rewrite.",
		"Each score is an integer from 0 to 100. Higher is better.",
		"compact JSON array output format",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("warmup missing %q: %s", want, got)
		}
	}
}
