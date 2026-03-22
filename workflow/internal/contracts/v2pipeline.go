package contracts

// V2PipelineItem represents a single dialogue block in the v2 translation pipeline.
// Each item flows through translate -> format -> score stages with lease-based claiming.
type V2PipelineItem struct {
	ID                string  // DialogueBlock.ID (path-based, e.g., "KnotName/g-0/c-1/blk-0")
	SortIndex         int     // global sequential order for deterministic claiming
	SourceFile        string  // TextAsset filename
	Knot              string  // knot name
	ContentType       string  // dialogue, spell, ui, item, system
	Speaker           string  // speaker label from ink # tag (e.g., "Braxo")
	Choice            string  // choice ID (e.g., "c-1") or empty
	Gate              string  // gate ID (e.g., "g-0") or empty
	SourceRaw         string  // original EN text
	SourceHash        string  // SHA-256 of SourceRaw
	HasTags           bool    // whether source contains rich-text tags
	State             string  // pipeline state (pending_translate, working_translate, etc.)
	KORaw             string  // Stage 1 output (tag-free Korean)
	KOFormatted       string  // Stage 2 output (Korean with tags restored)
	TranslateAttempts int     // number of translate attempts
	FormatAttempts    int     // number of format attempts
	ScoreAttempts     int     // number of score attempts
	ScoreFinal        float64 // final quality score (-1 = not scored)
	FailureType       string  // translation/format/both/pass per D-14
	LastError         string  // last error message
	AttemptLog        string  // JSON array of attempt records per D-16
	ClaimedBy         string  // worker ID holding the lease
	BatchID           string  // which Batch this block belongs to
}

// V2PipelineStore defines the persistence interface for the v2 pipeline state machine.
// Implementations manage the full lifecycle: ingest, claim, translate, format, score, retry.
type V2PipelineStore interface {
	// Seed inserts items into the pipeline. Deduplicates by source_hash (ON CONFLICT DO NOTHING).
	// Returns count of inserted vs skipped items.
	Seed(items []V2PipelineItem) (inserted int, skipped int, err error)

	// ClaimPending atomically claims up to batchSize items in pendingState,
	// transitioning them to workingState with a lease.
	ClaimPending(pendingState, workingState, workerID string, batchSize int, leaseSec int) ([]V2PipelineItem, error)

	// MarkState sets an item to a new state, clearing claim fields.
	MarkState(id, newState string) error

	// MarkTranslated sets ko_raw and routes: has_tags=true -> pending_format, else -> pending_score.
	MarkTranslated(id, koRaw string) error

	// MarkFormatted sets ko_formatted and advances to pending_score.
	MarkFormatted(id, koFormatted string) error

	// MarkScored applies score result and routes by failure_type per D-14:
	// "pass"->done, "translation"->pending_translate, "format"->pending_format, "both"->pending_translate.
	MarkScored(id string, scoreFinal float64, failureType, reason string) error

	// MarkFailed sets state to failed with an error message.
	MarkFailed(id, lastError string) error

	// AppendAttemptLog appends a JSON object to the attempt_log array.
	AppendAttemptLog(id string, entry map[string]interface{}) error

	// UpdateRetryState sets state to targetState, increments the specified attempts field, clears claim.
	UpdateRetryState(id, targetState string, incrementField string) error

	// CleanupStaleClaims reclaims items stuck in working_* states past their lease.
	CleanupStaleClaims(olderThanSec int) (int64, error)

	// CountByState returns counts of items grouped by state.
	CountByState() (map[string]int, error)

	// MarkDonePassthrough sets state=done with ko_formatted=source text (for punctuation-only blocks).
	MarkDonePassthrough(id, koFormatted string) error

	// GetPrevGateLines returns the last N source_raw texts from the previous gate
	// in the same knot, ordered by sort_index descending. Used for D-03 context injection.
	GetPrevGateLines(knot, currentGate string, limit int) ([]string, error)

	// GetItem retrieves a single pipeline item by ID.
	GetItem(id string) (*V2PipelineItem, error)

	// Close releases database resources.
	Close() error
}
