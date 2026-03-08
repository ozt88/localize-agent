package translation

import "testing"

func TestMaskRestoreTags_RichTextAndPlaceholders(t *testing.T) {
	src := `Do <i>you</i> like {food} and $NAME?`
	masked, maps := maskTags(src)
	if masked != `Do [T0]you[T1] like [T2] and [T3]?` {
		t.Fatalf("masked=%q", masked)
	}
	got, err := restoreTags(`너는 [T0]정말[T1] [T2]와 [T3]를 좋아해?`, maps)
	if err != nil {
		t.Fatalf("restoreTags error: %v", err)
	}
	want := `너는 <i>정말</i> {food}와 $NAME를 좋아해?`
	if got != want {
		t.Fatalf("got=%q want=%q", got, want)
	}
}

func TestRestoreTags_RejectsMissingPlaceholder(t *testing.T) {
	_, maps := maskTags(`<i>Hi</i>`)
	if _, err := restoreTags(`안녕`, maps); err == nil {
		t.Fatal("expected placeholder mismatch error")
	}
}
