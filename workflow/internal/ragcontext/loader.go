package ragcontext

import (
	"encoding/json"
	"os"
)

// BatchContext holds pre-built RAG hints indexed by batch_id.
type BatchContext struct {
	hints map[string][]RAGHint
}

// LoadBatchContext loads rag_batch_context.json into memory.
// Returns empty BatchContext (safe to use) if path is empty.
func LoadBatchContext(path string) (*BatchContext, error) {
	if path == "" {
		return &BatchContext{hints: map[string][]RAGHint{}}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m map[string][]RAGHint
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return &BatchContext{hints: m}, nil
}

// HintsForBatch returns the pre-matched hints for a batch_id.
// Returns nil if batch_id not found or receiver is nil.
func (bc *BatchContext) HintsForBatch(batchID string) []RAGHint {
	if bc == nil || bc.hints == nil {
		return nil
	}
	return bc.hints[batchID]
}
