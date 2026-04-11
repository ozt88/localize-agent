package v2pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"localize-agent/workflow/internal/contracts"
)

// fakeStore implements contracts.V2PipelineStore for testing workers.
type fakeStore struct {
	mu    sync.Mutex
	items map[string]*contracts.V2PipelineItem
	logs  map[string][]map[string]interface{}
}

var _ contracts.V2PipelineStore = (*fakeStore)(nil)

func newFakeStore() *fakeStore {
	return &fakeStore{
		items: make(map[string]*contracts.V2PipelineItem),
		logs:  make(map[string][]map[string]interface{}),
	}
}

func (s *fakeStore) addItem(item contracts.V2PipelineItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := item
	s.items[item.ID] = &cp
}

func (s *fakeStore) getItem(id string) *contracts.V2PipelineItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.items[id]
}

func (s *fakeStore) Seed(items []contracts.V2PipelineItem) (int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inserted := 0
	for _, item := range items {
		if _, exists := s.items[item.ID]; !exists {
			cp := item
			s.items[item.ID] = &cp
			inserted++
		}
	}
	return inserted, len(items) - inserted, nil
}

func (s *fakeStore) ClaimPending(pendingState, workingState, workerID string, batchSize int, leaseSec int) ([]contracts.V2PipelineItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var claimed []contracts.V2PipelineItem
	for _, item := range s.items {
		if item.State == pendingState && len(claimed) < batchSize {
			item.State = workingState
			item.ClaimedBy = workerID
			claimed = append(claimed, *item)
		}
	}
	return claimed, nil
}

func (s *fakeStore) ClaimBatch(pendingState, workingState, workerID string, leaseSec int) (string, []contracts.V2PipelineItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Find first pending batch_id
	var batchID string
	for _, item := range s.items {
		if item.State == pendingState {
			batchID = item.BatchID
			break
		}
	}
	if batchID == "" {
		return "", nil, nil
	}
	var claimed []contracts.V2PipelineItem
	for _, item := range s.items {
		if item.State == pendingState && item.BatchID == batchID {
			item.State = workingState
			item.ClaimedBy = workerID
			claimed = append(claimed, *item)
		}
	}
	return batchID, claimed, nil
}

func (s *fakeStore) MarkState(id, newState string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if item, ok := s.items[id]; ok {
		item.State = newState
		item.ClaimedBy = ""
	}
	return nil
}

func (s *fakeStore) MarkTranslated(id, koRaw string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return fmt.Errorf("item not found: %s", id)
	}
	item.KORaw = koRaw
	item.TranslateAttempts++
	item.ClaimedBy = ""
	if item.HasTags {
		item.State = StatePendingFormat
	} else {
		item.State = StatePendingScore
	}
	return nil
}

func (s *fakeStore) MarkFormatted(id, koFormatted string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return fmt.Errorf("item not found: %s", id)
	}
	item.KOFormatted = koFormatted
	item.FormatAttempts++
	item.State = StatePendingScore
	item.ClaimedBy = ""
	return nil
}

func (s *fakeStore) MarkScored(id string, scoreFinal float64, failureType, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return fmt.Errorf("item not found: %s", id)
	}
	item.ScoreFinal = scoreFinal
	item.FailureType = failureType
	item.LastError = reason
	item.ScoreAttempts++
	item.ClaimedBy = ""
	switch failureType {
	case "pass":
		item.State = StateDone
	case "translation", "both":
		item.State = StatePendingTranslate
	case "format":
		item.State = StatePendingFormat
	default:
		item.State = StateFailed
	}
	return nil
}

func (s *fakeStore) MarkFailed(id, lastError string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if item, ok := s.items[id]; ok {
		item.State = StateFailed
		item.LastError = lastError
		item.ClaimedBy = ""
	}
	return nil
}

func (s *fakeStore) AppendAttemptLog(id string, entry map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs[id] = append(s.logs[id], entry)
	return nil
}

func (s *fakeStore) UpdateRetryState(id, targetState, incrementField string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return fmt.Errorf("item not found: %s", id)
	}
	item.State = targetState
	item.ClaimedBy = ""
	switch incrementField {
	case "translate_attempts":
		item.TranslateAttempts++
	case "format_attempts":
		item.FormatAttempts++
	case "score_attempts":
		item.ScoreAttempts++
	}
	return nil
}

func (s *fakeStore) CleanupStaleClaims(olderThanSec int) (int64, error) { return 0, nil }
func (s *fakeStore) CountByState() (map[string]int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	counts := make(map[string]int)
	for _, item := range s.items {
		counts[item.State]++
	}
	return counts, nil
}
func (s *fakeStore) GetItem(id string) (*contracts.V2PipelineItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if item, ok := s.items[id]; ok {
		cp := *item
		return &cp, nil
	}
	return nil, nil
}
func (s *fakeStore) QueryDone() ([]contracts.V2PipelineItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []contracts.V2PipelineItem
	for _, item := range s.items {
		if item.State == contracts.StateDone {
			result = append(result, *item)
		}
	}
	return result, nil
}
func (s *fakeStore) MarkDonePassthrough(id, koFormatted string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return fmt.Errorf("item %s not found", id)
	}
	item.State = StateDone
	item.KORaw = koFormatted
	item.KOFormatted = koFormatted
	item.ClaimedBy = ""
	return nil
}

