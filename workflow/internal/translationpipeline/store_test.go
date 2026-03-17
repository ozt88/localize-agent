package translationpipeline

import (
	"database/sql"
	"encoding/json"
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
	if _, err := db.Exec(`ALTER TABLE items ADD COLUMN pack_json TEXT`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO items(id, status, pack_json) VALUES ('done-id', 'done', '{}'), ('new-id', 'pending', '{}')`); err != nil {
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

func TestStoreSeedBlocksScoreUntilNextDone(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT NOT NULL, pack_json TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO items(id, status, pack_json) VALUES 
		('a', 'done', '{"next_line_id":"b"}'),
		('b', 'new', '{"prev_line_id":"a"}')`); err != nil {
		t.Fatal(err)
	}

	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Seed([]string{"a", "b"}, 0); err != nil {
		t.Fatal(err)
	}

	blocked, err := store.ListByState(StateBlockedScore, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked) != 1 || blocked[0].ID != "a" {
		t.Fatalf("blocked_score mismatch: %#v", blocked)
	}
}

func TestStoreApplyScoresKeepsCurrentWhenDeltaSmall(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT NOT NULL, ko_json TEXT, pack_json TEXT, updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	koJSON, _ := json.Marshal(map[string]any{"Text": "fresh ko"})
	packJSON, _ := json.Marshal(map[string]any{"current_ko": "current ko", "fresh_ko": "fresh ko", "proposed_ko_restored": "fresh ko"})
	if _, err := store.db.Exec(`INSERT INTO items(id, status, ko_json, pack_json, updated_at) VALUES ('x', 'done', ?, ?, '2026-03-08T00:00:00Z')`, string(koJSON), string(packJSON)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at) VALUES ('x', ?, 0, -1, '', ?, '2026-03-08T00:00:00Z', '2026-03-08T00:10:00Z', '2026-03-08T00:00:00Z')`, StateWorkingScore, "score-worker"); err != nil {
		t.Fatal(err)
	}

	if err := store.ApplyScores([]PipelineItem{{ID: "x", RetryCount: 0}}, map[string]ScoreResult{
		"x": {CurrentScore: 71, FreshScore: 73, ScoreFinal: 73},
	}, 0.7, 3, "score-worker"); err != nil {
		t.Fatal(err)
	}

	rows, err := store.ListByState(StateDone, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "x" {
		t.Fatalf("done mismatch: %#v", rows)
	}
	var persisted string
	if err := store.db.QueryRow(`SELECT json_extract(ko_json, '$.Text') FROM items WHERE id='x'`).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != "current ko" {
		t.Fatalf("expected current winner to be persisted, got %q", persisted)
	}
	var decision string
	if err := store.db.QueryRow(`SELECT json_extract(pack_json, '$.score_decision') FROM items WHERE id='x'`).Scan(&decision); err != nil {
		t.Fatal(err)
	}
	if decision != "current" {
		t.Fatalf("expected current decision, got %q", decision)
	}
}

func TestStoreApplyScoresAcceptsFreshWinner(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT NOT NULL, ko_json TEXT, pack_json TEXT, updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	koJSON, _ := json.Marshal(map[string]any{"Text": "fresh ko"})
	packJSON, _ := json.Marshal(map[string]any{"current_ko": "current ko", "fresh_ko": "fresh ko", "proposed_ko_restored": "fresh ko"})
	if _, err := store.db.Exec(`INSERT INTO items(id, status, ko_json, pack_json, updated_at) VALUES ('x', 'done', ?, ?, '2026-03-08T00:00:00Z')`, string(koJSON), string(packJSON)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at) VALUES ('x', ?, 0, -1, '', ?, '2026-03-08T00:00:00Z', '2026-03-08T00:10:00Z', '2026-03-08T00:00:00Z')`, StateWorkingScore, "score-worker"); err != nil {
		t.Fatal(err)
	}

	if err := store.ApplyScores([]PipelineItem{{ID: "x", RetryCount: 0}}, map[string]ScoreResult{
		"x": {CurrentScore: 60, FreshScore: 85, ScoreFinal: 85},
	}, 0.7, 3, "score-worker"); err != nil {
		t.Fatal(err)
	}

	rows, err := store.ListByState(StateDone, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "x" {
		t.Fatalf("done mismatch: %#v", rows)
	}
	var persisted string
	if err := store.db.QueryRow(`SELECT json_extract(ko_json, '$.Text') FROM items WHERE id='x'`).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != "fresh ko" {
		t.Fatalf("expected fresh winner to be persisted, got %q", persisted)
	}
}

