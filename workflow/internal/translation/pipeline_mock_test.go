package translation

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"localize-agent/workflow/internal/contracts"
	"localize-agent/workflow/pkg/platform"
	"localize-agent/workflow/pkg/shared"
)

type fakeCheckpointStore struct {
	enabled bool
	upserts []string
}

func (f *fakeCheckpointStore) IsEnabled() bool { return f.enabled }
func (f *fakeCheckpointStore) LoadDoneIDs(pipelineVersion string) (map[string]bool, error) {
	return map[string]bool{}, nil
}
func (f *fakeCheckpointStore) UpsertItem(entryID, status, sourceHash string, attempts int, lastError string, latencyMs float64, koObj, packObj map[string]any) error {
	f.upserts = append(f.upserts, entryID+":"+status)
	return nil
}
func (f *fakeCheckpointStore) UpsertItems(items []contracts.TranslationCheckpointItem) error {
	for _, it := range items {
		f.upserts = append(f.upserts, it.EntryID+":"+it.Status)
	}
	return nil
}
func (f *fakeCheckpointStore) Close() error { return nil }

var _ contracts.TranslationCheckpointStore = (*fakeCheckpointStore)(nil)

type llmPromptResponder func(prompt string) (int, string)

func newServerClientForTest(t *testing.T, responder llmPromptResponder) *serverClient {
	t.Helper()

	var mu sync.Mutex
	sessionID := "s1"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{"id":"` + sessionID + `"}`))
			return
		case r.Method == http.MethodPost && r.URL.Path == "/session/"+sessionID+"/message":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			parts, _ := body["parts"].([]any)
			if len(parts) == 0 {
				http.Error(w, "missing parts", http.StatusBadRequest)
				return
			}
			p0, _ := parts[0].(map[string]any)
			prompt, _ := p0["text"].(string)

			mu.Lock()
			status, payload := responder(prompt)
			mu.Unlock()

			if status != http.StatusOK {
				http.Error(w, payload, status)
				return
			}
			w.Header().Set("content-type", "application/json")
			resp := map[string]any{"parts": []map[string]any{{"type": "text", "text": payload}}}
			_ = json.NewEncoder(w).Encode(resp)
			return
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)

	return &serverClient{
		llm: platform.NewSessionLLMClient(ts.URL, 2, &shared.MetricCollector{}, nil),
		profile: platform.LLMProfile{
			ProviderID: "test",
			ModelID:    "mock",
		},
	}
}

func TestCollectProposals_SingleFallbackID(t *testing.T) {
	client := newServerClientForTest(t, func(prompt string) (int, string) {
		if strings.Contains(prompt, `"id":"id-1"`) {
			return http.StatusOK, `{"id":"wrong","proposed_ko":"번역 하나 [T0]"}`
		}
		return http.StatusInternalServerError, "unexpected prompt"
	})

	rt := translationRuntime{cfg: Config{MaxAttempts: 1, BackoffSec: 0}, client: client, skill: newTranslateSkill("", "")}
	items := []translationTask{{ID: "id-1", BodyEN: "A [T0]", GroupKey: textKindDialogue, Lane: laneDefault}}

	props, skippedInvalid, skippedErr := collectProposals(rt, "slot-1", items)
	if skippedInvalid != 0 || skippedErr != 0 {
		t.Fatalf("skippedInvalid=%d skippedErr=%d", skippedInvalid, skippedErr)
	}
	if props["id-1"].ID != "id-1" || props["id-1"].ProposedKO != "번역 하나 [T0]" {
		t.Fatalf("proposal=%+v", props["id-1"])
	}
}

func TestCollectProposals_BatchErrorFallsBackToSingles(t *testing.T) {
	responses := map[string]string{
		"id-1": `{"id":"id-1","proposed_ko":"번역1 [T0]"}`,
	}
	client := newServerClientForTest(t, func(prompt string) (int, string) {
		if strings.Contains(prompt, "Input items") {
			return http.StatusInternalServerError, "batch failed"
		}
		for id, raw := range responses {
			if strings.Contains(prompt, `"id":"`+id+`"`) {
				return http.StatusOK, raw
			}
		}
		return http.StatusInternalServerError, "single failed"
	})

	rt := translationRuntime{cfg: Config{MaxAttempts: 1, BackoffSec: 0}, client: client, skill: newTranslateSkill("", "")}
	items := []translationTask{
		{ID: "id-1", BodyEN: "A [T0]", GroupKey: textKindDialogue, Lane: laneDefault},
		{ID: "id-2", BodyEN: "C [T0]", GroupKey: textKindDialogue, Lane: laneDefault},
	}

	props, skippedInvalid, skippedErr := collectProposals(rt, "slot-1", items)
	if skippedInvalid != 0 || skippedErr != 1 {
		t.Fatalf("skippedInvalid=%d skippedErr=%d", skippedInvalid, skippedErr)
	}
	if len(props) != 1 || props["id-1"].ProposedKO == "" {
		t.Fatalf("proposals=%v", props)
	}
}

func TestCollectProposals_RejectsDegenerateEllipsis(t *testing.T) {
	client := newServerClientForTest(t, func(prompt string) (int, string) {
		return http.StatusOK, `{"id":"id-1","proposed_ko":"..."}`
	})

	rt := translationRuntime{cfg: Config{MaxAttempts: 1, BackoffSec: 0}, client: client, skill: newTranslateSkill("", "")}
	items := []translationTask{{ID: "id-1", BodyEN: "A meaningful sentence", GroupKey: textKindDialogue, Lane: laneDefault}}

	props, skippedInvalid, skippedErr := collectProposals(rt, "slot-1", items)
	if skippedErr != 0 || skippedInvalid != 1 || len(props) != 0 {
		t.Fatalf("props=%v skippedInvalid=%d skippedErr=%d", props, skippedInvalid, skippedErr)
	}
}

func TestCollectProposals_SplitsMixedKinds(t *testing.T) {
	var prompts []string
	client := newServerClientForTest(t, func(prompt string) (int, string) {
		prompts = append(prompts, prompt)
		switch {
		case strings.Contains(prompt, `"id":"id-choice"`):
			return http.StatusOK, `[{"id":"id-choice","proposed_ko":"선택지 번역"}]`
		case strings.Contains(prompt, `"id":"id-rich"`):
			return http.StatusOK, `[{"id":"id-rich","proposed_ko":"강조 [T0]텍스트[T1] 번역"}]`
		default:
			return http.StatusInternalServerError, "unexpected"
		}
	})

	rt := translationRuntime{cfg: Config{MaxAttempts: 1, BackoffSec: 0}, client: client, skill: newTranslateSkill("", "")}
	items := []translationTask{
		{ID: "id-choice", BodyEN: "Do it.", GroupKey: textKindChoice, Lane: laneDefault},
		{ID: "id-rich", BodyEN: "Do [T0]you[T1] care?", GroupKey: textKindDialogue + "+rich", Lane: laneDefault},
	}

	props, skippedInvalid, skippedErr := collectProposals(rt, "slot-1", items)
	if skippedInvalid != 0 || skippedErr != 0 || len(props) != 2 || len(prompts) != 2 {
		t.Fatalf("props=%v skippedInvalid=%d skippedErr=%d prompts=%d", props, skippedInvalid, skippedErr, len(prompts))
	}
}

func TestPersistResults_RestoreAndCheckpoint(t *testing.T) {
	cp := &fakeCheckpointStore{enabled: true}
	rt := translationRuntime{
		cfg:        Config{SkipInvalid: true, PlaceholderRecoveryAttempts: 0},
		skill:      newTranslateSkill("", ""),
		checkpoint: cp,
	}

	proposals := map[string]proposal{
		"id-1": {ID: "id-1", ProposedKO: "localized [T0]"},
	}
	metas := map[string]itemMeta{
		"id-1": {
			id:            "id-1",
			sourceRaw:     "EN {name}",
			enText:        "EN [T0]",
			curText:       "CUR [T0]",
			sourceType:    "textasset",
			sourceFile:    "AR_Viira",
			resourceKey:   "UI_1",
			metaPathLabel: "Assets/TextAsset/AR_Viira.bytes:12",
			segmentID:     "seg-1",
			segmentPos:    intPtr(2),
			choiceBlockID: "choice-1",
			curObj:        map[string]any{"Text": "before"},
			mapTags:       []mapping{{placeholder: "[T0]", original: "{name}"}},
			profile:       textProfile{Kind: textKindDialogue},
		},
	}
	done := map[string]map[string]any{}
	var mu sync.Mutex

	out := persistResults(rt, "slot", proposals, metas, done, nil, &mu, nil)
	if out.abortWorker || out.skippedInvalid != 0 {
		t.Fatalf("persist=%+v", out)
	}
	if got := done["id-1"]["Text"]; got != "localized {name}" {
		t.Fatalf("restored text=%v", got)
	}
	if len(out.pack) != 1 {
		t.Fatalf("pack len=%d", len(out.pack))
	}
	if got := out.pack[0]["current_ko"]; got != "before" {
		t.Fatalf("current_ko=%v", got)
	}
	if out.pack[0]["source_file"] != "AR_Viira" || out.pack[0]["resource_key"] != "UI_1" {
		t.Fatalf("pack=%v", out.pack[0])
	}
	if out.pack[0]["segment_pos"] != 2 {
		t.Fatalf("segment_pos=%v", out.pack[0]["segment_pos"])
	}
	if len(cp.upserts) != 1 || cp.upserts[0] != "id-1:done" {
		t.Fatalf("checkpoint upserts=%v", cp.upserts)
	}
}

func TestPersistResults_RecoveryPromptFixesPlaceholder(t *testing.T) {
	client := newServerClientForTest(t, func(prompt string) (int, string) {
		if strings.Contains(prompt, "placeholder recovery task") {
			return http.StatusOK, `{"id":"id-1","proposed_ko":"fixed [T0]"}`
		}
		return http.StatusInternalServerError, "unexpected"
	})
	rt := translationRuntime{
		cfg:        Config{SkipInvalid: true, PlaceholderRecoveryAttempts: 1, MaxAttempts: 1, BackoffSec: 0},
		client:     client,
		skill:      newTranslateSkill("", ""),
		checkpoint: &fakeCheckpointStore{enabled: false},
	}

	proposals := map[string]proposal{
		"id-1": {ID: "id-1", ProposedKO: "broken no-tag"},
	}
	metas := map[string]itemMeta{
		"id-1": {
			id:        "id-1",
			sourceRaw: "EN {x}",
			enText:    "EN [T0]",
			curText:   "CUR [T0]",
			curObj:    map[string]any{"Text": "before"},
			mapTags:   []mapping{{placeholder: "[T0]", original: "{x}"}},
			profile:   textProfile{Kind: textKindDialogue},
		},
	}
	done := map[string]map[string]any{}
	var mu sync.Mutex

	out := persistResults(rt, "slot", proposals, metas, done, nil, &mu, nil)
	if out.skippedInvalid != 0 || out.abortWorker {
		t.Fatalf("persist=%+v", out)
	}
	if got := done["id-1"]["Text"]; got != "fixed {x}" {
		t.Fatalf("recovered text=%v", got)
	}
}

func TestPersistResults_PassthroughControlToken(t *testing.T) {
	cp := &fakeCheckpointStore{enabled: true}
	rt := translationRuntime{
		cfg:        Config{SkipInvalid: true},
		skill:      newTranslateSkill("", ""),
		checkpoint: cp,
	}

	done := map[string]map[string]any{}
	var mu sync.Mutex
	metas := map[string]itemMeta{
		"id-1": {
			id:          "id-1",
			sourceRaw:   ".CB_RuinBOT==1-",
			enText:      ".CB_RuinBOT==1-",
			curText:     "",
			curObj:      map[string]any{"Text": ""},
			profile:     textProfile{Kind: textKindDialogue},
			passthrough: true,
		},
	}

	out := persistResults(rt, "slot", map[string]proposal{}, metas, done, nil, &mu, nil)
	if out.abortWorker || out.skippedInvalid != 0 {
		t.Fatalf("persist=%+v", out)
	}
	if got := done["id-1"]["Text"]; got != ".CB_RuinBOT==1-" {
		t.Fatalf("restored text=%v", got)
	}
	if got := out.pack[0]["current_ko"]; got != "" {
		t.Fatalf("current_ko=%v", got)
	}
}

func TestRunPipeline_AggregatesCounts(t *testing.T) {
	client := newServerClientForTest(t, func(prompt string) (int, string) {
		if strings.Contains(prompt, `"id":"id-ok"`) {
			return http.StatusOK, `{"id":"id-ok","proposed_ko":"정상 [T0]"}`
		}
		return http.StatusInternalServerError, fmt.Sprintf("unexpected prompt: %s", prompt)
	})
	cp := &fakeCheckpointStore{enabled: true}

	rt := translationRuntime{
		cfg: Config{
			ServerURL:                   "http://mock",
			Concurrency:                 1,
			BatchSize:                   4,
			MaxAttempts:                 1,
			BackoffSec:                  0,
			MaxPlainLen:                 10,
			SkipInvalid:                 true,
			PlaceholderRecoveryAttempts: 0,
		},
		sourceStrings: map[string]map[string]any{
			"id-ok":   {"Text": "Hi {x}"},
			"id-long": {"Text": "this text is too long"},
			"id-miss": {"Text": "exists in source"},
		},
		currentStrings: map[string]map[string]any{
			"id-ok":   {"Text": "cur {x}"},
			"id-long": {"Text": "cur long"},
		},
		ids:                []string{"id-ok", "id-long", "id-miss", "id-done"},
		idIndex:            map[string]int{"id-ok": 0, "id-long": 1, "id-miss": 2, "id-done": 3},
		doneFromCheckpoint: map[string]bool{"id-done": true},
		client:             client,
		skill:              newTranslateSkill("", ""),
		checkpoint:         cp,
	}

	result := runPipeline(rt)
	if result.completedCount != 1 || result.skippedInvalid != 1 || result.skippedLong != 1 || result.skippedTranslatorErr != 0 {
		t.Fatalf("result=%+v", result)
	}
	if len(result.skippedLongIDs) != 1 || result.skippedLongIDs[0] != "id-long" {
		t.Fatalf("skippedLongIDs=%v", result.skippedLongIDs)
	}
}

func intPtr(v int) *int {
	return &v
}
