package translation

import "testing"

func TestIsDegenerateProposal_RejectsEnglishCopy(t *testing.T) {
	if !isDegenerateProposal("It won't budge without the hole being filled.", "It won't budge without the hole being filled.") {
		t.Fatal("expected english copy to be rejected")
	}
}

func TestIsDegenerateProposal_RejectsEnglishHeavyOutput(t *testing.T) {
	if !isDegenerateProposal("It won't budge without the hole being filled.", "Oh, you know it baby.") {
		t.Fatal("expected english-heavy output to be rejected")
	}
}

func TestIsDegenerateProposal_AllowsKoreanOutput(t *testing.T) {
	if isDegenerateProposal("It won't budge without the hole being filled.", "\uad6c\uba4d\uc774 \uba54\uc6cc\uc9c0\uc9c0 \uc54a\uc73c\uba74 \uafc8\uc801\ub3c4 \ud558\uc9c0 \uc54a\ub294\ub2e4.") {
		t.Fatal("expected Korean output to pass")
	}
}

func TestDegenerateProposalReason_ClassifiesCases(t *testing.T) {
	cases := []struct {
		name string
		en   string
		ko   string
		want string
	}{
		{name: "empty", en: "Hello.", ko: "", want: "empty"},
		{name: "punctuation_only", en: "Hello.", ko: "...", want: "punctuation_only"},
		{name: "exact_source_copy", en: "Hello.", ko: "Hello.", want: "exact_source_copy"},
		{name: "ascii_heavy", en: "Hello.", ko: "Oh, you know it baby.", want: "ascii_heavy"},
		{name: "ok", en: "Hello.", ko: "안녕.", want: ""},
	}
	for _, tc := range cases {
		if got := degenerateProposalReason(tc.en, tc.ko); got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestDegenerateProposalReason_AllowsOverlayPassthrough(t *testing.T) {
	cases := []struct {
		name string
		en   string
		ko   string
	}{
		{name: "ui_label_single", en: "Resistance", ko: "Resistance"},
		{name: "ui_label_two_words", en: "Burn Incense", ko: "Burn Incense"},
		{name: "proper_noun", en: "Commune", ko: "Commune"},
		{name: "credit_line", en: "Sound Design by Isac Johnsson", ko: "Sound Design by Isac Johnsson"},
		{name: "latin_italic", en: "<i>Ignem accende.</i>", ko: "<i>Ignem accende.</i>"},
		{name: "nordic_text", en: "Hôr, mina vânner", ko: "Hôr, mina vânner"},
		{name: "numeric_stat", en: "+1 X\n-1 Y", ko: "+1 X\n-1 Y"},
		{name: "lorem", en: "Lorem ipsum dolor sit amet", ko: "Lorem ipsum dolor sit amet"},
		{name: "finnish_italic", en: "<i>minun puuni</i>", ko: "<i>minun puuni</i>"},
	}
	for _, tc := range cases {
		if got := degenerateProposalReason(tc.en, tc.ko); got != "" {
			t.Fatalf("%s: got %q want empty (passthrough)", tc.name, got)
		}
	}
}

func TestDegenerateProposalReason_StillRejectsTranslatableText(t *testing.T) {
	cases := []struct {
		name string
		en   string
		ko   string
		want string
	}{
		{name: "normal_sentence", en: "The door is locked.", ko: "The door is locked.", want: "exact_source_copy"},
		{name: "long_sentence", en: "Please don't put it on.", ko: "Please don't put it on.", want: "exact_source_copy"},
		{name: "ascii_heavy_normal", en: "What happened to you?", ko: "Oh nothing much really.", want: "ascii_heavy"},
	}
	for _, tc := range cases {
		if got := degenerateProposalReason(tc.en, tc.ko); got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestDegenerateProposalReason_AllowsPlaceholderWrappedPassthrough(t *testing.T) {
	cases := []struct {
		name string
		en   string
		ko   string
	}{
		{name: "placeholder_wrapped_latin", en: "[[E0]]Ignem accende.[[/E0]]", ko: "[[E0]]Ignem accende.[[/E0]]"},
		{name: "placeholder_wrapped_georgian", en: "[[E0]]brdzola sheni[[/E0]]", ko: "[[E0]]brdzola sheni[[/E0]]"},
		{name: "multiline_credits", en: "Anders Bach\nBrian Batz\nKristian Paulsen", ko: "Anders Bach\nBrian Batz\nKristian Paulsen"},
		{name: "repeating_placeholder", en: "XX | XX | XX | XX | XX | XX", ko: "XX | XX | XX | XX | XX | XX"},
	}
	for _, tc := range cases {
		if got := degenerateProposalReason(tc.en, tc.ko); got != "" {
			t.Fatalf("%s: got %q want empty (passthrough)", tc.name, got)
		}
	}
}

func TestDegenerateProposalReason_AllowsPassthroughControlTokens(t *testing.T) {
	cases := []struct {
		name string
		en   string
		ko   string
	}{
		{name: "dot control", en: ".ITEMLOOTED_CB_RunTorch==1-", ko: ".ITEMLOOTED_CB_RunTorch==1-"},
		{name: "upper token", en: "<i>HWBT</i>.", ko: "<i>HWBT</i>."},
		{name: "wiggle punctuation", en: "<wiggle>...</wiggle>", ko: "<wiggle>...</wiggle>"},
	}
	for _, tc := range cases {
		if got := degenerateProposalReason(tc.en, tc.ko); got != "" {
			t.Fatalf("%s: got %q want empty", tc.name, got)
		}
	}
}
