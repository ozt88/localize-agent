package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"localize-agent/workflow/internal/fragmentcluster"
	"localize-agent/workflow/pkg/platform"
	"localize-agent/workflow/pkg/shared"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type rowData struct {
	ID         string
	EN         string
	CurrentKO  string
	TextRole   string
	PrevEN     string
	NextEN     string
	SegmentID  string
	SourceFile string
}

type targetEntry struct {
	Tier         string   `json:"tier"`
	Score        int      `json:"score"`
	OverlapCount int      `json:"overlap_count"`
	IDs          []string `json:"ids"`
	JoinedEN     string   `json:"joined_en"`
}

type resultEntry struct {
	ClusterName   string   `json:"cluster_name"`
	Tier          string   `json:"tier"`
	Score         int      `json:"score"`
	OverlapCount  int      `json:"overlap_count"`
	IDs           []string `json:"ids"`
	JoinedEN      string   `json:"joined_en"`
	Status        string   `json:"status"`
	BeforeEN      []string `json:"before_en,omitempty"`
	BeforeKO      []string `json:"before_ko,omitempty"`
	AfterKO       []string `json:"after_ko,omitempty"`
	ContextBefore string   `json:"context_before_en,omitempty"`
	ContextAfter  string   `json:"context_after_en,omitempty"`
	Error         string   `json:"error,omitempty"`
}

func main() {
	var projectDir string
	var targetsPath string
	var outputPath string
	var limit int
	var startIndex int
	var concurrency int

	fs := flag.NewFlagSet("go-fragment-cluster-batch-runner", flag.ExitOnError)
	fs.StringVar(&projectDir, "project-dir", "", "project directory containing project.json")
	fs.StringVar(&targetsPath, "targets-path", filepath.Join("workflow", "output", "low_score_cluster_targets.json"), "path to target JSON array")
	fs.StringVar(&outputPath, "output-path", filepath.Join("workflow", "output", "low_score_fragment_batch_report.json"), "path to output report JSON")
	fs.IntVar(&limit, "limit", 5, "number of targets to run")
	fs.IntVar(&startIndex, "start-index", 0, "0-based start index in targets list")
	fs.IntVar(&concurrency, "concurrency", 1, "number of clusters to run concurrently")
	fs.Parse(os.Args[1:])

	if strings.TrimSpace(projectDir) == "" {
		fmt.Fprintln(os.Stderr, "--project-dir is required")
		os.Exit(2)
	}
	if concurrency <= 0 {
		concurrency = 1
	}

	projectCfg, _, err := shared.LoadProjectConfig("", projectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "project load error: %v\n", err)
		os.Exit(1)
	}

	db, err := platform.OpenTranslationCheckpointDB(projectCfg.Translation.CheckpointBackend, projectCfg.Translation.CheckpointDB, projectCfg.Translation.CheckpointDSN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "checkpoint open error: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	targets, err := loadTargets(targetsPath, startIndex, limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "target load error: %v\n", err)
		os.Exit(1)
	}
	if len(targets) == 0 {
		fmt.Fprintf(os.Stderr, "no targets selected\n")
		os.Exit(1)
	}

	client, profile, err := buildClient(projectCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client build error: %v\n", err)
		os.Exit(1)
	}

	results := make([]resultEntry, 0, len(targets))
	var resultsMu sync.Mutex
	writeResults := func() {
		resultsMu.Lock()
		defer resultsMu.Unlock()
		sort.SliceStable(results, func(i, j int) bool {
			if results[i].Tier != results[j].Tier {
				return results[i].Tier < results[j].Tier
			}
			if results[i].Score != results[j].Score {
				return results[i].Score > results[j].Score
			}
			return results[i].ClusterName < results[j].ClusterName
		})
		raw, _ := json.MarshalIndent(results, "", "  ")
		_ = os.WriteFile(outputPath, raw, 0o644)
	}
	writeResults()

	type workItem struct {
		Index  int
		Target targetEntry
	}

	jobs := make(chan workItem)
	var completed atomic.Int64
	var wg sync.WaitGroup

	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func(workerIdx int) {
			defer wg.Done()
			for job := range jobs {
				clusterName := fmt.Sprintf("%s-score-%d-%s", job.Target.Tier, job.Target.Score, job.Target.IDs[0])
				fmt.Printf("[%d/%d] start %s\n", job.Index+1, len(targets), clusterName)
				entry := runTarget(db, projectCfg.Translation.CheckpointBackend, client, profile, clusterName, job.Target, job.Index)
				resultsMu.Lock()
				results = append(results, entry)
				resultsMu.Unlock()
				writeResults()
				done := completed.Add(1)
				fmt.Printf("[%d/%d] %s %s\n", done, len(targets), entry.Status, clusterName)
			}
		}(worker)
	}

	for idx, target := range targets {
		jobs <- workItem{Index: idx, Target: target}
	}
	close(jobs)
	wg.Wait()
	writeResults()
	fmt.Println(outputPath)
}

