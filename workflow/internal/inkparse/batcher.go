package inkparse

import "fmt"

// Batch format constants.
const (
	FormatScript     = "script"     // dialogue: numbered scene lines
	FormatCard       = "card"       // spell/item: structured cards
	FormatDictionary = "dictionary" // UI: key-value pairs
	FormatDocument   = "document"   // system: full document sections
)

// Batch size limits per content type.
const (
	dialogueMinBatch = 10
	dialogueMaxBatch = 30
	cardMinBatch     = 5
	cardMaxBatch     = 10
	dictMinBatch     = 50
	dictMaxBatch     = 100
)

// Batch represents a group of dialogue blocks ready for translation.
type Batch struct {
	ID          string          `json:"id"`           // e.g., "SourceFile/knot/g-0"
	ContentType string          `json:"content_type"` // dialogue, spell, ui, item, system
	Format      string          `json:"format"`       // script, card, dictionary, document
	Blocks      []DialogueBlock `json:"blocks"`
	SourceFile  string          `json:"source_file"`
	Knot        string          `json:"knot"`
	Gate        string          `json:"gate"` // gate = cluster boundary per D-22
}

// BuildBatches groups dialogue blocks into translation batches by content type
// and gate boundary. Passthrough blocks are excluded. Gate boundaries define
// cluster edges for dialogue batches (per D-22).
func BuildBatches(results []ParseResult) []Batch {
	// First classify all blocks and collect non-passthrough ones grouped by content type
	var dialogueBlocks []DialogueBlock
	var spellBlocks []DialogueBlock
	var uiBlocks []DialogueBlock
	var itemBlocks []DialogueBlock
	var systemBlocks []DialogueBlock

	for _, result := range results {
		for i := range result.Blocks {
			block := &result.Blocks[i]

			// Classify if not already classified
			if block.ContentType == "" {
				block.ContentType = Classify(block)
			}
			// Check passthrough if not already set
			if !block.IsPassthrough {
				block.IsPassthrough = IsPassthrough(block.Text)
			}

			// Skip passthrough blocks
			if block.IsPassthrough {
				continue
			}

			switch block.ContentType {
			case ContentDialogue:
				dialogueBlocks = append(dialogueBlocks, *block)
			case ContentSpell:
				spellBlocks = append(spellBlocks, *block)
			case ContentUI:
				uiBlocks = append(uiBlocks, *block)
			case ContentItem:
				itemBlocks = append(itemBlocks, *block)
			case ContentSystem:
				systemBlocks = append(systemBlocks, *block)
			default:
				dialogueBlocks = append(dialogueBlocks, *block)
			}
		}
	}

	var batches []Batch

	// Dialogue: group by gate boundary (D-22)
	batches = append(batches, buildDialogueBatches(dialogueBlocks)...)

	// Spell: card format, 5-10 items per batch
	batches = append(batches, buildFixedSizeBatches(spellBlocks, ContentSpell, FormatCard, cardMaxBatch)...)

	// UI: dictionary format, 50-100 items per batch
	batches = append(batches, buildFixedSizeBatches(uiBlocks, ContentUI, FormatDictionary, dictMaxBatch)...)

	// Item: card format, 5-10 items per batch
	batches = append(batches, buildFixedSizeBatches(itemBlocks, ContentItem, FormatCard, cardMaxBatch)...)

	// System: document format, entire section as one batch
	batches = append(batches, buildSystemBatches(systemBlocks)...)

	return batches
}

// gateKey creates a grouping key for dialogue blocks based on source file, knot, and gate.
type gateKey struct {
	sourceFile string
	knot       string
	gate       string
}

