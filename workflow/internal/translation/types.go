package translation

import "localize-agent/workflow/internal/shared"

type Config struct {
	Source                      string
	Current                     string
	IDsFile                     string
	TranslatorPackageChunks     string
	LLMBackend                  string
	ServerURL                   string
	Model                       string
	Agent                       string
	HighLLMBackend              string
	HighServerURL               string
	HighModel                   string
	HighAgent                   string
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
	OllamaStructuredOutput      bool
	OllamaBakedSystem          bool
	OllamaResetHistory          bool
	OllamaKeepAlive             string
	OllamaNumCtx                int
	OllamaTemperature           float64
	TranslatorResponseMode      string
	PipelineVersion             string
}

const (
	responseModePlain = "plain"
	responseModeJSON  = "json"
)

type mapping struct {
	placeholder string
	original    string
}

type emphasisSpan struct {
	openMarker string
	closeMarker string
	openTag    string
	closeTag   string
}

type itemMeta struct {
	id              string
	sourceRaw       string
	enText          string
	curText         string
	contextEN       string
	textRole        string
	speakerHint     string
	curObj          map[string]any
	mapTags         []mapping
	profile         textProfile
	choicePrefix    string
	controlPrefix   string
	emphasisSpans   []emphasisSpan
	passthrough     bool
	translationLane string
}

type proposal struct {
	ID         string `json:"id"`
	ProposedKO string `json:"proposed_ko"`
	Risk       string `json:"risk"`
	Notes      string `json:"notes"`
}

type translationTask struct {
	ID            string
	BodyEN        string
	ContextEN     string
	TextRole      string
	SpeakerHint   string
	GroupKey      string
	Lane          string
	Profile       textProfile
}

type chunkContext struct {
	ChunkID         string
	ParentSegmentID string
	ChunkPos        int
	ChunkCount      int
	LineIDs         []string
}

type lineContext struct {
	PrevLineID                  string
	NextLineID                  string
	TextRole                    string
	SpeakerHint                 string
	LineIsShortContextDependent bool
	LineHasEmphasis             bool
	LineIsImperative            bool
	Chunk                       chunkContext
}

func DefaultConfig() Config {
	return Config{
		Source:                      "enGB_original.json",
		Current:                     "enGB_new.json",
		LLMBackend:                  "opencode",
		ServerURL:                   "http://127.0.0.1:4112",
		Model:                       "openai/gpt-5.2",
		Agent:                       "rt-ko-translate-primary",
		HighLLMBackend:              "",
		HighServerURL:               "",
		HighModel:                   "",
		HighAgent:                   "",
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
		OllamaStructuredOutput:      false,
		OllamaBakedSystem:           false,
		OllamaResetHistory:          false,
		OllamaKeepAlive:             "",
		OllamaNumCtx:                0,
		OllamaTemperature:           -1,
		TranslatorResponseMode:      responseModePlain,
		PipelineVersion:             "chunkctx-v1",
	}
}
