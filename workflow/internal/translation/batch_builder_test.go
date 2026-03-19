package translation

import "testing"

func TestBuildBatch_FiltersAndCounts(t *testing.T) {
	rt := translationRuntime{
		cfg: Config{MaxPlainLen: 10},
		ids: []string{"id_done", "id_ok", "id_long", "id_nocur"},
		idIndex: map[string]int{
			"id_done":  0,
			"id_ok":    1,
			"id_long":  2,
			"id_nocur": 3,
		},
		doneFromCheckpoint: map[string]bool{"id_done": true},
		sourceStrings: map[string]map[string]any{
			"id_done":  {"Text": "ignored"},
			"id_ok":    {"Text": "Hi {x}"},
			"id_long":  {"Text": "this is definitely longer than ten"},
			"id_nocur": {"Text": "source only"},
		},
		currentStrings: map[string]map[string]any{
			"id_ok":   {"Text": "hello {x}"},
			"id_long": {"Text": "very long text"},
		},
	}

	batch := buildBatch(rt, []string{"id_done", "id_ok", "id_long", "id_nocur"})
	if batch.skippedInvalid != 1 {
		t.Fatalf("skippedInvalid=%d, want 1", batch.skippedInvalid)
	}
	if batch.skippedLong != 1 {
		t.Fatalf("skippedLong=%d, want 1", batch.skippedLong)
	}
	if len(batch.runItems) != 1 {
		t.Fatalf("runItems len=%d, want 1", len(batch.runItems))
	}
	item := batch.runItems[0]
	if item.ID != "id_ok" {
		t.Fatalf("id=%q", item.ID)
	}
	if item.BodyEN != "Hi [T0]" {
		t.Fatalf("body=%q", item.BodyEN)
	}
	if item.ContextEN != "" {
		t.Fatalf("context=%q", item.ContextEN)
	}
}

func TestBuildBatch_UsesPackageRoleMetadata(t *testing.T) {
	rt := translationRuntime{
		ids: []string{"id_target"},
		idIndex: map[string]int{
			"id_target": 0,
		},
		sourceStrings: map[string]map[string]any{
			"id_target": {"Text": "Stand up."},
		},
		currentStrings: map[string]map[string]any{
			"id_target": {"Text": ""},
		},
		lineContexts: map[string]lineContext{
			"id_target": {
				TextRole:         "choice",
				LineIsImperative: true,
				Chunk: chunkContext{
					ChunkID:         "chunk-1",
					ParentSegmentID: "seg-1",
					LineIDs:         []string{"id_target"},
				},
			},
		},
	}

	batch := buildBatch(rt, []string{"id_target"})
	item := batch.runItems[0]
	if item.Profile.Kind != textKindChoice {
		t.Fatalf("kind=%q, want choice", item.Profile.Kind)
	}
	if item.Lane != laneHigh {
		t.Fatalf("translation_lane=%q, want high", item.Lane)
	}
}

func TestBuildBatch_GameplayPrefixChoiceIsNotOverriddenByFragmentRole(t *testing.T) {
	rt := translationRuntime{
		ids: []string{"id_target"},
		idIndex: map[string]int{
			"id_target": 0,
		},
		sourceStrings: map[string]map[string]any{
			"id_target": {"Text": `DC13 wis-"I'm definitely a cleric.`},
		},
		currentStrings: map[string]map[string]any{
			"id_target": {"Text": ""},
		},
		lineContexts: map[string]lineContext{
			"id_target": {
				TextRole: "fragment",
				Chunk: chunkContext{
					ChunkID:         "chunk-1",
					ParentSegmentID: "seg-1",
					LineIDs:         []string{"id_target"},
				},
			},
		},
	}

	batch := buildBatch(rt, []string{"id_target"})
	if len(batch.runItems) != 1 {
		t.Fatalf("runItems len=%d, want 1", len(batch.runItems))
	}
	item := batch.runItems[0]
	if item.Profile.Kind != textKindChoice {
		t.Fatalf("kind=%q, want choice", item.Profile.Kind)
	}
	if item.BodyEN != `I'm definitely a cleric.` {
		t.Fatalf("body=%q", item.BodyEN)
	}
	meta := batch.metas["id_target"]
	if meta.choicePrefix != `DC13 wis-` {
		t.Fatalf("choicePrefix=%q, want %q", meta.choicePrefix, `DC13 wis-`)
	}
	if item.StatCheck != "WIS 13" {
		t.Fatalf("statCheck=%q, want WIS 13", item.StatCheck)
	}
	if item.ChoiceMode != "stat_check_action" {
		t.Fatalf("choiceMode=%q, want stat_check_action", item.ChoiceMode)
	}
	if !item.IsStatCheck {
		t.Fatalf("isStatCheck=%v, want true", item.IsStatCheck)
	}
}

