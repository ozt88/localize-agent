package translation

import (
	"strings"
	"testing"
)

func TestRestorePreparedText_DeduplicatesExistingStatCheckPrefix(t *testing.T) {
	meta := itemMeta{
		statCheck:   "DEX 25",
		isStatCheck: true,
	}

	got, err := restorePreparedText("[DEX 25] 금고를 딴다", meta)
	if err != nil {
		t.Fatalf("restorePreparedText error=%v", err)
	}
	if strings.Count(got, "25]") != 1 {
		t.Fatalf("got=%q", got)
	}
}
