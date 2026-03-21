package translationpipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"localize-agent/workflow/pkg/platform"
	"localize-agent/workflow/internal/semanticreview"
	"localize-agent/workflow/pkg/shared"
	"localize-agent/workflow/internal/translation"
)

const scoreFlushInterval = 5 * time.Second

type workerPhaseLogger struct {
	path string
	mu   sync.Mutex
}

func newWorkerPhaseLogger(rootDir string, role string, workerID string) *workerPhaseLogger {
	if strings.TrimSpace(rootDir) == "" || strings.TrimSpace(role) == "" || strings.TrimSpace(workerID) == "" {
		return nil
	}
	dir := filepath.Join(rootDir, "run_logs", "pipeline_heartbeats")
	_ = os.MkdirAll(dir, 0o755)
	name := fmt.Sprintf("%s__%s__%d.jsonl", role, workerID, os.Getpid())
	return &workerPhaseLogger{path: filepath.Join(dir, name)}
}

func (l *workerPhaseLogger) Log(phase string, details map[string]any) {
	if l == nil {
		return
	}
	payload := map[string]any{
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"phase":     phase,
	}
	for k, v := range details {
		payload[k] = v
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(raw, '\n'))
}

func Run(cfg Config) int {
	projectCfg, _, err := shared.LoadProjectConfig(cfg.Project, cfg.ProjectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pipeline project load error: %v\n", err)
		return 2
	}
	if projectCfg == nil {
		fmt.Fprintln(os.Stderr, "pipeline project load error: project config required")
		return 2
	}
	applyProjectPipelineDefaults(projectCfg, &cfg)

	ids, err := readIDsFile(projectCfg.Translation.IDsFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pipeline ids load error: %v\n", err)
		return 1
	}
	sourceMap, err := readSourceTextMap(projectCfg.Translation.Source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pipeline source load error: %v\n", err)
		return 1
	}

	dbPath := projectCfg.Translation.CheckpointDB
	if cfg.CheckpointDB != "" {
		dbPath = cfg.CheckpointDB
	}
	checkpointBackend := firstNonEmpty(cfg.CheckpointBackend, projectCfg.Translation.CheckpointBackend)
	checkpointDSN := firstNonEmpty(cfg.CheckpointDSN, projectCfg.Translation.CheckpointDSN)
	store, err := OpenStore(checkpointBackend, dbPath, checkpointDSN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pipeline store open error: %v\n", err)
		return 1
	}
	defer store.Close()

	if cfg.Reset {
		if err := store.Reset(); err != nil {
			fmt.Fprintf(os.Stderr, "pipeline reset error: %v\n", err)
			return 1
		}
	}
	if cfg.ResetScoring {
		n, err := store.ResetScoringState()
		if err != nil {
			fmt.Fprintf(os.Stderr, "pipeline scoring reset error: %v\n", err)
			return 1
		}
		fmt.Printf("Pipeline scoring reset: rows=%d\n", n)
	}
	if cfg.CleanupStaleClaims {
		summary, err := store.CleanupStaleClaims()
		if err != nil {
			fmt.Fprintf(os.Stderr, "pipeline stale-claim cleanup error: %v\n", err)
			return 1
		}
		fmt.Printf("Pipeline stale-claim cleanup complete: translate=%d score=%d retranslate=%d\n",
			summary.Translate, summary.Score, summary.Retranslate,
		)
		return 0
	}
	if cfg.RepairBlockedTranslate {
		summary, err := store.RepairBlockedTranslate(cfg.RequeueLimit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pipeline blocked-translate repair error: %v\n", err)
			return 1
		}
		fmt.Printf("Pipeline blocked-translate repair complete: released=%d still_blocked=%d\n", summary.Released, summary.StillBlocked)
		return 0
	}
	if cfg.RouteKnownFailedNoRow {
		summary, err := store.RouteKnownFailedNoDoneRow(cfg.RequeueLimit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pipeline failed-router error: %v\n", err)
			return 1
		}
		fmt.Printf("Pipeline failed-router complete: total=%d open_quote_other=%d action_open_quote=%d stat_like_open_quote=%d long_dialogue=%d expository=%d passthrough=%d current_rescue=%d\n",
			summary.Total,
			summary.OpenQuoteOther,
			summary.ActionOpenQuote,
			summary.StatLikeOpenQuote,
			summary.LongDialogue,
			summary.Expository,
			summary.Passthrough,
			summary.CurrentRescue,
		)
		return 0
	}
	if cfg.RouteOverlayUI {
		n, err := store.RouteOverlayUI(cfg.RequeueLimit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pipeline overlay-route error: %v\n", err)
			return 1
		}
		fmt.Printf("Pipeline overlay-route complete: routed=%d\n", n)
		return 0
	}

	role := normalizeWorkerRole(cfg.WorkerRole)
	shouldSeed := cfg.Reset || (!cfg.ResetScoring && (role == "" || role == "all" || cfg.InitOnly))
	if shouldSeed {
		if err := store.Seed(ids, cfg.SeedLimit); err != nil {
			fmt.Fprintf(os.Stderr, "pipeline seed error: %v\n", err)
			return 1
		}
	}
	if cfg.RequeueFailedNoRow {
		n, err := store.RequeueFailedNoDoneRow(cfg.RequeueLimit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pipeline requeue error: %v\n", err)
			return 1
		}
		fmt.Printf("Pipeline requeue complete: requeued=%d\n", n)
		return 0
	}
	if cfg.RequeueTranslateNoRowAsRetranslate {
		n, err := store.RequeueTranslateNoDoneRowAsRetranslate(cfg.RequeueLimit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pipeline requeue-to-retranslate error: %v\n", err)
			return 1
		}
		fmt.Printf("Pipeline requeue-to-retranslate complete: requeued=%d\n", n)
		return 0
	}
	if cfg.InitOnly {
		counts, err := store.CountStates()
		if err != nil {
			fmt.Fprintf(os.Stderr, "pipeline count error: %v\n", err)
			return 1
		}
		fmt.Printf("Pipeline init complete: pending_translate=%d pending_overlay_translate=%d pending_score=%d blocked_score=%d done=%d failed=%d\n",
			counts[StatePendingTranslate], counts[StatePendingOverlayTranslate], counts[StatePendingScore], counts[StateBlockedScore], counts[StateDone], counts[StateFailed],
		)
		return 0
	}
	if cfg.WorkerID == "" {
		role := cfg.WorkerRole
		if role == "" {
			role = "all"
		}
		cfg.WorkerID = role + "-" + strconv.Itoa(os.Getpid())
	}
	if cfg.LeaseSec <= 0 {
		cfg.LeaseSec = 300
	}
	if cfg.IdleSleepSec <= 0 {
		cfg.IdleSleepSec = 2
	}

	baseTransCfg := buildTranslationConfig(projectCfg, dbPath)
	failedRemediationCfg := buildFailedRemediationTranslationConfig(projectCfg, cfg, dbPath)
	overlayTransCfg := buildOverlayTranslationConfig(projectCfg, cfg, dbPath)
	retryTransCfg := buildRetryTranslationConfig(projectCfg, cfg, dbPath)
	scoreCfg := buildScoreConfig(projectCfg, cfg, dbPath)
	var scoreRunner *semanticreview.DirectReviewRunner
	if role == "" || role == "all" || role == "score" {
		scoreRunner, err = semanticreview.NewDirectReviewRunner(scoreCfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pipeline score runner init error: %v\n", err)
			return 1
		}
		defer scoreRunner.Close()
	}
	leaseDuration := time.Duration(cfg.LeaseSec) * time.Second
	idleSleep := time.Duration(cfg.IdleSleepSec) * time.Second
	if role != "" && role != "all" {
		return runDedicatedWorker(role, cfg, store, dbPath, sourceMap, baseTransCfg, failedRemediationCfg, overlayTransCfg, retryTransCfg, scoreCfg, scoreRunner, leaseDuration, idleSleep)
	}

	for {
		processed, err := processNextAvailable(cfg, store, dbPath, sourceMap, baseTransCfg, failedRemediationCfg, overlayTransCfg, retryTransCfg, scoreCfg, scoreRunner, leaseDuration)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pipeline process error: %v\n", err)
			return 1
		}
		if processed {
			continue
		}
		break
	}

	counts, err := store.CountStates()
	if err != nil {
		fmt.Fprintf(os.Stderr, "pipeline count error: %v\n", err)
		return 1
	}
	fmt.Printf("Pipeline summary: pending_translate=%d pending_overlay_translate=%d working_overlay_translate=%d pending_score=%d blocked_score=%d done=%d failed=%d\n",
		counts[StatePendingTranslate], counts[StatePendingOverlayTranslate], counts[StateWorkingOverlayTranslate], counts[StatePendingScore], counts[StateBlockedScore], counts[StateDone], counts[StateFailed],
	)
	return 0
}

