package translationpipeline

const (
	StatePendingTranslate   = "pending_translate"
	StatePendingScore       = "pending_score"
	StatePendingRetranslate = "pending_retranslate"
	StateWorkingTranslate   = "working_translate"
	StateWorkingScore       = "working_score"
	StateWorkingRetranslate = "working_retranslate"
	StateDone               = "done"
	StateFailed             = "failed"
)

type Config struct {
	Project              string
	ProjectDir           string
	CheckpointDB         string
	InitOnly             bool
	RequeueFailedNoRow   bool
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
