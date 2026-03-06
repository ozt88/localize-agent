package evaluation

import (
	"fmt"
	"os"

	"localize-agent/workflow/internal/contracts"
)

func prepareEvaluationWork(c Config, store contracts.EvalStore, files contracts.FileStore) ([]string, int) {
	if c.PackIn != "" {
		items, err := readPackItems(files, c.PackIn)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading pack-in: %v\n", err)
			return nil, 1
		}
		inserted, err := store.LoadPack(items)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading pack: %v\n", err)
			return nil, 1
		}
		fmt.Printf("Loaded pack: %d total, %d newly inserted\n", len(items), inserted)
	}

	if c.ReevalIDs != "" {
		n, err := store.ResetIDs(parseCSV(c.ReevalIDs))
		if err != nil {
			fmt.Fprintf(os.Stderr, "reeval reset error: %v\n", err)
			return nil, 1
		}
		fmt.Printf("Reset %d IDs -> pending for re-evaluation\n", n)
	}

	if c.Resume {
		n, err := store.ResetEvaluating()
		if err != nil {
			fmt.Fprintf(os.Stderr, "resume reset error: %v\n", err)
			return nil, 1
		}
		if n > 0 {
			fmt.Printf("Resume: recovered %d 'evaluating' items -> pending\n", n)
		}
	}

	pendingIDs, err := store.PendingIDs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading pending: %v\n", err)
		return nil, 1
	}
	fmt.Printf("Pending: %d items\n", len(pendingIDs))
	return pendingIDs, 0
}
