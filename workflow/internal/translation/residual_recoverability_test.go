package translation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"localize-agent/workflow/pkg/shared"
)

type residualNoRowFixture struct {
	Count int                  `json:"count"`
	Rows  []residualNoRowEntry `json:"rows"`
}

type residualNoRowEntry struct {
	ID          string `json:"id"`
	SourceRaw   string `json:"source_raw"`
	TextRole    string `json:"text_role"`
	PrevEN      string `json:"prev_en"`
	NextEN      string `json:"next_en"`
	ContextEN   string `json:"context_en"`
	StatCheck   string `json:"stat_check"`
	ChoiceMode  string `json:"choice_mode"`
	SourceFile  string `json:"source_file"`
	RetryReason string `json:"retry_reason"`
}

var (
	recoveryActionOpenQuoteRe = regexp.MustCompile(`^\([^)]*\)\s*"`)
	recoveryStatLikeQuoteRe   = regexp.MustCompile(`^(DC|ROLL|FC)\d+\s+[A-Za-z]+-".*`)
)

func loadResidualNoRowFixture(t *testing.T) residualNoRowFixture {
	t.Helper()
	path := filepath.Join("testdata", "residual_no_row_fixture.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read residual fixture: %v", err)
	}
	var fx residualNoRowFixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("decode residual fixture: %v", err)
	}
	if fx.Count == 0 || len(fx.Rows) == 0 {
		t.Fatalf("residual fixture is empty")
	}
	return fx
}

func TestResidualRecoverabilitySmoke(t *testing.T) {
	if os.Getenv("RUN_LIVE_RECOVERY") != "1" {
		t.Skip("set RUN_LIVE_RECOVERY=1 to run live recoverability harness")
	}

	projectDir := os.Getenv("RECOVERY_PROJECT_DIR")
	if projectDir == "" {
		projectDir = filepath.Clean(filepath.Join("..", "..", "..", "projects", "esoteric-ebb", "output", "batches", "canonical_full_retranslate_dual_score_20260311_1"))
	}
	cfg, _, err := shared.LoadProjectConfig("", projectDir)
	if err != nil {
		t.Fatalf("load project config: %v", err)
	}

	baseCfg := DefaultConfig()
	baseCfg.LLMBackend = cfg.Translation.LLMBackend
	baseCfg.ServerURL = cfg.Translation.ServerURL
	baseCfg.Model = cfg.Translation.Model
	if cfg.Pipeline.LowLLM.Model != "" {
		baseCfg.Model = cfg.Pipeline.LowLLM.Model
	}
	if cfg.Pipeline.LowLLM.LLMBackend != "" {
		baseCfg.LLMBackend = cfg.Pipeline.LowLLM.LLMBackend
	}
	if cfg.Pipeline.LowLLM.ServerURL != "" {
		baseCfg.ServerURL = cfg.Pipeline.LowLLM.ServerURL
	}
	baseCfg.Agent = cfg.Pipeline.LowLLM.Agent
	if baseCfg.Agent == "" {
		baseCfg.Agent = "rt-ko-translate-primary"
	}
	baseCfg.TimeoutSec = cfg.Pipeline.LowLLM.TimeoutSec
	if baseCfg.TimeoutSec == 0 {
		baseCfg.TimeoutSec = 120
	}
	baseCfg.MaxAttempts = 1
	baseCfg.BackoffSec = 0
	baseCfg.TranslatorResponseMode = cfg.Pipeline.LowLLM.TranslatorResponseMode
	if baseCfg.TranslatorResponseMode == "" {
		baseCfg.TranslatorResponseMode = responseModePlain
	}

	skill := newTranslateSkill(shared.LoadContext(cfg.Translation.ContextFiles), shared.LoadRules(cfg.Translation.RulesFile))
	client, err := newServerClientWithConfig(baseCfg.LLMBackend, baseCfg.ServerURL, baseCfg.Model, baseCfg.Agent, baseCfg, skill, baseCfg.TimeoutSec, &shared.MetricCollector{}, nil)
	if err != nil {
		t.Fatalf("create server client: %v", err)
	}

	rt := translationRuntime{
		cfg:    baseCfg,
		client: client,
		skill:  skill,
	}

	limitPerFamily := 10
	if raw := os.Getenv("RECOVERY_LIMIT_PER_FAMILY"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limitPerFamily = n
		}
	}

	fx := loadResidualNoRowFixture(t)
	families := map[string][]residualNoRowEntry{}
	for _, row := range fx.Rows {
		family := classifyResidualFamily(row)
		families[family] = append(families[family], row)
	}

	type familyResult struct {
		family    string
		tested    int
		recovered int
	}
	results := make([]familyResult, 0, len(families))

	for family, rows := range families {
		sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
		tested := 0
		recovered := 0
		for _, row := range rows {
			if tested >= limitPerFamily {
				break
			}
			tested++
			task := translationTask{
				ID:          row.ID,
				BodyEN:      row.SourceRaw,
				ContextEN:   row.ContextEN,
				PrevEN:      row.PrevEN,
				NextEN:      row.NextEN,
				TextRole:    row.TextRole,
				StatCheck:   row.StatCheck,
				ChoiceMode:  row.ChoiceMode,
				SourceFile:  row.SourceFile,
				RetryReason: row.RetryReason,
			}
			proposal, ok, invalid, transErr := collectSingleProposal(rt, "recoverability", task, skill.shapeHint(), client)
			if ok && invalid == 0 && transErr == 0 && strings.TrimSpace(proposal.ProposedKO) != "" {
				recovered++
			}
		}
		results = append(results, familyResult{family: family, tested: tested, recovered: recovered})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].family == results[j].family {
			return results[i].tested > results[j].tested
		}
		return results[i].family < results[j].family
	})
	for _, r := range results {
		var pct float64
		if r.tested > 0 {
			pct = float64(r.recovered) * 100.0 / float64(r.tested)
		}
		t.Logf("family=%s tested=%d recovered=%d recoverability=%.1f%%", r.family, r.tested, r.recovered, pct)
	}
}

func classifyResidualFamily(row residualNoRowEntry) string {
	src := row.SourceRaw
	if recoveryStatLikeQuoteRe.MatchString(src) {
		return "stat_like_open_quote"
	}
	if recoveryActionOpenQuoteRe.MatchString(src) {
		return "action_open_quote"
	}
	if strings.Contains(src, "\"") {
		return "open_quote_other"
	}
	role := row.TextRole
	if role == "glossary" || role == "quest" {
		if len(src) > 140 {
			if strings.Contains(src, " - ") {
				return "glossary_definition_long"
			}
			return "glossary_expository_long"
		}
	}
	if role == "system" || role == "narration" {
		if len(src) > 140 {
			if strings.Contains(src, " - ") {
				return role + "_definition_long"
			}
			return role + "_expository_long"
		}
	}
	return "other"
}
