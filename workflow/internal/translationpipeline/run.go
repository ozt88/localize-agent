package translationpipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"localize-agent/workflow/internal/semanticreview"
	"localize-agent/workflow/internal/shared"
	"localize-agent/workflow/internal/translation"
)

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
	store, err := Open(dbPath)
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

	role := normalizeWorkerRole(cfg.WorkerRole)
	shouldSeed := role == "" || role == "all" || cfg.InitOnly || cfg.Reset
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
	if cfg.InitOnly {
		counts, err := store.CountStates()
		if err != nil {
			fmt.Fprintf(os.Stderr, "pipeline count error: %v\n", err)
			return 1
		}
		fmt.Printf("Pipeline init complete: pending_translate=%d pending_score=%d pending_retranslate=%d done=%d failed=%d\n",
			counts[StatePendingTranslate], counts[StatePendingScore], counts[StatePendingRetranslate], counts[StateDone], counts[StateFailed],
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
	retryTransCfg := buildRetryTranslationConfig(projectCfg, cfg, dbPath)
	scoreCfg := buildScoreConfig(projectCfg, cfg, dbPath)
	leaseDuration := time.Duration(cfg.LeaseSec) * time.Second
	idleSleep := time.Duration(cfg.IdleSleepSec) * time.Second
	if role != "" && role != "all" {
		return runDedicatedWorker(role, cfg, store, dbPath, sourceMap, baseTransCfg, retryTransCfg, scoreCfg, leaseDuration, idleSleep)
	}

	for {
		processed, err := processNextAvailable(cfg, store, dbPath, sourceMap, baseTransCfg, retryTransCfg, scoreCfg, leaseDuration)
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
	fmt.Printf("Pipeline summary: pending_translate=%d pending_score=%d pending_retranslate=%d done=%d failed=%d\n",
		counts[StatePendingTranslate], counts[StatePendingScore], counts[StatePendingRetranslate], counts[StateDone], counts[StateFailed],
	)
	return 0
}

func runDedicatedWorker(role string, cfg Config, store *Store, dbPath string, sourceMap map[string]string, baseTransCfg translation.Config, retryTransCfg translation.Config, scoreCfg semanticreview.Config, leaseDuration, idleSleep time.Duration) int {
	for {
		processed, err := processRole(role, cfg, store, dbPath, sourceMap, baseTransCfg, retryTransCfg, scoreCfg, leaseDuration)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pipeline %s worker error: %v\n", role, err)
			return 1
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

func processNextAvailable(cfg Config, store *Store, dbPath string, sourceMap map[string]string, baseTransCfg translation.Config, retryTransCfg translation.Config, scoreCfg semanticreview.Config, leaseDuration time.Duration) (bool, error) {
	for _, role := range []string{"retranslate", "score", "translate"} {
		processed, err := processRole(role, cfg, store, dbPath, sourceMap, baseTransCfg, retryTransCfg, scoreCfg, leaseDuration)
		if err != nil {
			return false, err
		}
		if processed {
			return true, nil
		}
	}
	return false, nil
}

func processRole(role string, cfg Config, store *Store, dbPath string, sourceMap map[string]string, baseTransCfg translation.Config, retryTransCfg translation.Config, scoreCfg semanticreview.Config, leaseDuration time.Duration) (bool, error) {
	switch role {
	case "translate":
		startedAt := time.Now()
		rows, err := store.ClaimPending(StatePendingTranslate, StateWorkingTranslate, cfg.WorkerID, cfg.StageBatchSize, leaseDuration)
		if err != nil {
			return false, err
		}
		if len(rows) == 0 {
			return false, nil
		}
		ids := pipelineIDs(rows)
		if err := runTranslationStage(baseTransCfg, ids); err != nil {
			return false, err
		}
		if err := store.ResolveAfterTranslate(ids, sourceMap, false, cfg.WorkerID); err != nil {
			return false, err
		}
		if err := store.RecordWorkerBatchStat(cfg.WorkerID, role, len(ids), startedAt, time.Now()); err != nil {
			return false, err
		}
		return true, nil
	case "score":
		startedAt := time.Now()
		rows, err := store.ClaimPending(StatePendingScore, StateWorkingScore, cfg.WorkerID, cfg.StageBatchSize, leaseDuration)
		if err != nil {
			return false, err
		}
		if len(rows) == 0 {
			return false, nil
		}
		ids := pipelineIDs(rows)
		items, err := semanticreview.LoadDoneItemsByIDs(dbPath, ids)
		if err != nil {
			return false, err
		}
		report, err := semanticreview.ReviewDirectItems(scoreCfg, items)
		if err != nil {
			return false, err
		}
		scoreMap := map[string]float64{}
		for _, item := range report {
			scoreMap[item.ID] = item.ScoreFinal
		}
		if err := store.ApplyScores(rows, scoreMap, cfg.Threshold, cfg.MaxRetries, cfg.WorkerID); err != nil {
			return false, err
		}
		if err := store.RecordWorkerBatchStat(cfg.WorkerID, role, len(ids), startedAt, time.Now()); err != nil {
			return false, err
		}
		return true, nil
	case "retranslate":
		startedAt := time.Now()
		rows, err := store.ClaimPending(StatePendingRetranslate, StateWorkingRetranslate, cfg.WorkerID, cfg.StageBatchSize, leaseDuration)
		if err != nil {
			return false, err
		}
		if len(rows) == 0 {
			return false, nil
		}
		ids := pipelineIDs(rows)
		if err := runTranslationStage(retryTransCfg, ids); err != nil {
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

func normalizeWorkerRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "", "all":
		return strings.ToLower(strings.TrimSpace(role))
	case "translate", "score", "retranslate":
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
	cfg.Source = t.Source
	cfg.Current = t.Current
	cfg.IDsFile = t.IDsFile
	cfg.TranslatorPackageChunks = t.TranslatorPackageChunks
	cfg.CheckpointDB = checkpointDB
	cfg.RulesFile = t.RulesFile
	cfg.ContextFiles = append(cfg.ContextFiles[:0], t.ContextFiles...)
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
	applyLLMProfileToTranslationConfig(&cfg, projectCfg.Pipeline.LowLLM)
	return cfg
}

func buildRetryTranslationConfig(projectCfg *shared.ProjectConfig, cfg Config, checkpointDB string) translation.Config {
	out := buildTranslationConfig(projectCfg, checkpointDB)
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

func buildScoreConfig(projectCfg *shared.ProjectConfig, cfg Config, dbPath string) semanticreview.Config {
	contextFiles := []string{"projects/esoteric-ebb/context/esoteric_ebb_semantic_review_system.md"}
	if len(projectCfg.Pipeline.ScoreLLM.ContextFiles) > 0 {
		contextFiles = append([]string(nil), projectCfg.Pipeline.ScoreLLM.ContextFiles...)
	}
	out := semanticreview.Config{
		CheckpointDB: dbPath,
		Mode:         "direct",
		ScoreOnly:    true,
		LLMBackend:   cfg.ScoreBackend,
		ServerURL:    cfg.ScoreServerURL,
		Model:        cfg.ScoreModel,
		Agent:        cfg.ScoreAgent,
		ContextFiles: contextFiles,
		Concurrency:  4,
		BatchSize:    20,
		TimeoutSec:   120,
		OutputDir:    filepath.Join("projects", "esoteric-ebb", "output", "translation_pipeline_score_tmp"),
	}
	if cfg.ScoreConcurrency > 0 {
		out.Concurrency = cfg.ScoreConcurrency
	}
	if cfg.ScoreBatchSize > 0 {
		out.BatchSize = cfg.ScoreBatchSize
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
	stageCfg.TraceOut = ""
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
