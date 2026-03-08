package translation

import (
	"strings"
	"testing"
)

func TestNewTranslateSkill_MergesDefaultAndProjectRules(t *testing.T) {
	skill := newTranslateSkill("ctx", "PROJECT_RULE")
	warmup := skill.warmup()
	if !containsAll(warmup,
		"Reply to this warmup with exactly: OK",
		"Return only the contract defined by the project-local translator system prompt.",
		"PROJECT_RULE",
	) {
		t.Fatalf("warmup did not include merged rules:\n%s", warmup)
	}
}

func TestExtractObjects_ArrayPayload(t *testing.T) {
	raw := `[{"id":"a","proposed_ko":"alpha"},{"id":"b","proposed_ko":"beta"}]`
	got := extractObjects(raw)
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("ids=%q,%q", got[0].ID, got[1].ID)
	}
}

func TestBuildSinglePrompt_MinimalPayloadOnly(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:          "id-choice",
		BodyEN:      "Tell [[E0]]him[[/E0]] to leave.",
		ContextEN:   "Give back the papers.\nHis flesh turns to [[E1]]fine dust[[/E1]].",
		TextRole:    "dialogue",
		SpeakerHint: "Snell",
	}, `{"type":"object"}`, false)

	if !containsAll(prompt,
		`"id":"id-choice"`,
		`"body_en":"Tell [[E0]]him[[/E0]] to leave."`,
		`"context_en":"Give back the papers.\nHis flesh turns to [[E1]]fine dust[[/E1]]."`,
		`"text_role":"dialogue"`,
		`"speaker_hint":"Snell"`,
	) {
		t.Fatalf("prompt missing minimal fields:\n%s", prompt)
	}
	for _, forbidden := range []string{
		`"current_ko"`,
		`"prev_ko"`,
		`"next_ko"`,
		`"choice_prefix"`,
		`"chunk_id"`,
		`"parent_segment_id"`,
		`"line_is_imperative"`,
		`"line_is_short_context_dependent"`,
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt still contains %s:\n%s", forbidden, prompt)
		}
	}
}

func TestBuildSinglePrompt_OllamaPlainOutput(t *testing.T) {
	prompt := buildSinglePrompt(translationTask{
		ID:          "id-1",
		BodyEN:      "It is piss pot. Enjoy.",
		ContextEN:   "But can know for sure? No.\nIt is piss pot. Enjoy.",
		TextRole:    "dialogue",
		SpeakerHint: "Ost",
	}, `{"type":"object"}`, true)

	if !containsAll(prompt,
		"Return only the Korean translation text.",
		`"body_en":"It is piss pot. Enjoy."`,
		`"context_en":"But can know for sure? No.\nIt is piss pot. Enjoy."`,
	) {
		t.Fatalf("prompt missing plain-output guidance:\n%s", prompt)
	}
	if strings.Contains(prompt, "JSON") && !strings.Contains(prompt, "No JSON") {
		t.Fatalf("prompt should not require JSON:\n%s", prompt)
	}
}

func TestExtractPlainTranslation_StripsQuotedLine(t *testing.T) {
	got := extractPlainTranslation("\"이건 그냥 쓰레기통이에요. 즐기세요.\"")
	if got != "이건 그냥 쓰레기통이에요. 즐기세요." {
		t.Fatalf("got=%q", got)
	}
}

func TestExtractIndexedTranslations(t *testing.T) {
	raw := "0\t첫째 번역\n1\t둘째 번역"
	got := extractIndexedTranslations(raw)
	if len(got) != 2 || got[0] != "첫째 번역" || got[1] != "둘째 번역" {
		t.Fatalf("got=%v", got)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
