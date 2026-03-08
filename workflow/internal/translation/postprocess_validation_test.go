package translation

import "testing"

func TestValidateRestoredOutput_RejectsPoliteChoice(t *testing.T) {
	meta := itemMeta{
		sourceRaw: "ROLL14 str-Tell him to leave.",
		profile:   textProfile{Kind: textKindChoice},
	}
	err := validateRestoredOutput(meta, "ROLL14 str-그에게 떠나라고 말하십시오.")
	if err == nil {
		t.Fatal("expected polite choice rejection")
	}
}

func TestValidateRestoredOutput_RejectsEnglishInsideRichText(t *testing.T) {
	meta := itemMeta{
		sourceRaw: "Do <i>you</i> like maw pie?",
		profile:   textProfile{Kind: textKindDialogue, HasRichText: true},
	}
	err := validateRestoredOutput(meta, `너는 <i>fine dust</i>를 좋아해?`)
	if err == nil {
		t.Fatal("expected english-in-tag rejection")
	}
}

func TestValidateRestoredOutput_AllowsGoodRichChoice(t *testing.T) {
	meta := itemMeta{
		sourceRaw: "ROLL14 str-Tell <i>him</i> to leave.",
		profile:   textProfile{Kind: textKindChoice, HasRichText: true},
	}
	err := validateRestoredOutput(meta, "ROLL14 str-<i>그에게</i> 떠나라고 한다.")
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidateRestoredOutput_RejectsStructuredFieldResidue(t *testing.T) {
	meta := itemMeta{
		sourceRaw: `The goblin leans towards you and whispers, "Don't.`,
		profile:   textProfile{Kind: textKindDialogue},
	}
	err := validateRestoredOutput(meta, `고블린이 당신에게 기대며 속삭인다." 하지 마.", prev_ko":"고블린이 당신에게 기대며 속삭인다." 하지 마."`)
	if err == nil {
		t.Fatal("expected structured field residue rejection")
	}
}
