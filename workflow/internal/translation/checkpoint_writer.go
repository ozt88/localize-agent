package translation

import (
	"sync"
	"time"

	"localize-agent/workflow/internal/contracts"
)

type checkpointBatchWriter struct {
	store         contracts.TranslationCheckpointStore
	batchSize     int
	flushInterval time.Duration

	ch   chan contracts.TranslationCheckpointItem
	done chan struct{}

	mu       sync.Mutex
	firstErr error
}

func newCheckpointBatchWriter(store contracts.TranslationCheckpointStore, batchSize int, flushInterval time.Duration) *checkpointBatchWriter {
	if batchSize <= 0 {
		batchSize = 64
	}
	if flushInterval <= 0 {
		flushInterval = 100 * time.Millisecond
	}
	return &checkpointBatchWriter{
		store:         store,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		ch:            make(chan contracts.TranslationCheckpointItem, batchSize*4),
		done:          make(chan struct{}),
	}
}

func (w *checkpointBatchWriter) Start() {
	go func() {
		defer close(w.done)
		ticker := time.NewTicker(w.flushInterval)
		defer ticker.Stop()

		buf := make([]contracts.TranslationCheckpointItem, 0, w.batchSize)
		flush := func() {
			if len(buf) == 0 {
				return
			}
			if err := w.store.UpsertItems(buf); err != nil {
				w.setErr(err)
			}
			buf = buf[:0]
		}

		for {
			select {
			case item, ok := <-w.ch:
				if !ok {
					flush()
					return
				}
				buf = append(buf, item)
				if len(buf) >= w.batchSize {
					flush()
				}
			case <-ticker.C:
				flush()
			}
		}
	}()
}

func (w *checkpointBatchWriter) Enqueue(item contracts.TranslationCheckpointItem) error {
	if err := w.Err(); err != nil {
		return err
	}
	w.ch <- item
	return w.Err()
}

func (w *checkpointBatchWriter) Close() error {
	close(w.ch)
	<-w.done
	return w.Err()
}

func (w *checkpointBatchWriter) setErr(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.firstErr == nil {
		w.firstErr = err
	}
}

func (w *checkpointBatchWriter) Err() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.firstErr
}
