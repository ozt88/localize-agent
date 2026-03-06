package evaluation

import (
	"fmt"
	"os"
	"time"

	"localize-agent/workflow/internal/contracts"
	"localize-agent/workflow/internal/platform"
	"localize-agent/workflow/internal/shared"
)

func runEvaluationPipeline(c Config, store contracts.EvalStore, files contracts.FileStore) int {
	pendingIDs, prepCode := prepareEvaluationWork(c, store, files)
	if prepCode != 0 {
		return prepCode
	}
	if len(pendingIDs) == 0 {
		fmt.Println("No pending items. Use --status to check DB state.")
		return 0
	}
	fmt.Printf("Pending: %d items\n", len(pendingIDs))

	contextText := shared.LoadContext(c.ContextFiles)
	transSkill := newTranslateSkill(contextText, shared.LoadRules(c.RulesFile))
	evalSkill := newEvaluateSkill(contextText, shared.LoadRules(c.EvalRulesFile))
	metrics := &shared.MetricCollector{}
	traceSink, err := platform.NewJSONLTraceSink(c.TraceOut)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening trace sink: %v\n", err)
		return 1
	}
	if traceSink != nil {
		defer traceSink.Close()
	}

	client, err := newEvalClient(c.LLMBackend, c.ServerURL, c.TransModel, c.TransAgent, transSkill, c.EvalModel, c.EvalAgent, evalSkill, c.TimeoutSec, metrics, traceSink)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating client: %v\n", err)
		return 1
	}

	started := time.Now()
	runEvaluationWorkers(c, store, client, pendingIDs)

	counts, _ := store.StatusCounts()
	calls, errs, avg, p50, p95 := metrics.Summary()
	elapsed := time.Since(started).Seconds()
	fmt.Printf("\nDone. DB: %s\n", c.DB)
	fmt.Printf("Run: %s\n", c.RunName)
	fmt.Printf("  pass=%-5d revise=%-5d reject=%-5d pending=%-5d\n", counts[statusPass], counts[statusRevise], counts[statusReject], counts[statusPending])
	fmt.Printf("Server metrics: calls=%d err=%d avg_ms=%.1f p50_ms=%.1f p95_ms=%.1f\n", calls, errs, avg, p50, p95)
	fmt.Printf("Elapsed: %.2fs (%.2fm)\n", elapsed, elapsed/60)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  Status check : go run ./workflow/cmd/go-evaluate --db %s --status\n", c.DB)
	fmt.Printf("  Export JSON  : go run ./workflow/cmd/go-evaluate --db %s --export\n", c.DB)
	if counts[statusRevise] > 0 || counts[statusReject] > 0 {
		fmt.Printf("  Reset revise : go run ./workflow/cmd/go-evaluate --db %s --reset-status revise\n", c.DB)
	}
	return 0
}
