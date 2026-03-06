package translation

import "localize-agent/workflow/internal/shared"

type Config struct {
	Source                      string
	Current                     string
	IDsFile                     string
	LLMBackend                  string
	ServerURL                   string
	Model                       string
	Agent                       string
	Concurrency                 int
	BatchSize                   int
	MaxBatchChars               int
	TimeoutSec                  int
	MaxAttempts                 int
	BackoffSec                  float64
	MaxPlainLen                 int
	SkipInvalid                 bool
	SkipTimeout                 bool
	PlaceholderRecoveryAttempts int
	ContextFiles                shared.MultiFlag
	RulesFile                   string
	CheckpointDB                string
	TraceOut                    string
	ReviewExportOut             string
	ReviewStatuses              string
	Resume                      bool
}

type mapping struct {
	placeholder string
	original    string
}

type itemMeta struct {
	id      string
	enText  string
	curText string
	curObj  map[string]any
	mapTags []mapping
}

type proposal struct {
	ID         string `json:"id"`
	ProposedKO string `json:"proposed_ko"`
	Risk       string `json:"risk"`
	Notes      string `json:"notes"`
}

func DefaultConfig() Config {
	return Config{
		Source:                      "enGB_original.json",
		Current:                     "enGB_new.json",
		LLMBackend:                  "opencode",
		ServerURL:                   "http://127.0.0.1:4112",
		Model:                       "openai/gpt-5.2",
		Agent:                       "rt-ko-translate-primary",
		Concurrency:                 10,
		BatchSize:                   10,
		MaxBatchChars:               0,
		TimeoutSec:                  45,
		MaxAttempts:                 2,
		BackoffSec:                  1.0,
		MaxPlainLen:                 220,
		SkipInvalid:                 true,
		SkipTimeout:                 true,
		PlaceholderRecoveryAttempts: 1,
		CheckpointDB:                "workflow/output/translation_checkpoint.db",
		ReviewStatuses:              "done",
	}
}
