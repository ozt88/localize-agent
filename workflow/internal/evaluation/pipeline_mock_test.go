package evaluation

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"localize-agent/workflow/internal/contracts"
	"localize-agent/workflow/pkg/shared"
)

type fakeEvalStore struct {
	loadPackItems   []contracts.EvalPackItem
	resetIDsInput   []string
	resetStatusIn   []string
	resetEvalCalls  int
	pendingIDsValue []string
	saveCalls       []saveCall
	items           map[string]contracts.EvalPackItem
	markCalls       []string

	loadPackN  int
	loadPackE  error
	resetIDsN  int
	resetIDsE  error
	resetEvalN int
	resetEvalE error
	pendingE   error
	saveE      error
	getItemE   error
	markE      error
}

type saveCall struct {
	id, status, finalKO, finalRisk, finalNotes string
	revised                                    bool
	history                                    []contracts.EvalResult
}

func (f *fakeEvalStore) Close() {}
func (f *fakeEvalStore) LoadPack(items []contracts.EvalPackItem) (int, error) {
	f.loadPackItems = append([]contracts.EvalPackItem(nil), items...)
	return f.loadPackN, f.loadPackE
}
func (f *fakeEvalStore) PendingIDs() ([]string, error) {
	return append([]string(nil), f.pendingIDsValue...), f.pendingE
}
func (f *fakeEvalStore) GetItem(id string) (*contracts.EvalPackItem, error) {
	if f.getItemE != nil {
		return nil, f.getItemE
	}
	it, ok := f.items[id]
	if !ok {
		return nil, fmt.Errorf("not found: %s", id)
	}
	copied := it
	return &copied, nil
}
func (f *fakeEvalStore) MarkEvaluating(id string) error {
	f.markCalls = append(f.markCalls, id)
	return f.markE
}
func (f *fakeEvalStore) SaveResult(id, status, finalKO, finalRisk, finalNotes string, revised bool, history []contracts.EvalResult) error {
	f.saveCalls = append(f.saveCalls, saveCall{id: id, status: status, finalKO: finalKO, finalRisk: finalRisk, finalNotes: finalNotes, revised: revised, history: append([]contracts.EvalResult(nil), history...)})
	return f.saveE
}
func (f *fakeEvalStore) ResetToStatus(statuses []string) (int, error) {
	f.resetStatusIn = append([]string(nil), statuses...)
	return 0, nil
}
func (f *fakeEvalStore) ResetIDs(ids []string) (int, error) {
	f.resetIDsInput = append([]string(nil), ids...)
	return f.resetIDsN, f.resetIDsE
}
func (f *fakeEvalStore) ResetEvaluating() (int, error) {
	f.resetEvalCalls++
	return f.resetEvalN, f.resetEvalE
}
func (f *fakeEvalStore) StatusCounts() (map[string]int, error)                       { return map[string]int{}, nil }
func (f *fakeEvalStore) ExportByStatus(statuses ...string) ([]map[string]any, error) { return nil, nil }

var _ contracts.EvalStore = (*fakeEvalStore)(nil)

type fakeFileStore struct {
	jsonData map[string]any
	readErr  error
}

func (f *fakeFileStore) ReadJSON(path string, out any) error {
	if f.readErr != nil {
		return f.readErr
	}
	v, ok := f.jsonData[path]
	if !ok {
		return errors.New("missing path")
	}
	raw, _ := json.Marshal(v)
	return json.Unmarshal(raw, out)
}
func (f *fakeFileStore) WriteJSON(path string, v any) error           { return nil }
func (f *fakeFileStore) ReadLines(path string) ([]string, error)      { return nil, nil }
func (f *fakeFileStore) WriteLines(path string, lines []string) error { return nil }

var _ contracts.FileStore = (*fakeFileStore)(nil)

type evalResponder func(kind string, prompt string) (int, string)

