package contracts

type EvalPackItem struct {
	ID                 string `json:"id"`
	EN                 string `json:"en"`
	CurrentKO          string `json:"current_ko"`
	ProposedKORestored string `json:"proposed_ko_restored"`
	Risk               string `json:"risk"`
	Notes              string `json:"notes"`
}

type EvalResult struct {
	ID          string   `json:"id"`
	Fidelity    int      `json:"fidelity"`
	Fluency     int      `json:"fluency"`
	Tone        int      `json:"tone"`
	Tags        int      `json:"tags"`
	Consistency int      `json:"consistency"`
	Verdict     string   `json:"verdict"`
	Issues      []string `json:"issues"`
}

type EvalStore interface {
	Close()
	LoadPack(items []EvalPackItem) (int, error)
	PendingIDs() ([]string, error)
	GetItem(id string) (*EvalPackItem, error)
	MarkEvaluating(id string) error
	SaveResult(id, status, finalKO, finalRisk, finalNotes string, revised bool, history []EvalResult) error
	ResetToStatus(statuses []string) (int, error)
	ResetIDs(ids []string) (int, error)
	ResetEvaluating() (int, error)
	StatusCounts() (map[string]int, error)
	ExportByStatus(statuses ...string) ([]map[string]any, error)
}
