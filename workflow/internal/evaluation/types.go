package evaluation

import (
	"localize-agent/workflow/internal/contracts"
	"localize-agent/workflow/internal/shared"
)

type Config struct {
	Resume      bool
	StatusOnly  bool
	Export      bool
	ResetStatus string
	ReevalIDs   string

	PackIn  string
	DB      string
	RunName string

	LLMBackend string
	ServerURL  string
	TransModel string
	EvalModel  string
	TransAgent string
	EvalAgent  string

	Concurrency int
	TimeoutSec  int
	MaxAttempts int
	BackoffSec  float64
	MaxRetry    int

	ContextFiles  shared.MultiFlag
	RulesFile     string
	EvalRulesFile string
	TraceOut      string

	ReportOut       string
	RejectedOut     string
	RevisedOut      string
	ReviewExportOut string
	ReviewStatuses  string
}

type packItem = contracts.EvalPackItem
type evalResult = contracts.EvalResult

type revisedProposal struct {
	ID         string `json:"id"`
	ProposedKO string `json:"proposed_ko"`
	Risk       string `json:"risk"`
	Notes      string `json:"notes"`
}

const (
	statusPending    = "pending"
	statusEvaluating = "evaluating"
	statusPass       = "pass"
	statusRevise     = "revise"
	statusReject     = "reject"

	kindTrans = "trans"
	kindEval  = "eval"
)

type itemOutcome struct {
	id          string
	finalKO     string
	finalRisk   string
	finalNotes  string
	finalStatus string
	revised     bool
	history     []evalResult
}

func DefaultConfig() Config {
	return Config{
		DB:             "workflow/output/evaluation_unified.db",
		RunName:        "default",
		LLMBackend:     "opencode",
		ServerURL:      "http://127.0.0.1:4112",
		TransModel:     "openai/gpt-5.2",
		EvalModel:      "openai/gpt-5.2",
		TransAgent:     "rt-ko-translate-primary",
		EvalAgent:      "rt-ko-eval-primary",
		Concurrency:    3,
		TimeoutSec:     60,
		MaxAttempts:    2,
		BackoffSec:     1.0,
		MaxRetry:       2,
		ReportOut:      "workflow/output/evaluation_report.json",
		RejectedOut:    "workflow/output/evaluation_rejected.json",
		RevisedOut:     "workflow/output/evaluation_revised_candidate.json",
		ReviewStatuses: "pass,revise,reject",
	}
}
