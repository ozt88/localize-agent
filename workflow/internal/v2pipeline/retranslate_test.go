package v2pipeline

import (
	"testing"

	"localize-agent/workflow/internal/contracts"
)

// seedTestItems seeds a set of test items into the store and returns the store.
func seedRetranslateTestItems(t *testing.T) *Store {
	t.Helper()
	store := testStore(t)

	items := []contracts.V2PipelineItem{
		{ID: "knot1/g-0/blk-0", SortIndex: 0, SourceRaw: "Hello", SourceHash: "rhash1", State: StateDone, ContentType: "dialogue", Speaker: "Braxo", BatchID: "batch-1", KORaw: "안녕", KOFormatted: "안녕하세요", ScoreFinal: 5.0},
		{ID: "knot1/g-0/blk-1", SortIndex: 1, SourceRaw: "World", SourceHash: "rhash2", State: StateDone, ContentType: "dialogue", Speaker: "Braxo", BatchID: "batch-1", KORaw: "세계", KOFormatted: "세계여", ScoreFinal: 7.0},
		{ID: "knot2/g-0/blk-0", SortIndex: 2, SourceRaw: "Fire!", SourceHash: "rhash3", State: StateDone, ContentType: "spell", Speaker: "Mage", BatchID: "batch-2", KORaw: "불", KOFormatted: "불이야!", ScoreFinal: 9.0},
	}

	inserted, _, err := store.Seed(items)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if inserted != 3 {
		t.Fatalf("expected 3 inserted, got %d", inserted)
	}
	return store
}

