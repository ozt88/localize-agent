package semanticreview

import (
	"database/sql"
	"encoding/json"
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
		"prev_en":      "Prev.",
		"next_en":      "Next.",
		"text_role":    "dialogue",
		"speaker_hint": "Snell",
	})
	if _, err := db.Exec(`insert into items(id,status,ko_json,pack_json) values(?,?,?,?)`, "line-1", "done", string(ko), string(pack)); err != nil {
		t.Fatal(err)
	}

	items, err := LoadDoneItems(dbPath, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("len=%d", len(items))
	}
	if items[0].ID != "line-1" || items[0].SourceEN != `ROLL9 con-Hello.` || items[0].TranslatedKO != "안녕." {
		t.Fatalf("unexpected item: %+v", items[0])
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
		"en": "Hello.",
	})
	if _, err := db.Exec(`insert into items(id,status,ko_json,pack_json) values(?,?,?,?)`, "line-1", "done", string(ko), string(pack)); err != nil {
		t.Fatal(err)
	}

	items, err := LoadDoneItems(dbPath, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("len=%d", len(items))
	}
	if items[0].SourceEN != "Hello." {
		t.Fatalf("unexpected source en: %+v", items[0])
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
	pack, _ := json.Marshal(map[string]any{"en": "A"})
	if _, err := db.Exec(`insert into items(id,status,ko_json,pack_json) values(?,?,?,?), (?,?,?,?)`,
		"line-1", "done", string(ko), string(pack),
		"line-2", "done", string(ko), string(pack),
	); err != nil {
		t.Fatal(err)
	}

	items, err := loadDoneItemsFiltered(dbPath, []string{"line-2"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "line-2" {
		t.Fatalf("unexpected items: %#v", items)
	}
}
