package translation

import "testing"

func TestClassifyTextProfile(t *testing.T) {
	tests := []struct {
		name string
		text string
		want textProfile
	}{
		{name: "choice", text: "ROLL14 str-Tell him to leave.", want: textProfile{Kind: textKindChoice, HasRichText: false}},
		{name: "choice spaced prefix", text: "ROLL15 str Tell him to leave.", want: textProfile{Kind: textKindChoice, HasRichText: false}},
		{name: "choice rich", text: "ROLL14 str-Tell <i>him</i> to leave.", want: textProfile{Kind: textKindChoice, HasRichText: true}},
		{name: "narration rich", text: "He sees <i>you</i> in the mirror.", want: textProfile{Kind: textKindNarration, HasRichText: true}},
		{name: "dialogue", text: "What do you want?", want: textProfile{Kind: textKindDialogue, HasRichText: false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyTextProfile(tt.text); got != tt.want {
				t.Fatalf("classifyTextProfile(%q)=%+v want %+v", tt.text, got, tt.want)
			}
		})
	}
}
