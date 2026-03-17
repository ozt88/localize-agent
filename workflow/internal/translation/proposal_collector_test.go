package translation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"localize-agent/workflow/pkg/platform"
	"localize-agent/workflow/pkg/shared"
)

func TestGroupRunItemsByKind_IgnoresLaneButPreservesGroupBoundaries(t *testing.T) {
	items := []translationTask{
		{ID: "a", GroupKey: textKindDialogue, Lane: laneDefault},
		{ID: "b", GroupKey: textKindDialogue, Lane: laneHigh},
		{ID: "c", GroupKey: textKindDialogue, Lane: laneDefault},
	}

	grouped := groupRunItemsByKind(items)
	if len(grouped) != 1 {
		t.Fatalf("len=%d, want 1", len(grouped))
	}
	if grouped[0][0].ID != "a" || grouped[0][1].ID != "b" || grouped[0][2].ID != "c" {
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
		if !strings.Contains(prompt, "Return one valid JSON array with exactly 1 Korean string and nothing else.") {
			t.Fatalf("missing plain output guidance:\n%s", prompt)
		}
		if !strings.Contains(prompt, "without repairing or completing broken source fragments") {
			t.Fatalf("missing plain output guidance:\n%s", prompt)
		}
		return http.StatusOK, "[\"\uc774\uc81c \ub0a0 \ub193\uc544\uc918!\"]"
	})

	rt := translationRuntime{cfg: Config{MaxAttempts: 1, BackoffSec: 0}, client: client, skill: newTranslateSkill("", "")}
	items := []translationTask{{ID: "id-1", BodyEN: "Now let me go!", GroupKey: textKindDialogue, Lane: laneDefault}}

	props, skippedInvalid, skippedErr := collectProposals(rt, "slot-1", items)
	if skippedInvalid != 0 || skippedErr != 0 {
		t.Fatalf("skippedInvalid=%d skippedErr=%d", skippedInvalid, skippedErr)
	}
	if props["id-1"].ProposedKO != "\uc774\uc81c \ub0a0 \ub193\uc544\uc918!" {
		t.Fatalf("proposal=%+v", props["id-1"])
	}
}

