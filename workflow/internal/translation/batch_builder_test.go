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
	if item.BodyEN != `"I'm definitely a cleric.` {
		t.Fatalf("body=%q", item.BodyEN)
	}
	meta := batch.metas["id_target"]
	if meta.choicePrefix != `DC13 wis-` {
		t.Fatalf("choicePrefix=%q, want %q", meta.choicePrefix, `DC13 wis-`)
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
	if item.BodyEN != `"You ever hang out in a cozy little hag hut before?` {
		t.Fatalf("body=%q", item.BodyEN)
	}
	meta := batch.metas["id_control_quote"]
	if meta.controlPrefix != `.Lisa's_Place==1-` {
		t.Fatalf("controlPrefix=%q", meta.controlPrefix)
	}
}