func TestBuildBatch_FCGameplayPrefixChoiceIsNotOverriddenByFragmentRole(t *testing.T) {
	rt := translationRuntime{
		ids: []string{"id_target"},
		idIndex: map[string]int{
			"id_target": 0,
		},
		sourceStrings: map[string]map[string]any{
			"id_target": {"Text": `FC8 int-<i>Ragn?</i>`},
		},
		currentStrings: map[string]map[string]any{
			"id_target": {"Text": ""},
		},
		lineContexts: map[string]lineContext{
			"id_target": {
				TextRole: "fragment",
				Chunk: chunkContext{
					ChunkID:         "chunk-1",
					ParentSegmentID: "seg-1",
					LineIDs:         []string{"id_target"},
				},
			},
		},
	}

	batch := buildBatch(rt, []string{"id_target"})
	if len(batch.runItems) != 1 {
		t.Fatalf("runItems len=%d, want 1", len(batch.runItems))
	}
	item := batch.runItems[0]
	if item.Profile.Kind != textKindChoice {
		t.Fatalf("kind=%q, want choice", item.Profile.Kind)
	}
	if item.BodyEN != `[[E0]]Ragn?[[/E0]]` {
		t.Fatalf("body=%q", item.BodyEN)
	}
	meta := batch.metas["id_target"]
	if meta.choicePrefix != `FC8 int-` {
		t.Fatalf("choicePrefix=%q, want %q", meta.choicePrefix, `FC8 int-`)
	}
	if item.StatCheck != "INT 8" {
		t.Fatalf("statCheck=%q, want INT 8", item.StatCheck)
	}
	if item.ChoiceMode != "stat_check_action" {
		t.Fatalf("choiceMode=%q, want stat_check_action", item.ChoiceMode)
	}
	if !item.IsStatCheck {
		t.Fatalf("isStatCheck=%v, want true", item.IsStatCheck)
	}
}

func TestBuildBatch_ChoiceRoleWithStatCheckMarksChoiceStatCheck(t *testing.T) {
	rt := translationRuntime{
		ids:     []string{"id_target"},
		idIndex: map[string]int{"id_target": 0},
		sourceStrings: map[string]map[string]any{
			"id_target": {"Text": "ROLL14 str-(Force him to return the papers.)"},
		},
		currentStrings: map[string]map[string]any{
			"id_target": {"Text": ""},
		},
		lineContexts: map[string]lineContext{
			"id_target": {
				TextRole: "choice",
				Chunk:    chunkContext{LineIDs: []string{"id_target"}},
			},
		},
	}

	batch := buildBatch(rt, []string{"id_target"})
	item := batch.runItems[0]
	if item.ChoiceMode != "choice_stat_check" {
		t.Fatalf("choiceMode=%q, want choice_stat_check", item.ChoiceMode)
	}
	if !item.IsStatCheck {
		t.Fatalf("isStatCheck=%v, want true", item.IsStatCheck)
	}
}

