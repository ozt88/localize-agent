package semanticreview

import "testing"

func TestBuildReportItem_AddsFormatResidueTag(t *testing.T) {
	item := ReviewItem{
		ID:           "x",
		SourceEN:     `The goblin leans towards you and whispers, "Don't.`,
		TranslatedKO: `bad output", prev_ko":"...`,
	}
	got := buildReportItem(item, `The goblin leans toward you and whispers, "Don't."`, 0.9)
	if got.ScoreFinal <= 0 {
		t.Fatalf("ScoreFinal=%v", got.ScoreFinal)
	}
	found := false
	for _, tag := range got.ReasonTags {
		if tag == "format_residue" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected format_residue tag, got %v", got.ReasonTags)
	}
}

func TestAlignmentPenalty(t *testing.T) {
	src := "Fair enough. Rules are rules."
	prev := "The goblin leans towards you and whispers, don't."
	back := "The goblin leans towards you and whispers, don't."
	if p := alignmentPenalty(src, prev, back); p <= 0 {
		t.Fatalf("expected positive alignment penalty, got %v", p)
	}
}

func TestBuildDirectScoreReportItem(t *testing.T) {
	item := ReviewItem{
		ID:           "x",
		SourceEN:     "I'm not impressed.",
		TranslatedKO: "fresh ko",
		CurrentKO:    "current ko",
		FreshKO:      "fresh ko",
	}
	got := buildDirectScoreReportItem(item, directScoreResult{
		ID:           "x",
		CurrentScore: 73,
		FreshScore:   81,
		ReasonTags:   []string{"speech_act_drift"},
		ShortReason:  "judgment shifted from impressed to interesting",
	})
	if got.ScoreFinal != 81 {
		t.Fatalf("ScoreFinal=%v", got.ScoreFinal)
	}
	if got.CurrentScore != 73 || got.FreshScore != 81 {
		t.Fatalf("unexpected score fields: %+v", got)
	}
	if got.TranslatedKO != "fresh ko" {
		t.Fatalf("expected fresh candidate to lead report output, got %+v", got)
	}
	if got.ShortReason == "" || len(got.ReasonTags) != 1 {
		t.Fatalf("unexpected direct report item: %+v", got)
	}
}

func TestBuildDirectScoreReportItem_ScoreOnly(t *testing.T) {
	item := ReviewItem{
		ID:           "x",
		SourceEN:     "I'm not impressed.",
		TranslatedKO: "fresh ko",
		CurrentKO:    "current ko",
		FreshKO:      "fresh ko",
	}
	got := buildDirectScoreReportItem(item, directScoreResult{
		ID:           "x",
		CurrentScore: 66,
		FreshScore:   62,
	})
	if got.ScoreFinal != 66 {
		t.Fatalf("ScoreFinal=%v", got.ScoreFinal)
	}
	if got.TranslatedKO != "current ko" {
		t.Fatalf("expected current candidate to lead report output, got %+v", got)
	}
	if got.ShortReason != "" || len(got.ReasonTags) != 0 {
		t.Fatalf("expected score-only report, got %+v", got)
	}
}
