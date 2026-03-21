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

func TestValidateRestoredOutput_AllowsForeignTextInTags(t *testing.T) {
	cases := []struct {
		name string
		src  string
		ko   string
	}{
		{
			name: "latin_phrase",
			src:  "<i>Ignem accende.</i>",
			ko:   "<i>Ignem accende.</i>",
		},
		{
			name: "finnish_phrase",
			src:  "<i>minun puuni</i>",
			ko:   "<i>minun puuni</i>",
		},
		{
			name: "proper_noun_in_tags",
			src:  "The <i>Commune</i> stands firm.",
			ko:   "<i>Commune</i>는 굳건하다.",
		},
	}
	for _, tc := range cases {
		meta := itemMeta{
			sourceRaw: tc.src,
			profile:   textProfile{Kind: textKindDialogue, HasRichText: true},
		}
		if err := validateRestoredOutput(meta, tc.ko); err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
	}
}

func TestTokenCompatible_AllowsReorderedHTMLTags(t *testing.T) {
	// Same tags, different order — should pass
	if !tokenCompatible("<i>hello</i> <b>world</b>", "<b>world</b> <i>hello</i>") {
		t.Fatal("expected reordered HTML tags to be compatible")
	}
}

func TestTokenCompatible_RejectsStructuralTokenMismatch(t *testing.T) {
	// $variable tokens must match exactly
	if tokenCompatible("$name says hello", "$other says hello") {
		t.Fatal("expected structural token mismatch rejection")
	}
}

func TestTokenCompatible_AllowsDroppedHTMLTags(t *testing.T) {
	// Source has tags, output dropped them — should fail (tag count mismatch)
	if tokenCompatible("<i>hello</i> world", "hello world") {
		t.Fatal("expected dropped HTML tags to fail")
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

func TestValidateRestoredOutput_RequiresLocalizedStatCheckPrefix(t *testing.T) {
	meta := itemMeta{
		sourceRaw:    "ROLL14 str-Force him to return the papers.",
		profile:      textProfile{Kind: textKindChoice},
		statCheck:    "STR 14",
		isStatCheck:  true,
	}
	if err := validateRestoredOutput(meta, "[힘 14] 서류를 돌려놓으라고 밀어붙인다"); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if err := validateRestoredOutput(meta, "ROLL14 str-서류를 돌려놓으라고 밀어붙인다"); err == nil {
		t.Fatal("expected raw stat-check prefix rejection")
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

func TestValidateRestoredOutput_RejectsUnexpectedForeignScriptContamination(t *testing.T) {
	meta := itemMeta{
		sourceRaw: "No. I am telling the whole truth.",
		profile:   textProfile{Kind: textKindDialogue},
	}
	err := validateRestoredOutput(meta, "아니. 나는 전부 الحقيقة대로 말하고 있다.")
	if err == nil {
		t.Fatal("expected contamination error")
	}
	if err.Error() != "unexpected foreign-script contamination" {
		t.Fatalf("unexpected error: %v", err)
	}
}
