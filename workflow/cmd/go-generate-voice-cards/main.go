package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"localize-agent/workflow/internal/clustertranslate"
	"localize-agent/workflow/pkg/platform"
	"localize-agent/workflow/pkg/shared"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// abilityScoreSpeakers are excluded from voice card generation
// (managed separately via abilityScoreVoice in prompt.go).
var abilityScoreSpeakers = map[string]bool{
	"wis": true, "str": true, "int": true,
	"cha": true, "dex": true, "con": true,
}

// speakerEntry matches the speaker_allow_list.json format.
type speakerEntry struct {
	Name      string `json:"name"`
	Frequency int    `json:"frequency"`
	Verified  bool   `json:"verified"`
}

type speakerAllowList struct {
	Speakers []speakerEntry `json:"speakers"`
}

// kattegatCard is the hardcoded voice card for Kattegatt (ancient entity, archaic speech).
var kattegatCard = clustertranslate.VoiceCard{
	SpeechStyle: "고어체. 어미: ~도다, ~노라, ~옵니다. thou/thy → 그대/당신. 장중하고 느린 리듬",
	Honorific:   "존대 (모든 대상에게 고어 격식체)",
	Personality: "위엄 있는 고대 존재, 신탁적, 신비로운",
	Relationships: map[string]string{
		"*": "초월적 거리감 — 모두에게 고어체",
	},
}

func main() {
	os.Exit(run())
}