func runDedicatedWorker(role string, cfg Config, store *Store, dbPath string, sourceMap map[string]string, baseTransCfg translation.Config, failedRemediationCfg translation.Config, overlayTransCfg translation.Config, retryTransCfg translation.Config, scoreCfg semanticreview.Config, scoreRunner *semanticreview.DirectReviewRunner, leaseDuration, idleSleep time.Duration) int {
	switch role {
	case "translate":
		logTranslationStageConfig(role, baseTransCfg)
		if err := validateTranslationStageConfig(role, baseTransCfg); err != nil {
			fmt.Fprintf(os.Stderr, "pipeline %s config error: %v\n", role, err)
			return 1
		}
		if _, err := store.RequeueClaimsByWorker(StateWorkingTranslate, StatePendingTranslate, cfg.WorkerID); err != nil {
			fmt.Fprintf(os.Stderr, "pipeline %s worker startup cleanup error: %v\n", role, err)
			return 1
		}
	case "failed-translate":
		logTranslationStageConfig(role, failedRemediationCfg)
		if err := validateTranslationStageConfig(role, failedRemediationCfg); err != nil {
			fmt.Fprintf(os.Stderr, "pipeline %s config error: %v\n", role, err)
			return 1
		}
		if _, err := store.RequeueClaimsByWorker(StateWorkingFailedTranslate, StatePendingFailedTranslate, cfg.WorkerID); err != nil {
			fmt.Fprintf(os.Stderr, "pipeline %s worker startup cleanup error: %v\n", role, err)
			return 1
		}
	case "overlay-translate":
		logTranslationStageConfig(role, overlayTransCfg)
		if err := validateTranslationStageConfig(role, overlayTransCfg); err != nil {
			fmt.Fprintf(os.Stderr, "pipeline %s config error: %v\n", role, err)
			return 1
		}
		if _, err := store.RequeueClaimsByWorker(StateWorkingOverlayTranslate, StatePendingOverlayTranslate, cfg.WorkerID); err != nil {
			fmt.Fprintf(os.Stderr, "pipeline %s worker startup cleanup error: %v\n", role, err)
			return 1
		}
	case "score":
		if _, err := store.RequeueClaimsByWorker(StateWorkingScore, StatePendingScore, cfg.WorkerID); err != nil {
			fmt.Fprintf(os.Stderr, "pipeline %s worker startup cleanup error: %v\n", role, err)
			return 1
		}
	case "retranslate":
		logTranslationStageConfig(role, retryTransCfg)
		if err := validateTranslationStageConfig(role, retryTransCfg); err != nil {
			fmt.Fprintf(os.Stderr, "pipeline %s config error: %v\n", role, err)
			return 1
		}
		if _, err := store.RequeueClaimsByWorker(StateWorkingRetranslate, StatePendingRetranslate, cfg.WorkerID); err != nil {
			fmt.Fprintf(os.Stderr, "pipeline %s worker startup cleanup error: %v\n", role, err)
			return 1
		}
	}
	for {
		processed, err := processRole(role, cfg, store, dbPath, sourceMap, baseTransCfg, failedRemediationCfg, overlayTransCfg, retryTransCfg, scoreCfg, scoreRunner, leaseDuration)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pipeline %s worker error: %v\n", role, err)
			return 1
		}
		if cfg.Once && processed {
			break
		}
		if processed {
			continue
		}
		if cfg.Once {
			break
		}
		time.Sleep(idleSleep)
	}
	return 0
}

