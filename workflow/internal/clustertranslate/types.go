package clustertranslate

import "localize-agent/workflow/internal/inkparse"

// ClusterTask represents a batch of blocks to translate as a scene script.
type ClusterTask struct {
	Batch            inkparse.Batch
	PrevGateLines    []string          // last 3 lines of previous gate for [CONTEXT] per D-03
	GlossaryJSON     string            // per-batch glossary terms (excluding warmup)
	NextLines        []string          // next 3 lines EN context (CONT-01)
	PrevKO           []string          // prev 3 existing KO translations for retranslation (CONT-02)
	NextKO           []string          // next 3 existing KO translations for retranslation (CONT-02)
	VoiceCards       map[string]string // speaker -> voice guide text (TONE-02)
	ParentChoiceText string            // parent choice display text (BRANCH-01)
	RAGHints         string            // per-batch world-building RAG context (D-17)
}

// ClusterResult holds the parsed output from a cluster translation.
type ClusterResult struct {
	BatchID  string
	Lines    []TranslatedLine // parsed output lines
	RawOutput string
	Excluded []string // block IDs excluded (punctuation-only per D-13)
}

// TranslatedLine represents a single parsed line from LLM output.
type TranslatedLine struct {
	Number   int    // [NN] marker value
	Speaker  string // extracted speaker label (for formatter to strip)
	IsChoice bool   // had [CHOICE] marker
	Text     string // Korean translation text
}

// PromptMeta holds metadata about the constructed prompt for validation.
type PromptMeta struct {
	LineCount        int      // number of translatable lines (excluding excluded)
	ExcludedBlockIDs []string // block IDs excluded from prompt
	BlockIDOrder     []string // ordered block IDs matching line numbers
	EstimatedTokens  int      // rough token count approximation of prompt
}