func TestStoreApplyScoresClearsStaleRetryReasonOnSuccess(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT NOT NULL, ko_json TEXT, pack_json TEXT, updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	koJSON, _ := json.Marshal(map[string]any{"Text": "current ko"})
	packJSON, _ := json.Marshal(map[string]any{
		"current_ko":   "current ko",
		"fresh_ko":     "fresh ko",
		"retry_reason": "old scoring_error contamination",
	})
	if _, err := store.db.Exec(`INSERT INTO items(id, status, ko_json, pack_json, updated_at) VALUES ('x', 'done', ?, ?, '2026-03-08T00:00:00Z')`, string(koJSON), string(packJSON)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at) VALUES ('x', ?, 1, -1, '', ?, '2026-03-08T00:00:00Z', '2026-03-08T00:10:00Z', '2026-03-08T00:00:00Z')`, StateWorkingScore, "score-worker"); err != nil {
		t.Fatal(err)
	}

	if err := store.ApplyScores([]PipelineItem{{ID: "x", RetryCount: 1}}, map[string]ScoreResult{
		"x": {
			CurrentScore: 95,
			FreshScore:   93,
			ScoreFinal:   95,
		},
	}, 0.7, 3, "score-worker"); err != nil {
		t.Fatalf("ApplyScores error: %v", err)
	}

	var persistedPackJSON string
	if err := store.db.QueryRow(`SELECT pack_json FROM items WHERE id='x'`).Scan(&persistedPackJSON); err != nil {
		t.Fatalf("select pack_json: %v", err)
	}
	var packObj map[string]any
	if err := json.Unmarshal([]byte(persistedPackJSON), &packObj); err != nil {
		t.Fatalf("decode pack_json: %v", err)
	}
	if _, ok := packObj["retry_reason"]; ok {
		t.Fatalf("retry_reason still present in pack_json: %v", packObj["retry_reason"])
	}
}

func TestStoreApplyScoresQueuesRetranslateWhenScoresLow(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT NOT NULL, ko_json TEXT, pack_json TEXT, updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	koJSON, _ := json.Marshal(map[string]any{"Text": "fresh ko"})
	packJSON, _ := json.Marshal(map[string]any{"current_ko": "current ko", "fresh_ko": "fresh ko", "proposed_ko_restored": "fresh ko"})
	if _, err := store.db.Exec(`INSERT INTO items(id, status, ko_json, pack_json, updated_at) VALUES ('x', 'done', ?, ?, '2026-03-08T00:00:00Z')`, string(koJSON), string(packJSON)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at) VALUES ('x', ?, 0, -1, '', ?, '2026-03-08T00:00:00Z', '2026-03-08T00:10:00Z', '2026-03-08T00:00:00Z')`, StateWorkingScore, "score-worker"); err != nil {
		t.Fatal(err)
	}

	if err := store.ApplyScores([]PipelineItem{{ID: "x", RetryCount: 0}}, map[string]ScoreResult{
		"x": {CurrentScore: 40, FreshScore: 62, ScoreFinal: 62, ShortReason: "both candidates weak"},
	}, 0.7, 3, "score-worker"); err != nil {
		t.Fatal(err)
	}

	var persisted string
	if err := store.db.QueryRow(`SELECT json_extract(ko_json, '$.Text') FROM items WHERE id='x'`).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != "fresh ko" {
		t.Fatalf("expected best available candidate to be persisted before retry, got %q", persisted)
	}
	rows, err := store.ListByState(StatePendingRetranslate, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "x" {
		t.Fatalf("pending_retranslate mismatch: %#v", rows)
	}
	if rows[0].LastError == "" {
		t.Fatalf("expected retry reason to be preserved for pending_retranslate")
	}
}

func TestStoreApplyScoresAcceptsFreshWhenCurrentMissing(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT NOT NULL, ko_json TEXT, pack_json TEXT, updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	koJSON, _ := json.Marshal(map[string]any{"Text": "fresh ko"})
	packJSON, _ := json.Marshal(map[string]any{"current_ko": "", "fresh_ko": "fresh ko", "proposed_ko_restored": "fresh ko"})
	if _, err := store.db.Exec(`INSERT INTO items(id, status, ko_json, pack_json, updated_at) VALUES ('x', 'done', ?, ?, '2026-03-08T00:00:00Z')`, string(koJSON), string(packJSON)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at) VALUES ('x', ?, 0, -1, '', ?, '2026-03-08T00:00:00Z', '2026-03-08T00:10:00Z', '2026-03-08T00:00:00Z')`, StateWorkingScore, "score-worker"); err != nil {
		t.Fatal(err)
	}

	if err := store.ApplyScores([]PipelineItem{{ID: "x", RetryCount: 0}}, map[string]ScoreResult{
		"x": {CurrentScore: 0, FreshScore: 82, ScoreFinal: 82},
	}, 0.7, 3, "score-worker"); err != nil {
		t.Fatal(err)
	}

	rows, err := store.ListByState(StateDone, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "x" {
		t.Fatalf("done mismatch: %#v", rows)
	}
	var persisted string
	if err := store.db.QueryRow(`SELECT json_extract(ko_json, '$.Text') FROM items WHERE id='x'`).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != "fresh ko" {
		t.Fatalf("expected fresh winner to be persisted, got %q", persisted)
	}
}

func TestStoreApplyScoresRequeuesScoringErrorInsteadOfPersistingZero(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT NOT NULL, ko_json TEXT, pack_json TEXT, updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	koJSON, _ := json.Marshal(map[string]any{"Text": "fresh ko"})
	packJSON, _ := json.Marshal(map[string]any{"current_ko": "current ko", "fresh_ko": "fresh ko", "proposed_ko_restored": "fresh ko"})
	if _, err := store.db.Exec(`INSERT INTO items(id, status, ko_json, pack_json, updated_at) VALUES ('x', 'done', ?, ?, '2026-03-08T00:00:00Z')`, string(koJSON), string(packJSON)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at) VALUES ('x', ?, 0, -1, '', ?, '2026-03-08T00:00:00Z', '2026-03-08T00:10:00Z', '2026-03-08T00:00:00Z')`, StateWorkingScore, "score-worker"); err != nil {
		t.Fatal(err)
	}

	if err := store.ApplyScores([]PipelineItem{{ID: "x", RetryCount: 0}}, map[string]ScoreResult{
		"x": {CurrentScore: 0, FreshScore: 0, ScoreFinal: 0, ReasonTags: []string{"scoring_error"}, ShortReason: "unexpected end of JSON input"},
	}, 0.7, 3, "score-worker"); err != nil {
		t.Fatal(err)
	}

	rows, err := store.ListByState(StatePendingScore, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "x" {
		t.Fatalf("expected x in pending_score, got %#v", rows)
	}
	if rows[0].ScoreFinal != -1 {
		t.Fatalf("expected score_final to stay pending, got %#v", rows[0])
	}
	var persisted string
	if err := store.db.QueryRow(`SELECT json_extract(ko_json, '$.Text') FROM items WHERE id='x'`).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != "fresh ko" {
		t.Fatalf("expected existing checkpoint text to remain untouched, got %q", persisted)
	}
}

