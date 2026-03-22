package tagformat

import (
	"testing"
)

func TestExtractTags_Basic(t *testing.T) {
	tags := ExtractTags("<b>Watch</b> your <i>step</i>")
	want := []string{"<b>", "</b>", "<i>", "</i>"}
	assertStringSlice(t, want, tags)
}

func TestExtractTags_SizeParam(t *testing.T) {
	tags := ExtractTags("<size=50>big</size>")
	want := []string{"<size=50>", "</size>"}
	assertStringSlice(t, want, tags)
}

func TestExtractTags_Nested(t *testing.T) {
	tags := ExtractTags("<b><i>text</i></b>")
	want := []string{"<b>", "<i>", "</i>", "</b>"}
	assertStringSlice(t, want, tags)
}

func TestExtractTags_AllTagTypes(t *testing.T) {
	input := "<i>a</i><b>b</b><shake>c</shake><wiggle>d</wiggle><u>e</u><size=25>f</size><s>g</s>"
	tags := ExtractTags(input)
	if len(tags) != 14 {
		t.Errorf("expected 14 tags, got %d: %v", len(tags), tags)
	}
}

func TestHasRichTags_True(t *testing.T) {
	if !HasRichTags("<b>text</b>") {
		t.Error("expected HasRichTags to return true")
	}
}

func TestHasRichTags_False(t *testing.T) {
	if HasRichTags("plain text") {
		t.Error("expected HasRichTags to return false")
	}
}

func TestStripTags(t *testing.T) {
	got := StripTags("<b>bold</b> text")
	want := "bold text"
	if got != want {
		t.Errorf("StripTags: got %q, want %q", got, want)
	}
}

func TestCountTags(t *testing.T) {
	got := CountTags("<b>bold</b> and <i>italic</i>")
	if got != 4 {
		t.Errorf("CountTags: got %d, want 4", got)
	}
}

func assertStringSlice(t *testing.T, want, got []string) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("length mismatch: want %d, got %d\n  want: %v\n  got:  %v", len(want), len(got), want, got)
	}
	for i := range want {
		if want[i] != got[i] {
			t.Errorf("index %d: want %q, got %q", i, want[i], got[i])
		}
	}
}
