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
	"localize-agent/workflow/internal/platform"
	"localize-agent/workflow/internal/shared"
)

type fakeCheckpointStore struct {
	enabled bool
	upserts []string
}

func (f *fakeCheckpointStore) IsEnabled() bool                       { return f.enabled }
func (f *fakeCheckpointStore) LoadDoneIDs() (map[string]bool, error) { return map[string]bool{}, nil }
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
			Agent:      "",
			Warmup:     "",
		},
	}
}

func TestCollectProposals_SingleFallbackID(t *testing.T) {
	client := newServerClientForTest(t, func(prompt string) (int, string) {
		if strings.Contains(prompt, "Return ONE JSON line only") {
			return http.StatusOK, `{"id":"wrong","proposed_ko":"ko one [T0]","risk":"med","notes":"n"}`
		}
		return http.StatusInternalServerError, "unexpected prompt"
	})

	rt := translationRuntime{
		cfg:    Config{MaxAttempts: 1, BackoffSec: 0},
		client: client,
		skill:  newTranslateSkill("", ""),
	}
	items := []map[string]string{{"id": "id-1", "en": "A [T0]", "current_ko": "B [T0]"}}

	props, skippedInvalid, skippedErr := collectProposals(rt, "slot-1", items)
	if skippedInvalid != 0 {
		t.Fatalf("skippedInvalid = %d, want 0", skippedInvalid)
	}
	if skippedErr != 0 {
		t.Fatalf("skippedErr = %d, want 0", skippedErr)
	}
	p, ok := props["id-1"]
	if !ok {
		t.Fatalf("missing proposal for id-1")
	}
	if p.ID != "id-1" {
		t.Fatalf("proposal id=%q, want id-1", p.ID)
	}
	if p.ProposedKO != "ko one [T0]" {
		t.Fatalf("proposal text=%q", p.ProposedKO)
	}
}

