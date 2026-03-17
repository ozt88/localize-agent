package translationpipeline

const (
	StatePendingTranslate   = "pending_translate"
	StateBlockedTranslate   = "blocked_translate"
	StatePendingFailedTranslate = "pending_failed_translate"
	StatePendingOverlayTranslate = "pending_overlay_translate"
	StateBlockedScore       = "blocked_score"
	StatePendingScore       = "pending_score"
	StatePendingRetranslate = "pending_retranslate"
	StateWorkingTranslate   = "working_translate"
	StateWorkingFailedTranslate = "working_failed_translate"
	StateWorkingOverlayTranslate = "working_overlay_translate"
	StateWorkingScore       = "working_score"
	StateWorkingRetranslate = "working_retranslate"
	StateDone               = "done"
	StateFailed             = "failed"
)

type Config struct {
	Project              string
	ProjectDir           string
	CheckpointBackend    string
	CheckpointDB         string
	CheckpointDSN        string
	InitOnly             bool
	CleanupStaleClaims   bool
	RouteKnownFailedNoRow bool
	RouteOverlayUI       bool
	RepairBlockedTranslate bool
	ResetScoring         bool
	RequeueFailedNoRow   bool
	RequeueTranslateNoRowAsRetranslate bool
	RequeueLimit         int
	Reset                bool
	StageBatchSize       int
	SeedLimit            int
	Threshold            float64
	MaxRetries           int
	LowBackend           string
	LowServerURL         string
	LowModel             string
	LowAgent             string
	LowConcurrency       int
	LowBatchSize         int
	LowTimeoutSec        int
	RetranslateBackend   string
	RetranslateServerURL string
	RetranslateModel     string
	RetranslateAgent     string
	ScoreBackend         string
	ScoreServerURL       string
	ScoreModel           string
	ScoreAgent           string
	ScorePromptVariant   string
	ScoreConcurrency     int
	ScoreBatchSize       int
	ScoreTimeoutSec      int
	TraceOutDir          string
	WorkerRole           string
	WorkerID             string
	LeaseSec             int
	IdleSleepSec         int
	Once                 bool
}

type PipelineItem struct {
	ID         string
	SortIndex  int
	State      string
	RetryCount int
	ScoreFinal float64
	LastError  string
	ClaimedBy  string
	ClaimedAt  string
	LeaseUntil string
}

type WorkerBatchStat struct {
	ID             int64
	WorkerID       string
	Role           string
	ProcessedCount int
	ElapsedMs      int64
	StartedAt      string
	FinishedAt     string
}

type ScoreResult struct {
	CurrentScore float64
	FreshScore   float64
	ScoreFinal   float64
	ReasonTags   []string
	ShortReason  string
}
