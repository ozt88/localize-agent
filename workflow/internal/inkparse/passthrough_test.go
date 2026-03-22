package inkparse

import "testing"

func TestPassthroughControlPattern(t *testing.T) {
	// v1 pattern: ".SomeVariable>=10-"
	if !IsPassthrough(".SomeVariable>=10-") {
		t.Error("control pattern should be passthrough")
	}
}

func TestPassthroughSpellFormula(t *testing.T) {
	if !IsPassthrough("SPELL FireBolt-") {
		t.Error("SPELL formula should be passthrough")
	}
}

func TestPassthroughVariableRef(t *testing.T) {
	if !IsPassthrough("$var_reference") {
		t.Error("$variable should be passthrough")
	}
}

func TestPassthroughTemplateString(t *testing.T) {
	if !IsPassthrough("{some_template}") {
		t.Error("template string should be passthrough")
	}
}

func TestPassthroughInkControlEnd(t *testing.T) {
	if !IsPassthrough("end") {
		t.Error("ink control 'end' should be passthrough")
	}
}

func TestPassthroughDone(t *testing.T) {
	if !IsPassthrough("DONE") {
		t.Error("ink control 'DONE' should be passthrough")
	}
}

func TestPassthroughPunctuationOnly(t *testing.T) {
	if !IsPassthrough("...") {
		t.Error("punctuation-only should be passthrough")
	}
}

func TestPassthroughWhitespaceOnly(t *testing.T) {
	if !IsPassthrough("   ") {
		t.Error("whitespace-only should be passthrough")
	}
}

func TestPassthroughEmpty(t *testing.T) {
	if !IsPassthrough("") {
		t.Error("empty string should be passthrough")
	}
}

func TestNotPassthroughDialogue(t *testing.T) {
	if IsPassthrough("Hello, adventurer.") {
		t.Error("dialogue text should NOT be passthrough")
	}
}

func TestNotPassthroughTaggedText(t *testing.T) {
	// Has translatable text inside tags
	if IsPassthrough("<b>COLLECTION</b>") {
		t.Error("tagged text with real words should NOT be passthrough")
	}
}

func TestNotPassthroughChoiceExit(t *testing.T) {
	// E- prefix = choice exit text, translatable
	if IsPassthrough("E-That's all for now.") {
		t.Error("E- choice text should NOT be passthrough")
	}
}

func TestNotPassthroughLongText(t *testing.T) {
	if IsPassthrough("The ancient library stretches before you, filled with towering shelves of dusty tomes.") {
		t.Error("long narrative text should NOT be passthrough")
	}
}