func TestSchemaRetranslationSupport(t *testing.T) {
	store := testStore(t)

	// Verify retranslation_snapshots table exists by inserting a row
	_, err := store.db.Exec(`INSERT INTO retranslation_snapshots (id, gen, ko_raw, ko_formatted, score_final, snapshot_at) VALUES ('test-id', 1, 'ko', 'ko-fmt', 8.0, '2026-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("retranslation_snapshots table should exist: %v", err)
	}

	// Verify retranslation_gen column exists in pipeline_items_v2
	var gen int
	err = store.db.QueryRow(`SELECT retranslation_gen FROM pipeline_items_v2 LIMIT 1`).Scan(&gen)
	// sql.ErrNoRows is OK (no rows), but column-not-found is not
	if err != nil && err.Error() != "sql: no rows in result set" {
		t.Fatalf("retranslation_gen column should exist: %v", err)
	}
}

func TestScoreHistogram(t *testing.T) {
	store := seedRetranslateTestItems(t)

	buckets, err := store.ScoreHistogram(5.0)
	if err != nil {
		t.Fatalf("ScoreHistogram: %v", err)
	}

	// Items: 5.0, 7.0, 9.0 with bucket width 5.0
	// Bucket [0,5) -> 0 items (5.0 is in [5,10) bucket)
	// Bucket [5,10) -> 3 items (5.0, 7.0, 9.0)
	// Actually: CAST(5.0/5.0 AS INTEGER)*5.0 = 1*5 = 5.0, CAST(7.0/5.0 AS INTEGER)*5.0 = 1*5 = 5.0, CAST(9.0/5.0 AS INTEGER)*5.0 = 1*5 = 5.0
	// All three should be in the [5.0, 10.0) bucket
	if len(buckets) == 0 {
		t.Fatal("expected at least one bucket")
	}

	// Find the bucket containing our items
	found := false
	for _, b := range buckets {
		if b.LowerBound == 5.0 && b.Count == 3 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected bucket [5.0, 10.0) with 3 items, got: %+v", buckets)
	}
}

func TestSelectRetranslationBatches(t *testing.T) {
	store := seedRetranslateTestItems(t)

	// threshold=8.0 should select batch-1 (both items 5.0, 7.0 < 8.0)
	candidates, err := store.SelectRetranslationBatches(8.0, "")
	if err != nil {
		t.Fatalf("SelectRetranslationBatches: %v", err)
	}

	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate batch, got %d", len(candidates))
	}
	if candidates[0].BatchID != "batch-1" {
		t.Errorf("expected batch-1, got %s", candidates[0].BatchID)
	}
	if candidates[0].ItemCount != 2 {
		t.Errorf("expected 2 items in batch, got %d", candidates[0].ItemCount)
	}
	if candidates[0].MinScore != 5.0 {
		t.Errorf("expected min score 5.0, got %f", candidates[0].MinScore)
	}
}

func TestSelectRetranslationBatchesContentTypeFilter(t *testing.T) {
	store := seedRetranslateTestItems(t)

	// Filter by content_type=spell, threshold=10.0 -> batch-2
	candidates, err := store.SelectRetranslationBatches(10.0, "spell")
	if err != nil {
		t.Fatalf("SelectRetranslationBatches: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].BatchID != "batch-2" {
		t.Errorf("expected batch-2, got %s", candidates[0].BatchID)
	}
}

func TestResetForRetranslation(t *testing.T) {
	store := seedRetranslateTestItems(t)

	count, err := store.ResetForRetranslation("batch-1", 1)
	if err != nil {
		t.Fatalf("ResetForRetranslation: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 items reset, got %d", count)
	}

	// Verify state is pending_translate
	item, err := store.GetItem("knot1/g-0/blk-0")
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if item.State != StatePendingTranslate {
		t.Errorf("expected state %s, got %s", StatePendingTranslate, item.State)
	}
	if item.RetranslationGen != 1 {
		t.Errorf("expected retranslation_gen=1, got %d", item.RetranslationGen)
	}
	// ko_raw and ko_formatted should be cleared
	if item.KORaw != "" {
		t.Errorf("expected empty ko_raw, got %q", item.KORaw)
	}
	if item.KOFormatted != "" {
		t.Errorf("expected empty ko_formatted, got %q", item.KOFormatted)
	}
	if item.ScoreFinal != -1 {
		t.Errorf("expected score_final=-1, got %f", item.ScoreFinal)
	}
}

func TestResetForRetranslationPreservesSnapshot(t *testing.T) {
	store := seedRetranslateTestItems(t)

	_, err := store.ResetForRetranslation("batch-1", 1)
	if err != nil {
		t.Fatalf("ResetForRetranslation: %v", err)
	}

	// Check snapshot was preserved
	var koRaw, koFormatted string
	var scoreFinal float64
	err = store.db.QueryRow(`SELECT ko_raw, ko_formatted, score_final FROM retranslation_snapshots WHERE id = 'knot1/g-0/blk-0' AND gen = 1`).Scan(&koRaw, &koFormatted, &scoreFinal)
	if err != nil {
		t.Fatalf("query snapshot: %v", err)
	}
	if koRaw != "안녕" {
		t.Errorf("expected snapshot ko_raw='안녕', got %q", koRaw)
	}
	if koFormatted != "안녕하세요" {
		t.Errorf("expected snapshot ko_formatted='안녕하세요', got %q", koFormatted)
	}
	if scoreFinal != 5.0 {
		t.Errorf("expected snapshot score_final=5.0, got %f", scoreFinal)
	}
}

func TestResetForRetranslationMultipleGenerations(t *testing.T) {
	store := seedRetranslateTestItems(t)

	// First reset: gen=1
	_, err := store.ResetForRetranslation("batch-1", 1)
	if err != nil {
		t.Fatalf("ResetForRetranslation gen=1: %v", err)
	}

	// Simulate re-translation by manually updating ko_raw/ko_formatted/score/state
	_, err = store.db.Exec(`UPDATE pipeline_items_v2 SET ko_raw='새번역', ko_formatted='새번역입니다', score_final=6.5, state='done' WHERE batch_id='batch-1'`)
	if err != nil {
		t.Fatalf("manual update: %v", err)
	}

	// Second reset: gen=2
	count, err := store.ResetForRetranslation("batch-1", 2)
	if err != nil {
		t.Fatalf("ResetForRetranslation gen=2: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 items reset, got %d", count)
	}

	// Both gen=1 and gen=2 snapshots should exist
	var snapCount int
	err = store.db.QueryRow(`SELECT COUNT(*) FROM retranslation_snapshots WHERE id = 'knot1/g-0/blk-0'`).Scan(&snapCount)
	if err != nil {
		t.Fatalf("count snapshots: %v", err)
	}
	if snapCount != 2 {
		t.Errorf("expected 2 snapshots, got %d", snapCount)
	}

	// gen=2 snapshot should have the re-translated values
	var koRaw string
	err = store.db.QueryRow(`SELECT ko_raw FROM retranslation_snapshots WHERE id = 'knot1/g-0/blk-0' AND gen = 2`).Scan(&koRaw)
	if err != nil {
		t.Fatalf("query gen=2 snapshot: %v", err)
	}
	if koRaw != "새번역" {
		t.Errorf("expected gen=2 ko_raw='새번역', got %q", koRaw)
	}

	// Item should have gen=2
	item, err := store.GetItem("knot1/g-0/blk-0")
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if item.RetranslationGen != 2 {
		t.Errorf("expected retranslation_gen=2, got %d", item.RetranslationGen)
	}
}

// --- Tests for RunRetranslateSelect domain logic ---

func TestRunRetranslateSelectHistogram(t *testing.T) {
	store := seedRetranslateTestItems(t)

	cfg := RetranslateSelectConfig{
		Histogram: true,
	}

	rc := runRetranslateSelectWithStore(cfg, store)
	if rc != 0 {
		t.Errorf("expected return code 0, got %d", rc)
	}
}

func TestRunRetranslateSelectDryRun(t *testing.T) {
	store := seedRetranslateTestItems(t)

	cfg := RetranslateSelectConfig{
		ScoreThreshold: 7.0,
		DryRun:         true,
	}

	rc := runRetranslateSelectWithStore(cfg, store)
	if rc != 0 {
		t.Errorf("expected return code 0, got %d", rc)
	}

	// Verify no state change: items should still be done
	item, err := store.GetItem("knot1/g-0/blk-0")
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if item.State != StateDone {
		t.Errorf("dry-run should not change state, got %s", item.State)
	}
	if item.KORaw != "안녕" {
		t.Errorf("dry-run should not clear ko_raw, got %q", item.KORaw)
	}
}

func TestRunRetranslateSelectExecute(t *testing.T) {
	store := seedRetranslateTestItems(t)

	cfg := RetranslateSelectConfig{
		ScoreThreshold: 7.0,
		DryRun:         false,
	}

	rc := runRetranslateSelectWithStore(cfg, store)
	if rc != 0 {
		t.Errorf("expected return code 0, got %d", rc)
	}

	// Verify state changed: batch-1 items should be pending_translate
	item, err := store.GetItem("knot1/g-0/blk-0")
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if item.State != StatePendingTranslate {
		t.Errorf("expected state %s, got %s", StatePendingTranslate, item.State)
	}
	if item.RetranslationGen != 1 {
		t.Errorf("expected retranslation_gen=1, got %d", item.RetranslationGen)
	}

	// batch-2 should be untouched (score 9.0 >= 7.0)
	item2, err := store.GetItem("knot2/g-0/blk-0")
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if item2.State != StateDone {
		t.Errorf("batch-2 should be untouched, got state %s", item2.State)
	}
}

func TestRunRetranslateSelectInvalidThreshold(t *testing.T) {
	cfg := RetranslateSelectConfig{
		ScoreThreshold: 0,
		Histogram:      false,
	}
	rc := RunRetranslateSelect(cfg)
	if rc != 2 {
		t.Errorf("expected return code 2 for invalid config, got %d", rc)
	}
}
