package semanticreview

import "testing"

func TestBuildReportItem_AddsFormatResidueTag(t *testing.T) {
	item := ReviewItem{
		ID:           "x",
		SourceEN:     `The goblin leans towards you and whispers, "Don't.`,
		TranslatedKO: `고블린이 당신에게 기대며 속삭인다." 하지 마.", prev_ko":"...`,
	}
	got := BuildReportItem(item, `The goblin leans toward you and whispers, "Don't."`, 0.9)
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
		TranslatedKO: "별로 흥미롭지 않군.",
	}
	got := BuildDirectScoreReportItem(item, directScoreResult{
		ID:             "x",
		WeirdnessScore: 0.7,
		ReasonTags:     []string{"speech_act_drift"},
		ShortReason:    "judgment shifted from impressed to interesting",
	})
	if got.ScoreFinal != 0.7 {
		t.Fatalf("ScoreFinal=%v", got.ScoreFinal)
	}
	if got.ShortReason == "" || len(got.ReasonTags) != 1 {
		t.Fatalf("unexpected direct report item: %+v", got)
	}
}

func TestBuildDirectScoreReportItem_ScoreOnly(t *testing.T) {
	item := ReviewItem{
		ID:           "x",
		SourceEN:     "I'm not impressed.",
		TranslatedKO: "별로.",
	}
	got := BuildDirectScoreReportItem(item, directScoreResult{
		ID:             "x",
		WeirdnessScore: 0.6,
	})
	if got.ScoreFinal != 0.6 {
		t.Fatalf("ScoreFinal=%v", got.ScoreFinal)
	}
	if got.ShortReason != "" || len(got.ReasonTags) != 0 {
		t.Fatalf("expected score-only report, got %+v", got)
	}
}
