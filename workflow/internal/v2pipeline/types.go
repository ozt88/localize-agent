package v2pipeline

// Pipeline states per D-14/CONTEXT.md state flow.
// Items flow: pending_translate -> working_translate -> translated ->
// pending_format -> working_format -> formatted ->
// pending_score -> working_score -> done | failed
const (
	StatePendingTranslate = "pending_translate"
	StateWorkingTranslate = "working_translate"
	StateTranslated       = "translated"
	StatePendingFormat    = "pending_format"
	StateWorkingFormat    = "working_format"
	StateFormatted        = "formatted"
	StatePendingScore     = "pending_score"
	StateWorkingScore     = "working_score"
	StateDone             = "done"
	StateFailed           = "failed"
)

// Config for v2 pipeline orchestration.
type Config struct {
	Project    string
	ProjectDir string

	// DB
	CheckpointBackend string
	CheckpointDSN     string
	CheckpointDB      string // SQLite path for local dev

	// Translate stage (gpt-5.4)
	TranslateBackend     string
	TranslateServerURL   string
	TranslateModel       string
	TranslateConcurrency int
	TranslateBatchSize   int
	TranslateTimeoutSec  int

	// Format stage (codex-spark)
	FormatBackend     string
	FormatServerURL   string
	FormatModel       string
	FormatConcurrency int
	FormatBatchSize   int
	FormatTimeoutSec  int

	// Score stage
	ScoreBackend     string
	ScoreServerURL   string
	ScoreModel       string
	ScoreConcurrency int
	ScoreBatchSize   int
	ScoreTimeoutSec  int

	// Orchestrator
	WorkerRole         string
	WorkerID           string
	LeaseSec           int
	IdleSleepSec       int
	MaxRetries         int
	TraceOutDir        string
	CleanupStaleClaims bool
	Once               bool
}
