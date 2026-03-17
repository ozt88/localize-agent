package translation

import (
	"encoding/json"
	"strings"

	"localize-agent/workflow/pkg/platform"
)

func loadCheckpointPromptMetas(backend string, dbPath string, dsn string, ids []string) (map[string]checkpointPromptMeta, error) {
	if strings.TrimSpace(dbPath) == "" || len(ids) == 0 {
		return map[string]checkpointPromptMeta{}, nil
	}

	db, err := platform.OpenTranslationCheckpointDB(backend, dbPath, dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	const batchSize = 500
	out := make(map[string]checkpointPromptMeta, len(ids))
	for start := 0; start < len(ids); start += batchSize {
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		placeholders := make([]string, 0, end-start)
		args := make([]any, 0, end-start)
		for _, id := range ids[start:end] {
			placeholders = append(placeholders, "?")
			args = append(args, id)
		}
		rows, err := db.Query(
			platform.RebindSQL(backend, "SELECT id, pack_json FROM items WHERE pack_json IS NOT NULL AND id IN ("+strings.Join(placeholders, ",")+")"),
			args...,
		)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			var packJSONRaw any
			if err := rows.Scan(&id, &packJSONRaw); err != nil {
				rows.Close()
				return nil, err
			}
			packJSON := platform.NormalizeSQLValue(packJSONRaw)
			if strings.TrimSpace(packJSON) == "" {
				continue
			}
			var packObj map[string]any
			if err := json.Unmarshal([]byte(packJSON), &packObj); err != nil {
				continue
			}
			out[id] = checkpointPromptMeta{
				ContextEN:     stringField(packObj, "context_en"),
				CurrentKO:     stringField(packObj, "current_ko"),
				PrevEN:        stringField(packObj, "prev_en"),
				NextEN:        stringField(packObj, "next_en"),
				PrevKO:        stringField(packObj, "prev_ko"),
				NextKO:        stringField(packObj, "next_ko"),
				TextRole:      stringField(packObj, "text_role"),
				SpeakerHint:   stringField(packObj, "speaker_hint"),
				RetryReason:   stringField(packObj, "retry_reason"),
				TranslationPolicy: stringField(packObj, "translation_policy"),
				SourceType:    stringField(packObj, "source_type"),
				SourceFile:    stringField(packObj, "source_file"),
				ResourceKey:   stringField(packObj, "resource_key"),
				MetaPathLabel: stringField(packObj, "meta_path_label"),
				SceneHint:     stringField(packObj, "scene_hint"),
				SegmentID:     stringField(packObj, "segment_id"),
				SegmentPos:    intPointerField(packObj, "segment_pos"),
				ChoiceBlockID: stringField(packObj, "choice_block_id"),
				PrevLineID:    stringField(packObj, "prev_line_id"),
				NextLineID:    stringField(packObj, "next_line_id"),
				StatCheck:     stringField(packObj, "stat_check"),
				ChoiceMode:    stringField(packObj, "choice_mode"),
				IsStatCheck:   boolField(packObj, "is_stat_check"),
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return out, nil
}

func stringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}

func intPointerField(m map[string]any, key string) *int {
	if m == nil {
		return nil
	}
	switch v := m[key].(type) {
	case float64:
		n := int(v)
		return &n
	case int:
		n := v
		return &n
	case int64:
		n := int(v)
		return &n
	case json.Number:
		if i, err := v.Int64(); err == nil {
			n := int(i)
			return &n
		}
	}
	return nil
}

func boolField(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	switch v := m[key].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	}
	return false
}
