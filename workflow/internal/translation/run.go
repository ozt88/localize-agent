package translation

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"localize-agent/workflow/pkg/platform"
	"localize-agent/workflow/pkg/shared"
)

func Run(c Config) int {
	if c.CheckpointDB == "" {
		fmt.Fprintln(os.Stderr, "error: --checkpoint-db is required (translator now persists work to DB only)")
		return 2
	}
	if c.ReviewExportOut != "" {
		return runReviewExport(c)
	}

	started := time.Now()
	files := platform.NewOSFileStore()

	enStrings, err := readStrings(files, c.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading source: %v\n", err)
		return 1
	}
	curStrings, err := readStrings(files, c.Current)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading current: %v\n", err)
		return 1
	}
	ids, err := readIDs(files, c.IDsFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading ids: %v\n", err)
		return 1
	}
	lineContexts, chunkBatches, err := loadChunkContexts(c.TranslatorPackageChunks, ids)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading translator package chunks: %v\n", err)
		return 1
	}
	idIndex := make(map[string]int, len(ids))
	for i, id := range ids {
		idIndex[id] = i
	}

	checkpoint, err := platform.NewTranslationCheckpointStore(c.CheckpointBackend, c.CheckpointDB, c.CheckpointDSN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening checkpoint: %v\n", err)
		return 1
	}
	defer checkpoint.Close()

	if c.UseCheckpointCurrent {
		if err := overlayCurrentStringsFromCheckpoint(c.CheckpointBackend, c.CheckpointDB, c.CheckpointDSN, curStrings); err != nil {
			fmt.Fprintf(os.Stderr, "error overlaying checkpoint current strings: %v\n", err)
			return 1
		}
	}
	checkpointMetas, err := loadCheckpointPromptMetas(c.CheckpointBackend, c.CheckpointDB, c.CheckpointDSN, ids)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading checkpoint prompt metadata: %v\n", err)
		return 1
	}
	glossaryEntries, err := loadGlossaryEntries(c.GlossaryFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading glossary: %v\n", err)
		return 1
	}
	loreEntries, err := loadLoreEntries(c.LoreFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: lore file not loaded: %v\n", err)
		loreEntries = nil // non-fatal: proceed without lore
	}
	if len(loreEntries) > 0 {
		fmt.Printf("Lore: loaded %d entries from %s\n", len(loreEntries), c.LoreFile)
	}

	doneFromCheckpoint := map[string]bool{}
	if c.Resume && checkpoint.IsEnabled() {
		doneFromCheckpoint, err = checkpoint.LoadDoneIDs(c.PipelineVersion)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading checkpoint: %v\n", err)
			return 1
		}
		fmt.Printf("Resume: loaded %d done items from checkpoint\n", len(doneFromCheckpoint))
	}

	var skill *translateSkill
	if c.OverlayMode {
		skill = newOverlayTranslateSkill(shared.LoadContext(c.ContextFiles), shared.LoadRules(c.RulesFile))
	} else {
		skill = newTranslateSkill(shared.LoadContext(c.ContextFiles), shared.LoadRules(c.RulesFile))
	}
	metrics := &shared.MetricCollector{}
	traceSink, err := platform.NewJSONLTraceSink(c.TraceOut)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening trace sink: %v\n", err)
		return 1
	}
	if traceSink != nil {
		defer traceSink.Close()
	}

	client, err := newServerClientWithConfig(c.LLMBackend, c.ServerURL, c.Model, c.Agent, c, skill, c.TimeoutSec, metrics, traceSink)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating client: %v\n", err)
		return 1
	}
	var highClient *serverClient
	if c.HighModel != "" {
		highBackend := c.HighLLMBackend
		if highBackend == "" {
			highBackend = c.LLMBackend
		}
		highServerURL := c.HighServerURL
		if highServerURL == "" {
			highServerURL = c.ServerURL
		}
		highAgent := c.HighAgent
		if highAgent == "" {
			highAgent = c.Agent
		}
		if highBackend != c.LLMBackend || highServerURL != c.ServerURL || c.HighModel != c.Model || highAgent != c.Agent {
			highClient, err = newServerClientWithConfig(highBackend, highServerURL, c.HighModel, highAgent, c, skill, c.TimeoutSec, metrics, traceSink)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error creating high lane client: %v\n", err)
				return 1
			}
		}
	}

	result := runPipeline(translationRuntime{
		cfg:                c,
		sourceStrings:      enStrings,
		currentStrings:     curStrings,
		ids:                ids,
		idIndex:            idIndex,
		lineContexts:       lineContexts,
		chunkBatches:       chunkBatches,
		doneFromCheckpoint: doneFromCheckpoint,
		retryReasons:       c.RetryReasons,
		checkpointMetas:    checkpointMetas,
		glossaryEntries:    glossaryEntries,
		loreEntries:        loreEntries,
		client:             client,
		highClient:         highClient,
		skill:              skill,
		checkpoint:         checkpoint,
	})

	calls, errs, avg, p50, p95 := metrics.Summary()
	elapsed := time.Since(started).Seconds()
	fmt.Printf("Summary: completed=%d skipped_timeout=%d skipped_invalid=%d skipped_translator_error=%d skipped_long=%d requested=%d\n",
		result.completedCount, result.skippedTimeout, result.skippedInvalid, result.skippedTranslatorErr, result.skippedLong, len(ids))
	fmt.Printf("Server metrics: calls=%d err=%d avg_ms=%.3f p50_ms=%.3f p95_ms=%.3f\n", calls, errs, avg, p50, p95)
	fmt.Printf("Elapsed: %.2fs (%.2fm)\n", elapsed, elapsed/60)
	fmt.Printf("DB checkpoint: %s (done=%d, skipped_long=%d)\n", c.CheckpointDB, result.completedCount, len(result.skippedLongIDs))
	if result.checkpointErr != nil {
		fmt.Fprintf(os.Stderr, "checkpoint flush error: %v\n", result.checkpointErr)
		return 1
	}
	return 0
}

func overlayCurrentStringsFromCheckpoint(backend string, dbPath string, dsn string, currentStrings map[string]map[string]any) error {
	if strings.TrimSpace(dbPath) == "" {
		return nil
	}
	db, err := platform.OpenTranslationCheckpointDB(backend, dbPath, dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	rows, err := db.Query(platform.RebindSQL(backend, "SELECT id, ko_json FROM items WHERE status='done'"))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var koJSONRaw any
		if err := rows.Scan(&id, &koJSONRaw); err != nil {
			return err
		}
		koJSON := platform.NormalizeSQLValue(koJSONRaw)
		if strings.TrimSpace(koJSON) == "" {
			continue
		}
		var koObj map[string]any
		if err := json.Unmarshal([]byte(koJSON), &koObj); err != nil {
			continue
		}
		if _, ok := koObj["Text"].(string); !ok {
			continue
		}
		currentStrings[id] = koObj
	}
	return rows.Err()
}
