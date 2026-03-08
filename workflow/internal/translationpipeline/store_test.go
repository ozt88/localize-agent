package translationpipeline

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestStoreSeedUsesCheckpointDoneRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO items(id, status) VALUES ('done-id', 'done'), ('new-id', 'pending')`); err != nil {
		t.Fatal(err)
	}

	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Seed([]string{"done-id", "new-id"}, 0); err != nil {
		t.Fatal(err)
	}

	pendingScore, err := store.ListByState(StatePendingScore, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pendingScore) != 1 || pendingScore[0].ID != "done-id" {
		t.Fatalf("pending_score mismatch: %#v", pendingScore)
	}

	pendingTranslate, err := store.ListByState(StatePendingTranslate, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pendingTranslate) != 1 || pendingTranslate[0].ID != "new-id" {
		t.Fatalf("pending_translate mismatch: %#v", pendingTranslate)
	}
}

func TestStoreApplyScoresQueuesRetranslate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at) VALUES ('x', ?, 0, -1, '', ?, '2026-03-08T00:00:00Z', '2026-03-08T00:10:00Z', '2026-03-08T00:00:00Z')`, StateWorkingScore, "score-worker"); err != nil {
		t.Fatal(err)
	}

	if err := store.ApplyScores([]PipelineItem{{ID: "x", RetryCount: 0}}, map[string]float64{"x": 0.9}, 0.7, 3, "score-worker"); err != nil {
		t.Fatal(err)
	}

	rows, err := store.ListByState(StatePendingRetranslate, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "x" {
		t.Fatalf("pending_retranslate mismatch: %#v", rows)
	}
}

func TestResolveAfterTranslateLowNoDoneRowQueuesRetranslate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at) VALUES
		('x', ?, 0, -1, '', 'translate-worker', '2026-03-08T00:00:00Z', '2026-03-08T00:10:00Z', '2026-03-08T00:00:00Z')`, StateWorkingTranslate); err != nil {
		t.Fatal(err)
	}

	if err := store.ResolveAfterTranslate([]string{"x"}, map[string]string{"x": "Hello."}, false, "translate-worker"); err != nil {
		t.Fatal(err)
	}
	rows, err := store.ListByState(StatePendingRetranslate, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "x" {
		t.Fatalf("expected x in pending_retranslate, got %#v", rows)
	}
}

func TestResolveAfterTranslateRetryNoDoneRowFails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at) VALUES
		('x', ?, 1, 0.9, '', 'retranslate-worker', '2026-03-08T00:00:00Z', '2026-03-08T00:10:00Z', '2026-03-08T00:00:00Z')`, StateWorkingRetranslate); err != nil {
		t.Fatal(err)
	}

	if err := store.ResolveAfterTranslate([]string{"x"}, map[string]string{"x": "Hello."}, true, "retranslate-worker"); err != nil {
		t.Fatal(err)
	}
	rows, err := store.ListByState(StateFailed, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "x" {
		t.Fatalf("expected x in failed, got %#v", rows)
	}
}

func TestClaimPendingClaimsRowsAndSetsLease(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, updated_at) VALUES 
		('a', ?, 0, -1, '', '2026-03-08T00:00:00Z'),
		('b', ?, 0, -1, '', '2026-03-08T00:00:00Z')`, StatePendingTranslate, StatePendingTranslate); err != nil {
		t.Fatal(err)
	}

	rows, err := store.ClaimPending(StatePendingTranslate, StateWorkingTranslate, "translate-worker", 2, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 claimed rows, got %d", len(rows))
	}
	working, err := store.ListByState(StateWorkingTranslate, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(working) != 2 {
		t.Fatalf("expected 2 working rows, got %d", len(working))
	}
	for _, row := range working {
		if row.ClaimedBy != "translate-worker" {
			t.Fatalf("unexpected claimed_by: %#v", row)
		}
		if row.LeaseUntil == "" {
			t.Fatalf("expected lease_until to be set: %#v", row)
		}
	}
}

func TestClaimPendingCanReclaimExpiredLease(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at) VALUES
		('x', ?, 0, -1, '', 'old-worker', '2026-03-08T00:00:00Z', '2000-01-01T00:00:00Z', '2026-03-08T00:00:00Z')`, StatePendingScore); err != nil {
		t.Fatal(err)
	}
	rows, err := store.ClaimPending(StatePendingScore, StateWorkingScore, "new-worker", 1, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "x" || rows[0].ClaimedBy != "new-worker" {
		t.Fatalf("unexpected reclaimed rows: %#v", rows)
	}
}

func TestStoreResetClearsPipelineItems(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, updated_at) VALUES ('x', ?, 0, -1, '', '2026-03-08T00:00:00Z')`, StatePendingTranslate); err != nil {
		t.Fatal(err)
	}
	if err := store.Reset(); err != nil {
		t.Fatal(err)
	}
	counts, err := store.CountStates()
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) != 0 {
		t.Fatalf("expected empty counts after reset, got %#v", counts)
	}
}

func TestRequeueFailedNoDoneRowOnlyRequeuesMatchingFailures(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, updated_at) VALUES
		('requeue-me', ?, 0, -1, 'translator produced no done row', '2026-03-08T00:00:00Z'),
		('keep-failed', ?, 3, 0.91, 'score 0.910 >= threshold 0.700 after max retries', '2026-03-08T00:00:00Z')`,
		StateFailed, StateFailed); err != nil {
		t.Fatal(err)
	}

	n, err := store.RequeueFailedNoDoneRow(0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 requeued row, got %d", n)
	}

	var state string
	var lastError string
	if err := store.db.QueryRow(`SELECT state, last_error FROM pipeline_items WHERE id = 'requeue-me'`).Scan(&state, &lastError); err != nil {
		t.Fatal(err)
	}
	if state != StatePendingTranslate || lastError != "" {
		t.Fatalf("requeue-me not reset correctly: state=%q last_error=%q", state, lastError)
	}
	if err := store.db.QueryRow(`SELECT state, last_error FROM pipeline_items WHERE id = 'keep-failed'`).Scan(&state, &lastError); err != nil {
		t.Fatal(err)
	}
	if state != StateFailed || lastError == "" {
		t.Fatalf("keep-failed should remain failed: state=%q last_error=%q", state, lastError)
	}
}

func TestOpenMigratesExistingPipelineTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE pipeline_items (
			id TEXT PRIMARY KEY,
			state TEXT NOT NULL,
			retry_count INTEGER NOT NULL DEFAULT 0,
			score_final REAL NOT NULL DEFAULT -1,
			last_error TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)
	`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	rows, err := store.db.Query(`PRAGMA table_info(pipeline_items)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		cols[name] = true
	}
	for _, name := range []string{"claimed_by", "claimed_at", "lease_until"} {
		if !cols[name] {
			t.Fatalf("expected migrated column %q", name)
		}
	}
}

func TestRecordAndListRecentWorkerBatchStats(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	started := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	finished := started.Add(2 * time.Second)
	if err := store.RecordWorkerBatchStat("translate-1", "translate", 100, started, finished); err != nil {
		t.Fatal(err)
	}
	stats, err := store.ListRecentWorkerBatchStats(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 stat row, got %d", len(stats))
	}
	if stats[0].WorkerID != "translate-1" || stats[0].Role != "translate" || stats[0].ProcessedCount != 100 {
		t.Fatalf("unexpected stat row: %#v", stats[0])
	}
	if stats[0].ElapsedMs != 2000 {
		t.Fatalf("expected elapsed_ms=2000, got %d", stats[0].ElapsedMs)
	}
}
