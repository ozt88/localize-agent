package v2pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"localize-agent/workflow/internal/clustertranslate"
	"localize-agent/workflow/internal/glossary"
	"localize-agent/workflow/internal/ragcontext"
	"localize-agent/workflow/pkg/platform"
	"localize-agent/workflow/pkg/shared"
)

// Run is the main entry point for the v2 pipeline orchestrator.
// Returns exit code: 0 success, 1 runtime error, 2 config error.
func Run(cfg Config) int {
	// Load project config.
	projectCfg, projectDir, err := shared.LoadProjectConfig(cfg.Project, cfg.ProjectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "v2pipeline project load error: %v\n", err)
		return 2
	}
	if projectCfg == nil {
		fmt.Fprintln(os.Stderr, "v2pipeline project load error: project config required")
		return 2
	}

	// Apply project defaults.
	applyProjectDefaults(projectCfg, &cfg)

	// Ensure OpenCode server is running before opening store or creating LLM clients.
	if err := ensureOpenCode(cfg.TranslateServerURL); err != nil {
		fmt.Fprintf(os.Stderr, "v2pipeline opencode startup error: %v\n", err)
		return 1
	}

	// Open store.
	store, err := OpenStore(cfg.CheckpointBackend, cfg.CheckpointDB, cfg.CheckpointDSN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "v2pipeline store open error: %v\n", err)
		return 1
	}
	defer store.Close()

	// Cleanup stale claims if requested.
	if cfg.CleanupStaleClaims {
		reclaimed, err := store.CleanupStaleClaims(cfg.LeaseSec * 3)
		if err != nil {
			fmt.Fprintf(os.Stderr, "v2pipeline stale cleanup error: %v\n", err)
			return 1
		}
		fmt.Printf("v2pipeline stale cleanup: reclaimed=%d\n", reclaimed)
	}

	// Print initial state.
	counts, err := store.CountByState()
	if err != nil {
		fmt.Fprintf(os.Stderr, "v2pipeline count error: %v\n", err)
		return 1
	}
	fmt.Printf("v2pipeline initial state: %v\n", formatCounts(counts))

	// Load glossary.
	glossaryPath := resolveProjectPath(projectDir, "extract/1.1.3/ExportedProject/Assets/Resources/glossaryterms/GlossaryTerms.txt")
	locTextsDir := resolveProjectPath(projectDir, "extract/1.1.3/ExportedProject/Assets/StreamingAssets/TranslationPatch/localizationtexts")
	glossarySet, err := glossary.LoadGlossary(glossaryPath, locTextsDir, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "v2pipeline glossary load warning: %v (continuing without glossary)\n", err)
		glossarySet = &glossary.GlossarySet{}
	}

	// Load RAG batch context if configured.
	ragCtx, err := ragcontext.LoadBatchContext(cfg.RAGContextPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "v2pipeline ragcontext load warning: %v (continuing without RAG)\n", err)
		ragCtx, _ = ragcontext.LoadBatchContext("") // safe empty context
	}

	// Create metrics collector.
	metrics := &shared.MetricCollector{}

	// Set up trace sink if configured.
	var traceSink platform.LLMTraceSink
	if cfg.TraceOutDir != "" {
		traceFile := filepath.Join(cfg.TraceOutDir, fmt.Sprintf("v2pipeline_%s_%d.jsonl", cfg.WorkerID, time.Now().Unix()))
		sink, err := platform.NewJSONLTraceSink(traceFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "v2pipeline trace sink error: %v\n", err)
			return 1
		}
		if sink != nil {
			traceSink = sink
			defer sink.Close()
		}
	}

	// Build LLM profiles for 3 roles.
	translateProvider, translateModel := splitModel(cfg.TranslateModel)
	formatProvider, formatModel := splitModel(cfg.FormatModel)
	scoreProvider, scoreModel := splitModel(cfg.ScoreModel)

	// Build warmup content.
	warmupTerms := glossarySet.WarmupTerms(50)
	glossaryWarmupJSON := glossarySet.FormatJSON(warmupTerms)

	systemPrompt := loadFileContent(resolveProjectPath(projectDir, "context/v2_base_prompt.md"))
	contextText := loadFileContent(resolveProjectPath(projectDir, "context/esoteric_ebb_context.md"))
	translateWarmup := clustertranslate.BuildBaseWarmup(systemPrompt, contextText, "", glossaryWarmupJSON)

	formatWarmupText := loadFileContent(resolveProjectPath(projectDir, "context/v2_format_prompt.md"))
	scoreWarmupText := loadFileContent(resolveProjectPath(projectDir, "context/v2_score_prompt.md"))

	translateProfile := platform.LLMProfile{
		ProviderID: translateProvider,
		ModelID:    translateModel,
		Agent:      "v2-translate",
		Warmup:     translateWarmup,
	}
	highProfile := platform.LLMProfile{
		ProviderID: translateProvider,
		ModelID:    "gpt-5.4", // D-15: escalation model
		Agent:      "v2-translate-high",
		Warmup:     translateWarmup,
	}
	formatProfile := platform.LLMProfile{
		ProviderID: formatProvider,
		ModelID:    formatModel,
		Agent:      "v2-format",
		Warmup:     formatWarmupText,
	}
	formatHighProfile := platform.LLMProfile{
		ProviderID: translateProvider,
		ModelID:    "gpt-5.4",
		Agent:      "v2-format-high",
		Warmup:     formatWarmupText,
	}
	scoreProfile := platform.LLMProfile{
		ProviderID: scoreProvider,
		ModelID:    scoreModel,
		Agent:      "v2-score",
		Warmup:     scoreWarmupText,
	}

	// Create LLM clients per role (may use different server URLs/timeouts).
	translateLLM := platform.NewSessionLLMClient(cfg.TranslateServerURL, cfg.TranslateTimeoutSec, metrics, traceSink)
	formatLLM := platform.NewSessionLLMClient(cfg.FormatServerURL, cfg.FormatTimeoutSec, metrics, traceSink)
	scoreLLM := platform.NewSessionLLMClient(cfg.ScoreServerURL, cfg.ScoreTimeoutSec, metrics, traceSink)

	// Setup context with signal cancellation.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nv2pipeline: shutting down...")
		cancel()
	}()

	// Determine which roles to run.
	role := strings.ToLower(strings.TrimSpace(cfg.WorkerRole))
	runTranslate := role == "" || role == "all" || role == "translate"
	runFormat := role == "" || role == "all" || role == "format"
	runScore := role == "" || role == "all" || role == "score"

	// Launch worker goroutines.
	var wg sync.WaitGroup

	if runTranslate {
		for i := 0; i < cfg.TranslateConcurrency; i++ {
			wg.Add(1)
			wID := fmt.Sprintf("%s-t%d", cfg.WorkerID, i)
			go func(workerID string) {
				defer wg.Done()
				if err := TranslateWorker(ctx, cfg, store, translateLLM, glossarySet, translateProfile, highProfile, ragCtx, workerID); err != nil {
					if ctx.Err() == nil {
						fmt.Fprintf(os.Stderr, "translate worker %s error: %v\n", workerID, err)
					}
				}
			}(wID)
		}
	}

	if runFormat {
		for i := 0; i < cfg.FormatConcurrency; i++ {
			wg.Add(1)
			wID := fmt.Sprintf("%s-f%d", cfg.WorkerID, i)
			go func(workerID string) {
				defer wg.Done()
				if err := FormatWorker(ctx, cfg, store, formatLLM, formatProfile, formatHighProfile, workerID); err != nil {
					if ctx.Err() == nil {
						fmt.Fprintf(os.Stderr, "format worker %s error: %v\n", workerID, err)
					}
				}
			}(wID)
		}
	}

	if runScore {
		for i := 0; i < cfg.ScoreConcurrency; i++ {
			wg.Add(1)
			wID := fmt.Sprintf("%s-s%d", cfg.WorkerID, i)
			go func(workerID string) {
				defer wg.Done()
				if err := ScoreWorker(ctx, cfg, store, scoreLLM, scoreProfile, ragCtx, workerID); err != nil {
					if ctx.Err() == nil {
						fmt.Fprintf(os.Stderr, "score worker %s error: %v\n", workerID, err)
					}
				}
			}(wID)
		}
	}

	fmt.Printf("v2pipeline: started workers (translate=%d, format=%d, score=%d, role=%s)\n",
		cfg.TranslateConcurrency, cfg.FormatConcurrency, cfg.ScoreConcurrency, role)

	// Launch OpenCode health watchdog — restarts server if unresponsive.
	llmClients := []*platform.SessionLLMClient{translateLLM, formatLLM, scoreLLM}
	go openCodeWatchdog(ctx, cfg.TranslateServerURL, llmClients)

	// Wait for all workers.
	wg.Wait()

	// Print final state.
	finalCounts, err := store.CountByState()
	if err != nil {
		fmt.Fprintf(os.Stderr, "v2pipeline final count error: %v\n", err)
	} else {
		fmt.Printf("v2pipeline final state: %v\n", formatCounts(finalCounts))
	}

	// Print metrics summary.
	calls, errors, avg, _, p95 := metrics.Summary()
	fmt.Printf("v2pipeline metrics: avg=%.0fms p95=%.0fms calls=%d errors=%d\n",
		avg, p95, calls, errors)

	return 0
}