func run() int {
	var (
		projectPath  string
		outputPath   string
		speakersPath string
		wikiDir      string
		sampleCount  int
		minFrequency int
		dsn          string
		backend      string
		dbPath       string
	)

	flag.StringVar(&projectPath, "project", "projects/esoteric-ebb/project.json", "project.json path")
	flag.StringVar(&outputPath, "output", "projects/esoteric-ebb/context/voice_cards.json", "output voice_cards.json path")
	flag.StringVar(&speakersPath, "speakers", "projects/esoteric-ebb/context/speaker_allow_list.json", "speaker_allow_list.json path")
	flag.StringVar(&wikiDir, "wiki-dir", "", "wiki_markdown directory path for character background context")
	flag.IntVar(&sampleCount, "sample-count", 20, "dialogue samples per character")
	flag.IntVar(&minFrequency, "min-frequency", 100, "minimum frequency for voice card generation")
	flag.StringVar(&dsn, "dsn", "", "PostgreSQL DSN (overrides project config)")
	flag.StringVar(&backend, "backend", "", "DB backend: postgres or sqlite (overrides project config)")
	flag.StringVar(&dbPath, "db", "", "SQLite DB path (overrides project config)")
	flag.Parse()

	// 1. Load speaker allow list
	speakerRaw, err := os.ReadFile(speakersPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate-voice-cards: read speakers: %v\n", err)
		return 1
	}
	var allowList speakerAllowList
	if err := json.Unmarshal(speakerRaw, &allowList); err != nil {
		fmt.Fprintf(os.Stderr, "generate-voice-cards: parse speakers: %v\n", err)
		return 1
	}

	// Filter: frequency >= min-frequency, exclude ability-score speakers
	var targetSpeakers []string
	for _, s := range allowList.Speakers {
		lower := strings.ToLower(s.Name)
		if abilityScoreSpeakers[lower] {
			continue
		}
		if s.Frequency >= minFrequency || s.Name == "Kattegatt" {
			targetSpeakers = append(targetSpeakers, s.Name)
		}
	}
	fmt.Fprintf(os.Stderr, "generate-voice-cards: %d target speakers (freq >= %d)\n", len(targetSpeakers), minFrequency)

	// 2. Load existing voice cards (idempotent: skip already generated)
	existing := make(map[string]clustertranslate.VoiceCard)
	if data, err := os.ReadFile(outputPath); err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			fmt.Fprintf(os.Stderr, "generate-voice-cards: warning: existing file parse error: %v\n", err)
		}
	}

	// Always inject/update Kattegatt with hardcoded archaic profile.
	existing["Kattegatt"] = kattegatCard
	fmt.Fprintf(os.Stderr, "generate-voice-cards: injected hardcoded Kattegatt voice card\n")

	// Filter out already-generated speakers (except Kattegatt which is hardcoded above)
	var needGeneration []string
	for _, name := range targetSpeakers {
		if name == "Kattegatt" {
			continue // already handled
		}
		if _, ok := existing[name]; !ok {
			needGeneration = append(needGeneration, name)
		}
	}
	if len(needGeneration) == 0 {
		fmt.Fprintf(os.Stderr, "generate-voice-cards: all %d speakers already have voice cards\n", len(targetSpeakers))
		return writeOutput(outputPath, existing)
	}
	fmt.Fprintf(os.Stderr, "generate-voice-cards: %d speakers need generation\n", len(needGeneration))

	// 3. Load project config for DB/LLM settings
	projCfg, _, err := shared.LoadProjectConfig("esoteric-ebb", "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate-voice-cards: load project config: %v\n", err)
		return 1
	}

	// Determine DB backend and connection
	dbBackend := backend
	if dbBackend == "" {
		dbBackend = "sqlite"
	}
	dbDSN := dsn
	dbFilePath := dbPath
	if dbFilePath == "" {
		dbFilePath = projCfg.Translation.CheckpointDB
	}

	normalizedBackend, err := platform.NormalizeDBBackend(dbBackend)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate-voice-cards: %v\n", err)
		return 1
	}

	var db *sql.DB
	switch normalizedBackend {
	case platform.DBBackendSQLite:
		db, err = sql.Open("sqlite", dbFilePath)
	case platform.DBBackendPostgres:
		db, err = sql.Open("pgx", dbDSN)
	default:
		fmt.Fprintf(os.Stderr, "generate-voice-cards: unsupported backend: %s\n", normalizedBackend)
		return 1
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate-voice-cards: open db: %v\n", err)
		return 1
	}
	defer db.Close()

	// 4. Set up LLM client (use pipeline low_llm profile)
	llmProfile := projCfg.Pipeline.LowLLM
	providerID, modelID, err := platform.ParseModel(llmProfile.Model)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate-voice-cards: parse model: %v\n", err)
		return 1
	}

	metrics := &shared.MetricCollector{}
	llmClient := platform.NewSessionLLMClient(
		llmProfile.ServerURL,
		llmProfile.TimeoutSec,
		metrics,
		nil, // no trace sink needed
	)

	profile := platform.LLMProfile{
		ProviderID:   providerID,
		ModelID:      modelID,
		ResetHistory: true,
	}

	// 5. Generate voice cards for each speaker
	for _, name := range needGeneration {
		// Sample dialogues from DB
		samples, err := sampleDialogues(db, name, sampleCount)
		if err != nil {
			fmt.Fprintf(os.Stderr, "generate-voice-cards: sample %s: %v (skipping)\n", name, err)
			continue
		}
		if len(samples) == 0 {
			fmt.Fprintf(os.Stderr, "generate-voice-cards: %s has no dialogues (skipping)\n", name)
			continue
		}

		// Load co-occurring speakers from DB for relationship context.
		coSpeakers, _ := coOccurringSpeakers(db, name, 5)

		// Load wiki text if wiki-dir provided.
		wikiText := loadWikiText(wikiDir, name)

		// Build LLM prompt
		prompt := buildVoiceCardPrompt(name, samples, coSpeakers, wikiText)

		// Call LLM with server-restart retry (OpenCode may exit after each request).
		var resp string
		const maxLLMRetries = 3
		for attempt := 0; attempt < maxLLMRetries; attempt++ {
			// Ensure server is alive before each request.
			if !probeServer(llmProfile.ServerURL) {
				fmt.Fprintf(os.Stderr, "generate-voice-cards: server not reachable at %s, restarting...\n", llmProfile.ServerURL)
				if restartErr := restartOpenCode(llmProfile.ServerURL); restartErr != nil {
					fmt.Fprintf(os.Stderr, "generate-voice-cards: restart failed: %v\n", restartErr)
					break
				}
				llmClient.ResetAllSessions()
			}

			resp, err = llmClient.SendPrompt("voice-card-"+name, profile, prompt)
			if err == nil {
				break
			}
			fmt.Fprintf(os.Stderr, "generate-voice-cards: LLM attempt %d/%d for %s failed: %v\n", attempt+1, maxLLMRetries, name, err)
			llmClient.ResetAllSessions()
			time.Sleep(500 * time.Millisecond)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "generate-voice-cards: LLM error for %s after %d attempts: %v (skipping)\n", name, maxLLMRetries, err)
			continue
		}

		// Parse response JSON
		card, err := parseVoiceCardResponse(resp)
		if err != nil {
			fmt.Fprintf(os.Stderr, "generate-voice-cards: parse response for %s: %v (skipping)\n", name, err)
			continue
		}

		existing[name] = card
		fmt.Fprintf(os.Stderr, "generate-voice-cards: generated voice card for %s\n", name)
	}

	// 6. Write output
	return writeOutput(outputPath, existing)
}