func TestStoreApplyScoresRequeuesMissingScoreInsteadOfFailing(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT NOT NULL, ko_json TEXT, pack_json TEXT, updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	koJSON, _ := json.Marshal(map[string]any{"Text": "fresh ko"})
	packJSON, _ := json.Marshal(map[string]any{"current_ko": "current ko", "fresh_ko": "fresh ko", "proposed_ko_restored": "fresh ko"})
	if _, err := store.db.Exec(`INSERT INTO items(id, status, ko_json, pack_json, updated_at) VALUES ('x', 'done', ?, ?, '2026-03-08T00:00:00Z')`, string(koJSON), string(packJSON)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at) VALUES ('x', ?, 0, -1, '', ?, '2026-03-08T00:00:00Z', '2026-03-08T00:10:00Z', '2026-03-08T00:00:00Z')`, StateWorkingScore, "score-worker"); err != nil {
		t.Fatal(err)
	}

	if err := store.ApplyScores([]PipelineItem{{ID: "x", RetryCount: 0}}, map[string]ScoreResult{}, 0.7, 3, "score-worker"); err != nil {
		t.Fatal(err)
	}

	rows, err := store.ListByState(StatePendingScore, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "x" {
		t.Fatalf("expected x in pending_score, got %#v", rows)
	}
	if rows[0].LastError != "missing score" {
		t.Fatalf("expected last_error to preserve cause, got %#v", rows[0])
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

func TestStoreSeedAndClaimRespectInputOrder(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT NOT NULL, pack_json TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO items(id, status, pack_json) VALUES ('b', 'pending', '{}'), ('a', 'pending', '{}'), ('c', 'pending', '{}')`); err != nil {
		t.Fatal(err)
	}

	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Seed([]string{"b", "a", "c"}, 0); err != nil {
		t.Fatal(err)
	}

	rows, err := store.ClaimPending(StatePendingTranslate, StateWorkingTranslate, "translate-worker", 3, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 claimed rows, got %d", len(rows))
	}
	got := []string{rows[0].ID, rows[1].ID, rows[2].ID}
	want := []string{"b", "a", "c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("claim order mismatch got=%v want=%v", got, want)
		}
	}
}

