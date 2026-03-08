package contracts

type TranslationCheckpointItem struct {
	EntryID    string
	Status     string
	SourceHash string
	Attempts   int
	LastError  string
	LatencyMs  float64
	KOObj      map[string]any
	PackObj    map[string]any
}

type TranslationCheckpointStore interface {
	IsEnabled() bool
	LoadDoneIDs(pipelineVersion string) (map[string]bool, error)
	UpsertItem(entryID, status, sourceHash string, attempts int, lastError string, latencyMs float64, koObj, packObj map[string]any) error
	UpsertItems(items []TranslationCheckpointItem) error
	Close() error
}
