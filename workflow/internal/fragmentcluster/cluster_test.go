package fragmentcluster

import (
	"strings"
	"testing"
)

func TestBuildPromptIncludesLineAlignedRules(t *testing.T) {
	prompt := BuildPrompt(PromptInput{
		ClusterID:       "c1",
		ContextBeforeEN: "Your legs... no!",
		ContextAfterEN:  "You find yourself moments later...",
		ClusterJoinHint: "single_action_chain",
		Lines: []Line{
			{ID: "a", EN: "They.", CurrentKO: "다리가.", TextRole: "fragment"},
			{ID: "b", EN: "Give.", CurrentKO: "힘이.", TextRole: "fragment"},
			{ID: "c", EN: "Out.", CurrentKO: "끝.", TextRole: "dialogue"},
		},
	})
	for _, want := range []string{
		"Return one Korean line per input line.",
		"Keep the same number of output lines as input lines.",
		"Do not merge lines.",
		"Do not add honorifics or extra politeness",
		"preserve that emphasis",
		"Return only one JSON array of Korean strings.",
		`"cluster_join_hint":"single_action_chain"`,
		`"id":"a"`,
		`"en":"They."`,
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestParseOutputValidatesLength(t *testing.T) {
	got, err := ParseOutput(`["다리가","풀려","버린다."]`, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 || got[1] != "풀려" {
		t.Fatalf("unexpected output: %#v", got)
	}
	if _, err := ParseOutput(`["하나","둘"]`, 3); err == nil {
		t.Fatal("expected length validation error")
	}
}

func TestNormalizeOutputLinesRestoresSimpleTagPlaceholders(t *testing.T) {
	got, err := NormalizeOutputLines(
		[]string{`...[T0]가위[T1]!`},
		[]Line{{ID: "x", EN: `...[T0]SCISSORS[T1]!`, CurrentKO: `...<shake>가위</shake>!`}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != `...<shake>가위</shake>!` {
		t.Fatalf("unexpected normalized output: %#v", got)
	}
}

func TestNormalizeOutputLinesRestoresSimpleEmphasisMarkers(t *testing.T) {
	got, err := NormalizeOutputLines(
		[]string{`[[E0]]셋[[/E0]].`},
		[]Line{{ID: "x", EN: `[[E0]]Three[[/E0]].`, CurrentKO: `<i>셋</i>.`}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != `<i>셋</i>.` {
		t.Fatalf("unexpected normalized output: %#v", got)
	}
}

func TestNormalizeOutputLinesCarriesForwardSingleEmphasisWhenOutputDropsIt(t *testing.T) {
	got, err := NormalizeOutputLines(
		[]string{`쳇.`},
		[]Line{{ID: "x", EN: `[[E0]]Tsk[[/E0]].`, CurrentKO: `<i>쳇</i>.`}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != `<i>쳇</i>.` {
		t.Fatalf("unexpected normalized output: %#v", got)
	}
}

func TestNormalizeOutputLinesCarriesForwardSourceEmphasisWhenCurrentIsEmpty(t *testing.T) {
	got, err := NormalizeOutputLines(
		[]string{`인간.`},
		[]Line{{ID: "x", EN: `[[E0]]Human[[/E0]].`, CurrentKO: ``}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != `<i>인간</i>.` {
		t.Fatalf("unexpected normalized output: %#v", got)
	}
}

func TestNormalizeOutputLinesCarriesForwardLiteralSourceEmphasisWhenCurrentIsEmpty(t *testing.T) {
	got, err := NormalizeOutputLines(
		[]string{`인간.`},
		[]Line{{ID: "x", EN: `<i>Human</i>.`, CurrentKO: ``}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != `<i>인간</i>.` {
		t.Fatalf("unexpected normalized output: %#v", got)
	}
}
