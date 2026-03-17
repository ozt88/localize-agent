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
		{name: "fc choice", text: "FC8 int-<i>Ragn?</i>", want: textProfile{Kind: textKindChoice, HasRichText: true}},
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

func TestProfileGroupKey_MergesDialogueAndNarration(t *testing.T) {
	tests := []struct {
		name    string
		profile textProfile
		want    string
	}{
		{name: "dialogue", profile: textProfile{Kind: textKindDialogue}, want: textKindDialogue},
		{name: "narration", profile: textProfile{Kind: textKindNarration}, want: textKindDialogue},
		{name: "dialogue rich", profile: textProfile{Kind: textKindDialogue, HasRichText: true}, want: textKindDialogue + "+rich"},
		{name: "narration rich", profile: textProfile{Kind: textKindNarration, HasRichText: true}, want: textKindDialogue + "+rich"},
		{name: "choice", profile: textProfile{Kind: textKindChoice}, want: textKindChoice},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := profileGroupKey(tt.profile); got != tt.want {
				t.Fatalf("profileGroupKey(%+v)=%q want %q", tt.profile, got, tt.want)
			}
		})
	}
}

func TestIsUIRole(t *testing.T) {
	tests := map[string]bool{
		"ui_label":       true,
		"ui_description": true,
		"tooltip":        true,
		"button":         true,
		"dialogue":       false,
		"choice":         false,
	}
	for input, want := range tests {
		if got := isUIRole(input); got != want {
			t.Fatalf("isUIRole(%q)=%v want %v", input, got, want)
		}
	}
}

func TestShouldPreserveInternalUILabel(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		textRole    string
		retryReason string
		sourceFile  string
		want        bool
	}{
		{name: "prefab underscore", source: "Aventail_Horns", textRole: "ui_label", retryReason: "prefab_static_missing_from_canonical_source", want: true},
		{name: "prefab tech token", source: "ArmRig", textRole: "ui_label", retryReason: "prefab_static_missing_from_canonical_source", want: true},
		{name: "prefab source file", source: "CompanionTarget", textRole: "ui_label", sourceFile: "You.prefab", want: true},
		{name: "normal ui label stays translatable", source: "All Items", textRole: "ui_label", retryReason: "scene_runtime_static_missing_from_canonical_source", want: false},
		{name: "spell label stays translatable", source: "Cure Wounds", textRole: "ui_label", retryReason: "prefab_static_missing_from_canonical_source", want: false},
		{name: "tooltip ignored", source: "ArmRig", textRole: "tooltip", retryReason: "prefab_static_missing_from_canonical_source", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldPreserveInternalUILabel(tt.source, tt.textRole, tt.retryReason, tt.sourceFile); got != tt.want {
				t.Fatalf("shouldPreserveInternalUILabel(%q,%q,%q,%q)=%v want %v", tt.source, tt.textRole, tt.retryReason, tt.sourceFile, got, tt.want)
			}
		})
	}
}
