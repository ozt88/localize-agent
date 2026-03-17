package semanticreview

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestLoadDoneItems_PrefersSourceRaw(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ckpt.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`create table items (id text, status text, ko_json text, pack_json text)`); err != nil {
		t.Fatal(err)
	}
	ko, _ := json.Marshal(map[string]any{"Text": "안녕."})
	pack, _ := json.Marshal(map[string]any{
		"en":           "Hello.",
		"source_raw":   `ROLL9 con-Hello.`,
		"fresh_ko":     "안녕.",
		"prev_en":      "Prev.",
		"next_en":      "Next.",
		"text_role":    "dialogue",
		"speaker_hint": "Snell",
	})
	if _, err := db.Exec(`insert into items(id,status,ko_json,pack_json) values(?,?,?,?)`, "line-1", "done", string(ko), string(pack)); err != nil {
		t.Fatal(err)
	}

	items, err := loadDoneItems(Config{CheckpointDB: dbPath}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("len=%d", len(items))
	}
	if items[0].ID != "line-1" || items[0].SourceEN != `ROLL9 con-Hello.` || items[0].TranslatedKO != "안녕." {
		t.Fatalf("unexpected item: %+v", items[0])
	}
	if items[0].FreshKO != "안녕." {
		t.Fatalf("unexpected fresh ko: %+v", items[0])
	}
}

func TestLoadDoneItems_FallbackToENWhenSourceRawMissing(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ckpt.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`create table items (id text, status text, ko_json text, pack_json text)`); err != nil {
		t.Fatal(err)
	}
	ko, _ := json.Marshal(map[string]any{"Text": "안녕."})
	pack, _ := json.Marshal(map[string]any{
		"en":       "Hello.",
		"fresh_ko": "안녕.",
	})
	if _, err := db.Exec(`insert into items(id,status,ko_json,pack_json) values(?,?,?,?)`, "line-1", "done", string(ko), string(pack)); err != nil {
		t.Fatal(err)
	}

	items, err := loadDoneItems(Config{CheckpointDB: dbPath}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("len=%d", len(items))
	}
	if items[0].SourceEN != "Hello." {
		t.Fatalf("unexpected source en: %+v", items[0])
	}
	if items[0].FreshKO != "안녕." {
		t.Fatalf("unexpected fresh ko: %+v", items[0])
	}
}

func TestLoadDoneItems_LeavesCurrentKOEmptyWhenMissing(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ckpt.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`create table items (id text, status text, ko_json text, pack_json text)`); err != nil {
		t.Fatal(err)
	}
	ko, _ := json.Marshal(map[string]any{"Text": "새 번역"})
	pack, _ := json.Marshal(map[string]any{
		"en":       "Hello.",
		"fresh_ko": "새 번역",
	})
	if _, err := db.Exec(`insert into items(id,status,ko_json,pack_json) values(?,?,?,?)`, "line-1", "done", string(ko), string(pack)); err != nil {
		t.Fatal(err)
	}

	items, err := loadDoneItems(Config{CheckpointDB: dbPath}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("len=%d", len(items))
	}
	if items[0].CurrentKO != "" {
		t.Fatalf("current_ko should stay empty for clean retranslate: %+v", items[0])
	}
}

