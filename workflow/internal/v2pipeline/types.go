package v2pipeline

import "localize-agent/workflow/internal/contracts"

// Re-export pipeline state constants from contracts for backward compatibility.
// All state constants are canonically defined in contracts to avoid import cycles.
const (
	StatePendingTranslate = contracts.StatePendingTranslate
	StateWorkingTranslate = contracts.StateWorkingTranslate
	StateTranslated       = contracts.StateTranslated
	StatePendingFormat    = contracts.StatePendingFormat
	StateWorkingFormat    = contracts.StateWorkingFormat
	StateFormatted        = contracts.StateFormatted
	StatePendingScore     = contracts.StatePendingScore
	StateWorkingScore     = contracts.StateWorkingScore
	StateDone             = contracts.StateDone
	StateFailed           = contracts.StateFailed
)

// ScoreHistogramBucket represents a single bucket in a score distribution histogram.
type ScoreHistogramBucket struct {
	LowerBound float64
	Count      int
}

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

	// Context injection
	VoiceCardsPath string            // path to voice_cards.json (optional)
	VoiceCards     map[string]string // speaker -> formatted voice guide text (loaded at startup)
	RAGContextPath string            // path to rag_batch_context.json (optional)
}
