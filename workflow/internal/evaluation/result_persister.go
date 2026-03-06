package evaluation

import (
	"fmt"
	"os"

	"localize-agent/workflow/internal/contracts"
)

func persistEvaluationOutcome(store contracts.EvalStore, slot int, id string, outcome itemOutcome) {
	if err := store.SaveResult(
		id,
		outcome.finalStatus,
		outcome.finalKO,
		outcome.finalRisk,
		outcome.finalNotes,
		outcome.revised,
		outcome.history,
	); err != nil {
		fmt.Fprintf(os.Stderr, "[slot=%d] saveResult %s: %v\n", slot, id, err)
	}
}