func TestBuildBatch_UsesChunkContext(t *testing.T) {
	rt := translationRuntime{
		ids: []string{"id_a", "id_b", "id_c"},
		idIndex: map[string]int{
			"id_a": 0,
			"id_b": 1,
			"id_c": 2,
		},
		sourceStrings: map[string]map[string]any{
			"id_a": {"Text": "ROLL14 str-Give back the papers."},
			"id_b": {"Text": "Do <i>you</i> like maw pie?"},
			"id_c": {"Text": "His flesh- now turning to <i>fine dust</i>."},
		},
		currentStrings: map[string]map[string]any{
			"id_a": {"Text": "prev"},
			"id_b": {"Text": "cur"},
			"id_c": {"Text": "next"},
		},
		lineContexts: map[string]lineContext{
			"id_b": {
				PrevLineID:                  "id_a",
				NextLineID:                  "id_c",
				TextRole:                    "dialogue",
				LineIsShortContextDependent: true,
				Chunk: chunkContext{
					ChunkID:         "chunk-1",
					ParentSegmentID: "seg-1",
					LineIDs:         []string{"id_a", "id_b", "id_c"},
				},
			},
		},
	}

	batch := buildBatch(rt, []string{"id_b"})
	item := batch.runItems[0]
	wantContext := "Give back the papers.\nDo [[E0]]you[[/E0]] like maw pie?\nHis flesh- now turning to [[E0]]fine dust[[/E0]]."
	if item.ContextEN != wantContext {
		t.Fatalf("context=%q", item.ContextEN)
	}
}

func TestBuildBatch_UIRoleSuppressesCheckpointContext(t *testing.T) {
	rt := translationRuntime{
		ids:     []string{"id_ui"},
		idIndex: map[string]int{"id_ui": 0},
		sourceStrings: map[string]map[string]any{
			"id_ui": {"Text": "Pause/Back"},
		},
		currentStrings: map[string]map[string]any{
			"id_ui": {"Text": ""},
		},
		checkpointMetas: map[string]checkpointPromptMeta{
			"id_ui": {
				TextRole:  "ui_label",
				ContextEN: "Credits\nPause/Back\nMore noisy menu text",
			},
		},
	}

	batch := buildBatch(rt, []string{"id_ui"})
	if len(batch.runItems) != 1 {
		t.Fatalf("runItems len=%d, want 1", len(batch.runItems))
	}
	item := batch.runItems[0]
	if item.TextRole != "ui_label" {
		t.Fatalf("textRole=%q", item.TextRole)
	}
	if item.ContextEN != "" {
		t.Fatalf("context=%q, want empty", item.ContextEN)
	}
	if item.Profile.Kind != textKindDialogue && item.Profile.Kind != textKindNarration {
		t.Fatalf("unexpected kind=%q", item.Profile.Kind)
	}
}

func TestBuildBatch_PrefabInternalUILabelBecomesPassthrough(t *testing.T) {
	rt := translationRuntime{
		ids:     []string{"id_ui"},
		idIndex: map[string]int{"id_ui": 0},
		sourceStrings: map[string]map[string]any{
			"id_ui": {"Text": "ArmRig"},
		},
		currentStrings: map[string]map[string]any{
			"id_ui": {"Text": ""},
		},
		checkpointMetas: map[string]checkpointPromptMeta{
			"id_ui": {
				TextRole:    "ui_label",
				RetryReason: "prefab_static_missing_from_canonical_source",
				SourceFile:  "You.prefab",
			},
		},
	}

	batch := buildBatch(rt, []string{"id_ui"})
	if len(batch.runItems) != 0 {
		t.Fatalf("runItems len=%d, want 0", len(batch.runItems))
	}
	meta := batch.metas["id_ui"]
	if !meta.passthrough {
		t.Fatalf("passthrough=%v, want true", meta.passthrough)
	}
	if meta.sourceRaw != "ArmRig" {
		t.Fatalf("sourceRaw=%q", meta.sourceRaw)
	}
}