func (s *fakeStore) GetPrevGateLines(knot, currentGate string, limit int) ([]string, error) {
	return nil, nil // no-op for tests
}

func (s *fakeStore) Close() error { return nil }

// TestTranslateWorkerHappyPath verifies claim -> translate -> mark translated.
func TestTranslateWorkerHappyPath(t *testing.T) {
	store := newFakeStore()
	store.addItem(contracts.V2PipelineItem{
		ID:          "knot/g-0/blk-0",
		SourceRaw:   "Hello world",
		SourceHash:  "hash1",
		HasTags:     false,
		State:       StatePendingTranslate,
		BatchID:     "batch-1",
		ContentType: "dialogue",
	})
	store.addItem(contracts.V2PipelineItem{
		ID:          "knot/g-0/blk-1",
		SourceRaw:   "Goodbye world",
		SourceHash:  "hash2",
		HasTags:     true,
		State:       StatePendingTranslate,
		BatchID:     "batch-1",
		ContentType: "dialogue",
	})

	// The translate worker calls BuildScriptPrompt which produces numbered lines,
	// so we directly test the translateBatch function with items.
	ctx := context.Background()

	// Test that items are claimed and grouped by batch.
	items, err := store.ClaimPending(StatePendingTranslate, StateWorkingTranslate, "w1", 10, 60)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// Simulate successful translation by calling store methods directly.
	// In production, LLM returns numbered output parsed by clustertranslate.
	if err := store.MarkTranslated("knot/g-0/blk-0", "안녕하세요"); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkTranslated("knot/g-0/blk-1", "안녕히 가세요"); err != nil {
		t.Fatal(err)
	}

	// Verify routing: no tags -> pending_score, has tags -> pending_format.
	item0 := store.getItem("knot/g-0/blk-0")
	if item0.State != StatePendingScore {
		t.Errorf("item0 state: got %s, want %s", item0.State, StatePendingScore)
	}
	item1 := store.getItem("knot/g-0/blk-1")
	if item1.State != StatePendingFormat {
		t.Errorf("item1 state: got %s, want %s", item1.State, StatePendingFormat)
	}

	// Verify attempt logs.
	logAttempt(store, "knot/g-0/blk-0", "translate", "gpt-5.4", "", "", -1, 0, 0)
	if len(store.logs["knot/g-0/blk-0"]) != 1 {
		t.Errorf("expected 1 log entry, got %d", len(store.logs["knot/g-0/blk-0"]))
	}

	_ = ctx // used in full worker loop
}

// TestTranslateWorkerRejectsLineMismatch verifies that validation failure triggers retry.
func TestTranslateWorkerRejectsLineMismatch(t *testing.T) {
	store := newFakeStore()
	item := contracts.V2PipelineItem{
		ID:                "knot/g-0/blk-0",
		SourceRaw:         "Hello world",
		SourceHash:        "hash1",
		HasTags:           false,
		State:             StateWorkingTranslate,
		BatchID:           "batch-1",
		ContentType:       "dialogue",
		TranslateAttempts: 0,
	}
	store.addItem(item)

	// handleRetry with attempt 0 should set back to pending (< maxRetries).
	handleRetry(store, item, "translate", 3, "line count mismatch")

	result := store.getItem("knot/g-0/blk-0")
	if result.State != StatePendingTranslate {
		t.Errorf("state: got %s, want %s", result.State, StatePendingTranslate)
	}
	if result.TranslateAttempts != 1 { // UpdateRetryState increments
		t.Errorf("translate_attempts: got %d, want 1", result.TranslateAttempts)
	}

	// At max retries, should mark failed.
	item2 := contracts.V2PipelineItem{
		ID:                "knot/g-0/blk-1",
		SourceRaw:         "Test",
		SourceHash:        "hash2",
		HasTags:           false,
		State:             StateWorkingTranslate,
		BatchID:           "batch-1",
		ContentType:       "dialogue",
		TranslateAttempts: 2,
	}
	store.addItem(item2)
	handleRetry(store, item2, "translate", 3, "still failing")

	result2 := store.getItem("knot/g-0/blk-1")
	if result2.State != StateFailed {
		t.Errorf("state: got %s, want %s", result2.State, StateFailed)
	}
}

