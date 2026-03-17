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

func TestExtractRetryPackageEntries(t *testing.T) {
	raw := []byte(`{
  "format": "esoteric-ebb-retry-package.v1",
  "datasets": {
    "textasset_retry": {
      "count": 1,
      "items": [
        {
          "source_type": "textasset",
          "retry_lane": "textasset-remap-v1",
          "retry_reason": "ambiguous_context",
          "id": "line-a",
          "source_text": "I see.",
          "existing_target": "알겠군.",
          "context_en": "I see.\nYou pause.",
          "speaker_hint": "Snell",
          "text_role": "fragment",
          "top_candidates": [
            {
              "source_file": "AR_Viira",
              "meta_path_label": "AR_Viira/root/x",
              "segment_id": "seg-a",
              "segment_pos": 2
            }
          ]
        }
      ]
    },
    "resource_retry": {
      "count": 1,
      "items": [
        {
          "source_type": "resource",
          "retry_lane": "resource-keyed-v1",
          "retry_reason": "untranslated_resource",
          "id": "line-b",
          "source_text": "New Game",
          "text_role": "ui",
          "source_file": "UI.bytes",
          "resource_key": "UI_1",
          "meta_path_label": "resources/UI.bytes/UI_1",
          "scene_hint": "UI.bytes",
          "segment_id": "seg-b",
          "segment_pos": 0
        }
      ]
    }
  }
}`)

	entries, ok, err := extractRetryPackageEntries(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected retry package to be recognized")
	}
	if len(entries) != 2 {
		t.Fatalf("len=%d, want 2", len(entries))
	}
	if entries[0]["id"] != "line-a" || entries[0]["source"] != "I see." || entries[0]["category"] != "fragment" {
		t.Fatalf("entry0=%v", entries[0])
	}
	if entries[0]["target"] != "알겠군." || entries[0]["status"] != "new" {
		t.Fatalf("entry0 target/status=%v", entries[0])
	}
	if entries[0]["context_en"] != "I see.\nYou pause." || entries[0]["speaker_hint"] != "Snell" || entries[0]["source_file"] != "AR_Viira" {
		t.Fatalf("entry0 metadata=%v", entries[0])
	}
	if entries[1]["id"] != "line-b" || entries[1]["source"] != "New Game" || entries[1]["category"] != "ui" {
		t.Fatalf("entry1=%v", entries[1])
	}
	if entries[1]["resource_key"] != "UI_1" || entries[1]["scene_hint"] != "UI.bytes" || entries[1]["segment_id"] != "seg-b" {
		t.Fatalf("entry1 metadata=%v", entries[1])
	}
}
