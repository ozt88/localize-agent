package translation

import (
	"encoding/json"
	"os"

	"localize-agent/workflow/pkg/segmentchunk"
)

func loadChunkContexts(path string, requestedIDs []string) (map[string]lineContext, [][]string, error) {
	if path == "" {
		return nil, nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var pkg segmentchunk.ChunkedTranslatorPackage
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return nil, nil, err
	}

	requested := make(map[string]bool, len(requestedIDs))
	for _, id := range requestedIDs {
		requested[id] = true
	}

	lineMap := make(map[string]lineContext, len(requestedIDs))
	batches := make([][]string, 0, len(pkg.Chunks))
	seen := make(map[string]bool, len(requestedIDs))
	for _, chunk := range pkg.Chunks {
		chunkIDs := make([]string, 0, len(chunk.Lines))
		chunkCtx := chunkContext{
			ChunkID:         chunk.ChunkID,
			ParentSegmentID: chunk.ParentSegmentID,
			ChunkPos:        chunk.ChunkPos,
			ChunkCount:      chunk.ChunkCount,
			LineIDs:         make([]string, 0, len(chunk.Lines)),
		}
		for _, line := range chunk.Lines {
			chunkCtx.LineIDs = append(chunkCtx.LineIDs, line.LineID)
		}
		for _, line := range chunk.Lines {
			if !requested[line.LineID] {
				continue
			}
			ctx := lineContext{
				TextRole:                    line.TextRole,
				LineIsShortContextDependent: line.LineIsShortContextDependent,
				LineHasEmphasis:             line.LineHasEmphasis,
				LineIsImperative:            line.LineIsImperative,
				Chunk:                       chunkCtx,
			}
			if line.PrevLineID != nil {
				ctx.PrevLineID = *line.PrevLineID
			}
			if line.NextLineID != nil {
				ctx.NextLineID = *line.NextLineID
			}
			if line.SpeakerHint != nil {
				ctx.SpeakerHint = *line.SpeakerHint
			}
			lineMap[line.LineID] = ctx
			chunkIDs = append(chunkIDs, line.LineID)
			seen[line.LineID] = true
		}
		if len(chunkIDs) > 0 {
			batches = append(batches, chunkIDs)
		}
	}

	leftovers := make([]string, 0)
	for _, id := range requestedIDs {
		if !seen[id] {
			leftovers = append(leftovers, id)
		}
	}
	if len(leftovers) > 0 {
		batches = append(batches, leftovers)
	}
	return lineMap, batches, nil
}