// sampleDialogues fetches random dialogue samples for a speaker from the v2 pipeline DB.
func sampleDialogues(db *sql.DB, speaker string, limit int) ([]string, error) {
	query := `SELECT source_raw FROM pipeline_items_v2 WHERE speaker = $1 AND state = 'done' ORDER BY RANDOM() LIMIT $2`
	rows, err := db.Query(query, speaker, limit)
	if err != nil {
		// Fallback to ? placeholder for SQLite
		rows, err = db.Query(`SELECT source_raw FROM pipeline_items_v2 WHERE speaker = ? AND state = 'done' ORDER BY RANDOM() LIMIT ?`, speaker, limit)
		if err != nil {
			return nil, err
		}
	}
	defer rows.Close()

	var samples []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		samples = append(samples, s)
	}
	return samples, rows.Err()
}

// coOccurringSpeakers returns the top N speakers who appear most often in the same knot as the given speaker.
func coOccurringSpeakers(db *sql.DB, speaker string, limit int) ([]string, error) {
	query := `
		SELECT other.speaker, COUNT(*) as cnt
		FROM pipeline_items_v2 self
		JOIN pipeline_items_v2 other ON self.knot = other.knot AND other.speaker != self.speaker AND other.speaker != ''
		WHERE self.speaker = $1
		GROUP BY other.speaker
		ORDER BY cnt DESC
		LIMIT $2`
	rows, err := db.Query(query, speaker, limit)
	if err != nil {
		// Fallback for SQLite
		rows, err = db.Query(strings.ReplaceAll(strings.ReplaceAll(query, "$1", "?"), "$2", "?"), speaker, limit)
		if err != nil {
			return nil, err
		}
	}
	defer rows.Close()

	var speakers []string
	for rows.Next() {
		var s string
		var cnt int
		if err := rows.Scan(&s, &cnt); err != nil {
			return nil, err
		}
		speakers = append(speakers, s)
	}
	return speakers, rows.Err()
}

// loadWikiText searches wiki_markdown directory for a file matching the speaker name (case-insensitive contains).
// Returns empty string if wiki-dir is empty or no match found.
func loadWikiText(wikiDir, speakerName string) string {
	if wikiDir == "" {
		return ""
	}
	entries, err := os.ReadDir(wikiDir)
	if err != nil {
		return ""
	}
	lowerName := strings.ToLower(speakerName)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fileName := strings.ToLower(strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
		if strings.Contains(fileName, lowerName) {
			data, err := os.ReadFile(filepath.Join(wikiDir, entry.Name()))
			if err != nil {
				continue
			}
			text := string(data)
			// Truncate to ~2000 chars to avoid oversized prompts.
			if len(text) > 2000 {
				text = text[:2000] + "\n[truncated]"
			}
			return text
		}
	}
	return ""
}

// buildVoiceCardPrompt creates the LLM prompt for voice card generation.
// Includes wiki background, co-occurring speakers for relationship analysis.
func buildVoiceCardPrompt(name string, samples []string, coSpeakers []string, wikiText string) string {
	var sb strings.Builder

	if wikiText != "" {
		fmt.Fprintf(&sb, "## %s — 캐릭터 배경 (위키)\n\n%s\n\n", name, wikiText)
	}

	sb.WriteString(fmt.Sprintf("다음은 게임 캐릭터 \"%s\"의 영어 대사 샘플입니다:\n\n", name))
	for i, s := range samples {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, s)
	}
	sb.WriteString("\n")
	sb.WriteString("이 캐릭터의 기본 모드(baseline tone)를 분석하여 한국어 번역 시 적용할 voice profile을 JSON으로 작성하세요:\n")
	sb.WriteString("- speech_style: 이 캐릭터의 화법 스타일. 구체적인 어미 패턴 포함 필수 (예: \"어미: ~야, ~잖아, ~거든. 짧고 직접적인 문장\")\n")
	sb.WriteString("- honorific: 존댓말 레벨 (\"반말\", \"평어\", 또는 \"존대\" 중 하나)\n")
	sb.WriteString("- personality: 성격 키워드 2-3개 (예: \"내성적, 관찰력 있음, 가끔 날카로운 유머\")\n")

	if len(coSpeakers) > 0 {
		sb.WriteString(fmt.Sprintf("- relationships: 이 캐릭터가 다른 캐릭터(%s)와 대화할 때 어조/존댓말 변화를 JSON 객체로 작성. 예: {\"Braxo\": \"격식체 → 반말로 전환\", \"Snell\": \"항상 존대\"}\n", strings.Join(coSpeakers, ", ")))
	} else {
		sb.WriteString("- relationships: 빈 객체 {} 가능\n")
	}

	sb.WriteString("\nJSON만 출력하세요: {\"speech_style\":\"...\", \"honorific\":\"...\", \"personality\":\"...\", \"relationships\":{...}}\n")
	return sb.String()
}

