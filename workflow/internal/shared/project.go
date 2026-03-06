package shared

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ProjectConfig struct {
	Name        string             `json:"name"`
	Translation ProjectTranslation `json:"translation"`
	Evaluation  ProjectEvaluation  `json:"evaluation"`
}

type ProjectTranslation struct {
	Source       string   `json:"source"`
	Current      string   `json:"current"`
	IDsFile      string   `json:"ids_file"`
	CheckpointDB string   `json:"checkpoint_db"`
	ContextFiles []string `json:"context_files"`
	RulesFile    string   `json:"rules_file"`
	ServerURL    string   `json:"server_url"`
	Model        string   `json:"model"`
	LLMBackend   string   `json:"llm_backend"`
}

type ProjectEvaluation struct {
	PackIn        string   `json:"pack_in"`
	DB            string   `json:"db"`
	RunName       string   `json:"run_name"`
	ContextFiles  []string `json:"context_files"`
	RulesFile     string   `json:"rules_file"`
	EvalRulesFile string   `json:"eval_rules_file"`
	ServerURL     string   `json:"server_url"`
	TransModel    string   `json:"trans_model"`
	EvalModel     string   `json:"eval_model"`
	LLMBackend    string   `json:"llm_backend"`
}

func LoadProjectConfig(projectName, projectDir string) (*ProjectConfig, string, error) {
	projectName = strings.TrimSpace(projectName)
	projectDir = strings.TrimSpace(projectDir)
	if projectName == "" && projectDir == "" {
		return nil, "", nil
	}
	base := projectDir
	if base == "" {
		candidates := []string{
			filepath.Join("projects", projectName),
			filepath.Join("workflow", "projects", projectName), // backward compatibility
		}
		found := ""
		for _, c := range candidates {
			if _, err := os.Stat(filepath.Join(c, "project.json")); err == nil {
				found = c
				break
			}
		}
		if found == "" {
			return nil, "", fmt.Errorf("project config not found for %q (tried: %s)", projectName, strings.Join(candidates, ", "))
		}
		base = found
	}
	cfgPath := filepath.Join(base, "project.json")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, "", err
	}
	var cfg ProjectConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, "", fmt.Errorf("invalid project config %s: %w", cfgPath, err)
	}
	resolveProjectPaths(&cfg, base)
	return &cfg, base, nil
}

func resolveProjectPaths(cfg *ProjectConfig, base string) {
	cfg.Translation.Source = resolvePath(base, cfg.Translation.Source)
	cfg.Translation.Current = resolvePath(base, cfg.Translation.Current)
	cfg.Translation.IDsFile = resolvePath(base, cfg.Translation.IDsFile)
	cfg.Translation.CheckpointDB = resolvePath(base, cfg.Translation.CheckpointDB)
	cfg.Translation.RulesFile = resolvePath(base, cfg.Translation.RulesFile)
	for i := range cfg.Translation.ContextFiles {
		cfg.Translation.ContextFiles[i] = resolvePath(base, cfg.Translation.ContextFiles[i])
	}

	cfg.Evaluation.PackIn = resolvePath(base, cfg.Evaluation.PackIn)
	cfg.Evaluation.DB = resolvePath(base, cfg.Evaluation.DB)
	cfg.Evaluation.RulesFile = resolvePath(base, cfg.Evaluation.RulesFile)
	cfg.Evaluation.EvalRulesFile = resolvePath(base, cfg.Evaluation.EvalRulesFile)
	for i := range cfg.Evaluation.ContextFiles {
		cfg.Evaluation.ContextFiles[i] = resolvePath(base, cfg.Evaluation.ContextFiles[i])
	}
}

func resolvePath(base, p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Clean(filepath.Join(base, p))
}
