package translation

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"localize-agent/workflow/internal/contracts"
	"localize-agent/workflow/pkg/platform"
)

func TestCheckpointBatchWriter_ConcurrentEnqueue(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "checkpoint.db")
	store, err := platform.NewSQLiteCheckpointStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteCheckpointStore error: %v", err)
	}
	defer store.Close()

	writer := newCheckpointBatchWriter(store, 64, 10*time.Millisecond)
	writer.Start()

	const workers = 32
	const perWorker = 100
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				id := fmt.Sprintf("id-%03d-%03d", worker, i)
				item := contracts.TranslationCheckpointItem{
					EntryID:    id,
					Status:     "done",
					SourceHash: "h",
					KOObj:      map[string]any{"Text": "ko"},
					PackObj:    map[string]any{"id": id, "proposed_ko_restored": "ko"},
				}
				if err := writer.Enqueue(item); err != nil {
					t.Errorf("enqueue error: %v", err)
					return
				}
			}
		}(w)
	}
	wg.Wait()
	if err := writer.Close(); err != nil {
		t.Fatalf("writer close error: %v", err)
	}

	done, err := store.LoadDoneIDs("")
	if err != nil {
		t.Fatalf("LoadDoneIDs error: %v", err)
	}
	want := workers * perWorker
	if len(done) != want {
		t.Fatalf("done rows=%d, want %d", len(done), want)
	}
}
