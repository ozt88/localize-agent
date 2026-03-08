package semanticreview

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

func LoadDoneItems(dbPath string, limit int) ([]ReviewItem, error) {
	return loadDoneItemsFiltered(dbPath, nil, limit)
}

func loadDoneItemsFiltered(dbPath string, ids []string, limit int) ([]ReviewItem, error) {
	db, err := sql.Open("sqlite", dbPath)
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
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ReviewItem
	for rows.Next() {
		var id string
		var koJSON string
		var packJSON string
		if err := rows.Scan(&id, &koJSON, &packJSON); err != nil {
			return nil, err
		}
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
		sourceEN := sourceRaw
		if sourceEN == "" {
			sourceEN = en
		}
		if text == "" || sourceEN == "" {
			continue
		}
		item := ReviewItem{
			ID:           id,
			SourceEN:     sourceEN,
			TranslatedKO: text,
		}
		if v, ok := packObj["prev_en"].(string); ok {
			item.PrevEN = v
		}
		if v, ok := packObj["next_en"].(string); ok {
			item.NextEN = v
		}
		if v, ok := packObj["text_role"].(string); ok {
			item.TextRole = v
		}
		if v, ok := packObj["speaker_hint"].(string); ok {
			item.SpeakerHint = v
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
