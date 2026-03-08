package main

import "testing"

func TestExtractTranslatorPackageEntries(t *testing.T) {
	raw := []byte(`{
  "format": "esoteric-ebb-translator-package.v1",
  "segments": [
    {
      "lines": [
        {"line_id": "line-1", "source_text": "Hello.", "text_role": "dialogue"},
        {"line_id": "line-2", "source_text": "Look.", "text_role": "choice"}
      ]
    }
  ]
}`)

	entries, ok, err := extractTranslatorPackageEntries(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected translator package to be recognized")
	}
	if len(entries) != 2 {
		t.Fatalf("len=%d, want 2", len(entries))
	}
	if entries[0]["id"] != "line-1" || entries[0]["source"] != "Hello." || entries[0]["category"] != "dialogue" {
		t.Fatalf("entry0=%v", entries[0])
	}
	if entries[1]["id"] != "line-2" || entries[1]["source"] != "Look." || entries[1]["category"] != "choice" {
		t.Fatalf("entry1=%v", entries[1])
	}
}