func TestStoreSeedBlocksNonRootTranslateRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT NOT NULL, pack_json TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO items(id, status, pack_json) VALUES 
		('root', 'pending', '{"next_line_id":"child"}'),
		('child', 'pending', '{"prev_line_id":"root","next_line_id":"leaf"}'),
		('leaf', 'pending', '{"prev_line_id":"child"}')`); err != nil {
		t.Fatal(err)
	}

	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Seed([]string{"root", "child", "leaf"}, 0); err != nil {
		t.Fatal(err)
	}

	pending, err := store.ListByState(StatePendingTranslate, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != "root" {
		t.Fatalf("pending roots mismatch: %#v", pending)
	}
	blocked, err := store.ListByState(StateBlockedTranslate, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked) != 2 || blocked[0].ID != "child" || blocked[1].ID != "leaf" {
		t.Fatalf("blocked rows mismatch: %#v", blocked)
	}
}

func TestResolveAfterTranslateUnlocksNextBlockedRow(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT NOT NULL, ko_json TEXT, pack_json TEXT, updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	rootPack := `{"next_line_id":"child"}`
	childPack := `{"prev_line_id":"root"}`
	rootKO := `{"Text":"done"}`
	if _, err := store.db.Exec(`INSERT INTO items(id, status, ko_json, pack_json, updated_at) VALUES
		('root', 'done', ?, ?, '2026-03-08T00:00:00Z'),
		('child', 'new', '{}', ?, '2026-03-08T00:00:00Z')`, rootKO, rootPack, childPack); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, sort_index, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at) VALUES
		('root', 0, ?, 0, -1, '', 'translate-worker', '2026-03-08T00:00:00Z', '2026-03-08T00:10:00Z', '2026-03-08T00:00:00Z'),
		('child', 1, ?, 0, -1, '', '', '', '', '2026-03-08T00:00:00Z')`, StateWorkingTranslate, StateBlockedTranslate); err != nil {
		t.Fatal(err)
	}

	if err := store.ResolveAfterTranslate([]string{"root"}, map[string]string{"root": "Hello."}, false, "translate-worker"); err != nil {
		t.Fatal(err)
	}

	var state string
	if err := store.db.QueryRow(`SELECT state FROM pipeline_items WHERE id='child'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != StatePendingTranslate {
		t.Fatalf("expected child to unlock to pending_translate, got %q", state)
	}
}

func TestResolveAfterTranslateBlocksScoreUntilNextDone(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT NOT NULL, ko_json TEXT, pack_json TEXT, updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	rootPack := `{"next_line_id":"child"}`
	childPack := `{"prev_line_id":"root"}`
	rootKO := `{"Text":"done"}`
	if _, err := store.db.Exec(`INSERT INTO items(id, status, ko_json, pack_json, updated_at) VALUES
		('root', 'done', ?, ?, '2026-03-08T00:00:00Z'),
		('child', 'new', '{}', ?, '2026-03-08T00:00:00Z')`, rootKO, rootPack, childPack); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, sort_index, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at) VALUES
		('root', 0, ?, 0, -1, '', 'translate-worker', '2026-03-08T00:00:00Z', '2026-03-08T00:10:00Z', '2026-03-08T00:00:00Z'),
		('child', 1, ?, 0, -1, '', '', '', '', '2026-03-08T00:00:00Z')`, StateWorkingTranslate, StateBlockedTranslate); err != nil {
		t.Fatal(err)
	}

	if err := store.ResolveAfterTranslate([]string{"root"}, map[string]string{"root": "Hello."}, false, "translate-worker"); err != nil {
		t.Fatal(err)
	}

	var rootState, childState string
	if err := store.db.QueryRow(`SELECT state FROM pipeline_items WHERE id='root'`).Scan(&rootState); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT state FROM pipeline_items WHERE id='child'`).Scan(&childState); err != nil {
		t.Fatal(err)
	}
	if rootState != StateBlockedScore {
		t.Fatalf("expected root to wait in blocked_score, got %q", rootState)
	}
	if childState != StatePendingTranslate {
		t.Fatalf("expected child to unlock to pending_translate, got %q", childState)
	}
}

func TestResolveAfterTranslateUnlocksPrevBlockedScore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT NOT NULL, ko_json TEXT, pack_json TEXT, updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	prevKO := `{"Text":"prev done"}`
	nextKO := `{"Text":"next done"}`
	if _, err := store.db.Exec(`INSERT INTO items(id, status, ko_json, pack_json, updated_at) VALUES
		('prev', 'done', ?, '{"next_line_id":"next"}', '2026-03-08T00:00:00Z'),
		('next', 'done', ?, '{"prev_line_id":"prev"}', '2026-03-08T00:00:00Z')`, prevKO, nextKO); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, sort_index, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at) VALUES
		('prev', 0, ?, 0, -1, '', '', '', '', '2026-03-08T00:00:00Z'),
		('next', 1, ?, 0, -1, '', 'translate-worker', '2026-03-08T00:00:00Z', '2026-03-08T00:10:00Z', '2026-03-08T00:00:00Z')`, StateBlockedScore, StateWorkingTranslate); err != nil {
		t.Fatal(err)
	}

	if err := store.ResolveAfterTranslate([]string{"next"}, map[string]string{"next": "Next."}, false, "translate-worker"); err != nil {
		t.Fatal(err)
	}

	var prevState string
	if err := store.db.QueryRow(`SELECT state FROM pipeline_items WHERE id='prev'`).Scan(&prevState); err != nil {
		t.Fatal(err)
	}
	if prevState != StatePendingScore {
		t.Fatalf("expected prev to unlock to pending_score, got %q", prevState)
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
	if _, err := store.db.Exec(`INSERT INTO pipeline_worker_stats(worker_id, role, processed_count, started_at, finished_at, elapsed_ms) VALUES ('worker-x', 'retranslate', 10, '2026-03-08T00:00:00Z', '2026-03-08T00:00:01Z', 1000)`); err != nil {
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
	var statCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM pipeline_worker_stats`).Scan(&statCount); err != nil {
		t.Fatal(err)
	}
	if statCount != 0 {
		t.Fatalf("expected worker stats to be cleared after reset, got %d", statCount)
	}
}

func TestResetScoringStateMovesDoneItemsBackToPendingScore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT NOT NULL, pack_json TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO items(id, status, pack_json) VALUES ('done-1', 'done', '{"next_line_id":"pending-x"}'), ('done-2', 'done', '{}'), ('pending-x', 'pending', '{"prev_line_id":"done-1"}')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at) VALUES
		('done-1', ?, 3, 0.95, 'old failure', 'worker', '2026-03-08T00:00:00Z', '2026-03-08T00:05:00Z', '2026-03-08T00:00:00Z'),
		('pending-x', ?, 0, -1, '', '', '', '', '2026-03-08T00:00:00Z'),
		('stale-failed', ?, 1, 0.88, 'translator produced no done row', '', '', '', '2026-03-08T00:00:00Z')`,
		StateFailed,
		StateFailed, StatePendingTranslate); err != nil {
		t.Fatal(err)
	}

	n, err := store.ResetScoringState()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 updated existing row, got %d", n)
	}

	rows, err := store.ListByState(StatePendingScore, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != "done-2" {
		t.Fatalf("expected only tail done row in pending_score, got %#v", rows)
	}
	for _, row := range rows {
		if row.RetryCount != 0 || row.ScoreFinal != -1 || row.LastError != "" || row.ClaimedBy != "" || row.LeaseUntil != "" {
			t.Fatalf("row not reset cleanly: %#v", row)
		}
	}
	blockedScore, err := store.ListByState(StateBlockedScore, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(blockedScore) != 1 || blockedScore[0].ID != "done-1" {
		t.Fatalf("expected done-1 in blocked_score, got %#v", blockedScore)
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM pipeline_items WHERE id IN ('pending-x', 'stale-failed')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected non-done pipeline rows to be pruned, got count=%d", count)
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

func TestRequeueTranslateNoDoneRowAsRetranslateOnlyRequeuesMatchingFailures(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, updated_at) VALUES
		('requeue-me', ?, 2, -1, 'translator produced no done row', '2026-03-08T00:00:00Z'),
		('leave-failed', ?, 3, -1, 'other', '2026-03-08T00:00:00Z'),
		('leave-pending', ?, 1, -1, '', '2026-03-08T00:00:00Z')`,
		StateFailed, StateFailed, StatePendingRetranslate); err != nil {
		t.Fatal(err)
	}

	n, err := store.RequeueTranslateNoDoneRowAsRetranslate(0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row to be requeued, got %d", n)
	}

	rows, err := store.ListByState(StatePendingRetranslate, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 pending_retranslate rows, got %#v", rows)
	}
	for _, row := range rows {
		if row.ID == "requeue-me" && row.RetryCount != 0 {
			t.Fatalf("expected retry_count reset for requeued row, got %#v", row)
		}
	}
}

func TestRequeueExpiredWorkingMovesOnlyExpiredClaimedRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at) VALUES
		('expired', ?, 0, -1, '', 'old-worker', '2026-03-08T00:00:00Z', '2000-01-01T00:00:00Z', '2026-03-08T00:00:00Z'),
		('fresh', ?, 0, -1, '', 'live-worker', '2026-03-08T00:00:00Z', '2999-01-01T00:00:00Z', '2026-03-08T00:00:00Z'),
		('unclaimed', ?, 0, -1, '', '', '', '', '2026-03-08T00:00:00Z')`,
		StateWorkingRetranslate, StateWorkingRetranslate, StateWorkingRetranslate); err != nil {
		t.Fatal(err)
	}

	n, err := store.RequeueExpiredWorking(StateWorkingRetranslate, StatePendingRetranslate)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 expired row to be requeued, got %d", n)
	}

	var state, claimedBy string
	var leaseUntil sql.NullString
	if err := store.db.QueryRow(`SELECT state, claimed_by, lease_until FROM pipeline_items WHERE id = 'expired'`).Scan(&state, &claimedBy, &leaseUntil); err != nil {
		t.Fatal(err)
	}
	if state != StatePendingRetranslate || claimedBy != "" || leaseUntil.Valid {
		t.Fatalf("expired row not reset correctly: state=%q claimed_by=%q lease_until=%q", state, claimedBy, leaseUntil.String)
	}
	if err := store.db.QueryRow(`SELECT state, claimed_by, lease_until FROM pipeline_items WHERE id = 'fresh'`).Scan(&state, &claimedBy, &leaseUntil); err != nil {
		t.Fatal(err)
	}
	if state != StateWorkingRetranslate || claimedBy != "live-worker" || !leaseUntil.Valid || leaseUntil.String == "" {
		t.Fatalf("fresh row should remain claimed: state=%q claimed_by=%q lease_until=%q", state, claimedBy, leaseUntil.String)
	}
	if err := store.db.QueryRow(`SELECT state, claimed_by, lease_until FROM pipeline_items WHERE id = 'unclaimed'`).Scan(&state, &claimedBy, &leaseUntil); err != nil {
		t.Fatal(err)
	}
	if state != StateWorkingRetranslate || claimedBy != "" || (leaseUntil.Valid && leaseUntil.String != "") {
		t.Fatalf("unclaimed row should remain unchanged: state=%q claimed_by=%q lease_until=%q", state, claimedBy, leaseUntil.String)
	}
}