func TestBuildBatch_TranslationPolicyPreserveBecomesPassthrough(t *testing.T) {
	rt := translationRuntime{
		ids:     []string{"id_ui"},
		idIndex: map[string]int{"id_ui": 0},
		sourceStrings: map[string]map[string]any{
			"id_ui": {"Text": "Plume of Righteousness"},
		},
		currentStrings: map[string]map[string]any{
			"id_ui": {"Text": ""},
		},
		checkpointMetas: map[string]checkpointPromptMeta{
			"id_ui": {
				TextRole:          "ui_label",
				TranslationPolicy: "preserve",
				RetryReason:       "prefab_static_missing_from_canonical_source",
				SourceFile:        "Some.prefab",
			},
		},
	}

	batch := buildBatch(rt, []string{"id_ui"})
	if len(batch.runItems) != 0 {
		t.Fatalf("runItems len=%d, want 0", len(batch.runItems))
	}
	meta := batch.metas["id_ui"]
	if !meta.passthrough {
		t.Fatalf("passthrough=%v, want true", meta.passthrough)
	}
	if meta.translationPolicy != "preserve" {
		t.Fatalf("translationPolicy=%q", meta.translationPolicy)
	}
}

func TestNormalizeStatCheck(t *testing.T) {
	tests := map[string]string{
		"ROLL14 str-": "STR 14",
		"DC13 wis-":   "WIS 13",
		"FC8 int-":    "INT 8",
		"":            "",
		"BUY10 -":     "",
	}
	for in, want := range tests {
		if got := normalizeStatCheck(in); got != want {
			t.Fatalf("normalizeStatCheck(%q)=%q want %q", in, got, want)
		}
	}
}

func TestBuildBatch_LabelsDialogueSpeakersInsideChunkContext(t *testing.T) {
	rt := translationRuntime{
		ids: []string{"id_a", "id_b", "id_c"},
		idIndex: map[string]int{
			"id_a": 0,
			"id_b": 1,
			"id_c": 2,
		},
		sourceStrings: map[string]map[string]any{
			"id_a": {"Text": "Stop."},
			"id_b": {"Text": "Why?"},
			"id_c": {"Text": "Orders."},
		},
		currentStrings: map[string]map[string]any{
			"id_a": {"Text": ""},
			"id_b": {"Text": ""},
			"id_c": {"Text": ""},
		},
		lineContexts: map[string]lineContext{
			"id_a": {
				TextRole:    "dialogue",
				SpeakerHint: "Tinn",
				Chunk:       chunkContext{LineIDs: []string{"id_a", "id_b", "id_c"}},
			},
			"id_b": {
				TextRole:    "dialogue",
				SpeakerHint: "Player",
				Chunk:       chunkContext{LineIDs: []string{"id_a", "id_b", "id_c"}},
			},
			"id_c": {
				TextRole:    "dialogue",
				SpeakerHint: "Tinn",
				Chunk:       chunkContext{LineIDs: []string{"id_a", "id_b", "id_c"}},
			},
		},
	}

	batch := buildBatch(rt, []string{"id_b"})
	item := batch.runItems[0]
	want := "Tinn: Stop.\nPlayer: Why?\nTinn: Orders."
	if item.ContextEN != want {
		t.Fatalf("context=%q", item.ContextEN)
	}
	if item.ContextLine != 1 {
		t.Fatalf("contextLine=%d", item.ContextLine)
	}
}

func TestBuildBatch_LoreCueUsesHighLane(t *testing.T) {
	rt := translationRuntime{
		ids:     []string{"id_target"},
		idIndex: map[string]int{"id_target": 0},
		sourceStrings: map[string]map[string]any{
			"id_target": {"Text": "But it will be the last Band I tackle. And it might be my doom."},
		},
		currentStrings: map[string]map[string]any{
			"id_target": {"Text": ""},
		},
		lineContexts: map[string]lineContext{
			"id_target": {TextRole: "dialogue"},
		},
	}

	batch := buildBatch(rt, []string{"id_target"})
	if got := batch.runItems[0].Lane; got != laneHigh {
		t.Fatalf("translation_lane=%q, want high", got)
	}
}