func logTranslationStageConfig(role string, cfg translation.Config) {
	fmt.Printf("pipeline %s config: backend=%s server=%s model=%s agent=%s batch=%d concurrency=%d use_checkpoint_current=%t response_mode=%s\n",
		role,
		cfg.LLMBackend,
		cfg.ServerURL,
		cfg.Model,
		cfg.Agent,
		cfg.BatchSize,
		cfg.Concurrency,
		cfg.UseCheckpointCurrent,
		cfg.TranslatorResponseMode,
	)
}

func validateTranslationStageConfig(role string, cfg translation.Config) error {
	backend := strings.TrimSpace(cfg.LLMBackend)
	serverURL := strings.TrimSpace(cfg.ServerURL)
	model := strings.TrimSpace(cfg.Model)
	if backend == "" {
		return fmt.Errorf("%s missing llm backend", role)
	}
	if serverURL == "" {
		return fmt.Errorf("%s missing server url", role)
	}
	if model == "" {
		return fmt.Errorf("%s missing model", role)
	}
	return nil
}

func processNextAvailable(cfg Config, store *Store, dbPath string, sourceMap map[string]string, baseTransCfg translation.Config, failedRemediationCfg translation.Config, overlayTransCfg translation.Config, retryTransCfg translation.Config, scoreCfg semanticreview.Config, scoreRunner *semanticreview.DirectReviewRunner, leaseDuration time.Duration) (bool, error) {
	for _, role := range []string{"score", "retranslate", "overlay-translate", "failed-translate", "translate"} {
		processed, err := processRole(role, cfg, store, dbPath, sourceMap, baseTransCfg, failedRemediationCfg, overlayTransCfg, retryTransCfg, scoreCfg, scoreRunner, leaseDuration)
		if err != nil {
			return false, err
		}
		if processed {
			return true, nil
		}
	}
	return false, nil
}