func TestCleanupStaleClaimsSummarizesPerRole(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at) VALUES
		('t-expired', ?, 0, -1, '', 'translate-worker', '2026-03-08T00:00:00Z', '2000-01-01T00:00:00Z', '2026-03-08T00:00:00Z'),
		('s-expired', ?, 0, -1, '', 'score-worker', '2026-03-08T00:00:00Z', '2000-01-01T00:00:00Z', '2026-03-08T00:00:00Z'),
		('r-expired', ?, 0, -1, '', 'retranslate-worker', '2026-03-08T00:00:00Z', '2000-01-01T00:00:00Z', '2026-03-08T00:00:00Z'),
		('t-fresh', ?, 0, -1, '', 'translate-worker', '2026-03-08T00:00:00Z', '2999-01-01T00:00:00Z', '2026-03-08T00:00:00Z')`,
		StateWorkingTranslate, StateWorkingScore, StateWorkingRetranslate, StateWorkingTranslate); err != nil {
		t.Fatal(err)
	}

	summary, err := store.CleanupStaleClaims()
	if err != nil {
		t.Fatal(err)
	}
	if summary.Translate != 1 || summary.Score != 1 || summary.Retranslate != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	var state string
	if err := store.db.QueryRow(`SELECT state FROM pipeline_items WHERE id='t-expired'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != StatePendingTranslate {
		t.Fatalf("t-expired state=%q", state)
	}
	if err := store.db.QueryRow(`SELECT state FROM pipeline_items WHERE id='s-expired'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != StatePendingScore {
		t.Fatalf("s-expired state=%q", state)
	}
	if err := store.db.QueryRow(`SELECT state FROM pipeline_items WHERE id='r-expired'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != StatePendingRetranslate {
		t.Fatalf("r-expired state=%q", state)
	}
	if err := store.db.QueryRow(`SELECT state FROM pipeline_items WHERE id='t-fresh'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != StateWorkingTranslate {
		t.Fatalf("t-fresh should remain working, got %q", state)
	}
}

func TestRouteKnownFailedNoDoneRowUsesFailedTranslateLane(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT NOT NULL, pack_json TEXT)`); err != nil {
		t.Fatal(err)
	}
	packJSON, _ := json.Marshal(map[string]any{
		"source_raw": `(Raise a hand.) "Me?`,
		"text_role":  "choice",
	})
	if _, err := db.Exec(`INSERT INTO items(id, status, pack_json) VALUES ('x', 'new', ?)`, string(packJSON)); err != nil {
		t.Fatal(err)
	}
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, updated_at) VALUES ('x', ?, 0, -1, 'translator produced no done row', '2026-03-08T00:00:00Z')`, StateFailed); err != nil {
		t.Fatal(err)
	}

	summary, err := store.RouteKnownFailedNoDoneRow(0)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Total != 1 || summary.ActionOpenQuote != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	var state string
	if err := store.db.QueryRow(`SELECT state FROM pipeline_items WHERE id='x'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != StatePendingFailedTranslate {
		t.Fatalf("expected pending_failed_translate, got %q", state)
	}
}