func TestLoadDoneItemsFiltered_ByIDs(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ckpt.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`create table items (id text, status text, ko_json text, pack_json text)`); err != nil {
		t.Fatal(err)
	}
	ko, _ := json.Marshal(map[string]any{"Text": "A"})
	pack, _ := json.Marshal(map[string]any{"en": "A", "fresh_ko": "에이"})
	if _, err := db.Exec(`insert into items(id,status,ko_json,pack_json) values(?,?,?,?), (?,?,?,?)`,
		"line-1", "done", string(ko), string(pack),
		"line-2", "done", string(ko), string(pack),
	); err != nil {
		t.Fatal(err)
	}

	items, err := loadDoneItemsFiltered(Config{CheckpointDB: dbPath}, []string{"line-2"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "line-2" {
		t.Fatalf("unexpected items: %#v", items)
	}
}

func TestLoadDoneItems_DerivesNeighborContextFromChunkPackage(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ckpt.db")
	chunkPath := filepath.Join(t.TempDir(), "chunks.json")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`create table items (id text, status text, ko_json text, pack_json text)`); err != nil {
		t.Fatal(err)
	}
	makeRow := func(id, ko, en string) {
		koJSON, _ := json.Marshal(map[string]any{"Text": ko})
		packJSON, _ := json.Marshal(map[string]any{"en": en, "source_raw": en, "fresh_ko": ko})
		if _, err := db.Exec(`insert into items(id,status,ko_json,pack_json) values(?,?,?,?)`, id, "done", string(koJSON), string(packJSON)); err != nil {
			t.Fatal(err)
		}
	}
	makeRow("line-a", "이전 줄", "Prev line.")
	makeRow("line-b", "현재 줄", "Current line.")
	makeRow("line-c", "다음 줄", "Next line.")

	raw := `{
	  "format":"esoteric-ebb-translator-package-chunked.v1",
	  "instructions":{"translate_unit":"chunk","return_unit":"line"},
	  "chunks":[
	    {"chunk_id":"chunk-1","parent_segment_id":"seg-1","chunk_pos":1,"chunk_count":1,"source_file":"x","scene_hint":"x","block_kind":"script_block","choice_block_id":null,"source_text":"Prev line.\nCurrent line.\nNext line.","lines":[
	      {"line_id":"line-a","segment_pos":0,"source_text":"Prev line.","text_role":"dialogue","speaker_hint":"Snell","prev_line_id":null,"next_line_id":"line-b","line_is_short_context_dependent":false,"line_has_emphasis":false,"line_is_imperative":false,"tags":[]},
	      {"line_id":"line-b","segment_pos":1,"source_text":"Current line.","text_role":"dialogue","speaker_hint":"Snell","prev_line_id":"line-a","next_line_id":"line-c","line_is_short_context_dependent":true,"line_has_emphasis":false,"line_is_imperative":false,"tags":[]},
	      {"line_id":"line-c","segment_pos":2,"source_text":"Next line.","text_role":"dialogue","speaker_hint":"Snell","prev_line_id":"line-b","next_line_id":null,"line_is_short_context_dependent":false,"line_has_emphasis":false,"line_is_imperative":false,"tags":[]}
	    ]}
	  ]
	}`
	if err := os.WriteFile(chunkPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	items, err := loadDoneItemsFiltered(Config{
		CheckpointDB:            dbPath,
		TranslatorPackageChunks: chunkPath,
	}, []string{"line-b"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("len=%d", len(items))
	}
	got := items[0]
	if got.PrevKO != "이전 줄" || got.NextKO != "다음 줄" || got.PrevEN != "Prev line." || got.NextEN != "Next line." {
		t.Fatalf("unexpected derived neighbors: %+v", got)
	}
	if got.ContextEN == "" || got.TextRole != "dialogue" || got.SpeakerHint != "Snell" {
		t.Fatalf("missing derived context fields: %+v", got)
	}
}

func TestLoadDoneItems_LeavesNeighborKOEmptyWhenNeighborNotDone(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ckpt.db")
	chunkPath := filepath.Join(t.TempDir(), "chunks.json")
	currentPath := filepath.Join(t.TempDir(), "current.json")
	sourcePath := filepath.Join(t.TempDir(), "source.json")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`create table items (id text, status text, ko_json text, pack_json text)`); err != nil {
		t.Fatal(err)
	}
	makeRow := func(id, status, ko, en string) {
		koJSON, _ := json.Marshal(map[string]any{"Text": ko})
		packJSON, _ := json.Marshal(map[string]any{"en": en, "source_raw": en, "fresh_ko": ko})
		if _, err := db.Exec(`insert into items(id,status,ko_json,pack_json) values(?,?,?,?)`, id, status, string(koJSON), string(packJSON)); err != nil {
			t.Fatal(err)
		}
	}
	makeRow("line-a", "pending_score", "이전 줄", "Prev line.")
	makeRow("line-b", "done", "현재 줄", "Current line.")
	makeRow("line-c", "pending_score", "다음 줄", "Next line.")

	raw := `{
	  "format":"esoteric-ebb-translator-package-chunked.v1",
	  "instructions":{"translate_unit":"chunk","return_unit":"line"},
	  "chunks":[
	    {"chunk_id":"chunk-1","parent_segment_id":"seg-1","chunk_pos":1,"chunk_count":1,"source_file":"x","scene_hint":"x","block_kind":"script_block","choice_block_id":null,"source_text":"Prev line.\nCurrent line.\nNext line.","lines":[
	      {"line_id":"line-a","segment_pos":0,"source_text":"Prev line.","text_role":"dialogue","speaker_hint":"Snell","prev_line_id":null,"next_line_id":"line-b","line_is_short_context_dependent":false,"line_has_emphasis":false,"line_is_imperative":false,"tags":[]},
	      {"line_id":"line-b","segment_pos":1,"source_text":"Current line.","text_role":"dialogue","speaker_hint":"Snell","prev_line_id":"line-a","next_line_id":"line-c","line_is_short_context_dependent":true,"line_has_emphasis":false,"line_is_imperative":false,"tags":[]},
	      {"line_id":"line-c","segment_pos":2,"source_text":"Next line.","text_role":"dialogue","speaker_hint":"Snell","prev_line_id":"line-b","next_line_id":null,"line_is_short_context_dependent":false,"line_has_emphasis":false,"line_is_imperative":false,"tags":[]}
	    ]}
	  ]
	}`
	if err := os.WriteFile(chunkPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	currentJSON := `{"strings":{"line-a":{"Text":"current prev"},"line-c":{"Text":"current next"}}}`
	if err := os.WriteFile(currentPath, []byte(currentJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceJSON := `{"strings":{"line-a":{"Text":"Prev line."},"line-c":{"Text":"Next line."}}}`
	if err := os.WriteFile(sourcePath, []byte(sourceJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	items, err := loadDoneItemsFiltered(Config{
		CheckpointDB:            dbPath,
		TranslatorPackageChunks: chunkPath,
		SourcePath:              sourcePath,
		CurrentPath:             currentPath,
	}, []string{"line-b"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("len=%d", len(items))
	}
	if items[0].PrevKO != "" || items[0].NextKO != "" {
		t.Fatalf("neighbor ko should stay empty for non-done neighbors: %+v", items[0])
	}
	if items[0].PrevEN != "Prev line." || items[0].NextEN != "Next line." {
		t.Fatalf("neighbor en should still hydrate: %+v", items[0])
	}
}
