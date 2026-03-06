package translation

import (
	"reflect"
	"testing"
)

func TestBuildJobBatches_FixedBatchSize(t *testing.T) {
	rt := translationRuntime{
		cfg: Config{BatchSize: 3, MaxBatchChars: 0},
		ids: []string{"a", "b", "c", "d", "e"},
	}
	got := buildJobBatches(rt)
	want := [][]string{
		{"a", "b", "c"},
		{"d", "e"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("batches=%v, want=%v", got, want)
	}
}

func TestBuildJobBatches_MaxBatchChars(t *testing.T) {
	rt := translationRuntime{
		cfg: Config{BatchSize: 8, MaxBatchChars: 10},
		ids: []string{"a", "b", "c", "d"},
		sourceStrings: map[string]map[string]any{
			"a": {"Text": "123456"},    // 6
			"b": {"Text": "123456"},    // 6
			"c": {"Text": "12"},        // 2
			"d": {"Text": "123456789"}, // 9
		},
	}
	got := buildJobBatches(rt)
	want := [][]string{
		{"a"},
		{"b", "c"},
		{"d"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("batches=%v, want=%v", got, want)
	}
}

func TestBuildJobBatches_RespectsBothConstraints(t *testing.T) {
	rt := translationRuntime{
		cfg: Config{BatchSize: 2, MaxBatchChars: 100},
		ids: []string{"a", "b", "c", "d", "e"},
		sourceStrings: map[string]map[string]any{
			"a": {"Text": "x"},
			"b": {"Text": "x"},
			"c": {"Text": "x"},
			"d": {"Text": "x"},
			"e": {"Text": "x"},
		},
	}
	got := buildJobBatches(rt)
	want := [][]string{
		{"a", "b"},
		{"c", "d"},
		{"e"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("batches=%v, want=%v", got, want)
	}
}
