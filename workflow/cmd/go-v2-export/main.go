package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"localize-agent/workflow/internal/contracts"
	"localize-agent/workflow/internal/inkparse"
	"localize-agent/workflow/internal/v2pipeline"
	"localize-agent/workflow/pkg/platform"
	"localize-agent/workflow/pkg/shared"
)

func main() { os.Exit(run()) }

func run() int {
	var (
		dsn             string
		backend         string
		dbPath          string
		outDir          string
		textassetDir    string  // original TextAsset .txt files (ink JSON)
		localizationDir string  // original localizationtexts CSV dir (8 files)
		localizationOut string  // output dir for translated CSVs
		project         string  // project name for project config loading
		projectDir      string  // project directory path
		minCoverage     float64
	)

	flag.StringVar(&dsn, "dsn", "", "PostgreSQL DSN")
	flag.StringVar(&backend, "backend", "postgres", "DB backend: postgres or sqlite")
	flag.StringVar(&dbPath, "db", "", "SQLite DB path")
	flag.StringVar(&outDir, "out-dir", "", "output directory for patch artifacts (required)")
	flag.StringVar(&textassetDir, "textasset-dir", "", "directory containing original TextAsset .txt files (required for textasset export)")
	flag.StringVar(&localizationDir, "localization-dir", "", "directory containing original localizationtexts CSV files (required for CSV export)")
	flag.StringVar(&localizationOut, "localization-out", "", "output directory for translated CSVs (defaults to {out-dir}/localizationtexts)")
	flag.StringVar(&project, "project", "", "project name for LLM configuration (required for CSV translation)")
	flag.StringVar(&projectDir, "project-dir", "", "project directory path (alternative to -project)")
	flag.Float64Var(&minCoverage, "min-coverage", 0.0, "minimum done ratio 0.0-1.0; 0 = no check (per D-10)")
	flag.Parse()

	if outDir == "" {
		fmt.Fprintf(os.Stderr, "v2-export: -out-dir is required\n")
		return 1
	}

	// Open store
	store, err := v2pipeline.OpenStore(backend, dbPath, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "v2-export: open store: %v\n", err)
		return 1
	}
	defer store.Close()

	// Coverage check (per D-08, D-10)
	counts, err := store.CountByState()
	if err != nil {
		fmt.Fprintf(os.Stderr, "v2-export: count: %v\n", err)
		return 1
	}
	total := 0
	for _, c := range counts {
		total += c
	}
	doneCount := counts[contracts.StateDone]
	failedCount := counts[contracts.StateFailed]
	if total == 0 {
		fmt.Fprintf(os.Stderr, "v2-export: no items in pipeline\n")
		return 1
	}
	coverage := float64(doneCount) / float64(total)
	failRate := float64(failedCount) / float64(total)
	fmt.Fprintf(os.Stderr, "Pipeline: total=%d done=%d failed=%d coverage=%.1f%% failRate=%.1f%%\n",
		total, doneCount, failedCount, coverage*100, failRate*100)

	// Fail rate threshold (per D-08): >5% failed = warning, >20% = abort
	if failRate > 0.20 {
		fmt.Fprintf(os.Stderr, "v2-export: ABORT -- failure rate %.1f%% exceeds 20%% threshold\n", failRate*100)
		return 1
	}
	if failRate > 0.05 {
		fmt.Fprintf(os.Stderr, "v2-export: WARNING -- failure rate %.1f%% exceeds 5%%\n", failRate*100)
	}
	if minCoverage > 0 && coverage < minCoverage {
		fmt.Fprintf(os.Stderr, "v2-export: coverage %.1f%% below minimum %.1f%%\n", coverage*100, minCoverage*100)
		return 1
	}

	// Query done items
	items, err := store.QueryDone()
	if err != nil {
		fmt.Fprintf(os.Stderr, "v2-export: query: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "Loaded %d done items\n", len(items))

	// --- 1. translations.json (PATCH-01) ---
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "v2-export: mkdir %s: %v\n", outDir, err)
		return 1
	}
	sidecar := v2pipeline.BuildV3Sidecar(items)
	tjPath := filepath.Join(outDir, "translations.json")
	if err := v2pipeline.WriteTranslationsJSON(tjPath, sidecar); err != nil {
		fmt.Fprintf(os.Stderr, "v2-export: write translations.json: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "Written: %s (%d entries)\n", tjPath, len(sidecar.Entries))

	// --- 2. TextAsset injection (PATCH-02) ---
	if textassetDir != "" {
		taOutDir := filepath.Join(outDir, "textassets")
		if err := os.MkdirAll(taOutDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "v2-export: mkdir %s: %v\n", taOutDir, err)
			return 1
		}

		// Build source_hash -> ko_formatted map from done items.
		// Apply the same LLM escape normalization used in BuildV3Sidecar
		// so TextAsset injection gets clean text (literal \n → real newline, etc.)
		hashToKO := make(map[string]string, len(items))
		for _, item := range items {
			ko := item.KOFormatted
			if ko == "" && item.KORaw != "" {
				ko = item.KORaw
			}
			if ko != "" {
				hashToKO[item.SourceHash] = v2pipeline.CleanTarget(ko)
			}
		}

		// Process each .txt file in textassetDir
		entries, err := os.ReadDir(textassetDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "v2-export: read textasset dir: %v\n", err)
			return 1
		}
		totalTA, replacedTA, missingTA, fileCount := 0, 0, 0, 0
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".txt" {
				continue
			}
			srcPath := filepath.Join(textassetDir, entry.Name())
			data, err := os.ReadFile(srcPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "v2-export: read %s: %v\n", entry.Name(), err)
				continue
			}
			sourceFile := strings.TrimSuffix(entry.Name(), ".txt")
			injected, report, err := inkparse.InjectTranslations(data, sourceFile, hashToKO)
			if err != nil {
				fmt.Fprintf(os.Stderr, "v2-export: inject %s: %v\n", sourceFile, err)
				continue
			}
			totalTA += report.Total
			replacedTA += report.Replaced
			missingTA += report.Missing
			fileCount++

			// Output as .json per D-06 -- ink JSON content, .json extension
			// NOTE: current Plugin.cs scans *.txt only; Phase 4 (PLUGIN-01) will add *.json scanning
			outName := sourceFile + ".json"
			outPath := filepath.Join(taOutDir, outName)
			if err := shared.AtomicWriteFile(outPath, injected, 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "v2-export: write %s: %v\n", outPath, err)
				return 1
			}
		}
		fmt.Fprintf(os.Stderr, "TextAssets: files=%d blocks_total=%d replaced=%d missing=%d\n",
			fileCount, totalTA, replacedTA, missingTA)
	}

	// --- 3. localizationtexts CSV translation (PATCH-03) ---
	if localizationDir != "" {
		csvOutDir := localizationOut
		if csvOutDir == "" {
			csvOutDir = filepath.Join(outDir, "localizationtexts")
		}
		if err := os.MkdirAll(csvOutDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "v2-export: mkdir %s: %v\n", csvOutDir, err)
			return 1
		}

		// Initialize LLM client for CSV translation
		if project == "" && projectDir == "" {
			fmt.Fprintf(os.Stderr, "v2-export: -project or -project-dir is required for CSV translation\n")
			return 1
		}
		projCfg, _, err := shared.LoadProjectConfig(project, projectDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "v2-export: load project config: %v\n", err)
			return 1
		}
		if projCfg == nil {
			fmt.Fprintf(os.Stderr, "v2-export: project config not found\n")
			return 1
		}
		// Use high_llm profile for CSV translation (same quality as main pipeline)
		highLLM := projCfg.Pipeline.HighLLM
		providerID, modelID, err := platform.ParseModel(highLLM.Model)
		if err != nil {
			fmt.Fprintf(os.Stderr, "v2-export: parse model: %v\n", err)
			return 1
		}

		timeoutSec := highLLM.TimeoutSec
		if timeoutSec <= 0 {
			timeoutSec = 120
		}
		metrics := &shared.MetricCollector{}
		llmClient := platform.NewSessionLLMClient(highLLM.ServerURL, timeoutSec, metrics, nil)

		llmProfile := platform.LLMProfile{
			ProviderID: providerID,
			ModelID:    modelID,
			Agent:      highLLM.Agent,
		}

		// Translate each CSV file -- per D-11 all 8 files fully translated
		csvFiles := []string{
			"Feats.txt", "ItemTexts.txt", "JournalTexts.txt", "Popups.txt",
			"QuestPoints.txt", "SheetInfo.txt", "SpellTexts.txt", "UIElements.txt",
		}
		for _, csvFile := range csvFiles {
			srcPath := filepath.Join(localizationDir, csvFile)
			rows, err := v2pipeline.ReadCSVFile(srcPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "v2-export: read CSV %s: %v\n", csvFile, err)
				continue
			}

			// translateFn wraps LLM client for single-string translation
			translateFn := func(english string) (string, error) {
				prompt := fmt.Sprintf("Translate the following English text to Korean. Output ONLY the Korean translation, nothing else.\n\n%s", english)
				return llmClient.SendPrompt("csv-translate", llmProfile, prompt)
			}

			report, err := v2pipeline.TranslateCSVRows(rows, translateFn)
			if err != nil {
				fmt.Fprintf(os.Stderr, "v2-export: translate CSV %s: %v\n", csvFile, err)
				continue
			}
			report.FileName = csvFile

			outPath := filepath.Join(csvOutDir, csvFile)
			if err := v2pipeline.WriteCSVFile(outPath, rows); err != nil {
				fmt.Fprintf(os.Stderr, "v2-export: write CSV %s: %v\n", csvFile, err)
				continue
			}
			fmt.Fprintf(os.Stderr, "CSV %s: total=%d translated=%d skipped=%d errors=%d\n",
				csvFile, report.Total, report.Translated, report.Skipped, report.Errors)
		}
	}

	fmt.Fprintf(os.Stderr, "Export complete.\n")
	return 0
}