func TestCollectProposals_OllamaBatchArrayOutput(t *testing.T) {
	client := newPromptOnlyClient(t, platform.LLMBackendOllama, responseModePlain, func(prompt string) (int, string) {
		if !strings.Contains(prompt, "Return ONLY one JSON array of exactly the requested number of Korean strings.") {
			t.Fatalf("missing array output guidance:\n%s", prompt)
		}
		if !strings.Contains(prompt, "without repairing or completing broken source fragments") {
			t.Fatalf("missing array output guidance:\n%s", prompt)
		}
		return http.StatusOK, "[\"\uccab \ubc88\uc9f8 \ubc88\uc5ed\",\"\ub450 \ubc88\uc9f8 \ubc88\uc5ed\"]"
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
	if props["id-1"].ProposedKO != "\uccab \ubc88\uc9f8 \ubc88\uc5ed" || props["id-2"].ProposedKO != "\ub450 \ubc88\uc9f8 \ubc88\uc5ed" {
		t.Fatalf("props=%v", props)
	}
}

func TestCollectProposals_OpenCodeSinglePlainOutput(t *testing.T) {
	client := newPromptOnlyClient(t, platform.LLMBackendOpencode, responseModePlain, func(prompt string) (int, string) {
		if !strings.Contains(prompt, "Return one valid JSON array with exactly 1 Korean string and nothing else.") {
			t.Fatalf("missing plain output guidance:\n%s", prompt)
		}
		if !strings.Contains(prompt, "without repairing or completing broken source fragments") {
			t.Fatalf("missing plain output guidance:\n%s", prompt)
		}
		return http.StatusOK, "[\"\uc9c0\uae08 \ubcf4\ub0b4\uc918!\"]"
	})

	rt := translationRuntime{cfg: Config{MaxAttempts: 1, BackoffSec: 0}, client: client, skill: newTranslateSkill("", "")}
	items := []translationTask{{ID: "id-1", BodyEN: "Send me now!", GroupKey: textKindDialogue, Lane: laneDefault}}

	props, skippedInvalid, skippedErr := collectProposals(rt, "slot-1", items)
	if skippedInvalid != 0 || skippedErr != 0 {
		t.Fatalf("skippedInvalid=%d skippedErr=%d", skippedInvalid, skippedErr)
	}
	if props["id-1"].ProposedKO != "\uc9c0\uae08 \ubcf4\ub0b4\uc918!" {
		t.Fatalf("proposal=%+v", props["id-1"])
	}
}

func TestCollectProposals_OpenCodeSinglePlainOutput_SalvagesQuoteFragmentArray(t *testing.T) {
	client := newPromptOnlyClient(t, platform.LLMBackendOpencode, responseModePlain, func(prompt string) (int, string) {
		return http.StatusOK, "[\"(한숨을 쉰다.) \"좋아. 뭘 알고 싶은데?\"]"
	})

	rt := translationRuntime{cfg: Config{MaxAttempts: 1, BackoffSec: 0}, client: client, skill: newTranslateSkill("", "")}
	items := []translationTask{{ID: "id-1", BodyEN: `(Sigh.) "Fine. What do you want to know?`, GroupKey: textKindDialogue, Lane: laneDefault}}

	props, skippedInvalid, skippedErr := collectProposals(rt, "slot-1", items)
	if skippedErr != 0 {
		t.Fatalf("skippedErr=%d", skippedErr)
	}
	if skippedInvalid != 0 {
		t.Fatalf("skippedInvalid=%d", skippedInvalid)
	}
	if props["id-1"].ProposedKO != `(한숨을 쉰다.) "좋아. 뭘 알고 싶은데?` {
		t.Fatalf("proposal=%+v", props["id-1"])
	}
}

func TestCollectProposals_FallsBackToHighClientOnDegenerateLowResult(t *testing.T) {
	lowClient := newPromptOnlyClient(t, platform.LLMBackendOllama, responseModePlain, func(prompt string) (int, string) {
		return http.StatusOK, "[\"Oh, you know it baby.\"]"
	})
	highClient := newPromptOnlyClient(t, platform.LLMBackendOpencode, responseModePlain, func(prompt string) (int, string) {
		return http.StatusOK, "[\"\uad6c\uba4d\uc774 \uba54\uc6cc\uc9c0\uc9c0 \uc54a\uc73c\uba74 \uafc8\uc801\ub3c4 \ud558\uc9c0 \uc54a\uc744 \uac70\ub2e4.\"]"
	})

	rt := translationRuntime{
		cfg:        Config{MaxAttempts: 1, BackoffSec: 0},
		client:     lowClient,
		highClient: highClient,
		skill:      newTranslateSkill("", ""),
	}
	items := []translationTask{{
		ID:       "id-1",
		BodyEN:   "It won't budge without the hole being filled.",
		GroupKey: textKindDialogue,
		Lane:     laneDefault,
	}}

	props, skippedInvalid, skippedErr := collectProposals(rt, "slot-1", items)
	if skippedErr != 0 {
		t.Fatalf("skippedErr=%d", skippedErr)
	}
	if skippedInvalid == 0 {
		t.Fatalf("expected low-client degenerate result to be counted as invalid fallback trigger")
	}
	if props["id-1"].ProposedKO != "\uad6c\uba4d\uc774 \uba54\uc6cc\uc9c0\uc9c0 \uc54a\uc73c\uba74 \uafc8\uc801\ub3c4 \ud558\uc9c0 \uc54a\uc744 \uac70\ub2e4." {
		t.Fatalf("proposal=%+v", props["id-1"])
	}
}

func TestCollectProposals_BatchDuplicateOutputsFallBackToSingles(t *testing.T) {
	lowCalls := 0
	lowClient := newPromptOnlyClient(t, platform.LLMBackendOllama, responseModePlain, func(prompt string) (int, string) {
		lowCalls++
		if strings.Contains(prompt, "Return one valid JSON array with exactly 1 Korean string and nothing else.") {
			if strings.Contains(prompt, `"id":"id-1"`) {
				return http.StatusOK, "[\"\uccab \ubc88\uc9f8 \ubc88\uc5ed\"]"
			}
			if strings.Contains(prompt, `"id":"id-2"`) {
				return http.StatusOK, "[\"\ub450 \ubc88\uc9f8 \ubc88\uc5ed\"]"
			}
		}
		return http.StatusOK, "[\"\uadf8\uac74 \uc81c\uac8c \ub108\ubb34 \ube44\uc2f8\uc694!\",\"\uadf8\uac74 \uc81c\uac8c \ub108\ubb34 \ube44\uc2f8\uc694!\"]"
	})
	highClient := newPromptOnlyClient(t, platform.LLMBackendOpencode, responseModePlain, func(prompt string) (int, string) {
		if strings.Contains(prompt, `"id":"id-1"`) {
			return http.StatusOK, "[\"\uccab \ubc88\uc9f8 \ubc88\uc5ed\"]"
		}
		if strings.Contains(prompt, `"id":"id-2"`) {
			return http.StatusOK, "[\"\ub450 \ubc88\uc9f8 \ubc88\uc5ed\"]"
		}
		return http.StatusInternalServerError, "unexpected"
	})

	rt := translationRuntime{
		cfg:        Config{MaxAttempts: 1, BackoffSec: 0},
		client:     lowClient,
		highClient: highClient,
		skill:      newTranslateSkill("", ""),
	}
	items := []translationTask{
		{ID: "id-1", BodyEN: "Safe & Secure", GroupKey: textKindDialogue, Lane: laneDefault},
		{ID: "id-2", BodyEN: "No Refunds", GroupKey: textKindDialogue, Lane: laneDefault},
	}

	props, skippedInvalid, skippedErr := collectProposals(rt, "slot-1", items)
	if skippedErr != 0 {
		t.Fatalf("skippedErr=%d", skippedErr)
	}
	if skippedInvalid < 2 {
		t.Fatalf("expected duplicated batch rows to be rejected, skippedInvalid=%d", skippedInvalid)
	}
	if props["id-1"].ProposedKO != "\uccab \ubc88\uc9f8 \ubc88\uc5ed" || props["id-2"].ProposedKO != "\ub450 \ubc88\uc9f8 \ubc88\uc5ed" {
		t.Fatalf("props=%v", props)
	}
	if lowCalls < 3 {
		t.Fatalf("expected batch + single retries, lowCalls=%d", lowCalls)
	}
}