func newEvalClientForTest(t *testing.T, responder evalResponder) *evalClient {
	t.Helper()

	var mu sync.Mutex
	sessionSeq := 0
	sessions := map[string]string{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			sessionSeq++
			id := fmt.Sprintf("s%d", sessionSeq)
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{"id":"` + id + `"}`))
			return
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/session/") && strings.HasSuffix(r.URL.Path, "/message"):
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			parts, _ := body["parts"].([]any)
			p0, _ := parts[0].(map[string]any)
			prompt, _ := p0["text"].(string)

			rawPath := strings.TrimPrefix(r.URL.Path, "/session/")
			sid := strings.TrimSuffix(rawPath, "/message")
			sid = strings.TrimSuffix(sid, "/")
			if sid == "" {
				http.Error(w, "missing sid", http.StatusBadRequest)
				return
			}

			kind := "trans"
			mu.Lock()
			if known, ok := sessions[sid]; ok {
				kind = known
			} else {
				model, _ := body["model"].(map[string]any)
				if id, _ := model["modelID"].(string); strings.Contains(id, "eval") {
					kind = "eval"
				}
				sessions[sid] = kind
			}
			status, payload := responder(kind, prompt)
			mu.Unlock()

			if status != http.StatusOK {
				http.Error(w, payload, status)
				return
			}
			w.Header().Set("content-type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"parts": []map[string]any{{"type": "text", "text": payload}}})
			return
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)

	client, err := newEvalClient(
		"opencode",
		ts.URL,
		"test/trans-model", "",
		newTranslateSkill("", ""),
		"test/eval-model", "",
		newEvaluateSkill("", ""),
		2,
		&shared.MetricCollector{},
		nil,
	)
	if err != nil {
		t.Fatalf("newEvalClient error: %v", err)
	}
	return client
}

func TestPrepareEvaluationWork_UsesPackReevalResume(t *testing.T) {
	giveFiles := &fakeFileStore{jsonData: map[string]any{
		"pack.json": map[string]any{"items": []map[string]any{{"id": "a", "en": "EN", "current_ko": "CUR", "proposed_ko_restored": "KO", "risk": "low", "notes": ""}}},
	}}
	giveStore := &fakeEvalStore{
		loadPackN:       1,
		resetIDsN:       2,
		resetEvalN:      3,
		pendingIDsValue: []string{"p1", "p2"},
	}
	giveCfg := Config{PackIn: "pack.json", ReevalIDs: "x, y", Resume: true}

	pending, code := prepareEvaluationWork(giveCfg, giveStore, giveFiles)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if len(giveStore.loadPackItems) != 1 {
		t.Fatalf("len(loadPackItems) = %d, want 1", len(giveStore.loadPackItems))
	}
	if giveStore.loadPackItems[0].ID != "a" {
		t.Fatalf("loadPackItems[0].ID = %q, want %q", giveStore.loadPackItems[0].ID, "a")
	}
	wantIDs := []string{"x", "y"}
	if !reflect.DeepEqual(giveStore.resetIDsInput, wantIDs) {
		t.Fatalf("resetIDsInput = %v, want %v", giveStore.resetIDsInput, wantIDs)
	}
	if giveStore.resetEvalCalls != 1 {
		t.Fatalf("resetEvalCalls = %d, want 1", giveStore.resetEvalCalls)
	}
	if !reflect.DeepEqual(pending, []string{"p1", "p2"}) {
		t.Fatalf("pending = %v, want [p1 p2]", pending)
	}
}

func TestPrepareEvaluationWork_ReturnsErrorOnReadPackFailure(t *testing.T) {
	giveFiles := &fakeFileStore{readErr: errors.New("boom")}
	giveStore := &fakeEvalStore{}
	giveCfg := Config{PackIn: "pack.json"}

	pending, code := prepareEvaluationWork(giveCfg, giveStore, giveFiles)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if pending != nil {
		t.Fatalf("pending=%v, want nil", pending)
	}
}

func TestRunEvalItem_ReviseThenPass(t *testing.T) {
	var calls []string
	client := newEvalClientForTest(t, func(kind string, prompt string) (int, string) {
		calls = append(calls, kind)
		if kind == kindEval {
			if strings.Contains(prompt, `"ko":"revised-ko"`) {
				return http.StatusOK, `{"id":"x","verdict":"pass","issues":[]}`
			}
			return http.StatusOK, `{"id":"x","verdict":"revise","issues":["tone"]}`
		}
		return http.StatusOK, `{"id":"x","proposed_ko":"revised-ko","risk":"med","notes":"fixed"}`
	})

	item := &packItem{ID: "x", EN: "EN", CurrentKO: "CUR", ProposedKORestored: "orig-ko", Risk: "low", Notes: "n"}
	out := runEvalItem(client, "slot", item, 1, 0, 1)
	if out.finalStatus != statusPass {
		t.Fatalf("finalStatus=%s, want pass", out.finalStatus)
	}
	if out.finalKO != "revised-ko" {
		t.Fatalf("finalKO=%s, want revised-ko", out.finalKO)
	}
	if !out.revised {
		t.Fatalf("revised=false, want true")
	}
	if len(out.history) != 2 {
		t.Fatalf("history len=%d, want 2", len(out.history))
	}
	sort.Strings(calls)
	if !reflect.DeepEqual(calls, []string{"eval", "eval", "eval", "trans", "trans"}) {
		t.Fatalf("calls=%v, want eval,eval,eval,trans,trans", calls)
	}
}

func TestRunEvalItem_EvalErrorDefaultsPass(t *testing.T) {
	client := newEvalClientForTest(t, func(kind string, prompt string) (int, string) {
		if kind == kindEval {
			return http.StatusInternalServerError, "eval fail"
		}
		return http.StatusOK, `{"id":"x","proposed_ko":"never-used","risk":"low","notes":""}`
	})

	item := &packItem{ID: "x", EN: "EN", CurrentKO: "CUR", ProposedKORestored: "orig", Risk: "low", Notes: "n"}
	out := runEvalItem(client, "slot", item, 1, 0, 1)
	if out.finalStatus != statusPass {
		t.Fatalf("finalStatus=%s, want pass", out.finalStatus)
	}
	if out.finalKO != "orig" {
		t.Fatalf("finalKO=%s, want orig", out.finalKO)
	}
	if len(out.history) != 0 {
		t.Fatalf("history len=%d, want 0", len(out.history))
	}
}

func TestPersistEvaluationOutcome_SavesFullPayload(t *testing.T) {
	store := &fakeEvalStore{}
	outcome := itemOutcome{
		id:          "id-1",
		finalKO:     "ko",
		finalRisk:   "med",
		finalNotes:  "notes",
		finalStatus: statusRevise,
		revised:     true,
		history:     []evalResult{{ID: "id-1", Verdict: "revise", Issues: []string{"tone"}}},
	}

	persistEvaluationOutcome(store, 1, "id-1", outcome)
	if len(store.saveCalls) != 1 {
		t.Fatalf("len(saveCalls) = %d, want 1", len(store.saveCalls))
	}
	call := store.saveCalls[0]
	if call.id != "id-1" {
		t.Fatalf("call.id = %q, want %q", call.id, "id-1")
	}
	if call.status != statusRevise {
		t.Fatalf("call.status = %q, want %q", call.status, statusRevise)
	}
	if call.finalKO != "ko" {
		t.Fatalf("call.finalKO = %q, want %q", call.finalKO, "ko")
	}
	if !call.revised {
		t.Fatalf("call.revised = false, want true")
	}
	if len(call.history) != 1 || call.history[0].Verdict != "revise" {
		t.Fatalf("history=%v", call.history)
	}
}

func TestRunEvaluationWorkers_ProcessesPending(t *testing.T) {
	store := &fakeEvalStore{
		items: map[string]contracts.EvalPackItem{
			"a": {ID: "a", EN: "EN-a", CurrentKO: "CUR-a", ProposedKORestored: "KO-a", Risk: "low"},
			"b": {ID: "b", EN: "EN-b", CurrentKO: "CUR-b", ProposedKORestored: "KO-b", Risk: "med"},
		},
	}
	client := newEvalClientForTest(t, func(kind string, prompt string) (int, string) {
		if kind == kindEval && strings.Contains(prompt, "Evaluate this translation unit") {
			return http.StatusOK, `{"id":"x","verdict":"pass","issues":[]}`
		}
		return http.StatusOK, "OK"
	})

	runEvaluationWorkers(
		Config{Concurrency: 1, ServerURL: "http://mock", MaxAttempts: 1, BackoffSec: 0, MaxRetry: 0},
		store,
		client,
		[]string{"a", "b"},
	)

	if len(store.markCalls) != 2 {
		t.Fatalf("mark calls=%v, want 2 items", store.markCalls)
	}
	if len(store.saveCalls) != 2 {
		t.Fatalf("save calls=%d, want 2", len(store.saveCalls))
	}
	for _, c := range store.saveCalls {
		if c.status != statusPass {
			t.Fatalf("status=%s, want pass", c.status)
		}
	}
}

func TestRunEvaluationWorkers_SkipsMissingItems(t *testing.T) {
	store := &fakeEvalStore{
		items: map[string]contracts.EvalPackItem{
			"a": {ID: "a", EN: "EN-a", CurrentKO: "CUR-a", ProposedKORestored: "KO-a", Risk: "low"},
		},
	}
	client := newEvalClientForTest(t, func(kind string, prompt string) (int, string) {
		if kind == kindEval && strings.Contains(prompt, "Evaluate this translation unit") {
			return http.StatusOK, `{"id":"x","verdict":"pass","issues":[]}`
		}
		return http.StatusOK, "OK"
	})

	runEvaluationWorkers(
		Config{Concurrency: 1, ServerURL: "http://mock", MaxAttempts: 1, BackoffSec: 0, MaxRetry: 0},
		store,
		client,
		[]string{"a", "missing"},
	)

	if len(store.saveCalls) != 1 || store.saveCalls[0].id != "a" {
		t.Fatalf("save calls=%v, want only id=a", store.saveCalls)
	}
}