// applyProjectDefaults fills Config fields from ProjectConfig pipeline settings.
func applyProjectDefaults(pc *shared.ProjectConfig, cfg *Config) {
	pp := pc.Pipeline

	if cfg.TranslateServerURL == "" {
		cfg.TranslateServerURL = pp.LowLLM.ServerURL
	}
	if cfg.TranslateModel == "" {
		cfg.TranslateModel = pp.LowLLM.Model
	}
	if cfg.TranslateConcurrency <= 0 {
		cfg.TranslateConcurrency = pp.LowLLM.Concurrency
		if cfg.TranslateConcurrency <= 0 {
			cfg.TranslateConcurrency = 2
		}
	}
	if cfg.TranslateBatchSize <= 0 {
		cfg.TranslateBatchSize = pp.LowLLM.BatchSize
		if cfg.TranslateBatchSize <= 0 {
			cfg.TranslateBatchSize = 10
		}
	}
	if cfg.TranslateTimeoutSec <= 0 {
		cfg.TranslateTimeoutSec = pp.LowLLM.TimeoutSec
		if cfg.TranslateTimeoutSec <= 0 {
			cfg.TranslateTimeoutSec = 120
		}
	}

	if cfg.FormatServerURL == "" {
		cfg.FormatServerURL = pp.LowLLM.ServerURL
	}
	if cfg.FormatModel == "" {
		cfg.FormatModel = "openai/gpt-5.3-codex-spark" // D-07a default
	}
	if cfg.FormatConcurrency <= 0 {
		cfg.FormatConcurrency = 2
	}
	if cfg.FormatBatchSize <= 0 {
		cfg.FormatBatchSize = 5 // D-06
	}
	if cfg.FormatTimeoutSec <= 0 {
		cfg.FormatTimeoutSec = 60
	}

	if cfg.ScoreServerURL == "" {
		cfg.ScoreServerURL = pp.ScoreLLM.ServerURL
	}
	if cfg.ScoreModel == "" {
		cfg.ScoreModel = pp.ScoreLLM.Model
	}
	if cfg.ScoreConcurrency <= 0 {
		cfg.ScoreConcurrency = pp.ScoreLLM.Concurrency
		if cfg.ScoreConcurrency <= 0 {
			cfg.ScoreConcurrency = 2
		}
	}
	if cfg.ScoreBatchSize <= 0 {
		cfg.ScoreBatchSize = pp.ScoreLLM.BatchSize
		if cfg.ScoreBatchSize <= 0 {
			cfg.ScoreBatchSize = 10
		}
	}
	if cfg.ScoreTimeoutSec <= 0 {
		cfg.ScoreTimeoutSec = pp.ScoreLLM.TimeoutSec
		if cfg.ScoreTimeoutSec <= 0 {
			cfg.ScoreTimeoutSec = 120
		}
	}

	if cfg.CheckpointBackend == "" {
		cfg.CheckpointBackend = pc.Translation.CheckpointBackend
		if cfg.CheckpointBackend == "" {
			cfg.CheckpointBackend = "postgres"
		}
	}
	if cfg.CheckpointDSN == "" {
		cfg.CheckpointDSN = pc.Translation.CheckpointDSN
	}

	if cfg.LeaseSec <= 0 {
		cfg.LeaseSec = 300
	}
	if cfg.IdleSleepSec <= 0 {
		cfg.IdleSleepSec = 5
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = pp.MaxRetries
		if cfg.MaxRetries <= 0 {
			cfg.MaxRetries = 3
		}
	}
}