func processRole(role string, cfg Config, store *Store, dbPath string, sourceMap map[string]string, baseTransCfg translation.Config, failedRemediationCfg translation.Config, overlayTransCfg translation.Config, retryTransCfg translation.Config, scoreCfg semanticreview.Config, scoreRunner *semanticreview.DirectReviewRunner, leaseDuration time.Duration) (bool, error) {
	switch role {
	case "translate":
		if _, err := store.RequeueExpiredWorking(StateWorkingTranslate, StatePendingTranslate); err != nil {
			return false, err
		}
		startedAt := time.Now()
		rows, err := store.ClaimPending(StatePendingTranslate, StateWorkingTranslate, cfg.WorkerID, cfg.StageBatchSize, leaseDuration)
		if err != nil {
			return false, err
		}
		if len(rows) == 0 {
			return false, nil
		}
		ids := pipelineIDs(rows)
		stopHeartbeat := startLeaseHeartbeat(store, ids, StateWorkingTranslate, cfg.WorkerID, leaseDuration)
		stageCfg := baseTransCfg
		stageCfg.TraceOut = platform.BuildLLMTracePath(
			cfg.TraceOutDir,
			filepath.Join(filepath.Dir(dbPath), "run_logs", "llm_traces"),
			"translate",
			cfg.WorkerID,
		)
		err = runTranslationStage(stageCfg, ids)
		stopHeartbeat()
		if err != nil {
			return false, err
		}
		if err := store.ResolveAfterTranslate(ids, sourceMap, false, cfg.WorkerID); err != nil {
			return false, err
		}
		if err := store.RecordWorkerBatchStat(cfg.WorkerID, role, len(ids), startedAt, time.Now()); err != nil {
			return false, err
		}
		return true, nil
	case "failed-translate":
		if _, err := store.RequeueExpiredWorking(StateWorkingFailedTranslate, StatePendingFailedTranslate); err != nil {
			return false, err
		}
		startedAt := time.Now()
		rows, err := store.ClaimPending(StatePendingFailedTranslate, StateWorkingFailedTranslate, cfg.WorkerID, cfg.StageBatchSize, leaseDuration)
		if err != nil {
			return false, err
		}
		if len(rows) == 0 {
			return false, nil
		}
		ids := pipelineIDs(rows)
		stopHeartbeat := startLeaseHeartbeat(store, ids, StateWorkingFailedTranslate, cfg.WorkerID, leaseDuration)
		stageCfg := failedRemediationCfg
		stageCfg.TraceOut = platform.BuildLLMTracePath(
			cfg.TraceOutDir,
			filepath.Join(filepath.Dir(dbPath), "run_logs", "llm_traces"),
			"failed-translate",
			cfg.WorkerID,
		)
		err = runTranslationStage(stageCfg, ids)
		stopHeartbeat()
		if err != nil {
			return false, err
		}
		if err := store.ResolveAfterFailedTranslate(ids, sourceMap, cfg.WorkerID); err != nil {
			return false, err
		}
		if err := store.RecordWorkerBatchStat(cfg.WorkerID, role, len(ids), startedAt, time.Now()); err != nil {
			return false, err
		}
		return true, nil
	case "overlay-translate":
		summary, err := store.ApplyPreservePolicy(cfg.StageBatchSize)
		if err != nil {
			return false, err
		}
		if summary.Applied > 0 {
			return true, nil
		}
		if _, err := store.RequeueExpiredWorking(StateWorkingOverlayTranslate, StatePendingOverlayTranslate); err != nil {
			return false, err
		}
		startedAt := time.Now()
		rows, err := store.ClaimPending(StatePendingOverlayTranslate, StateWorkingOverlayTranslate, cfg.WorkerID, cfg.StageBatchSize, leaseDuration)
		if err != nil {
			return false, err
		}
		if len(rows) == 0 {
			return false, nil
		}
		ids := pipelineIDs(rows)
		stopHeartbeat := startLeaseHeartbeat(store, ids, StateWorkingOverlayTranslate, cfg.WorkerID, leaseDuration)
		stageCfg := overlayTransCfg
		stageCfg.TraceOut = platform.BuildLLMTracePath(
			cfg.TraceOutDir,
			filepath.Join(filepath.Dir(dbPath), "run_logs", "llm_traces"),
			"overlay-translate",
			cfg.WorkerID,
		)
		err = runTranslationStage(stageCfg, ids)
		stopHeartbeat()
		if err != nil {
			return false, err
		}
		if err := store.ResolveAfterOverlayTranslate(ids, sourceMap, cfg.WorkerID); err != nil {
			return false, err
		}
		if err := store.RecordWorkerBatchStat(cfg.WorkerID, role, len(ids), startedAt, time.Now()); err != nil {
			return false, err
		}
		return true, nil
	case "score":
		if _, err := store.RequeueExpiredWorking(StateWorkingScore, StatePendingScore); err != nil {
			return false, err
		}
		phaseLog := newWorkerPhaseLogger(filepath.Dir(dbPath), "score", cfg.WorkerID)
		startedAt := time.Now()
		rows, err := store.ClaimPending(StatePendingScore, StateWorkingScore, cfg.WorkerID, cfg.StageBatchSize, leaseDuration)
		if err != nil {
			return false, err
		}
		if len(rows) == 0 {
			phaseLog.Log("claim_empty", map[string]any{
				"stage_batch_size": cfg.StageBatchSize,
			})
			return false, nil
		}
		ids := pipelineIDs(rows)
		phaseLog.Log("claim_acquired", map[string]any{
			"count":            len(ids),
			"first_id":         ids[0],
			"last_id":          ids[len(ids)-1],
			"stage_batch_size": cfg.StageBatchSize,
		})
		stopHeartbeat := startLeaseHeartbeat(store, ids, StateWorkingScore, cfg.WorkerID, leaseDuration)
		phaseLog.Log("load_done_items_start", map[string]any{
			"count": len(ids),
		})
		items, err := semanticreview.LoadDoneItemsByIDs(scoreCfg, ids)
		if err != nil {
			phaseLog.Log("load_done_items_error", map[string]any{
				"count": len(ids),
				"error": err.Error(),
			})
			stopHeartbeat()
			return false, err
		}
		phaseLog.Log("load_done_items_done", map[string]any{
			"count":       len(items),
			"claimed_ids": len(ids),
		})
		loadedIDs := make([]string, 0, len(items))
		for _, item := range items {
			loadedIDs = append(loadedIDs, item.ID)
		}
		if len(items) < len(ids) {
			recoverySummary, err := store.RecoverUnscoreableWorkingScore(ids, loadedIDs, cfg.WorkerID)
			if err != nil {
				phaseLog.Log("recover_unscoreable_score_error", map[string]any{
					"claimed_ids": len(ids),
					"loaded_ids":  len(items),
					"error":       err.Error(),
				})
				stopHeartbeat()
				return false, err
			}
			phaseLog.Log("recover_unscoreable_score_done", map[string]any{
				"claimed_ids":              len(ids),
				"loaded_ids":               len(items),
				"to_pending_retranslate":   recoverySummary.ToPendingRetranslate,
				"to_pending_translate":     recoverySummary.ToPendingTranslate,
				"to_failed":                recoverySummary.ToFailed,
			})
		}
		if len(items) == 0 {
			stopHeartbeat()
			phaseLog.Log("heartbeat_stop", map[string]any{
				"count": len(ids),
			})
			return true, nil
		}
		rowByID := make(map[string]PipelineItem, len(rows))
		for _, row := range rows {
			rowByID[row.ID] = row
		}
		reviewBatchSize := max(1, scoreCfg.BatchSize)
		flushLimit := reviewBatchSize
		if flushLimit < 1 {
			flushLimit = 1
		}
		pendingRows := make([]PipelineItem, 0, flushLimit)
		pendingReports := make(map[string]ScoreResult, flushLimit)
		lastFlush := time.Now()
		flush := func() error {
			if len(pendingRows) == 0 {
				return nil
			}
			phaseLog.Log("apply_scores_start", map[string]any{
				"count": len(pendingRows),
			})
			if err := store.ApplyScores(pendingRows, pendingReports, cfg.Threshold, cfg.MaxRetries, cfg.WorkerID); err != nil {
				phaseLog.Log("apply_scores_error", map[string]any{
					"count": len(pendingRows),
					"error": err.Error(),
				})
				return err
			}
			phaseLog.Log("apply_scores_done", map[string]any{
				"count": len(pendingRows),
			})
			pendingRows = pendingRows[:0]
			pendingReports = make(map[string]ScoreResult, flushLimit)
			lastFlush = time.Now()
			return nil
		}
		for i := 0; i < len(items); i += reviewBatchSize {
			end := i + reviewBatchSize
			if end > len(items) {
				end = len(items)
			}
			batch := items[i:end]
			if scoreRunner == nil {
				phaseLog.Log("review_items_error", map[string]any{
					"count": len(batch),
					"error": "score runner is nil",
				})
				stopHeartbeat()
				_, _ = store.RequeueClaimedIDsByWorker(pipelineIDs(rows), StateWorkingScore, StatePendingScore, cfg.WorkerID, "score runner is nil")
				return false, fmt.Errorf("score runner is nil")
			}
			phaseLog.Log("review_items_start", map[string]any{
				"count":    len(batch),
				"first_id": batch[0].ID,
				"last_id":  batch[len(batch)-1].ID,
			})
			report, err := scoreRunner.ReviewItems(batch)
			if err != nil {
				phaseLog.Log("review_items_error", map[string]any{
					"count":    len(batch),
					"first_id": batch[0].ID,
					"last_id":  batch[len(batch)-1].ID,
					"error":    err.Error(),
				})
				stopHeartbeat()
				_, _ = store.RequeueClaimedIDsByWorker(pipelineIDs(rows), StateWorkingScore, StatePendingScore, cfg.WorkerID, err.Error())
				return false, err
			}
			phaseLog.Log("review_items_done", map[string]any{
				"count":        len(batch),
				"reported":     len(report),
				"first_id":     batch[0].ID,
				"last_id":      batch[len(batch)-1].ID,
				"pending_rows": len(pendingRows),
			})
			for _, item := range report {
				row, ok := rowByID[item.ID]
				if !ok {
					continue
				}
				pendingRows = append(pendingRows, row)
				pendingReports[item.ID] = ScoreResult{
					CurrentScore: item.CurrentScore,
					FreshScore:   item.FreshScore,
					ScoreFinal:   item.ScoreFinal,
					ReasonTags:   append([]string(nil), item.ReasonTags...),
					ShortReason:  item.ShortReason,
				}
			}
			if len(pendingRows) >= flushLimit || time.Since(lastFlush) >= scoreFlushInterval {
				if err := flush(); err != nil {
					stopHeartbeat()
					return false, err
				}
			}
		}
		stopHeartbeat()
		phaseLog.Log("heartbeat_stop", map[string]any{
			"count": len(ids),
		})
		if err := flush(); err != nil {
			return false, err
		}
		if err := store.RecordWorkerBatchStat(cfg.WorkerID, role, len(ids), startedAt, time.Now()); err != nil {
			phaseLog.Log("record_batch_stat_error", map[string]any{
				"count": len(ids),
				"error": err.Error(),
			})
			return false, err
		}
		phaseLog.Log("record_batch_stat_done", map[string]any{
			"count": len(ids),
		})
		return true, nil
	case "retranslate":
		if _, err := store.RequeueExpiredWorking(StateWorkingRetranslate, StatePendingRetranslate); err != nil {
			return false, err
		}
		startedAt := time.Now()
		rows, err := store.ClaimPending(StatePendingRetranslate, StateWorkingRetranslate, cfg.WorkerID, cfg.StageBatchSize, leaseDuration)
		if err != nil {
			return false, err
		}
		if len(rows) == 0 {
			return false, nil
		}
		ids := pipelineIDs(rows)
		stopHeartbeat := startLeaseHeartbeat(store, ids, StateWorkingRetranslate, cfg.WorkerID, leaseDuration)
		stageCfg := retryTransCfg
		stageCfg.TraceOut = platform.BuildLLMTracePath(
			cfg.TraceOutDir,
			filepath.Join(filepath.Dir(dbPath), "run_logs", "llm_traces"),
			"retranslate",
			cfg.WorkerID,
		)
		err = runTranslationStage(stageCfg, ids)
		stopHeartbeat()
		if err != nil {
			return false, err
		}
		if err := store.ResolveAfterTranslate(ids, sourceMap, true, cfg.WorkerID); err != nil {
			return false, err
		}
		if err := store.RecordWorkerBatchStat(cfg.WorkerID, role, len(ids), startedAt, time.Now()); err != nil {
			return false, err
		}
		return true, nil
	default:
		return false, fmt.Errorf("unknown worker role %q", role)
	}
}

