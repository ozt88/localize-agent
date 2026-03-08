package evaluation

import (
	"reflect"
	"strings"
	"testing"
)

func TestNewEvaluateSkill_MergesDefaultAndProjectRules(t *testing.T) {
	skill := newEvaluateSkill("ctx", "PROJECT_RULE")
	warmup := skill.warmup()
	for _, sub := range []string{
		"You are a strict quality evaluator, NOT a translator.",
		"PROJECT_RULE",
	} {
		if !strings.Contains(warmup, sub) {
			t.Fatalf("warmup missing %q:\n%s", sub, warmup)
		}
	}
}

func TestParseCSV(t *testing.T) {
	tests := []struct {
		name string
		give string
		want []string
	}{
		{name: "empty", give: "", want: nil},
		{name: "whitespace", give: "   \n\t  ", want: nil},
		{name: "normal", give: "pass, revise,reject", want: []string{"pass", "revise", "reject"}},
		{name: "drops-empty", give: "pass, , revise,,", want: []string{"pass", "revise"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCSV(tt.give)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseCSV(%q) = %v, want %v", tt.give, got, tt.want)
			}
		})
	}
}

func TestSelectRevised(t *testing.T) {
	items := []map[string]any{
		{"id": "a", "revised": true},
		{"id": "b", "revised": false},
		{"id": "c", "revised": "true"},
		{"id": "d"},
		{"id": "e", "revised": true},
	}

	got := selectRevised(items)
	if len(got) != 2 {
		t.Fatalf("len(selectRevised)=%d, want 2", len(got))
	}
	if got[0]["id"] != "a" || got[1]["id"] != "e" {
		t.Fatalf("selected ids=%v,%v, want a,e", got[0]["id"], got[1]["id"])
	}
}