// splitModel splits "provider/model" into (provider, model).
func splitModel(model string) (string, string) {
	parts := strings.SplitN(model, "/", 2)
	if len(parts) != 2 {
		return "", model
	}
	return parts[0], parts[1]
}

// resolveProjectPath resolves a relative path under the project directory.
func resolveProjectPath(projectDir, relPath string) string {
	if projectDir == "" {
		return relPath
	}
	return filepath.Join(projectDir, relPath)
}

// loadFileContent reads a file and returns its content as string, or empty on error.
func loadFileContent(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// ensureOpenCode checks if the OpenCode server is reachable and starts it if not.
// It uses the manage-opencode-serve.ps1 script to start the server and waits up to 30s.
func ensureOpenCode(serverURL string) error {
	if serverURL == "" {
		return nil
	}

	// Quick health check — try to connect.
	if probeServer(serverURL) {
		fmt.Printf("v2pipeline: opencode server already running at %s\n", serverURL)
		return nil
	}

	fmt.Printf("v2pipeline: opencode server not reachable at %s, starting...\n", serverURL)

	// Find the manage script relative to repo root.
	repoRoot := findRepoRoot()
	if repoRoot == "" {
		return fmt.Errorf("cannot find repo root to locate manage-opencode-serve.ps1")
	}
	script := filepath.Join(repoRoot, "scripts", "manage-opencode-serve.ps1")
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("manage script not found: %s", script)
	}

	cmd := exec.Command("powershell.exe",
		"-NoProfile", "-ExecutionPolicy", "Bypass",
		"-File", script,
		"-Action", "start",
	)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("opencode start failed: %v\n%s", err, string(out))
	}
	fmt.Printf("v2pipeline: opencode start output: %s\n", strings.TrimSpace(string(out)))

	// Wait for server to become reachable (up to 30s).
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if probeServer(serverURL) {
			fmt.Printf("v2pipeline: opencode server ready at %s\n", serverURL)
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("opencode server did not become reachable at %s within 30s", serverURL)
}

