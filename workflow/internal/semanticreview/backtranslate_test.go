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
	got := buildBatchBacktranslationPrompt([]ReviewItem{{ID: "x", TranslatedKO: "?덈뀞."}})
	if got == "" {
		t.Fatal("empty prompt")
	}
	if want := `"id":"x"`; !strings.Contains(got, want) {
		t.Fatalf("prompt missing %s: %s", want, got)
	}
}

func TestExtractDirectScoreObjects_Array(t *testing.T) {
	raw := `[{"id":"a","weirdness_score":0.8,"reason_tags":["semantic_drift"],"short_reason":"meaning changed"}]`
	got := extractDirectScoreObjects(raw, nil, false)
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].ID != "a" || got[0].WeirdnessScore != 0.8 {
		t.Fatalf("unexpected row: %+v", got[0])
	}
}

func TestExtractDirectScoreObjects_ScoreOnlyArray(t *testing.T) {
	raw := `[{"id":"a","weirdness_score":0.8}]`
	got := extractDirectScoreObjects(raw, nil, true)
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].ID != "a" || got[0].WeirdnessScore != 0.8 {
		t.Fatalf("unexpected row: %+v", got[0])
	}
}

func TestExtractDirectScoreObjects_ScoreOnlyPlain(t *testing.T) {
	raw := "0\t0.8\n1\t0.2"
	items := []ReviewItem{{ID: "a"}, {ID: "b"}}
	got := extractDirectScoreObjects(raw, items, true)
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].ID != "a" || got[0].WeirdnessScore != 0.8 {
		t.Fatalf("unexpected first row: %+v", got[0])
	}
	if got[1].ID != "b" || got[1].WeirdnessScore != 0.2 {
		t.Fatalf("unexpected second row: %+v", got[1])
	}
}

func TestExtractDirectScoreObjects_ScoreOnlyPlainWithTags(t *testing.T) {
	raw := "0\t0.9\tsemantic_drift,wrong_speech_act"
	items := []ReviewItem{{ID: "a"}}
	got := extractDirectScoreObjects(raw, items, true)
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].ID != "a" || got[0].WeirdnessScore != 0.9 {
		t.Fatalf("unexpected row: %+v", got[0])
	}
	if len(got[0].ReasonTags) != 2 || got[0].ReasonTags[0] != "semantic_drift" || got[0].ReasonTags[1] != "wrong_speech_act" {
		t.Fatalf("unexpected tags: %+v", got[0].ReasonTags)
	}
}

func TestExtractDirectScoreObjects_ScoreOnlyPlainNormalizesPercentStyleScores(t *testing.T) {
	raw := "0\t12\n1\t87"
	items := []ReviewItem{{ID: "a"}, {ID: "b"}}
	got := extractDirectScoreObjects(raw, items, true)
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].WeirdnessScore != 0.12 {
		t.Fatalf("unexpected first score: %+v", got[0])
	}
	if got[1].WeirdnessScore != 0.87 {
		t.Fatalf("unexpected second score: %+v", got[1])
	}
}
