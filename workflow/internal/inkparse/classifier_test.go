package inkparse

import "testing"

func TestClassifyDialogueByTSPrefix(t *testing.T) {
	block := &DialogueBlock{SourceFile: "TS_Snell_Meeting", Speaker: "snell"}
	got := Classify(block)
	if got != ContentDialogue {
		t.Errorf("TS_ prefix: want %q, got %q", ContentDialogue, got)
	}
}

func TestClassifyDialogueByARPrefix(t *testing.T) {
	block := &DialogueBlock{SourceFile: "AR_CoastMap"}
	got := Classify(block)
	if got != ContentDialogue {
		t.Errorf("AR_ prefix: want %q, got %q", ContentDialogue, got)
	}
}

func TestClassifyDialogueBySpeaker(t *testing.T) {
	block := &DialogueBlock{SourceFile: "UnknownFile", Speaker: "npc_name"}
	got := Classify(block)
	if got != ContentDialogue {
		t.Errorf("has speaker: want %q, got %q", ContentDialogue, got)
	}
}

func TestClassifySpellByGTPrefix(t *testing.T) {
	// GT_ = GlossaryTerms-related files (spell/ability descriptions)
	block := &DialogueBlock{SourceFile: "GT_Archives", Tags: []string{"spell"}}
	got := Classify(block)
	if got != ContentSpell {
		t.Errorf("GT_ with spell tag: want %q, got %q", ContentSpell, got)
	}
}

func TestClassifySpellBySpellTag(t *testing.T) {
	block := &DialogueBlock{SourceFile: "SomeFile", Tags: []string{"spell", "level3"}}
	got := Classify(block)
	if got != ContentSpell {
		t.Errorf("spell tag: want %q, got %q", ContentSpell, got)
	}
}

func TestClassifyUIByShortTextNoSpeaker(t *testing.T) {
	block := &DialogueBlock{SourceFile: "UnknownFile", Text: "Accept", Speaker: ""}
	got := Classify(block)
	if got != ContentUI {
		t.Errorf("short no speaker: want %q, got %q", ContentUI, got)
	}
}

func TestClassifyUINotTriggeredWithSpeaker(t *testing.T) {
	// Short text WITH speaker should be dialogue, not UI
	block := &DialogueBlock{SourceFile: "UnknownFile", Text: "Accept", Speaker: "npc"}
	got := Classify(block)
	if got != ContentDialogue {
		t.Errorf("short with speaker: want %q (not ui), got %q", ContentDialogue, got)
	}
}

func TestClassifyItemByEncPrefix(t *testing.T) {
	// Enc_ = encounter/item files
	block := &DialogueBlock{SourceFile: "Enc_Below", Tags: []string{"OBJ"}}
	got := Classify(block)
	if got != ContentItem {
		t.Errorf("Enc_ with OBJ tag: want %q, got %q", ContentItem, got)
	}
}

func TestClassifySystemByTUPrefix(t *testing.T) {
	// TU_ = Tutorial files
	block := &DialogueBlock{SourceFile: "TU_Combat"}
	got := Classify(block)
	if got != ContentSystem {
		t.Errorf("TU_ prefix: want %q, got %q", ContentSystem, got)
	}
}

func TestClassifyDefaultsToDialogue(t *testing.T) {
	block := &DialogueBlock{
		SourceFile: "CB_Apartment",
		Text:       "A longer text that would be typical dialogue in the game world.",
		Speaker:    "",
	}
	got := Classify(block)
	if got != ContentDialogue {
		t.Errorf("default: want %q, got %q", ContentDialogue, got)
	}
}

func TestClassifyAllContentTypes(t *testing.T) {
	// Verify all 5 content types are reachable
	types := map[string]bool{
		ContentDialogue: false,
		ContentSpell:    false,
		ContentUI:       false,
		ContentItem:     false,
		ContentSystem:   false,
	}

	cases := []struct {
		block *DialogueBlock
		want  string
	}{
		{&DialogueBlock{SourceFile: "TS_Test", Speaker: "npc"}, ContentDialogue},
		{&DialogueBlock{SourceFile: "X", Tags: []string{"spell"}}, ContentSpell},
		{&DialogueBlock{SourceFile: "X", Text: "OK"}, ContentUI},
		{&DialogueBlock{SourceFile: "Enc_X", Tags: []string{"OBJ"}}, ContentItem},
		{&DialogueBlock{SourceFile: "TU_X"}, ContentSystem},
	}

	for _, tc := range cases {
		got := Classify(tc.block)
		if got != tc.want {
			t.Errorf("block %+v: want %q, got %q", tc.block, tc.want, got)
		}
		types[got] = true
	}

	for ct, reached := range types {
		if !reached {
			t.Errorf("content type %q was never produced", ct)
		}
	}
}
