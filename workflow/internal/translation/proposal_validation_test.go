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