func loadTargets(path string, startIndex int, limit int) ([]targetEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw = []byte(strings.TrimPrefix(string(raw), "\ufeff"))
	var all []targetEntry
	if err := json.Unmarshal(raw, &all); err != nil {
		return nil, err
	}
	if startIndex < 0 {
		startIndex = 0
	}
	if startIndex >= len(all) {
		return nil, nil
	}
	all = all[startIndex:]
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	return all, nil
}

func buildClient(projectCfg *shared.ProjectConfig) (interface {
	SendPrompt(key string, profile platform.LLMProfile, prompt string) (string, error)
}, platform.LLMProfile, error) {
	profileCfg := projectCfg.Pipeline.HighLLM
	backend, err := platform.NormalizeLLMBackend(firstNonEmpty(profileCfg.LLMBackend, projectCfg.Translation.LLMBackend))
	if err != nil {
		return nil, platform.LLMProfile{}, err
	}
	serverURL := firstNonEmpty(profileCfg.ServerURL, projectCfg.Translation.ServerURL)
	model := firstNonEmpty(profileCfg.Model, projectCfg.Translation.Model)
	agent := profileCfg.Agent
	timeoutSec := profileCfg.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 120
	}

	profile := platform.LLMProfile{
		ProviderID: backend,
		ModelID:    model,
		Agent:      agent,
	}
	switch backend {
	case platform.LLMBackendOpencode:
		providerID, modelID, err := platform.ParseModel(model)
		if err != nil {
			return nil, platform.LLMProfile{}, err
		}
		profile.ProviderID = providerID
		profile.ModelID = modelID
		return platform.NewSessionLLMClient(serverURL, timeoutSec, &shared.MetricCollector{}, nil), profile, nil
	case platform.LLMBackendOllama:
		return platform.NewOllamaLLMClient(serverURL, timeoutSec, &shared.MetricCollector{}, nil), profile, nil
	default:
		return nil, platform.LLMProfile{}, fmt.Errorf("unsupported backend: %s", backend)
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

func runTarget(db *sql.DB, backend string, client interface {
	SendPrompt(key string, profile platform.LLMProfile, prompt string) (string, error)
}, profile platform.LLMProfile, clusterName string, target targetEntry, idx int) resultEntry {
	rows, err := loadRows(db, backend, target.IDs)
	if err != nil {
		return resultEntry{
			ClusterName:  clusterName,
			Tier:         target.Tier,
			Score:        target.Score,
			OverlapCount: target.OverlapCount,
			IDs:          target.IDs,
			JoinedEN:     target.JoinedEN,
			Status:       "error",
			Error:        err.Error(),
		}
	}
	out, err := executeCluster(client, profile, clusterName, rows, idx)
	if err != nil {
		return resultEntry{
			ClusterName:  clusterName,
			Tier:         target.Tier,
			Score:        target.Score,
			OverlapCount: target.OverlapCount,
			IDs:          target.IDs,
			JoinedEN:     target.JoinedEN,
			Status:       "error",
			Error:        err.Error(),
		}
	}
	return resultEntry{
		ClusterName:  clusterName,
		Tier:         target.Tier,
		Score:        target.Score,
		OverlapCount: target.OverlapCount,
		IDs:          target.IDs,
		JoinedEN:     target.JoinedEN,
		Status:       "ok",
		BeforeEN:     stringSlice(out["before_en"]),
		BeforeKO:     stringSlice(out["before_ko"]),
		AfterKO:      stringSlice(out["after_ko"]),
		ContextBefore: stringValue(out["context_before_en"]),
		ContextAfter:  stringValue(out["context_after_en"]),
	}
}

func stringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		if direct, ok := v.([]string); ok {
			return direct
		}
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func loadRows(db *sql.DB, backend string, ids []string) ([]rowData, error) {
	placeholders := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	query := platform.RebindSQL(backend, `
select
  id,
  coalesce(pack_json::jsonb->>'en','') as en,
  coalesce(pack_json::jsonb->>'current_ko','') as current_ko,
  coalesce(pack_json::jsonb->>'text_role','') as text_role,
  coalesce(pack_json::jsonb->>'prev_en','') as prev_en,
  coalesce(pack_json::jsonb->>'next_en','') as next_en,
  coalesce(pack_json::jsonb->>'segment_id','') as segment_id,
  coalesce(pack_json::jsonb->>'source_file','') as source_file
from items
where id in (`+strings.Join(placeholders, ",")+`)`)
	if backend != platform.DBBackendPostgres {
		query = `
select
  id,
  coalesce(json_extract(pack_json,'$.en'),'') as en,
  coalesce(json_extract(pack_json,'$.current_ko'),'') as current_ko,
  coalesce(json_extract(pack_json,'$.text_role'),'') as text_role,
  coalesce(json_extract(pack_json,'$.prev_en'),'') as prev_en,
  coalesce(json_extract(pack_json,'$.next_en'),'') as next_en,
  coalesce(json_extract(pack_json,'$.segment_id'),'') as segment_id,
  coalesce(json_extract(pack_json,'$.source_file'),'') as source_file
from items
where id in (` + strings.Join(placeholders, ",") + `)`
		query = platform.RebindSQL(backend, query)
	}
	r, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	byID := map[string]rowData{}
	for r.Next() {
		var row rowData
		if err := r.Scan(&row.ID, &row.EN, &row.CurrentKO, &row.TextRole, &row.PrevEN, &row.NextEN, &row.SegmentID, &row.SourceFile); err != nil {
			return nil, err
		}
		row.EN = trimJSONScalar(row.EN)
		row.CurrentKO = trimJSONScalar(row.CurrentKO)
		row.TextRole = trimJSONScalar(row.TextRole)
		row.PrevEN = trimJSONScalar(row.PrevEN)
		row.NextEN = trimJSONScalar(row.NextEN)
		row.SegmentID = trimJSONScalar(row.SegmentID)
		row.SourceFile = trimJSONScalar(row.SourceFile)
		byID[row.ID] = row
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	out := make([]rowData, 0, len(ids))
	for _, id := range ids {
		row, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("missing id %s", id)
		}
		out = append(out, row)
	}
	return out, nil
}

func trimJSONScalar(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && strings.HasPrefix(v, "\"") && strings.HasSuffix(v, "\"") {
		var out string
		if err := json.Unmarshal([]byte(v), &out); err == nil {
			return out
		}
	}
	return v
}

func collectBeforeEN(rows []rowData) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.EN)
	}
	return out
}

