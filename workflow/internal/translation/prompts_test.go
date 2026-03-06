package translation

import "testing"

func TestExtractObjects_ArrayPayload(t *testing.T) {
	raw := `[{"id":"a","proposed_ko":"alpha","risk":"low","notes":""},{"id":"b","proposed_ko":"beta","risk":"med","notes":"x"}]`
	got := extractObjects(raw)
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("ids=%q,%q", got[0].ID, got[1].ID)
	}
}