func TestResolveAfterFailedTranslateEscalatesNoDoneRowsToRetranslate(t *testing.T) {
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
		('x', ?, 0, -1, '', 'failed-translate-worker', '2026-03-08T00:00:00Z', '2026-03-08T00:10:00Z', '2026-03-08T00:00:00Z')`, StateWorkingFailedTranslate); err != nil {
		t.Fatal(err)
	}

	if err := store.ResolveAfterFailedTranslate([]string{"x"}, map[string]string{"x": "Hello."}, "failed-translate-worker"); err != nil {
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

func TestRouteOverlayUIQueuesOverlayRowsToPendingOverlayTranslate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT NOT NULL, pack_json TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO items(id, status, pack_json) VALUES
		('ovl-mainmenu-1', 'new', '{}'),
		('line-normal', 'new', '{}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, updated_at) VALUES
		('ovl-mainmenu-1', ?, 0, -1, 'translator produced no done row', '2026-03-08T00:00:00Z'),
		('line-normal', ?, 0, -1, 'translator produced no done row', '2026-03-08T00:00:00Z')`,
		StateFailed, StateFailed); err != nil {
		t.Fatal(err)
	}

	n, err := store.RouteOverlayUI(0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 routed row, got %d", n)
	}
	var state string
	if err := store.db.QueryRow(`SELECT state FROM pipeline_items WHERE id='ovl-mainmenu-1'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != StatePendingOverlayTranslate {
		t.Fatalf("expected pending_overlay_translate, got %q", state)
	}
	if err := store.db.QueryRow(`SELECT state FROM pipeline_items WHERE id='line-normal'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != StateFailed {
		t.Fatalf("expected line-normal to stay failed, got %q", state)
	}
}

