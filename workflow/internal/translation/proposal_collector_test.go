package translation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"localize-agent/workflow/internal/platform"
	"localize-agent/workflow/internal/shared"
)

func TestGroupRunItemsByKind_PreservesLaneAndGroupBoundaries(t *testing.T) {
	items := []translationTask{
		{ID: "a", GroupKey: textKindDialogue, Lane: laneDefault},
		{ID: "b", GroupKey: textKindDialogue, Lane: laneHigh},
		{ID: "c", GroupKey: textKindDialogue, Lane: laneDefault},
	}

	grouped := groupRunItemsByKind(items)
	if len(grouped) != 2 {
		t.Fatalf("len=%d, want 2", len(grouped))
	}
	if grouped[0][0].ID != "a" || grouped[0][1].ID != "c" || grouped[1][0].ID != "b" {
		t.Fatalf("grouped=%v", grouped)
	}
}

func newPromptOnlyClient(t *testing.T, provider string, responseMode string, responder func(prompt string) (int, string)) *serverClient {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(body.Messages) == 0 {
			http.Error(w, "missing messages", http.StatusBadRequest)
			return
		}
		status, payload := responder(body.Messages[len(body.Messages)-1].Content)
		if status != http.StatusOK {
			http.Error(w, payload, status)
			return
		}
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]any{"role": "assistant", "content": payload},
		})
	}))
	t.Cleanup(ts.Close)

	return &serverClient{
		llm: platform.NewOllamaLLMClient(ts.URL, 2, &shared.MetricCollector{}, nil),
		profile: platform.LLMProfile{
			ProviderID: provider,
			ModelID:    "mock",
		},
		responseMode:  responseMode,
		sessionPrefix: ts.URL,
	}
}

func TestCollectProposals_OllamaSinglePlainOutput(t *testing.T) {
	client := newPromptOnlyClient(t, platform.LLMBackendOllama, responseModePlain, func(prompt string) (int, string) {
		if !strings.Contains(prompt, "Return only the Korean translation text.") {
			t.Fatalf("missing plain output guidance:\n%s", prompt)
		}
		return http.StatusOK, "이제 나를 보내 줘!"
	})

	rt := translationRuntime{cfg: Config{MaxAttempts: 1, BackoffSec: 0}, client: client, skill: newTranslateSkill("", "")}
	items := []translationTask{{ID: "id-1", BodyEN: "Now let me go!", GroupKey: textKindDialogue, Lane: laneDefault}}

	props, skippedInvalid, skippedErr := collectProposals(rt, "slot-1", items)
	if skippedInvalid != 0 || skippedErr != 0 {
		t.Fatalf("skippedInvalid=%d skippedErr=%d", skippedInvalid, skippedErr)
	}
	if props["id-1"].ProposedKO != "이제 나를 보내 줘!" {
		t.Fatalf("proposal=%+v", props["id-1"])
	}
}

func TestCollectProposals_OllamaBatchIndexedOutput(t *testing.T) {
	client := newPromptOnlyClient(t, platform.LLMBackendOllama, responseModePlain, func(prompt string) (int, string) {
		if !strings.Contains(prompt, "Each line must use the format <index>\\t<korean translation>.") {
			t.Fatalf("missing indexed output guidance:\n%s", prompt)
		}
		return http.StatusOK, "0\t첫째 번역\n1\t둘째 번역"
	})

	rt := translationRuntime{cfg: Config{MaxAttempts: 1, BackoffSec: 0}, client: client, skill: newTranslateSkill("", "")}
	items := []translationTask{
		{ID: "id-1", BodyEN: "One.", GroupKey: textKindDialogue, Lane: laneDefault},
		{ID: "id-2", BodyEN: "Two.", GroupKey: textKindDialogue, Lane: laneDefault},
	}

	props, skippedInvalid, skippedErr := collectProposals(rt, "slot-1", items)
	if skippedInvalid != 0 || skippedErr != 0 {
		t.Fatalf("skippedInvalid=%d skippedErr=%d", skippedInvalid, skippedErr)
	}
	if props["id-1"].ProposedKO != "첫째 번역" || props["id-2"].ProposedKO != "둘째 번역" {
		t.Fatalf("props=%v", props)
	}
}

func TestCollectProposals_OpenCodeSinglePlainOutput(t *testing.T) {
	client := newPromptOnlyClient(t, platform.LLMBackendOpencode, responseModePlain, func(prompt string) (int, string) {
		if !strings.Contains(prompt, "Return only the Korean translation text.") {
			t.Fatalf("missing plain output guidance:\n%s", prompt)
		}
		return http.StatusOK, "지금 보내줘!"
	})

	rt := translationRuntime{cfg: Config{MaxAttempts: 1, BackoffSec: 0}, client: client, skill: newTranslateSkill("", "")}
	items := []translationTask{{ID: "id-1", BodyEN: "Send me now!", GroupKey: textKindDialogue, Lane: laneDefault}}

	props, skippedInvalid, skippedErr := collectProposals(rt, "slot-1", items)
	if skippedInvalid != 0 || skippedErr != 0 {
		t.Fatalf("skippedInvalid=%d skippedErr=%d", skippedInvalid, skippedErr)
	}
	if props["id-1"].ProposedKO != "지금 보내줘!" {
		t.Fatalf("proposal=%+v", props["id-1"])
	}
}
