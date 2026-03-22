package v2pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"localize-agent/workflow/internal/contracts"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestSeedInsertsAndDeduplicates(t *testing.T) {
	store := testStore(t)

	items := []contracts.V2PipelineItem{
		{ID: "knot1/g-0/blk-0", SortIndex: 0, SourceRaw: "Hello", SourceHash: "hash1", State: StatePendingTranslate, ContentType: "dialogue"},
		{ID: "knot1/g-0/blk-1", SortIndex: 1, SourceRaw: "World", SourceHash: "hash2", State: StatePendingTranslate, ContentType: "dialogue"},
		{ID: "knot1/g-0/blk-2", SortIndex: 2, SourceRaw: "...", SourceHash: "hash3", State: StateDone, KOFormatted: "...", ContentType: "dialogue"},
	}

	// First seed
	inserted, skipped, err := store.Seed(items)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if inserted != 3 {
		t.Errorf("expected 3 inserted, got %d", inserted)
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}

	// Second seed with same source_hash values -> all skipped
	items2 := []contracts.V2PipelineItem{
		{ID: "knot1/g-0/blk-0-dup", SortIndex: 10, SourceRaw: "Hello", SourceHash: "hash1", State: StatePendingTranslate},
		{ID: "knot1/g-0/blk-1-dup", SortIndex: 11, SourceRaw: "World", SourceHash: "hash2", State: StatePendingTranslate},
		{ID: "knot1/g-0/blk-3", SortIndex: 12, SourceRaw: "New", SourceHash: "hash4", State: StatePendingTranslate},
	}

	inserted, skipped, err = store.Seed(items2)
	if err != nil {
		t.Fatalf("seed2: %v", err)
	}
	if inserted != 1 {
		t.Errorf("expected 1 inserted, got %d", inserted)
	}
	if skipped != 2 {
		t.Errorf("expected 2 skipped, got %d", skipped)
	}

	// Verify passthrough item was stored as done
	item, err := store.GetItem("knot1/g-0/blk-2")
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if item == nil {
		t.Fatal("expected item, got nil")
	}
	if item.State != StateDone {
		t.Errorf("expected state %q, got %q", StateDone, item.State)
	}
	if item.KOFormatted != "..." {
		t.Errorf("expected ko_formatted '...', got %q", item.KOFormatted)
	}
}

func TestClaimPendingAndRelease(t *testing.T) {
	store := testStore(t)

	items := []contracts.V2PipelineItem{
		{ID: "blk-0", SortIndex: 0, SourceRaw: "A", SourceHash: "ha", State: StatePendingTranslate},
		{ID: "blk-1", SortIndex: 1, SourceRaw: "B", SourceHash: "hb", State: StatePendingTranslate},
		{ID: "blk-2", SortIndex: 2, SourceRaw: "C", SourceHash: "hc", State: StatePendingTranslate},
	}
	if _, _, err := store.Seed(items); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Claim 2
	claimed, err := store.ClaimPending(StatePendingTranslate, StateWorkingTranslate, "worker-1", 2, 300)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(claimed) != 2 {
		t.Fatalf("expected 2 claimed, got %d", len(claimed))
	}
	if claimed[0].ID != "blk-0" || claimed[1].ID != "blk-1" {
		t.Errorf("expected blk-0,blk-1 but got %s,%s", claimed[0].ID, claimed[1].ID)
	}
	if claimed[0].State != StateWorkingTranslate {
		t.Errorf("expected state %q, got %q", StateWorkingTranslate, claimed[0].State)
	}

	// Claim remaining
	claimed2, err := store.ClaimPending(StatePendingTranslate, StateWorkingTranslate, "worker-2", 5, 300)
	if err != nil {
		t.Fatalf("claim2: %v", err)
	}
	if len(claimed2) != 1 {
		t.Errorf("expected 1 claimed, got %d", len(claimed2))
	}

	// Release first item back
	if err := store.MarkState("blk-0", StatePendingTranslate); err != nil {
		t.Fatalf("mark state: %v", err)
	}
	item, err := store.GetItem("blk-0")
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if item.State != StatePendingTranslate {
		t.Errorf("expected state %q, got %q", StatePendingTranslate, item.State)
	}
	if item.ClaimedBy != "" {
		t.Errorf("expected empty claimed_by, got %q", item.ClaimedBy)
	}
}

