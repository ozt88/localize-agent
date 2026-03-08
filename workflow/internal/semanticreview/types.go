package semanticreview

type Config struct {
	CheckpointDB string
	Mode         string
	ScoreOnly    bool
	LLMBackend   string
	ServerURL    string
	Model        string
	Agent        string
	ContextFiles []string
	Concurrency  int
	BatchSize    int
	TimeoutSec   int
	Limit        int
	OutputDir    string
	TraceOut     string
}

type ReviewItem struct {
	ID           string `json:"id"`
	SourceEN     string `json:"source_en"`
	TranslatedKO string `json:"translated_ko"`
	PrevEN       string `json:"prev_en,omitempty"`
	NextEN       string `json:"next_en,omitempty"`
	TextRole     string `json:"text_role,omitempty"`
	SpeakerHint  string `json:"speaker_hint,omitempty"`
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
	ReasonTags         []string `json:"reason_tags,omitempty"`
	ShortReason        string   `json:"short_reason,omitempty"`
}
