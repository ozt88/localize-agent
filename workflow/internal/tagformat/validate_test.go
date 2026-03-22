package tagformat

import (
	"strings"
	"testing"
)

func TestValidateTagMatch_Pass(t *testing.T) {
	en := "<b>Watch</b> your <i>step</i>"
	ko := "<i>발</i> 조심 <b>해</b>"
	if err := ValidateTagMatch(en, ko); err != nil {
		t.Errorf("expected pass, got error: %v", err)
	}
}

func TestValidateTagMatch_CountMismatch(t *testing.T) {
	en := "<b>Watch</b> your <i>step</i>" // 4 tags
	ko := "<b>조심해</b>"                    // 2 tags
	err := ValidateTagMatch(en, ko)
	if err == nil {
		t.Fatal("expected error for count mismatch")
	}
	if !strings.Contains(err.Error(), "count") {
		t.Errorf("error should mention count: %v", err)
	}
}

func TestValidateTagMatch_Reordered(t *testing.T) {
	// Same tags, different order (Korean word order) -- should pass per D-07
	en := "<i>critters</i> are <b>dangerous</b>"
	ko := "<b>위험한</b> <i>놈들</i>"
	if err := ValidateTagMatch(en, ko); err != nil {
		t.Errorf("reordered tags should pass per D-07, got error: %v", err)
	}
}

func TestValidateTagMatch_MissingTag(t *testing.T) {
	en := "<b>Watch</b> your <i>step</i>"
	ko := "<b>조심해</b> 발걸음"
	err := ValidateTagMatch(en, ko)
	if err == nil {
		t.Fatal("expected error for missing tag")
	}
	if !strings.Contains(err.Error(), "<i>") || !strings.Contains(err.Error(), "</i>") {
		t.Errorf("error should mention missing <i> and </i>: %v", err)
	}
}

func TestValidateTagMatch_AttributeMismatch(t *testing.T) {
	en := "<size=50>big</size> text"
	ko := "<size=60>큰</size> 텍스트"
	err := ValidateTagMatch(en, ko)
	if err == nil {
		t.Fatal("expected error for attribute mismatch")
	}
	if !strings.Contains(err.Error(), "<size=50>") {
		t.Errorf("error should mention missing <size=50>: %v", err)
	}
}

func TestValidateTagMatch_NoTags(t *testing.T) {
	en := "plain text"
	ko := "일반 텍스트"
	if err := ValidateTagMatch(en, ko); err != nil {
		t.Errorf("no tags should pass: %v", err)
	}
}