func TestMarkTranslatedRoutesCorrectly(t *testing.T) {
	store := testStore(t)

	items := []contracts.V2PipelineItem{
		{ID: "with-tags", SortIndex: 0, SourceRaw: "<b>bold</b>", SourceHash: "ht1", State: StateWorkingTranslate, HasTags: true},
		{ID: "no-tags", SortIndex: 1, SourceRaw: "plain text", SourceHash: "ht2", State: StateWorkingTranslate, HasTags: false},
	}
	if _, _, err := store.Seed(items); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Mark with-tags as translated -> should go to pending_format
	if err := store.MarkTranslated("with-tags", "굵은 글씨"); err != nil {
		t.Fatalf("mark translated: %v", err)
	}
	item, _ := store.GetItem("with-tags")
	if item.State != StatePendingFormat {
		t.Errorf("with-tags: expected state %q, got %q", StatePendingFormat, item.State)
	}
	if item.KORaw != "굵은 글씨" {
		t.Errorf("with-tags: expected ko_raw, got %q", item.KORaw)
	}
	if item.TranslateAttempts != 1 {
		t.Errorf("with-tags: expected translate_attempts=1, got %d", item.TranslateAttempts)
	}

	// Mark no-tags as translated -> should go to pending_score
	if err := store.MarkTranslated("no-tags", "일반 텍스트"); err != nil {
		t.Fatalf("mark translated: %v", err)
	}
	item, _ = store.GetItem("no-tags")
	if item.State != StatePendingScore {
		t.Errorf("no-tags: expected state %q, got %q", StatePendingScore, item.State)
	}
}

func TestMarkScoredRoutesFailureType(t *testing.T) {
	store := testStore(t)

	cases := []struct {
		id          string
		failureType string
		wantState   string
	}{
		{"item-pass", "pass", StateDone},
		{"item-trans", "translation", StatePendingTranslate},
		{"item-fmt", "format", StatePendingFormat},
		{"item-both", "both", StatePendingTranslate},
	}

	items := make([]contracts.V2PipelineItem, len(cases))
	for i, c := range cases {
		items[i] = contracts.V2PipelineItem{
			ID: c.id, SortIndex: i, SourceRaw: "text " + c.id,
			SourceHash: "sh" + c.id, State: StateWorkingScore,
		}
	}
	if _, _, err := store.Seed(items); err != nil {
		t.Fatalf("seed: %v", err)
	}

	for _, c := range cases {
		if err := store.MarkScored(c.id, 0.85, c.failureType, "test reason"); err != nil {
			t.Fatalf("mark scored %s: %v", c.id, err)
		}
		item, _ := store.GetItem(c.id)
		if item.State != c.wantState {
			t.Errorf("%s (failure_type=%s): expected state %q, got %q", c.id, c.failureType, c.wantState, item.State)
		}
		if item.ScoreAttempts != 1 {
			t.Errorf("%s: expected score_attempts=1, got %d", c.id, item.ScoreAttempts)
		}
		if item.ScoreFinal != 0.85 {
			t.Errorf("%s: expected score_final=0.85, got %f", c.id, item.ScoreFinal)
		}
	}
}