func startLeaseHeartbeat(store *Store, ids []string, workingState string, workerID string, leaseDuration time.Duration) func() {
	if len(ids) == 0 || workerID == "" || leaseDuration <= 0 {
		return func() {}
	}
	interval := leaseDuration / 3
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	if interval > 60*time.Second {
		interval = 60 * time.Second
	}
	ticker := time.NewTicker(interval)
	done := make(chan struct{})
	var once sync.Once
	go func() {
		for {
			select {
			case <-ticker.C:
				_, _ = store.ExtendLease(ids, workingState, workerID, leaseDuration)
			case <-done:
				ticker.Stop()
				return
			}
		}
	}()
	return func() {
		once.Do(func() {
			close(done)
		})
	}
}

func normalizeWorkerRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "", "all":
		return strings.ToLower(strings.TrimSpace(role))
	case "translate", "failed-translate", "overlay-translate", "score", "retranslate":
		return strings.ToLower(strings.TrimSpace(role))
	default:
		return strings.ToLower(strings.TrimSpace(role))
	}
}

func pipelineIDs(rows []PipelineItem) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ID)
	}
	return out
}

func buildTranslationConfig(projectCfg *shared.ProjectConfig, checkpointDB string) translation.Config {
	cfg := translation.DefaultConfig()
	t := projectCfg.Translation
	cfg.CheckpointBackend = t.CheckpointBackend
	cfg.CheckpointDSN = t.CheckpointDSN
	cfg.Source = t.Source
	cfg.Current = t.Current
	cfg.IDsFile = t.IDsFile
	cfg.TranslatorPackageChunks = t.TranslatorPackageChunks
	cfg.CheckpointDB = checkpointDB
	cfg.RulesFile = t.RulesFile
	cfg.ContextFiles = append(cfg.ContextFiles[:0], t.ContextFiles...)
	cfg.GlossaryFile = firstExistingPath(
		"projects/esoteric-ebb/patch/output/source_overlay_analysis/esoteric_ebb_glossary_user_merged_20260316.json",
		"projects/esoteric-ebb/patch/output/source_overlay_analysis/esoteric_ebb_glossary_curated_20260316.json",
		"projects/esoteric-ebb/patch/output/source_overlay_analysis/spell_item_glossary_curated_20260315.json",
	)
	cfg.LoreFile = t.LoreFile
	if t.LoreMaxHints > 0 {
		cfg.LoreMaxHints = t.LoreMaxHints
	}
	cfg.ServerURL = t.ServerURL
	cfg.Model = t.Model
	cfg.LLMBackend = t.LLMBackend
	cfg.TranslatorResponseMode = t.TranslatorResponseMode
	cfg.OllamaStructuredOutput = t.OllamaStructuredOutput
	cfg.OllamaBakedSystem = t.OllamaBakedSystem
	cfg.OllamaResetHistory = t.OllamaResetHistory
	cfg.OllamaKeepAlive = t.OllamaKeepAlive
	cfg.OllamaNumCtx = t.OllamaNumCtx
	cfg.OllamaTemperature = t.OllamaTemperature
	cfg.Resume = false
	cfg.UseCheckpointCurrent = false
	applyLLMProfileToTranslationConfig(&cfg, projectCfg.Pipeline.LowLLM)
	return cfg
}

