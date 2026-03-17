package translation

import (
	"path/filepath"
	"testing"

	"localize-agent/workflow/pkg/platform"
)

func TestLoadCheckpointPromptMetas_ReadsPackJSONFields(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "checkpoint.db")
	store, err := platform.NewSQLiteCheckpointStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteCheckpointStore error: %v", err)
	}
	defer store.Close()

	if err := store.UpsertItem(
		"id-1",
		"done",
		"h",
		0,
		"",
		0,
		map[string]any{"Text": "새 게임"},
		map[string]any{
			"id":              "id-1",
			"context_en":      "Main Menu",
			"text_role":       "ui",
			"choice_mode":     "stat_check_action",
			"is_stat_check":   true,
			"speaker_hint":    "Narrator",
			"source_type":     "resource",
			"source_file":     "UIElements.bytes",
			"resource_key":    "UI_1",
			"meta_path_label": "Assets/Resources/localization/UIElements.bytes:UI_1",
			"segment_id":      "seg-ui-1",
			"segment_pos":     4,
			"choice_block_id": "choice-ui",
		},
	); err != nil {
		t.Fatalf("UpsertItem error: %v", err)
	}

	metas, err := loadCheckpointPromptMetas("sqlite", dbPath, "", []string{"id-1"})
	if err != nil {
		t.Fatalf("loadCheckpointPromptMetas error: %v", err)
	}
	meta := metas["id-1"]
	if meta.SourceFile != "UIElements.bytes" || meta.ResourceKey != "UI_1" {
		t.Fatalf("meta=%+v", meta)
	}
	if meta.SegmentPos == nil || *meta.SegmentPos != 4 {
		t.Fatalf("segment_pos=%v", meta.SegmentPos)
	}
	if meta.ChoiceMode != "stat_check_action" || !meta.IsStatCheck {
		t.Fatalf("choice/stat meta=%+v", meta)
	}
}