func TestBuildBatch_PassthroughControlTokenSkipsRunItem(t *testing.T) {
	rt := translationRuntime{
		ids: []string{"id_control"},
		idIndex: map[string]int{
			"id_control": 0,
		},
		sourceStrings: map[string]map[string]any{
			"id_control": {"Text": ".CB_RuinBOT==1-"},
		},
		currentStrings: map[string]map[string]any{
			"id_control": {"Text": ""},
		},
	}

	batch := buildBatch(rt, []string{"id_control"})
	if len(batch.runItems) != 0 {
		t.Fatalf("runItems len=%d, want 0", len(batch.runItems))
	}
	meta := batch.metas["id_control"]
	if !meta.passthrough {
		t.Fatalf("passthrough=%v, want true", meta.passthrough)
	}
}

func TestBuildBatch_ControlQuotedTailSplitsPrefix(t *testing.T) {
	rt := translationRuntime{
		ids: []string{"id_control_quote"},
		idIndex: map[string]int{
			"id_control_quote": 0,
		},
		sourceStrings: map[string]map[string]any{
			"id_control_quote": {"Text": `.Lisa's_Place==1-"You ever hang out in a cozy little hag hut before?`},
		},
		currentStrings: map[string]map[string]any{
			"id_control_quote": {"Text": ""},
		},
	}

	batch := buildBatch(rt, []string{"id_control_quote"})
	if len(batch.runItems) != 1 {
		t.Fatalf("runItems len=%d, want 1", len(batch.runItems))
	}
	item := batch.runItems[0]
	if item.BodyEN != `You ever hang out in a cozy little hag hut before?` {
		t.Fatalf("body=%q", item.BodyEN)
	}
	meta := batch.metas["id_control_quote"]
	if meta.controlPrefix != `.Lisa's_Place==1-` {
		t.Fatalf("controlPrefix=%q", meta.controlPrefix)
	}
}

func TestBuildBatch_UsesCheckpointPromptMetadata(t *testing.T) {
	segmentPos := 3
	rt := translationRuntime{
		cfg: Config{UseCheckpointCurrent: true},
		ids: []string{"id_target"},
		idIndex: map[string]int{
			"id_target": 0,
		},
		sourceStrings: map[string]map[string]any{
			"id_target": {"Text": "New Game"},
		},
		currentStrings: map[string]map[string]any{
			"id_target": {"Text": "새 게임"},
		},
		checkpointMetas: map[string]checkpointPromptMeta{
			"id_target": {
				ContextEN:     "Main Menu",
				CurrentKO:     "기존 번역",
				TextRole:      "ui",
				SpeakerHint:   "Narrator",
				RetryReason:   "resource_retry",
				SourceType:    "resource",
				SourceFile:    "UIElements.bytes",
				ResourceKey:   "UI_1",
				MetaPathLabel: "Assets/Resources/localization/UIElements.bytes:UI_1",
				SegmentID:     "seg-ui-1",
				SegmentPos:    &segmentPos,
				ChoiceBlockID: "choice-ui",
			},
		},
		lineContexts: map[string]lineContext{
			"id_target": {
				TextRole:    "dialogue",
				SpeakerHint: "Wrong",
			},
		},
	}

	batch := buildBatch(rt, []string{"id_target"})
	if len(batch.runItems) != 1 {
		t.Fatalf("runItems len=%d, want 1", len(batch.runItems))
	}
	item := batch.runItems[0]
	if item.ContextEN != "Main Menu" {
		t.Fatalf("context=%q", item.ContextEN)
	}
	if item.CurrentKO != "기존 번역" {
		t.Fatalf("current_ko=%q", item.CurrentKO)
	}
	if item.TextRole != "ui" {
		t.Fatalf("text_role=%q", item.TextRole)
	}
	if item.SourceFile != "UIElements.bytes" || item.ResourceKey != "UI_1" {
		t.Fatalf("source_file=%q resource_key=%q", item.SourceFile, item.ResourceKey)
	}
	if item.MetaPath != "Assets/Resources/localization/UIElements.bytes:UI_1" {
		t.Fatalf("meta_path=%q", item.MetaPath)
	}
	if item.SegmentPos == nil || *item.SegmentPos != 3 {
		t.Fatalf("segment_pos=%v", item.SegmentPos)
	}
	meta := batch.metas["id_target"]
	if meta.sourceType != "resource" || meta.choiceBlockID != "choice-ui" {
		t.Fatalf("meta=%+v", meta)
	}
}

