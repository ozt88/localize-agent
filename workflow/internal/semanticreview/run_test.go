package semanticreview

import "testing"

func TestSplitReviewBatches(t *testing.T) {
	items := []ReviewItem{
		{ID: "a"},
		{ID: "b"},
		{ID: "c"},
		{ID: "d"},
		{ID: "e"},
	}
	batches := splitReviewBatches(items, 2)
	if len(batches) != 3 {
		t.Fatalf("len(batches)=%d", len(batches))
	}
	if len(batches[0]) != 2 || len(batches[1]) != 2 || len(batches[2]) != 1 {
		t.Fatalf("unexpected batch sizes: %d, %d, %d", len(batches[0]), len(batches[1]), len(batches[2]))
	}
	if batches[2][0].ID != "e" {
		t.Fatalf("unexpected tail item: %+v", batches[2][0])
	}
}
