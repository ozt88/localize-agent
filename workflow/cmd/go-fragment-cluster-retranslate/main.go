package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"localize-agent/workflow/internal/fragmentcluster"
	"localize-agent/workflow/pkg/platform"
	"localize-agent/workflow/pkg/shared"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type rowData struct {
	ID        string
	EN        string
	CurrentKO string
	TextRole  string
	PrevEN    string
	NextEN    string
	SegmentID string
	SourceFile string
}

func main() {
	var projectDir string
	var idsCSV string
	var clusterName string
	var tier string
	var limit int
	fs := flag.NewFlagSet("go-fragment-cluster-retranslate", flag.ExitOnError)
	fs.StringVar(&projectDir, "project-dir", "", "project directory containing project.json")
	fs.StringVar(&idsCSV, "ids", "", "comma-separated line ids in cluster order")
	fs.StringVar(&clusterName, "cluster-name", "", "optional cluster label")
	fs.StringVar(&tier, "tier", "", "optional tier from workflow/output/cluster_tier_report.json")
	fs.IntVar(&limit, "limit", 0, "optional number of tier clusters to run")
	fs.Parse(os.Args[1:])

	if strings.TrimSpace(projectDir) == "" {
		fmt.Fprintln(os.Stderr, "--project-dir is required")
		os.Exit(2)
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

	scoreProfile := projectCfg.Pipeline.HighLLM
	backend, err := platform.NormalizeLLMBackend(firstNonEmpty(scoreProfile.LLMBackend, projectCfg.Translation.LLMBackend))
	if err != nil {
		fmt.Fprintf(os.Stderr, "llm backend error: %v\n", err)
		os.Exit(1)
	}
	serverURL := firstNonEmpty(scoreProfile.ServerURL, projectCfg.Translation.ServerURL)
	model := firstNonEmpty(scoreProfile.Model, projectCfg.Translation.Model)
	agent := scoreProfile.Agent
	timeoutSec := scoreProfile.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 120
	}

	var client interface {
		SendPrompt(key string, profile platform.LLMProfile, prompt string) (string, error)
	}
	profile := platform.LLMProfile{
		ProviderID: backend,
		ModelID:    model,
		Agent:      agent,
		Warmup:     "",
	}
	switch backend {
	case platform.LLMBackendOpencode:
		providerID, modelID, err := platform.ParseModel(model)
		if err != nil {
			fmt.Fprintf(os.Stderr, "model parse error: %v\n", err)
			os.Exit(1)
		}
		profile.ProviderID = providerID
		profile.ModelID = modelID
		client = platform.NewSessionLLMClient(serverURL, timeoutSec, &shared.MetricCollector{}, nil)
	case platform.LLMBackendOllama:
		client = platform.NewOllamaLLMClient(serverURL, timeoutSec, &shared.MetricCollector{}, nil)
	default:
		fmt.Fprintf(os.Stderr, "unsupported backend: %s\n", backend)
		os.Exit(1)
	}

	if strings.TrimSpace(tier) != "" {
		reportPath := filepath.Join("workflow", "output", "cluster_tier_report.json")
		batches, err := loadTierClusters(reportPath, tier, limit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tier load error: %v\n", err)
			os.Exit(1)
		}
		out := make([]map[string]any, 0, len(batches))
		for idx, batch := range batches {
			rows, err := loadRows(db, projectCfg.Translation.CheckpointBackend, batch.IDs)
			if err != nil {
				fmt.Fprintf(os.Stderr, "cluster load error: %v\n", err)
				os.Exit(1)
			}
			result, err := executeCluster(client, profile, batch.Name, rows, idx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "cluster prompt error: %v\n", err)
				os.Exit(1)
			}
			out = append(out, result)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return
	}

	ids := splitCSV(idsCSV)
	if len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "either --ids or --tier is required")
		os.Exit(1)
	}
	rows, err := loadRows(db, projectCfg.Translation.CheckpointBackend, ids)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cluster load error: %v\n", err)
		os.Exit(1)
	}
	out, err := executeCluster(client, profile, clusterName, rows, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prompt error: %v\n", err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
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

type tierReport struct {
	Counts   map[string]int         `json:"counts"`
	Clusters map[string][]tierEntry `json:"clusters"`
}

type tierEntry struct {
	Score int      `json:"score"`
	IDs   []string `json:"ids"`
}

type namedCluster struct {
	Name string
	IDs  []string
}

func loadTierClusters(path string, tier string, limit int) ([]namedCluster, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var report tierReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return nil, err
	}
	list, ok := report.Clusters[tier]
	if !ok {
		return nil, fmt.Errorf("tier %s not found", tier)
	}
	if limit > 0 && limit < len(list) {
		list = list[:limit]
	}
	out := make([]namedCluster, 0, len(list))
	for idx, item := range list {
		out = append(out, namedCluster{
			Name: fmt.Sprintf("%s-%03d", tier, idx+1),
			IDs:  item.IDs,
		})
	}
	return out, nil
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
	raw, err := client.SendPrompt(fmt.Sprintf("fragment-cluster-retranslate-%d", idx), profile, fragmentcluster.BuildPrompt(input))
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