func TestBuildBatch_DoesNotFallbackNeighborKOFromCurrentStrings(t *testing.T) {
	rt := translationRuntime{
		cfg: Config{UseCheckpointCurrent: true},
		ids: []string{"id_prev", "id_target", "id_next"},
		idIndex: map[string]int{
			"id_prev":   0,
			"id_target": 1,
			"id_next":   2,
		},
		sourceStrings: map[string]map[string]any{
			"id_prev":   {"Text": "Prev line."},
			"id_target": {"Text": "Current line."},
			"id_next":   {"Text": "Next line."},
		},
		currentStrings: map[string]map[string]any{
			"id_prev":   {"Text": "현재 prev"},
			"id_target": {"Text": "현재 target"},
			"id_next":   {"Text": "현재 next"},
		},
		checkpointMetas: map[string]checkpointPromptMeta{
			"id_target": {
				CurrentKO: "기존 번역",
			},
		},
		lineContexts: map[string]lineContext{
			"id_target": {
				PrevLineID: "id_prev",
				NextLineID: "id_next",
			},
		},
	}

	batch := buildBatch(rt, []string{"id_target"})
	if len(batch.runItems) != 1 {
		t.Fatalf("runItems len=%d, want 1", len(batch.runItems))
	}
	item := batch.runItems[0]
	if item.PrevKO != "" || item.NextKO != "" {
		t.Fatalf("neighbor ko should come only from checkpoint meta: %+v", item)
	}
	meta := batch.metas["id_target"]
	if meta.prevKO != "" || meta.nextKO != "" {
		t.Fatalf("meta neighbor ko should stay empty: %+v", meta)
	}
}

func TestBuildBatch_DoesNotInferNeighborENFromIDOrderWithoutLineContext(t *testing.T) {
	rt := translationRuntime{
		ids: []string{"id_prev", "id_target", "id_next"},
		idIndex: map[string]int{
			"id_prev":   0,
			"id_target": 1,
			"id_next":   2,
		},
		sourceStrings: map[string]map[string]any{
			"id_prev":   {"Text": "Prev line."},
			"id_target": {"Text": "Current line."},
			"id_next":   {"Text": "Next line."},
		},
		currentStrings: map[string]map[string]any{
			"id_prev":   {"Text": "prev"},
			"id_target": {"Text": ""},
			"id_next":   {"Text": "next"},
		},
	}

	batch := buildBatch(rt, []string{"id_target"})
	if len(batch.runItems) != 1 {
		t.Fatalf("runItems len=%d, want 1", len(batch.runItems))
	}
	item := batch.runItems[0]
	if item.PrevEN != "" || item.NextEN != "" || item.ContextEN != "" {
		t.Fatalf("neighbor/context should stay empty without real line context: %+v", item)
	}
	meta := batch.metas["id_target"]
	if meta.prevEN != "" || meta.nextEN != "" || meta.contextEN != "" {
		t.Fatalf("meta neighbor/context should stay empty: %+v", meta)
	}
}
