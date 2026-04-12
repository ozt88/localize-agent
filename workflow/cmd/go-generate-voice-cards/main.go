package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

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

func main() {
	os.Exit(run())
}

func run() int {
	var (
		projectPath  string
		outputPath   string
		speakersPath string
		sampleCount  int
		minFrequency int
		dsn          string
		backend      string
		dbPath       string
	)

	flag.StringVar(&projectPath, "project", "projects/esoteric-ebb/project.json", "project.json path")
	flag.StringVar(&outputPath, "output", "projects/esoteric-ebb/context/voice_cards.json", "output voice_cards.json path")
	flag.StringVar(&speakersPath, "speakers", "projects/esoteric-ebb/context/speaker_allow_list.json", "speaker_allow_list.json path")
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
		if s.Frequency >= minFrequency {
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

	// Filter out already-generated speakers
	var needGeneration []string
	for _, name := range targetSpeakers {
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

		// Build LLM prompt
		prompt := buildVoiceCardPrompt(name, samples)

		// Call LLM
		resp, err := llmClient.SendPrompt("voice-card-"+name, profile, prompt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "generate-voice-cards: LLM error for %s: %v (skipping)\n", name, err)
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
	query := `SELECT source_raw FROM pipeline_items_v2 WHERE speaker = ? AND state = 'done' ORDER BY RANDOM() LIMIT ?`
	rows, err := db.Query(query, speaker, limit)
	if err != nil {
		return nil, err
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

// buildVoiceCardPrompt creates the LLM prompt for voice card generation.
func buildVoiceCardPrompt(name string, samples []string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("다음은 게임 캐릭터 \"%s\"의 영어 대사 샘플입니다:\n\n", name))
	for i, s := range samples {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, s)
	}
	sb.WriteString("\n")
	sb.WriteString("이 캐릭터의 기본 모드(baseline tone)를 분석하여 한국어 번역 시 적용할 voice profile을 JSON으로 작성하세요:\n")
	sb.WriteString("- speech_style: 이 캐릭터의 화법 스타일 (예: \"조용하고 신중한 어조, 짧은 문장 선호\")\n")
	sb.WriteString("- honorific: 존댓말 레벨 (\"반말\", \"평어\", 또는 \"존대\" 중 하나)\n")
	sb.WriteString("- personality: 성격 키워드 2-3개 (예: \"내성적, 관찰력 있음, 가끔 날카로운 유머\")\n")
	sb.WriteString("\nJSON만 출력하세요: {\"speech_style\":\"...\", \"honorific\":\"...\", \"personality\":\"...\"}\n")
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
