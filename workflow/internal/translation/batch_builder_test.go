package translation

import "testing"

func TestBuildBatch_FiltersAndCounts(t *testing.T) {
	giveRT := translationRuntime{
		cfg: Config{
			MaxPlainLen: 10,
		},
		doneFromCheckpoint: map[string]bool{
			"id_done": true,
		},
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

	giveIDs := []string{"id_done", "id_ok", "id_long", "id_nocur"}
	batch := buildBatch(giveRT, giveIDs)

	if batch.skippedInvalid != 1 {
		t.Fatalf("skippedInvalid=%d, want 1", batch.skippedInvalid)
	}
	if batch.skippedLong != 1 {
		t.Fatalf("skippedLong=%d, want 1", batch.skippedLong)
	}
	if len(batch.skippedLongIDs) != 1 || batch.skippedLongIDs[0] != "id_long" {
		t.Fatalf("skippedLongIDs=%v, want [id_long]", batch.skippedLongIDs)
	}
	if len(batch.runItems) != 1 {
		t.Fatalf("runItems len = %d, want 1", len(batch.runItems))
	}

	item := batch.runItems[0]
	if item["id"] != "id_ok" {
		t.Fatalf("runItems[0].id = %s, want id_ok", item["id"])
	}
	if item["en"] != "Hi [T0]" {
		t.Fatalf("masked en=%q, want %q", item["en"], "Hi [T0]")
	}
	if item["current_ko"] != "hello [T0]" {
		t.Fatalf("masked current_ko=%q, want %q", item["current_ko"], "hello [T0]")
	}

	meta, ok := batch.metas["id_ok"]
	if !ok {
		t.Fatalf("missing meta for id_ok")
	}
	if len(meta.mapTags) != 1 || meta.mapTags[0].original != "{x}" || meta.mapTags[0].placeholder != "[T0]" {
		t.Fatalf("meta.mapTags=%v, want one mapping {x}<->[T0]", meta.mapTags)
	}
}
