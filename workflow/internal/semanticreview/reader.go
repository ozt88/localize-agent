package semanticreview

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"localize-agent/workflow/pkg/platform"
	"localize-agent/workflow/pkg/segmentchunk"
)

type reviewContext struct {
	prevID      string
	nextID      string
	textRole    string
	speakerHint string
	contextEN   string
}

func loadDoneItems(cfg Config, limit int) ([]ReviewItem, error) {
	return loadDoneItemsFiltered(cfg, nil, limit)
}

func loadDoneItemsFiltered(cfg Config, ids []string, limit int) ([]ReviewItem, error) {
	db, err := platform.OpenTranslationCheckpointDB(cfg.CheckpointBackend, cfg.CheckpointDB, cfg.CheckpointDSN)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	query := `select id, ko_json, pack_json from items where status = 'done' order by id`
	args := []any{}
	if len(ids) > 0 {
		placeholders := make([]string, 0, len(ids))
		for _, id := range ids {
			placeholders = append(placeholders, "?")
			args = append(args, id)
		}
		query = fmt.Sprintf(`select id, ko_json, pack_json from items where status = 'done' and id in (%s) order by id`, strings.Join(placeholders, ","))
	}
	if limit > 0 {
		query = fmt.Sprintf("%s limit %d", query, limit)
	}
	rows, err := db.Query(platform.RebindSQL(cfg.CheckpointBackend, query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	currentByID := map[string]string{}
	sourceByID := map[string]string{}
	packByID := map[string]map[string]any{}
	ordered := make([]ReviewItem, 0)
	for rows.Next() {
		var id string
		var koJSONRaw any
		var packJSONRaw any
		if err := rows.Scan(&id, &koJSONRaw, &packJSONRaw); err != nil {
			return nil, err
		}
		koJSON := platform.NormalizeSQLValue(koJSONRaw)
		packJSON := platform.NormalizeSQLValue(packJSONRaw)
		var koObj map[string]any
		var packObj map[string]any
		if err := json.Unmarshal([]byte(koJSON), &koObj); err != nil {
			continue
		}
		if err := json.Unmarshal([]byte(packJSON), &packObj); err != nil {
			continue
		}
		text, _ := koObj["Text"].(string)
		sourceRaw, _ := packObj["source_raw"].(string)
		en, _ := packObj["en"].(string)
		freshKO := stringField(packObj, "fresh_ko")
		if freshKO == "" {
			freshKO = text
		}
		sourceEN := sourceRaw
		if sourceEN == "" {
			sourceEN = en
		}
		if freshKO == "" || sourceEN == "" {
			continue
		}
		item := ReviewItem{
			ID:           id,
			SourceEN:     sourceEN,
			TranslatedKO: freshKO,
			CurrentKO:    stringField(packObj, "current_ko"),
			FreshKO:      freshKO,
			PrevEN:       stringField(packObj, "prev_en"),
			NextEN:       stringField(packObj, "next_en"),
			PrevKO:       stringField(packObj, "prev_ko"),
			NextKO:       stringField(packObj, "next_ko"),
			TextRole:     stringField(packObj, "text_role"),
			SpeakerHint:  stringField(packObj, "speaker_hint"),
			ContextEN:    stringField(packObj, "context_en"),
			RetryReason:  stringField(packObj, "retry_reason"),
		}
		ordered = append(ordered, item)
		currentByID[id] = text
		sourceByID[id] = sourceEN
		packByID[id] = packObj
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(ordered) == 0 {
		return ordered, nil
	}

	contextMap, err := loadReviewContexts(cfg, currentByID, sourceByID)
	if err != nil {
		return nil, err
	}
	if err := hydrateNeighborMaps(cfg, contextMap, currentByID, sourceByID); err != nil {
		return nil, err
	}
	for i := range ordered {
		item := &ordered[i]
		if ctx, ok := contextMap[item.ID]; ok {
			if item.PrevEN == "" {
				item.PrevEN = sourceByID[ctx.prevID]
			}
			if item.NextEN == "" {
				item.NextEN = sourceByID[ctx.nextID]
			}
			if item.PrevKO == "" {
				item.PrevKO = currentByID[ctx.prevID]
			}
			if item.NextKO == "" {
				item.NextKO = currentByID[ctx.nextID]
			}
			if item.TextRole == "" {
				item.TextRole = ctx.textRole
			}
			if item.SpeakerHint == "" {
				item.SpeakerHint = ctx.speakerHint
			}
			if item.ContextEN == "" {
				item.ContextEN = ctx.contextEN
			}
		}
		if item.ContextEN == "" {
			item.ContextEN = stringField(packByID[item.ID], "context_en")
		}
	}
	return ordered, nil
}

func loadReviewContexts(cfg Config, currentByID, sourceByID map[string]string) (map[string]reviewContext, error) {
	if strings.TrimSpace(cfg.TranslatorPackageChunks) == "" {
		return map[string]reviewContext{}, nil
	}
	raw, err := os.ReadFile(cfg.TranslatorPackageChunks)
	if err != nil {
		return nil, err
	}
	var pkg segmentchunk.ChunkedTranslatorPackage
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return nil, err
	}

	out := map[string]reviewContext{}
	for _, chunk := range pkg.Chunks {
		lineIDs := make([]string, 0, len(chunk.Lines))
		for _, line := range chunk.Lines {
			lineIDs = append(lineIDs, line.LineID)
		}
		contextEN := buildChunkContextEN(lineIDs, sourceByID)
		for _, line := range chunk.Lines {
			if _, ok := currentByID[line.LineID]; !ok {
				continue
			}
			ctx := reviewContext{
				textRole:  line.TextRole,
				contextEN: contextEN,
			}
			if line.PrevLineID != nil {
				ctx.prevID = *line.PrevLineID
			}
			if line.NextLineID != nil {
				ctx.nextID = *line.NextLineID
			}
			if line.SpeakerHint != nil {
				ctx.speakerHint = *line.SpeakerHint
			}
			out[line.LineID] = ctx
		}
	}
	return out, nil
}

func hydrateNeighborMaps(cfg Config, contextMap map[string]reviewContext, currentByID, sourceByID map[string]string) error {
	missing := map[string]bool{}
	for _, ctx := range contextMap {
		if ctx.prevID != "" && (currentByID[ctx.prevID] == "" || sourceByID[ctx.prevID] == "") {
			missing[ctx.prevID] = true
		}
		if ctx.nextID != "" && (currentByID[ctx.nextID] == "" || sourceByID[ctx.nextID] == "") {
			missing[ctx.nextID] = true
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if err := loadNeighborDataFromCheckpoint(cfg.CheckpointBackend, cfg.CheckpointDB, cfg.CheckpointDSN, missing, currentByID, sourceByID); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.SourcePath) != "" {
		if sourceStrings, err := readStringTexts(cfg.SourcePath); err == nil {
			for id := range missing {
				if sourceByID[id] == "" {
					sourceByID[id] = sourceStrings[id]
				}
			}
		}
	}
	return nil
}

func loadNeighborDataFromCheckpoint(backend string, dbPath string, dsn string, ids map[string]bool, currentByID, sourceByID map[string]string) error {
	if len(ids) == 0 {
		return nil
	}
	db, err := platform.OpenTranslationCheckpointDB(backend, dbPath, dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	placeholders := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	rows, err := db.Query(
		platform.RebindSQL(backend, fmt.Sprintf("SELECT id, ko_json, pack_json FROM items WHERE status = 'done' AND id IN (%s)", strings.Join(placeholders, ","))),
		args...,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var koJSONRaw any
		var packJSONRaw any
		if err := rows.Scan(&id, &koJSONRaw, &packJSONRaw); err != nil {
			return err
		}
		koJSON := platform.NormalizeSQLValue(koJSONRaw)
		packJSON := platform.NormalizeSQLValue(packJSONRaw)
		if currentByID[id] == "" && strings.TrimSpace(koJSON) != "" {
			var koObj map[string]any
			if json.Unmarshal([]byte(koJSON), &koObj) == nil {
				currentByID[id], _ = koObj["Text"].(string)
			}
		}
		if sourceByID[id] == "" && strings.TrimSpace(packJSON) != "" {
			var packObj map[string]any
			if json.Unmarshal([]byte(packJSON), &packObj) == nil {
				sourceByID[id] = stringField(packObj, "source_raw")
				if sourceByID[id] == "" {
					sourceByID[id] = stringField(packObj, "en")
				}
			}
		}
	}
	return rows.Err()
}

func readStringTexts(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	items, _ := root["strings"].(map[string]any)
	out := make(map[string]string, len(items))
	for id, value := range items {
		obj, _ := value.(map[string]any)
		text, _ := obj["Text"].(string)
		if text != "" {
			out[id] = text
		}
	}
	return out, nil
}

func buildChunkContextEN(lineIDs []string, sourceByID map[string]string) string {
	parts := make([]string, 0, len(lineIDs))
	for _, id := range lineIDs {
		if text := strings.TrimSpace(sourceByID[id]); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}