func TestCollectProposals_BatchErrorFallsBackToSingles(t *testing.T) {
	responses := map[string]string{
		"id-1": `{"id":"id-1","proposed_ko":"ko1 [T0]","risk":"low","notes":""}`,
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

	rt := translationRuntime{
		cfg:    Config{MaxAttempts: 1, BackoffSec: 0},
		client: client,
		skill:  newTranslateSkill("", ""),
	}
	items := []map[string]string{
		{"id": "id-1", "en": "A [T0]", "current_ko": "B [T0]"},
		{"id": "id-2", "en": "C [T0]", "current_ko": "D [T0]"},
	}

	props, skippedInvalid, skippedErr := collectProposals(rt, "slot-1", items)
	if skippedInvalid != 0 {
		t.Fatalf("skippedInvalid=%d, want 0", skippedInvalid)
	}
	if skippedErr != 1 {
		t.Fatalf("skippedTranslatorErr=%d, want 1", skippedErr)
	}
	if len(props) != 1 || props["id-1"].ProposedKO == "" {
		t.Fatalf("proposals=%v, want only id-1", props)
	}
}

func TestCollectProposals_RejectsDegenerateEllipsis(t *testing.T) {
	client := newServerClientForTest(t, func(prompt string) (int, string) {
		return http.StatusOK, `{"id":"id-1","proposed_ko":"...","risk":"low","notes":""}`
	})

	rt := translationRuntime{
		cfg:    Config{MaxAttempts: 1, BackoffSec: 0},
		client: client,
		skill:  newTranslateSkill("", ""),
	}
	items := []map[string]string{{"id": "id-1", "en": "A meaningful sentence", "current_ko": ""}}

	props, skippedInvalid, skippedErr := collectProposals(rt, "slot-1", items)
	if skippedErr != 0 {
		t.Fatalf("skippedErr=%d, want 0", skippedErr)
	}
	if skippedInvalid != 1 {
		t.Fatalf("skippedInvalid=%d, want 1", skippedInvalid)
	}
	if len(props) != 0 {
		t.Fatalf("proposals=%v, want empty", props)
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
		"id-1": {ID: "id-1", ProposedKO: "localized [T0]", Risk: "", Notes: "ok"},
	}
	metas := map[string]itemMeta{
		"id-1": {
			id:      "id-1",
			enText:  "EN [T0]",
			curText: "CUR [T0]",
			curObj:  map[string]any{"Text": "before"},
			mapTags: []mapping{{placeholder: "[T0]", original: "{name}"}},
		},
	}
	done := map[string]map[string]any{}
	pack := []map[string]any{}
	var mu sync.Mutex

	out := persistResults(rt, "slot", proposals, metas, done, pack, &mu, nil)
	if out.abortWorker {
		t.Fatalf("abortWorker=true, want false")
	}
	if out.skippedInvalid != 0 {
		t.Fatalf("skippedInvalid=%d, want 0", out.skippedInvalid)
	}
	if got := done["id-1"]["Text"]; got != "localized {name}" {
		t.Fatalf("restored text=%v, want localized {name}", got)
	}
	if len(out.pack) != 1 {
		t.Fatalf("pack len=%d, want 1", len(out.pack))
	}
	if out.pack[0]["risk"] != "low" {
		t.Fatalf("risk=%v, want low default", out.pack[0]["risk"])
	}
	if len(cp.upserts) != 1 || cp.upserts[0] != "id-1:done" {
		t.Fatalf("checkpoint upserts=%v", cp.upserts)
	}
}

func TestPersistResults_RecoveryPromptFixesPlaceholder(t *testing.T) {
	client := newServerClientForTest(t, func(prompt string) (int, string) {
		if strings.Contains(prompt, "placeholder recovery task") {
			return http.StatusOK, `{"id":"id-1","proposed_ko":"fixed [T0]","risk":"low","notes":""}`
		}
		return http.StatusInternalServerError, "unexpected"
	})
	cp := &fakeCheckpointStore{enabled: false}
	rt := translationRuntime{
		cfg:        Config{SkipInvalid: true, PlaceholderRecoveryAttempts: 1, MaxAttempts: 1, BackoffSec: 0},
		client:     client,
		skill:      newTranslateSkill("", ""),
		checkpoint: cp,
	}

	proposals := map[string]proposal{
		"id-1": {ID: "id-1", ProposedKO: "broken no-tag", Risk: "med", Notes: ""},
	}
	metas := map[string]itemMeta{
		"id-1": {
			id:      "id-1",
			enText:  "EN [T0]",
			curText: "CUR [T0]",
			curObj:  map[string]any{"Text": "before"},
			mapTags: []mapping{{placeholder: "[T0]", original: "{x}"}},
		},
	}
	done := map[string]map[string]any{}
	var mu sync.Mutex

	out := persistResults(rt, "slot", proposals, metas, done, nil, &mu, nil)
	if out.skippedInvalid != 0 {
		t.Fatalf("skippedInvalid = %d, want 0", out.skippedInvalid)
	}
	if out.abortWorker {
		t.Fatalf("abortWorker = true, want false")
	}
	if got := done["id-1"]["Text"]; got != "fixed {x}" {
		t.Fatalf("recovered text=%v, want fixed {x}", got)
	}
}

func TestRunPipeline_AggregatesCounts(t *testing.T) {
	client := newServerClientForTest(t, func(prompt string) (int, string) {
		if strings.Contains(prompt, `"id":"id-ok"`) {
			return http.StatusOK, `{"id":"id-ok","proposed_ko":"ok [T0]","risk":"low","notes":""}`
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
		doneFromCheckpoint: map[string]bool{"id-done": true},
		client:             client,
		skill:              newTranslateSkill("", ""),
		checkpoint:         cp,
	}

	result := runPipeline(rt)
	if result.completedCount != 1 {
		t.Fatalf("completed=%d, want 1", result.completedCount)
	}
	if result.skippedInvalid != 1 {
		t.Fatalf("skippedInvalid=%d, want 1", result.skippedInvalid)
	}
	if result.skippedLong != 1 {
		t.Fatalf("skippedLong=%d, want 1", result.skippedLong)
	}
	if len(result.skippedLongIDs) != 1 || result.skippedLongIDs[0] != "id-long" {
		t.Fatalf("skippedLongIDs=%v", result.skippedLongIDs)
	}
	if result.skippedTranslatorErr != 0 {
		t.Fatalf("skippedTranslatorErr=%d, want 0", result.skippedTranslatorErr)
	}
}