// parseVoiceCardResponse extracts a VoiceCard from LLM response text.
func parseVoiceCardResponse(resp string) (clustertranslate.VoiceCard, error) {
	// Try to find JSON in the response
	resp = strings.TrimSpace(resp)

	// Strip markdown code fences if present
	if strings.HasPrefix(resp, "```") {
		lines := strings.Split(resp, "\n")
		var jsonLines []string
		inBlock := false
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inBlock = !inBlock
				continue
			}
			if inBlock {
				jsonLines = append(jsonLines, line)
			}
		}
		resp = strings.Join(jsonLines, "\n")
	}

	// Find JSON object boundaries
	start := strings.Index(resp, "{")
	end := strings.LastIndex(resp, "}")
	if start < 0 || end < 0 || end <= start {
		return clustertranslate.VoiceCard{}, fmt.Errorf("no JSON object found in response")
	}
	jsonStr := resp[start : end+1]

	var card clustertranslate.VoiceCard
	if err := json.Unmarshal([]byte(jsonStr), &card); err != nil {
		return clustertranslate.VoiceCard{}, fmt.Errorf("parse JSON: %w", err)
	}
	if card.SpeechStyle == "" || card.Honorific == "" || card.Personality == "" {
		return clustertranslate.VoiceCard{}, fmt.Errorf("incomplete voice card: missing required fields")
	}
	return card, nil
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

// restartOpenCode starts a fresh OpenCode server on the port extracted from serverURL.
// It spawns the process detached and waits up to 20s for readiness.
func restartOpenCode(serverURL string) error {
	// Extract port from URL (e.g. "http://127.0.0.1:4113" -> "4113")
	port := "4112"
	if idx := strings.LastIndex(serverURL, ":"); idx >= 0 {
		candidate := serverURL[idx+1:]
		// Strip path if any
		if slash := strings.Index(candidate, "/"); slash >= 0 {
			candidate = candidate[:slash]
		}
		if candidate != "" {
			port = candidate
		}
	}

	// Find OpenCode executable via Scoop path.
	opencodeExe := `C:\Users\DELL\scoop\apps\opencode\current\opencode.exe`
	if _, err := os.Stat(opencodeExe); err != nil {
		return fmt.Errorf("opencode executable not found: %s", opencodeExe)
	}

	// Use an isolated working directory so OpenCode doesn't scan project files.
	isolatedDir := filepath.Join(os.TempDir(), "opencode-serve-isolated")
	_ = os.MkdirAll(isolatedDir, 0755)

	// Launch detached — we don't wait for it since it runs as a server.
	cmd := exec.Command("powershell.exe",
		"-NoProfile", "-ExecutionPolicy", "Bypass",
		"-Command",
		fmt.Sprintf("Start-Process -FilePath '%s' -ArgumentList 'serve','--port','%s' -WindowStyle Hidden -WorkingDirectory '%s'",
			opencodeExe, port, isolatedDir),
	)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch opencode: %w", err)
	}
	// Don't wait for the powershell wrapper; let it spawn the server.
	go cmd.Wait() //nolint:errcheck

	// Wait up to 20s for server to become reachable.
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(1 * time.Second)
		if probeServer(serverURL) {
			fmt.Fprintf(os.Stderr, "generate-voice-cards: server ready at %s\n", serverURL)
			return nil
		}
	}
	return fmt.Errorf("server did not become reachable at %s within 20s", serverURL)
}

// writeOutput writes voice cards to JSON file.
func writeOutput(path string, cards map[string]clustertranslate.VoiceCard) int {
	data, err := json.MarshalIndent(cards, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate-voice-cards: marshal: %v\n", err)
		return 1
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "generate-voice-cards: write: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "generate-voice-cards: wrote %d voice cards to %s\n", len(cards), path)
	return 0
}
