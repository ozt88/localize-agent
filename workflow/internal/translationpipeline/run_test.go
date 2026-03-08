package translationpipeline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSourceTextMap_UnwrapsStringsWrapper(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.json")
	data := `{"strings":{"line-1":{"Text":"Hello"},"line-2":{"Text":"World"}}}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readSourceTextMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if got["line-1"] != "Hello" || got["line-2"] != "World" {
		t.Fatalf("unexpected source map: %#v", got)
	}
}

func TestReadSourceTextMap_ReadsFlatMap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.json")
	data := `{"line-1":{"Text":"Hello"}}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readSourceTextMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if got["line-1"] != "Hello" {
		t.Fatalf("unexpected source map: %#v", got)
	}
}