func firstExistingPath(paths ...string) string {
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func buildRetryTranslationConfig(projectCfg *shared.ProjectConfig, cfg Config, checkpointDB string) translation.Config {
	out := buildTranslationConfig(projectCfg, checkpointDB)
	out.UseCheckpointCurrent = true
	applyLLMProfileToTranslationConfig(&out, projectCfg.Pipeline.HighLLM)
	if strings.TrimSpace(cfg.RetranslateBackend) != "" {
		out.LLMBackend = cfg.RetranslateBackend
	}
	if strings.TrimSpace(cfg.RetranslateServerURL) != "" {
		out.ServerURL = cfg.RetranslateServerURL
	}
	if strings.TrimSpace(cfg.RetranslateModel) != "" {
		out.Model = cfg.RetranslateModel
	}
	if strings.TrimSpace(cfg.RetranslateAgent) != "" {
		out.Agent = cfg.RetranslateAgent
	}
	return out
}

func buildFailedRemediationTranslationConfig(projectCfg *shared.ProjectConfig, cfg Config, checkpointDB string) translation.Config {
	out := buildTranslationConfig(projectCfg, checkpointDB)
	out.UseCheckpointCurrent = true
	applyLLMProfileToTranslationConfig(&out, projectCfg.Pipeline.HighLLM)
	out.Concurrency = 1
	out.BatchSize = 1
	// Failed-remediation needs to actually attempt long explanatory rows instead of dropping them at the plain-length gate.
	if out.MaxPlainLen < 1200 {
		out.MaxPlainLen = 1200
	}
	return out
}

func buildOverlayTranslationConfig(projectCfg *shared.ProjectConfig, cfg Config, checkpointDB string) translation.Config {
	out := buildTranslationConfig(projectCfg, checkpointDB)
	out.UseCheckpointCurrent = false
	out.OverlayMode = true
	applyLLMProfileToTranslationConfig(&out, projectCfg.Pipeline.HighLLM)
	out.Concurrency = 2
	out.BatchSize = 8
	if out.MaxPlainLen < 1200 {
		out.MaxPlainLen = 1200
	}
	return out
}

func buildScoreConfig(projectCfg *shared.ProjectConfig, cfg Config, dbPath string) semanticreview.Config {
	contextFiles := []string{"projects/esoteric-ebb/context/esoteric_ebb_semantic_review_system.md"}
	if len(projectCfg.Pipeline.ScoreLLM.ContextFiles) > 0 {
		contextFiles = append([]string(nil), projectCfg.Pipeline.ScoreLLM.ContextFiles...)
	}
	traceOut := platform.BuildLLMTracePath(
		cfg.TraceOutDir,
		filepath.Join(filepath.Dir(dbPath), "run_logs", "llm_traces"),
		"score",
		cfg.WorkerID,
	)
	out := semanticreview.Config{
		CheckpointBackend:       projectCfg.Translation.CheckpointBackend,
		CheckpointDSN:           projectCfg.Translation.CheckpointDSN,
		CheckpointDB:            dbPath,
		SourcePath:              projectCfg.Translation.Source,
		CurrentPath:             projectCfg.Translation.Current,
		IDsFile:                 projectCfg.Translation.IDsFile,
		TranslatorPackageChunks: projectCfg.Translation.TranslatorPackageChunks,
		Mode:                    "direct",
		ScoreOnly:               false,
		LLMBackend:              cfg.ScoreBackend,
		ServerURL:               cfg.ScoreServerURL,
		Model:                   cfg.ScoreModel,
		Agent:                   cfg.ScoreAgent,
		PromptVariant:           cfg.ScorePromptVariant,
		ContextFiles:            contextFiles,
		Concurrency:             4,
		BatchSize:               20,
		TimeoutSec:              120,
		OutputDir:               filepath.Join("projects", "esoteric-ebb", "output", "translation_pipeline_score_tmp"),
		TraceOut:                traceOut,
	}
	if cfg.ScoreConcurrency > 0 {
		out.Concurrency = cfg.ScoreConcurrency
	}
	if cfg.ScoreBatchSize > 0 {
		out.BatchSize = cfg.ScoreBatchSize
	}
	if strings.EqualFold(strings.TrimSpace(out.PromptVariant), "ultra") && out.BatchSize > 4 {
		out.BatchSize = 4
	}
	if cfg.ScoreTimeoutSec > 0 {
		out.TimeoutSec = cfg.ScoreTimeoutSec
	}
	return out
}

func runTranslationStage(cfg translation.Config, ids []string) error {
	tempFile, err := os.CreateTemp("", "translation-pipeline-ids-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()
	if _, err := tempFile.WriteString(strings.Join(ids, "\n")); err != nil {
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	stageCfg := cfg
	stageCfg.IDsFile = tempFile.Name()
	stageCfg.ReviewExportOut = ""
	if code := translation.Run(stageCfg); code != 0 {
		return fmt.Errorf("translation.Run exited with code %d", code)
	}
	return nil
}

func readIDsFile(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out, nil
}

func readSourceTextMap(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rawObj map[string]any
	if err := json.Unmarshal(raw, &rawObj); err != nil {
		return nil, err
	}
	obj := map[string]map[string]any{}
	if stringsMap, ok := rawObj["strings"].(map[string]any); ok {
		for id, row := range stringsMap {
			if rowMap, ok := row.(map[string]any); ok {
				obj[id] = rowMap
			}
		}
	} else {
		for id, row := range rawObj {
			if rowMap, ok := row.(map[string]any); ok {
				obj[id] = rowMap
			}
		}
	}
	out := make(map[string]string, len(obj))
	for id, row := range obj {
		if text, _ := row["Text"].(string); text != "" {
			out[id] = text
		}
	}
	return out, nil
}

func applyProjectPipelineDefaults(projectCfg *shared.ProjectConfig, cfg *Config) {
	p := projectCfg.Pipeline
	if cfg.StageBatchSize == 100 && p.StageBatchSize > 0 {
		cfg.StageBatchSize = p.StageBatchSize
	}
	if cfg.Threshold == 0.7 && p.Threshold > 0 {
		cfg.Threshold = p.Threshold
	}
	if cfg.MaxRetries == 3 && p.MaxRetries > 0 {
		cfg.MaxRetries = p.MaxRetries
	}
	if cfg.LowBackend == "" {
		cfg.LowBackend = firstNonEmpty(p.LowLLM.LLMBackend, projectCfg.Translation.LLMBackend)
	}
	if cfg.LowServerURL == "" {
		cfg.LowServerURL = firstNonEmpty(p.LowLLM.ServerURL, projectCfg.Translation.ServerURL)
	}
	if cfg.LowModel == "" {
		cfg.LowModel = firstNonEmpty(p.LowLLM.Model, projectCfg.Translation.Model)
	}
	if cfg.LowAgent == "" {
		cfg.LowAgent = p.LowLLM.Agent
	}
	if cfg.LowConcurrency == 0 {
		cfg.LowConcurrency = p.LowLLM.Concurrency
	}
	if cfg.LowBatchSize == 0 {
		cfg.LowBatchSize = p.LowLLM.BatchSize
	}
	if cfg.LowTimeoutSec == 0 {
		cfg.LowTimeoutSec = p.LowLLM.TimeoutSec
	}
	if cfg.RetranslateBackend == "" {
		cfg.RetranslateBackend = p.HighLLM.LLMBackend
	}
	if cfg.RetranslateServerURL == "" {
		cfg.RetranslateServerURL = p.HighLLM.ServerURL
	}
	if cfg.RetranslateModel == "" {
		cfg.RetranslateModel = p.HighLLM.Model
	}
	if cfg.RetranslateAgent == "" {
		cfg.RetranslateAgent = p.HighLLM.Agent
	}
	scoreProfile := p.ScoreLLM
	if scoreProfile.LLMBackend == "" && scoreProfile.ServerURL == "" && scoreProfile.Model == "" && scoreProfile.Agent == "" {
		scoreProfile = p.HighLLM
	}
	if cfg.ScoreBackend == "" {
		cfg.ScoreBackend = scoreProfile.LLMBackend
	}
	if cfg.ScoreServerURL == "" {
		cfg.ScoreServerURL = scoreProfile.ServerURL
	}
	if cfg.ScoreModel == "" {
		cfg.ScoreModel = scoreProfile.Model
	}
	if cfg.ScoreAgent == "" {
		cfg.ScoreAgent = scoreProfile.Agent
	}
	if cfg.ScorePromptVariant == "" {
		cfg.ScorePromptVariant = scoreProfile.PromptVariant
	}
	if cfg.ScoreConcurrency == 0 {
		cfg.ScoreConcurrency = scoreProfile.Concurrency
	}
	if cfg.ScoreBatchSize == 0 {
		cfg.ScoreBatchSize = scoreProfile.BatchSize
	}
	if cfg.ScoreTimeoutSec == 0 {
		cfg.ScoreTimeoutSec = scoreProfile.TimeoutSec
	}
}

func applyLLMProfileToTranslationConfig(cfg *translation.Config, profile shared.ProjectLLMProfile) {
	if profile.LLMBackend != "" {
		cfg.LLMBackend = profile.LLMBackend
	}
	if profile.ServerURL != "" {
		cfg.ServerURL = profile.ServerURL
	}
	if profile.Model != "" {
		cfg.Model = profile.Model
	}
	if profile.Agent != "" {
		cfg.Agent = profile.Agent
	}
	if len(profile.ContextFiles) > 0 {
		cfg.ContextFiles = append(cfg.ContextFiles[:0], profile.ContextFiles...)
	}
	if profile.TranslatorResponseMode != "" {
		cfg.TranslatorResponseMode = profile.TranslatorResponseMode
	}
	if profile.OllamaStructuredOutput {
		cfg.OllamaStructuredOutput = true
	}
	if profile.OllamaBakedSystem {
		cfg.OllamaBakedSystem = true
	}
	if profile.OllamaResetHistory {
		cfg.OllamaResetHistory = true
	}
	if profile.OllamaKeepAlive != "" {
		cfg.OllamaKeepAlive = profile.OllamaKeepAlive
	}
	if profile.OllamaNumCtx > 0 {
		cfg.OllamaNumCtx = profile.OllamaNumCtx
	}
	if profile.OllamaTemperature >= 0 {
		cfg.OllamaTemperature = profile.OllamaTemperature
	}
	if profile.Concurrency > 0 {
		cfg.Concurrency = profile.Concurrency
	}
	if profile.BatchSize > 0 {
		cfg.BatchSize = profile.BatchSize
	}
	if profile.TimeoutSec > 0 {
		cfg.TimeoutSec = profile.TimeoutSec
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
