package evaluation

import (
	"fmt"
	"os"

	"localize-agent/workflow/internal/contracts"
	"localize-agent/workflow/internal/platform"
)

func Run(c Config) int {
	store, err := platform.NewSQLiteEvalStore(c.DB, c.RunName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening db: %v\n", err)
		return 1
	}
	defer store.Close()
	files := platform.NewOSFileStore()

	modeHandled, code := handleModes(c, store, files)
	if modeHandled {
		return code
	}
	return runEvaluationPipeline(c, store, files)
}

func handleModes(c Config, store contracts.EvalStore, files contracts.FileStore) (bool, int) {
	if c.StatusOnly {
		return true, runStatusMode(c, store)
	}
	if c.Export {
		return true, runExportMode(c, store, files)
	}
	if c.ReviewExportOut != "" {
		return true, runReviewExportMode(c, store, files)
	}
	if c.ResetStatus != "" {
		return true, runResetMode(c, store)
	}
	return false, 0
}