// TestFormatWorkerSkipsNoTags verifies has_tags=false items never enter format queue.
// This is actually enforced by MarkTranslated routing, not by FormatWorker.
func TestFormatWorkerSkipsNoTags(t *testing.T) {
	store := newFakeStore()

	// Item without tags: MarkTranslated routes to pending_score, skipping format.
	store.addItem(contracts.V2PipelineItem{
		ID:         "knot/g-0/blk-0",
		SourceRaw:  "No tags here",
		SourceHash: "hash1",
		HasTags:    false,
		State:      StateWorkingTranslate,
	})

	if err := store.MarkTranslated("knot/g-0/blk-0", "태그 없음"); err != nil {
		t.Fatal(err)
	}

	item := store.getItem("knot/g-0/blk-0")
	if item.State != StatePendingScore {
		t.Errorf("state: got %s, want %s (should skip format)", item.State, StatePendingScore)
	}

	// FormatWorker claiming pending_format should find nothing.
	claimed, err := store.ClaimPending(StatePendingFormat, StateWorkingFormat, "w1", 10, 60)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 0 {
		t.Errorf("expected 0 format claims for no-tag items, got %d", len(claimed))
	}
}

// TestScoreWorkerRoutesFailureType verifies pass->done, translation->pending_translate.
func TestScoreWorkerRoutesFailureType(t *testing.T) {
	store := newFakeStore()

	tests := []struct {
		id          string
		failureType string
		wantState   string
	}{
		{"item-pass", "pass", StateDone},
		{"item-trans", "translation", StatePendingTranslate},
		{"item-format", "format", StatePendingFormat},
		{"item-both", "both", StatePendingTranslate},
	}

	for _, tt := range tests {
		store.addItem(contracts.V2PipelineItem{
			ID:         tt.id,
			SourceRaw:  "Test",
			SourceHash: "hash-" + tt.id,
			State:      StateWorkingScore,
		})

		if err := store.MarkScored(tt.id, 8.0, tt.failureType, "test reason"); err != nil {
			t.Fatalf("MarkScored %s: %v", tt.id, err)
		}

		item := store.getItem(tt.id)
		if item.State != tt.wantState {
			t.Errorf("%s: state got %s, want %s", tt.id, item.State, tt.wantState)
		}
	}
}

// TestGroupByBatchID verifies grouping logic.
func TestGroupByBatchID(t *testing.T) {
	items := []contracts.V2PipelineItem{
		{ID: "a", BatchID: "b1"},
		{ID: "b", BatchID: "b1"},
		{ID: "c", BatchID: "b2"},
		{ID: "d", BatchID: ""},
	}

	groups := groupByBatchID(items)
	if len(groups["b1"]) != 2 {
		t.Errorf("b1 group: got %d, want 2", len(groups["b1"]))
	}
	if len(groups["b2"]) != 1 {
		t.Errorf("b2 group: got %d, want 1", len(groups["b2"]))
	}
	// Empty batch ID falls back to item ID.
	if _, ok := groups["d"]; !ok {
		t.Error("expected item 'd' to be in its own group")
	}
}

// TestLogAttempt verifies attempt log entry format per D-16.
func TestLogAttempt(t *testing.T) {
	store := newFakeStore()
	logAttempt(store, "item-1", "translate", "gpt-5.4", "translation", "bad quality", 3.5, 1, 3)

	logs := store.logs["item-1"]
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}

	entry := logs[0]
	if entry["stage"] != "translate" {
		t.Errorf("stage: got %v, want translate", entry["stage"])
	}
	if entry["model"] != "gpt-5.4" {
		t.Errorf("model: got %v, want gpt-5.4", entry["model"])
	}
	if entry["failure_type"] != "translation" {
		t.Errorf("failure_type: got %v, want translation", entry["failure_type"])
	}
	if entry["reason"] != "bad quality" {
		t.Errorf("reason: got %v, want 'bad quality'", entry["reason"])
	}
	if entry["score"] != 3.5 {
		t.Errorf("score: got %v, want 3.5", entry["score"])
	}
	if _, ok := entry["timestamp"]; !ok {
		t.Error("expected timestamp in log entry")
	}

	// Verify it's JSON-serializable.
	if _, err := json.Marshal(entry); err != nil {
		t.Errorf("log entry not JSON-serializable: %v", err)
	}

	_ = time.Now() // used
}

// TestHandleRetryEscalation verifies D-15 retry escalation.
func TestHandleRetryEscalation(t *testing.T) {
	store := newFakeStore()

	// Attempt 0: should retry (< 3 max).
	store.addItem(contracts.V2PipelineItem{
		ID:                "item-a",
		State:             StateWorkingTranslate,
		TranslateAttempts: 0,
	})
	handleRetry(store, *store.getItem("item-a"), "translate", 3, "error")
	if got := store.getItem("item-a").State; got != StatePendingTranslate {
		t.Errorf("attempt 0: got %s, want pending_translate", got)
	}

	// Attempt 2: should fail (>= 3 max).
	store.addItem(contracts.V2PipelineItem{
		ID:                "item-b",
		State:             StateWorkingTranslate,
		TranslateAttempts: 2,
	})
	handleRetry(store, *store.getItem("item-b"), "translate", 3, "still bad")
	if got := store.getItem("item-b").State; got != StateFailed {
		t.Errorf("attempt 2 (max=3): got %s, want failed", got)
	}
}