func TestCountByState(t *testing.T) {
	store := testStore(t)

	items := []contracts.V2PipelineItem{
		{ID: "a", SortIndex: 0, SourceRaw: "a", SourceHash: "sa", State: StatePendingTranslate},
		{ID: "b", SortIndex: 1, SourceRaw: "b", SourceHash: "sb", State: StatePendingTranslate},
		{ID: "c", SortIndex: 2, SourceRaw: "c", SourceHash: "sc", State: StateDone, KOFormatted: "c_ko"},
		{ID: "d", SortIndex: 3, SourceRaw: "d", SourceHash: "sd", State: StateFailed},
	}
	if _, _, err := store.Seed(items); err != nil {
		t.Fatalf("seed: %v", err)
	}

	counts, err := store.CountByState()
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if counts[StatePendingTranslate] != 2 {
		t.Errorf("expected 2 pending_translate, got %d", counts[StatePendingTranslate])
	}
	if counts[StateDone] != 1 {
		t.Errorf("expected 1 done, got %d", counts[StateDone])
	}
	if counts[StateFailed] != 1 {
		t.Errorf("expected 1 failed, got %d", counts[StateFailed])
	}

	// Mark one translated, verify counts change
	if err := store.MarkTranslated("a", "번역됨"); err != nil {
		t.Fatalf("mark translated: %v", err)
	}
	counts, _ = store.CountByState()
	if counts[StatePendingTranslate] != 1 {
		t.Errorf("after translate: expected 1 pending_translate, got %d", counts[StatePendingTranslate])
	}
	if counts[StatePendingScore] != 1 {
		t.Errorf("after translate: expected 1 pending_score, got %d", counts[StatePendingScore])
	}
}

func TestAppendAttemptLog(t *testing.T) {
	store := testStore(t)

	items := []contracts.V2PipelineItem{
		{ID: "log-test", SortIndex: 0, SourceRaw: "test", SourceHash: "slog", State: StatePendingTranslate},
	}
	if _, _, err := store.Seed(items); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Append first entry
	entry1 := map[string]interface{}{"attempt": 1, "stage": "translate", "model": "gpt-5.4"}
	if err := store.AppendAttemptLog("log-test", entry1); err != nil {
		t.Fatalf("append log 1: %v", err)
	}

	// Append second entry
	entry2 := map[string]interface{}{"attempt": 2, "stage": "format", "model": "codex-spark"}
	if err := store.AppendAttemptLog("log-test", entry2); err != nil {
		t.Fatalf("append log 2: %v", err)
	}

	item, _ := store.GetItem("log-test")
	if item.AttemptLog == "" {
		t.Fatal("expected non-empty attempt_log")
	}

	// Verify it's valid JSON array
	if item.AttemptLog[0] != '[' {
		t.Errorf("expected JSON array, got: %s", item.AttemptLog)
	}
}

func TestUpdateRetryState(t *testing.T) {
	store := testStore(t)

	items := []contracts.V2PipelineItem{
		{ID: "retry-test", SortIndex: 0, SourceRaw: "test", SourceHash: "sretry", State: StateWorkingTranslate, ClaimedBy: "worker-1"},
	}
	if _, _, err := store.Seed(items); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := store.UpdateRetryState("retry-test", StatePendingTranslate, "translate_attempts"); err != nil {
		t.Fatalf("update retry state: %v", err)
	}

	item, _ := store.GetItem("retry-test")
	if item.State != StatePendingTranslate {
		t.Errorf("expected state %q, got %q", StatePendingTranslate, item.State)
	}
	if item.TranslateAttempts != 1 {
		t.Errorf("expected translate_attempts=1, got %d", item.TranslateAttempts)
	}
	if item.ClaimedBy != "" {
		t.Errorf("expected empty claimed_by, got %q", item.ClaimedBy)
	}

	// Invalid field should error
	if err := store.UpdateRetryState("retry-test", StatePendingTranslate, "invalid_field"); err == nil {
		t.Error("expected error for invalid field")
	}
}

func TestMarkFailed(t *testing.T) {
	store := testStore(t)

	items := []contracts.V2PipelineItem{
		{ID: "fail-test", SortIndex: 0, SourceRaw: "test", SourceHash: "sfail", State: StateWorkingTranslate},
	}
	if _, _, err := store.Seed(items); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := store.MarkFailed("fail-test", "max retries exceeded"); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	item, _ := store.GetItem("fail-test")
	if item.State != StateFailed {
		t.Errorf("expected state %q, got %q", StateFailed, item.State)
	}
	if item.LastError != "max retries exceeded" {
		t.Errorf("expected last_error, got %q", item.LastError)
	}
}

// Ensure temp dir is writable (Windows-specific guard).
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