// probeServer checks if a server URL is reachable with a short timeout.
func probeServer(serverURL string) bool {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(serverURL)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

// findRepoRoot walks up from cwd looking for a go.mod file.
func findRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// openCodeWatchdog periodically probes the OpenCode server and restarts it if unresponsive.
// After restart, all LLM client sessions are reset so workers create fresh sessions.
func openCodeWatchdog(ctx context.Context, serverURL string, clients []*platform.SessionLLMClient) {
	if serverURL == "" {
		return
	}

	const (
		checkInterval  = 2 * time.Minute
		failThreshold  = 3 // consecutive failures before restart
		probeTimeout   = 10 * time.Second
	)

	consecutiveFails := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(checkInterval):
		}

		// Shallow probe: check HTTP reachability only.
		// deepProbe is avoided here because it sends a real LLM request and can false-trigger
		// when the server is legitimately busy with translation work.
		if probeServer(serverURL) {
			if consecutiveFails > 0 {
				fmt.Printf("v2pipeline watchdog: server recovered (was failing for %d checks)\n", consecutiveFails)
			}
			consecutiveFails = 0
			continue
		}

		consecutiveFails++
		fmt.Printf("v2pipeline watchdog: probe failed (%d/%d)\n", consecutiveFails, failThreshold)

		if consecutiveFails < failThreshold {
			continue
		}

		// Restart OpenCode.
		fmt.Println("v2pipeline watchdog: restarting OpenCode server...")
		if err := restartOpenCode(serverURL); err != nil {
			fmt.Fprintf(os.Stderr, "v2pipeline watchdog: restart failed: %v\n", err)
			continue
		}

		// Reset all LLM client sessions so workers create fresh ones.
		for _, c := range clients {
			c.ResetAllSessions()
		}
		fmt.Println("v2pipeline watchdog: sessions reset, workers will reconnect")
		consecutiveFails = 0
	}
}

// deepProbe creates a session and sends a trivial message to verify end-to-end health.
func deepProbe(serverURL string, timeout time.Duration) bool {
	client := &http.Client{Timeout: timeout}

	// Create session.
	resp, err := client.Post(serverURL+"/session", "application/json", strings.NewReader("{}"))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var session struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil || session.ID == "" {
		return false
	}

	// Send trivial message.
	body := `{"model":{"providerID":"openai","modelID":"gpt-5.2"},"parts":[{"type":"text","text":"ping"}]}`
	resp2, err := client.Post(serverURL+"/session/"+session.ID+"/message", "application/json", strings.NewReader(body))
	if err != nil {
		return false
	}
	defer resp2.Body.Close()
	data, _ := io.ReadAll(resp2.Body)
	return len(data) > 0
}

// restartOpenCode kills all OpenCode processes and starts a fresh instance.
func restartOpenCode(serverURL string) error {
	// Kill existing.
	killCmd := exec.Command("taskkill", "/F", "/IM", "opencode.exe")
	killCmd.CombinedOutput() // ignore errors if no process

	time.Sleep(3 * time.Second)

	// Start via manage script.
	repoRoot := findRepoRoot()
	if repoRoot == "" {
		return fmt.Errorf("cannot find repo root")
	}
	script := filepath.Join(repoRoot, "scripts", "manage-opencode-serve.ps1")
	cmd := exec.Command("powershell.exe",
		"-NoProfile", "-ExecutionPolicy", "Bypass",
		"-File", script,
		"-Action", "start",
	)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("start failed: %v\n%s", err, string(out))
	}
	fmt.Printf("v2pipeline watchdog: start output: %s\n", strings.TrimSpace(string(out)))

	// Wait for readiness.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if probeServer(serverURL) {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("server did not become reachable within 30s")
}

// formatCounts formats state counts for display.
func formatCounts(counts map[string]int) string {
	var parts []string
	total := 0
	for state, count := range counts {
		parts = append(parts, fmt.Sprintf("%s=%d", state, count))
		total += count
	}
	parts = append(parts, fmt.Sprintf("total=%d", total))
	return strings.Join(parts, " ")
}
