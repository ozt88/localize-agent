package semanticreview

type Config struct {
	CheckpointBackend       string
	CheckpointDSN           string
	CheckpointDB            string
	SourcePath              string
	CurrentPath             string
	IDsFile                 string
	TranslatorPackageChunks string
	Mode                    string
	ScoreOnly               bool
	LLMBackend              string
	ServerURL               string
	Model                   string
	Agent                   string
	PromptVariant           string
	ContextFiles            []string
	Concurrency             int
	BatchSize               int
	TimeoutSec              int
	Limit                   int
	OutputDir               string
	TraceOut                string
}

type ReviewItem struct {
	ID           string `json:"id"`
	SourceEN     string `json:"source_en"`
	TranslatedKO string `json:"translated_ko"`
	CurrentKO    string `json:"current_ko,omitempty"`
	FreshKO      string `json:"fresh_ko,omitempty"`
	PrevEN       string `json:"prev_en,omitempty"`
	NextEN       string `json:"next_en,omitempty"`
	PrevKO       string `json:"prev_ko,omitempty"`
	NextKO       string `json:"next_ko,omitempty"`
	TextRole     string `json:"text_role,omitempty"`
	SpeakerHint  string `json:"speaker_hint,omitempty"`
	ContextEN    string `json:"context_en,omitempty"`
	RetryReason  string `json:"retry_reason,omitempty"`
}

type ReportItem struct {
	ID                 string   `json:"id"`
	SourceEN           string   `json:"source_en"`
	TranslatedKO       string   `json:"translated_ko"`
	BacktranslatedEN   string   `json:"backtranslated_en,omitempty"`
	ScoreSemantic      float64  `json:"score_semantic,omitempty"`
	ScoreLexical       float64  `json:"score_lexical,omitempty"`
	ScorePrevAlignment float64  `json:"score_prev_alignment,omitempty"`
	ScoreNextAlignment float64  `json:"score_next_alignment,omitempty"`
	ScoreFinal         float64  `json:"score_final"`
	CurrentScore       float64  `json:"current_score,omitempty"`
	FreshScore         float64  `json:"fresh_score,omitempty"`
	ReasonTags         []string `json:"reason_tags,omitempty"`
	ShortReason        string   `json:"short_reason,omitempty"`
	Winner             string   `json:"winner,omitempty"`
	ReplacementKO      string   `json:"replacement_ko,omitempty"`
}
