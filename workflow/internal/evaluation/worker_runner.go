package evaluation

import (
	"fmt"
	"os"
	"sync"

	"localize-agent/workflow/internal/contracts"
)

func runEvaluationWorkers(c Config, store contracts.EvalStore, client *evalClient, pendingIDs []string) {
	jobCh := make(chan string, c.Concurrency*2)
	var wg sync.WaitGroup

	for i := 0; i < c.Concurrency; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			slotKey := fmt.Sprintf("%s#eval_slot%d", c.ServerURL, slot)
			_ = client.ensureContext(slotKey, kindEval)
			_ = client.ensureContext(slotKey, kindTrans)

			for id := range jobCh {
				item, err := store.GetItem(id)
				if err != nil {
					fmt.Fprintf(os.Stderr, "[slot=%d] getItem %s: %v\n", slot, id, err)
					continue
				}
				_ = store.MarkEvaluating(id)
				outcome := runEvalItem(client, slotKey, item, c.MaxAttempts, c.BackoffSec, c.MaxRetry)
				persistEvaluationOutcome(store, slot, id, outcome)
				fmt.Printf("[slot=%d] id=%s status=%s revised=%v history_len=%d\n", slot, id, outcome.finalStatus, outcome.revised, len(outcome.history))
			}
		}(i + 1)
	}

	for _, id := range pendingIDs {
		jobCh <- id
	}
	close(jobCh)
	wg.Wait()
}