// buildDialogueBatches groups dialogue blocks by gate boundary.
// Each gate = one batch. If gate has >30 blocks, split at 30.
// If gate has <10 blocks, try to merge with adjacent gate in same knot.
func buildDialogueBatches(blocks []DialogueBlock) []Batch {
	if len(blocks) == 0 {
		return nil
	}

	// Group by source file + knot + gate (preserving order)
	type gateGroup struct {
		key    gateKey
		blocks []DialogueBlock
	}

	var groups []gateGroup
	groupIndex := make(map[gateKey]int)

	for _, block := range blocks {
		key := gateKey{
			sourceFile: block.SourceFile,
			knot:       block.Knot,
			gate:       block.Gate,
		}
		if idx, ok := groupIndex[key]; ok {
			groups[idx].blocks = append(groups[idx].blocks, block)
		} else {
			groupIndex[key] = len(groups)
			groups = append(groups, gateGroup{key: key, blocks: []DialogueBlock{block}})
		}
	}

	// Merge small adjacent gates in same knot, then split large gates
	var batches []Batch
	var pending []DialogueBlock
	var pendingKey gateKey

	flushPending := func() {
		if len(pending) == 0 {
			return
		}
		for _, chunk := range splitAtMax(pending, dialogueMaxBatch) {
			batches = append(batches, Batch{
				ID:          batchID(pendingKey.sourceFile, pendingKey.knot, pendingKey.gate, len(batches)),
				ContentType: ContentDialogue,
				Format:      FormatScript,
				Blocks:      chunk,
				SourceFile:  pendingKey.sourceFile,
				Knot:        pendingKey.knot,
				Gate:        pendingKey.gate,
			})
		}
		pending = nil
	}

	for _, group := range groups {
		// Different knot or source file -> flush
		if len(pending) > 0 && (group.key.knot != pendingKey.knot || group.key.sourceFile != pendingKey.sourceFile) {
			flushPending()
		}

		if len(pending) == 0 {
			pending = group.blocks
			pendingKey = group.key
			continue
		}

		// Same knot: try to merge small gates
		combined := len(pending) + len(group.blocks)
		if combined <= dialogueMaxBatch {
			pending = append(pending, group.blocks...)
			// Update gate to reflect merged range
			pendingKey.gate = pendingKey.gate + "+" + group.key.gate
		} else {
			flushPending()
			pending = group.blocks
			pendingKey = group.key
		}
	}
	flushPending()

	return batches
}

// buildFixedSizeBatches creates batches of a fixed maximum size.
func buildFixedSizeBatches(blocks []DialogueBlock, contentType, format string, maxSize int) []Batch {
	if len(blocks) == 0 {
		return nil
	}

	var batches []Batch
	for _, chunk := range splitAtMax(blocks, maxSize) {
		var sourceFile, knot string
		if len(chunk) > 0 {
			sourceFile = chunk[0].SourceFile
			knot = chunk[0].Knot
		}
		batches = append(batches, Batch{
			ID:          fmt.Sprintf("%s/%s/batch-%d", contentType, sourceFile, len(batches)),
			ContentType: contentType,
			Format:      format,
			Blocks:      chunk,
			SourceFile:  sourceFile,
			Knot:        knot,
		})
	}
	return batches
}

// buildSystemBatches groups system blocks by source file as one batch each.
func buildSystemBatches(blocks []DialogueBlock) []Batch {
	if len(blocks) == 0 {
		return nil
	}

	// Group by source file
	groups := make(map[string][]DialogueBlock)
	var order []string
	for _, block := range blocks {
		if _, ok := groups[block.SourceFile]; !ok {
			order = append(order, block.SourceFile)
		}
		groups[block.SourceFile] = append(groups[block.SourceFile], block)
	}

	var batches []Batch
	for _, sf := range order {
		group := groups[sf]
		batches = append(batches, Batch{
			ID:          fmt.Sprintf("system/%s/batch-0", sf),
			ContentType: ContentSystem,
			Format:      FormatDocument,
			Blocks:      group,
			SourceFile:  sf,
		})
	}
	return batches
}

// splitAtMax splits a slice into chunks of at most maxSize.
func splitAtMax(blocks []DialogueBlock, maxSize int) [][]DialogueBlock {
	if len(blocks) == 0 {
		return nil
	}
	var chunks [][]DialogueBlock
	for i := 0; i < len(blocks); i += maxSize {
		end := i + maxSize
		if end > len(blocks) {
			end = len(blocks)
		}
		chunks = append(chunks, blocks[i:end])
	}
	return chunks
}

// batchID creates a batch identifier.
func batchID(sourceFile, knot, gate string, seq int) string {
	if gate != "" {
		return fmt.Sprintf("%s/%s/%s/batch-%d", sourceFile, knot, gate, seq)
	}
	return fmt.Sprintf("%s/%s/batch-%d", sourceFile, knot, seq)
}