func TestApplyPreservePolicyDirectlyCompletesRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT NOT NULL, ko_json TEXT, pack_json TEXT, updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	packJSON, _ := json.Marshal(map[string]any{
		"source_raw":          "ArmRig",
		"en":                  "ArmRig",
		"text_role":           "ui_label",
		"translation_policy":  "preserve",
		"translation_policy_reason": "internal_name",
	})
	koJSON, _ := json.Marshal(map[string]any{"Text": "팔 리그"})
	if _, err := store.db.Exec(`INSERT INTO items(id, status, ko_json, pack_json, updated_at) VALUES ('x', 'pending', ?, ?, '2026-03-08T00:00:00Z')`, string(koJSON), string(packJSON)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, updated_at) VALUES ('x', ?, 2, 12, 'overlay translator produced no done row', '2026-03-08T00:00:00Z')`, StateFailed); err != nil {
		t.Fatal(err)
	}

	summary, err := store.ApplyPreservePolicy(100)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Applied != 1 {
		t.Fatalf("applied=%d, want 1", summary.Applied)
	}

	var state string
	var scoreFinal float64
	if err := store.db.QueryRow(`SELECT state, score_final FROM pipeline_items WHERE id='x'`).Scan(&state, &scoreFinal); err != nil {
		t.Fatal(err)
	}
	if state != StateDone || scoreFinal != 100 {
		t.Fatalf("pipeline row = state=%q score_final=%v", state, scoreFinal)
	}

	var status string
	var persistedKO string
	var persistedPack string
	if err := store.db.QueryRow(`SELECT status, json_extract(ko_json, '$.Text'), pack_json FROM items WHERE id='x'`).Scan(&status, &persistedKO, &persistedPack); err != nil {
		t.Fatal(err)
	}
	if status != "done" || persistedKO != "ArmRig" {
		t.Fatalf("item row = status=%q ko=%q", status, persistedKO)
	}
	var packObj map[string]any
	if err := json.Unmarshal([]byte(persistedPack), &packObj); err != nil {
		t.Fatal(err)
	}
	if packObj["winner"] != "preserve" {
		t.Fatalf("winner=%v", packObj["winner"])
	}
}

func TestRepairBlockedTranslateReleasesRowsWithSatisfiedPredecessor(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT NOT NULL, pack_json TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO items(id, status, pack_json) VALUES
		('prev-done', 'done', '{}'),
		('prev-pending', 'pending', '{}'),
		('released', 'pending', '{"prev_line_id":"prev-done"}'),
		('still-blocked', 'pending', '{"prev_line_id":"prev-pending"}'),
		('root-line', 'pending', '{}')`); err != nil {
		t.Fatal(err)
	}
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, updated_at) VALUES
		('prev-done', ?, 0, -1, '', '2026-03-08T00:00:00Z'),
		('prev-pending', ?, 0, -1, '', '2026-03-08T00:00:00Z'),
		('released', ?, 0, -1, '', '2026-03-08T00:00:00Z'),
		('still-blocked', ?, 0, -1, '', '2026-03-08T00:00:00Z'),
		('root-line', ?, 0, -1, '', '2026-03-08T00:00:00Z')`,
		StateDone, StatePendingTranslate, StateBlockedTranslate, StateBlockedTranslate, StateBlockedTranslate); err != nil {
		t.Fatal(err)
	}

	summary, err := store.RepairBlockedTranslate(0)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Released != 2 || summary.StillBlocked != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}

	var state string
	if err := store.db.QueryRow(`SELECT state FROM pipeline_items WHERE id='released'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != StatePendingTranslate {
		t.Fatalf("released state=%q", state)
	}
	if err := store.db.QueryRow(`SELECT state FROM pipeline_items WHERE id='root-line'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != StatePendingTranslate {
		t.Fatalf("root-line state=%q", state)
	}
	if err := store.db.QueryRow(`SELECT state FROM pipeline_items WHERE id='still-blocked'`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != StateBlockedTranslate {
		t.Fatalf("still-blocked state=%q", state)
	}
}

func TestRecoverUnscoreableWorkingScoreQueuesRetranslateForEmptyDoneRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`CREATE TABLE items (id TEXT PRIMARY KEY, status TEXT NOT NULL, ko_json TEXT, pack_json TEXT, updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	packJSON, _ := json.Marshal(map[string]any{
		"source_raw": "You are welcome, <i>Ragn</i>.",
		"fresh_ko":   "",
		"current_ko": "",
	})
	koJSON, _ := json.Marshal(map[string]any{"Text": ""})
	if _, err := store.db.Exec(`INSERT INTO items(id, status, ko_json, pack_json, updated_at) VALUES ('x', 'done', ?, ?, '2026-03-08T00:00:00Z')`, string(koJSON), string(packJSON)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at) VALUES ('x', ?, 1, -1, '', 'score-worker', '2026-03-08T00:00:00Z', '2099-01-01T00:00:00Z', '2026-03-08T00:00:00Z')`, StateWorkingScore); err != nil {
		t.Fatal(err)
	}

	summary, err := store.RecoverUnscoreableWorkingScore([]string{"x"}, nil, "score-worker")
	if err != nil {
		t.Fatal(err)
	}
	if summary.ToPendingRetranslate != 1 || summary.ToPendingTranslate != 0 || summary.ToFailed != 0 {
		t.Fatalf("unexpected summary: %#v", summary)
	}

	var state, claimedBy, lastError string
	var scoreFinal float64
	if err := store.db.QueryRow(`SELECT state, claimed_by, last_error, score_final FROM pipeline_items WHERE id='x'`).Scan(&state, &claimedBy, &lastError, &scoreFinal); err != nil {
		t.Fatal(err)
	}
	if state != StatePendingRetranslate || claimedBy != "" || lastError != "" || scoreFinal != -1 {
		t.Fatalf("row not rerouted correctly: state=%q claimed_by=%q last_error=%q score_final=%v", state, claimedBy, lastError, scoreFinal)
	}
}

func TestExtendLeaseRefreshesClaimedRowsOnly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at) VALUES
		('keep', ?, 0, -1, '', 'worker-a', '2026-03-08T00:00:00Z', '2000-01-01T00:00:00Z', '2026-03-08T00:00:00Z'),
		('other-worker', ?, 0, -1, '', 'worker-b', '2026-03-08T00:00:00Z', '2000-01-01T00:00:00Z', '2026-03-08T00:00:00Z'),
		('wrong-state', ?, 0, -1, '', 'worker-a', '2026-03-08T00:00:00Z', '2000-01-01T00:00:00Z', '2026-03-08T00:00:00Z')`,
		StateWorkingScore, StateWorkingScore, StatePendingScore); err != nil {
		t.Fatal(err)
	}

	n, err := store.ExtendLease([]string{"keep", "other-worker", "wrong-state"}, StateWorkingScore, "worker-a", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected only one row lease to be extended, got %d", n)
	}

	var keepLease, otherLease, wrongLease string
	if err := store.db.QueryRow(`SELECT lease_until FROM pipeline_items WHERE id='keep'`).Scan(&keepLease); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT lease_until FROM pipeline_items WHERE id='other-worker'`).Scan(&otherLease); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT lease_until FROM pipeline_items WHERE id='wrong-state'`).Scan(&wrongLease); err != nil {
		t.Fatal(err)
	}
	if keepLease == "2000-01-01T00:00:00Z" {
		t.Fatalf("expected keep lease to change")
	}
	if otherLease != "2000-01-01T00:00:00Z" {
		t.Fatalf("expected other worker lease to remain unchanged, got %q", otherLease)
	}
	if wrongLease != "2000-01-01T00:00:00Z" {
		t.Fatalf("expected wrong state lease to remain unchanged, got %q", wrongLease)
	}
}

