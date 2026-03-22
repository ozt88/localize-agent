package v2pipeline

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestReadCSVFile(t *testing.T) {
	// Create a BOM-prefixed CSV file
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	content := append([]byte{0xEF, 0xBB, 0xBF}, []byte("ID,ENGLISH,KOREAN\nFEAT_01,\"Lone Cleric - heal extra\",\nFEAT_02,Simple text,\n")...)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	rows, err := ReadCSVFile(path)
	if err != nil {
		t.Fatalf("ReadCSVFile: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// Header
	if rows[0][0] != "ID" || rows[0][1] != "ENGLISH" || rows[0][2] != "KOREAN" {
		t.Errorf("header mismatch: %v", rows[0])
	}

	// Quoted field
	if rows[1][1] != "Lone Cleric - heal extra" {
		t.Errorf("quoted field mismatch: %q", rows[1][1])
	}

	// Simple field
	if rows[2][1] != "Simple text" {
		t.Errorf("simple field mismatch: %q", rows[2][1])
	}
}

func TestReadCSVFileNoBOM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nobom.txt")

	content := []byte("ID,ENGLISH,KOREAN\nUI_1,New Game,\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	rows, err := ReadCSVFile(path)
	if err != nil {
		t.Fatalf("ReadCSVFile: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[1][0] != "UI_1" {
		t.Errorf("expected UI_1, got %q", rows[1][0])
	}
}

func TestWriteCSVFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	rows := [][]string{
		{"ID", "ENGLISH", "KOREAN"},
		{"UI_1", "New Game", "새 게임"},
	}

	if err := WriteCSVFile(path, rows); err != nil {
		t.Fatalf("WriteCSVFile: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Check BOM prefix
	if !bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		t.Error("output missing BOM prefix")
	}

	// Check content is valid CSV
	stripped := bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if !bytes.Contains(stripped, []byte("UI_1")) {
		t.Error("output missing UI_1")
	}
	if !bytes.Contains(stripped, []byte("새 게임")) {
		t.Error("output missing Korean text")
	}
}

func TestTranslateCSVRows(t *testing.T) {
	rows := [][]string{
		{"ID", "ENGLISH", "KOREAN"},
		{"FEAT_01", "Lone Cleric", ""},
		{"FEAT_02", "Simple text", ""},
		{"FEAT_03", "", ""},  // empty ENGLISH -> skip
	}

	translateFn := func(english string) (string, error) {
		return "번역:" + english, nil
	}

	report, err := TranslateCSVRows(rows, translateFn)
	if err != nil {
		t.Fatalf("TranslateCSVRows: %v", err)
	}

	// Header preserved
	if rows[0][0] != "ID" {
		t.Error("header modified")
	}

	// KOREAN filled
	if rows[1][2] != "번역:Lone Cleric" {
		t.Errorf("row 1 KOREAN: got %q", rows[1][2])
	}
	if rows[2][2] != "번역:Simple text" {
		t.Errorf("row 2 KOREAN: got %q", rows[2][2])
	}

	// Empty ENGLISH skipped
	if rows[3][2] != "" {
		t.Errorf("row 3 KOREAN should be empty, got %q", rows[3][2])
	}

	// Report
	if report.Total != 3 {
		t.Errorf("Total: got %d, want 3", report.Total)
	}
	if report.Translated != 2 {
		t.Errorf("Translated: got %d, want 2", report.Translated)
	}
	if report.Skipped != 1 {
		t.Errorf("Skipped: got %d, want 1", report.Skipped)
	}
}

func TestTranslateCSVRowsOverwrite(t *testing.T) {
	// Per D-11: existing KOREAN values are overwritten
	rows := [][]string{
		{"ID", "ENGLISH", "KOREAN"},
		{"UI_1", "New Game", "기존번역"},
	}

	translateFn := func(english string) (string, error) {
		return "새번역:" + english, nil
	}

	report, err := TranslateCSVRows(rows, translateFn)
	if err != nil {
		t.Fatalf("TranslateCSVRows: %v", err)
	}

	if rows[1][2] != "새번역:New Game" {
		t.Errorf("existing KOREAN not overwritten: got %q", rows[1][2])
	}
	if report.Translated != 1 {
		t.Errorf("Translated: got %d, want 1", report.Translated)
	}
}

func TestTranslateCSVRowsError(t *testing.T) {
	rows := [][]string{
		{"ID", "ENGLISH", "KOREAN"},
		{"FEAT_01", "Text one", ""},
		{"FEAT_02", "Text two", ""},
	}

	callCount := 0
	translateFn := func(english string) (string, error) {
		callCount++
		if callCount == 2 {
			return "", fmt.Errorf("LLM error")
		}
		return "번역:" + english, nil
	}

	report, err := TranslateCSVRows(rows, translateFn)
	if err != nil {
		t.Fatalf("TranslateCSVRows: %v", err)
	}

	if report.Translated != 1 {
		t.Errorf("Translated: got %d, want 1", report.Translated)
	}
	if report.Errors != 1 {
		t.Errorf("Errors: got %d, want 1", report.Errors)
	}

	// First row translated, second errored (KOREAN left empty)
	if rows[1][2] != "번역:Text one" {
		t.Errorf("row 1: got %q", rows[1][2])
	}
	if rows[2][2] != "" {
		t.Errorf("row 2 should be empty on error, got %q", rows[2][2])
	}
}

func TestTranslateCSVRowsShortColumns(t *testing.T) {
	// Rows with fewer than 3 columns should be extended
	rows := [][]string{
		{"ID", "ENGLISH", "KOREAN"},
		{"FEAT_01", "Text"},  // only 2 columns
	}

	translateFn := func(english string) (string, error) {
		return "번역:" + english, nil
	}

	report, err := TranslateCSVRows(rows, translateFn)
	if err != nil {
		t.Fatalf("TranslateCSVRows: %v", err)
	}

	if len(rows[1]) < 3 {
		t.Fatalf("row not extended to 3 columns: %v", rows[1])
	}
	if rows[1][2] != "번역:Text" {
		t.Errorf("row 1 KOREAN: got %q", rows[1][2])
	}
	if report.Translated != 1 {
		t.Errorf("Translated: got %d, want 1", report.Translated)
	}
}
