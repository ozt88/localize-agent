package translation

import "testing"

func TestPreparePromptText_ChoiceAndEmphasis(t *testing.T) {
	prepared := preparePromptText("ROLL14 str-Tell <i>him</i> to leave.", "ROLL14 str-...", textProfile{
		Kind:        textKindChoice,
		HasRichText: true,
	})

	if prepared.choicePrefix != "ROLL14 str-" {
		t.Fatalf("choicePrefix=%q", prepared.choicePrefix)
	}
	if prepared.source != "Tell [[E0]]him[[/E0]] to leave." {
		t.Fatalf("source=%q", prepared.source)
	}
	if len(prepared.emphasisSpans) != 1 {
		t.Fatalf("emphasisSpans=%v", prepared.emphasisSpans)
	}
}

func TestPreparePromptText_ChoiceWithSpacedPrefix(t *testing.T) {
	prepared := preparePromptText("ROLL15 str Tell him to leave.", "ROLL15 str ...", textProfile{
		Kind:        textKindChoice,
		HasRichText: false,
	})

	if prepared.choicePrefix != "ROLL15 str " {
		t.Fatalf("choicePrefix=%q", prepared.choicePrefix)
	}
	if prepared.source != "Tell him to leave." {
		t.Fatalf("source=%q", prepared.source)
	}
}

func TestPreparePromptText_PassthroughsPureNonEnglishLine(t *testing.T) {
	prepared := preparePromptText("<i>Hôr, mina vânner frân djupets sal!</i>", "", textProfile{
		Kind:        textKindNarration,
		HasRichText: true,
	})
	if !prepared.passthrough {
		t.Fatalf("passthrough=%v, want true", prepared.passthrough)
	}
}

func TestPreparePromptText_DoesNotPassthroughMixedForeignAndEnglishLine(t *testing.T) {
	prepared := preparePromptText("<b>Leva och dô i Askan</b>. That is your fate.", "", textProfile{
		Kind:        textKindNarration,
		HasRichText: true,
	})
	if prepared.passthrough {
		t.Fatalf("passthrough=%v, want false", prepared.passthrough)
	}
}

func TestRestorePreparedText_ReattachesPrefixAndEmphasis(t *testing.T) {
	meta := itemMeta{
		choicePrefix: "ROLL14 str-",
		emphasisSpans: []emphasisSpan{{
			openMarker: "[[E0]]",
			closeMarker: "[[/E0]]",
			openTag:    "<i>",
			closeTag:   "</i>",
		}},
	}

	got, err := restorePreparedText("[[E0]]그에게[[/E0]] 떠나라고 한다.", meta)
	if err != nil {
		t.Fatalf("restorePreparedText error=%v", err)
	}
	if got != "ROLL14 str-<i>그에게</i> 떠나라고 한다." {
		t.Fatalf("got=%q", got)
	}
}

func TestRestorePreparedText_UsesLocalizedStatCheckPrefix(t *testing.T) {
	meta := itemMeta{
		statCheck:   "STR 14",
		isStatCheck: true,
	}

	got, err := restorePreparedText("강하게 밀어붙인다", meta)
	if err != nil {
		t.Fatalf("restorePreparedText error=%v", err)
	}
	if got != "[힘 14] 강하게 밀어붙인다" {
		t.Fatalf("got=%q", got)
	}
}