func TestRequeueClaimsByWorkerClearsOnlyMatchingWorkerAndState(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pipeline.db")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`INSERT INTO pipeline_items(id, state, retry_count, score_final, last_error, claimed_by, claimed_at, lease_until, updated_at) VALUES
		('mine', ?, 0, -1, '', 'worker-a', '2026-03-08T00:00:00Z', '2099-01-01T00:00:00Z', '2026-03-08T00:00:00Z'),
		('other-state', ?, 0, -1, '', 'worker-a', '2026-03-08T00:00:00Z', '2099-01-01T00:00:00Z', '2026-03-08T00:00:00Z'),
		('other-worker', ?, 0, -1, '', 'worker-b', '2026-03-08T00:00:00Z', '2099-01-01T00:00:00Z', '2026-03-08T00:00:00Z')`,
		StateWorkingScore, StateWorkingTranslate, StateWorkingScore); err != nil {
		t.Fatal(err)
	}

	n, err := store.RequeueClaimsByWorker(StateWorkingScore, StatePendingScore, "worker-a")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected only one row to be requeued, got %d", n)
	}

	var state, claimedBy string
	if err := store.db.QueryRow(`SELECT state, claimed_by FROM pipeline_items WHERE id='mine'`).Scan(&state, &claimedBy); err != nil {
		t.Fatal(err)
	}
	if state != StatePendingScore || claimedBy != "" {
		t.Fatalf("mine not reset correctly: state=%q claimed_by=%q", state, claimedBy)
	}
	if err := store.db.QueryRow(`SELECT state, claimed_by FROM pipeline_items WHERE id='other-state'`).Scan(&state, &claimedBy); err != nil {
		t.Fatal(err)
	}
	if state != StateWorkingTranslate || claimedBy != "worker-a" {
		t.Fatalf("other-state should remain unchanged: state=%q claimed_by=%q", state, claimedBy)
	}
	if err := store.db.QueryRow(`SELECT state, claimed_by FROM pipeline_items WHERE id='other-worker'`).Scan(&state, &claimedBy); err != nil {
		t.Fatal(err)
	}
	if state != StateWorkingScore || claimedBy != "worker-b" {
		t.Fatalf("other-worker should remain unchanged: state=%q claimed_by=%q", state, claimedBy)
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
			sort_index INTEGER NOT NULL DEFAULT 0,
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
	for _, name := range []string{"sort_index", "claimed_by", "claimed_at", "lease_until"} {
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