func collectBeforeKO(rows []rowData) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.CurrentKO)
	}
	return out
}

func executeCluster(client interface {
	SendPrompt(key string, profile platform.LLMProfile, prompt string) (string, error)
}, profile platform.LLMProfile, clusterName string, rows []rowData, idx int) (map[string]any, error) {
	input := fragmentcluster.PromptInput{
		ClusterID:       clusterName,
		Lines:           make([]fragmentcluster.Line, 0, len(rows)),
		ContextBeforeEN: rows[0].PrevEN,
		ContextAfterEN:  rows[len(rows)-1].NextEN,
		ClusterJoinHint: "fragment_chain",
		SegmentID:       rows[0].SegmentID,
		SourceFile:      rows[0].SourceFile,
	}
	for _, row := range rows {
		input.Lines = append(input.Lines, fragmentcluster.Line{
			ID:        row.ID,
			EN:        row.EN,
			CurrentKO: row.CurrentKO,
			TextRole:  row.TextRole,
		})
	}
	raw, err := client.SendPrompt(fmt.Sprintf("fragment-cluster-batch-%d", idx), profile, fragmentcluster.BuildPrompt(input))
	if err != nil {
		return nil, err
	}
	lines, err := fragmentcluster.ParseOutput(raw, len(input.Lines))
	if err != nil {
		return nil, fmt.Errorf("parse error: %w raw: %s", err, raw)
	}
	lines, err = fragmentcluster.NormalizeOutputLines(lines, input.Lines)
	if err != nil {
		return nil, fmt.Errorf("normalize error: %w raw: %s", err, raw)
	}
	return map[string]any{
		"cluster_id":        input.ClusterID,
		"context_before_en": input.ContextBeforeEN,
		"context_after_en":  input.ContextAfterEN,
		"before_en":         collectBeforeEN(rows),
		"before_ko":         collectBeforeKO(rows),
		"after_ko":          lines,
	}, nil
}
