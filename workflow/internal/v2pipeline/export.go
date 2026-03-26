package v2pipeline

import (
	"encoding/json"
	"regexp"

	"localize-agent/workflow/internal/contracts"
	"localize-agent/workflow/pkg/shared"
)

// abilityPrefixRe matches ability score prefixes that LLM may have included
// in translation output (e.g., "con: ", "str: "). The game engine reads
// speaker from ink # tags, not from text content. Character names (Braxo, etc.)
// are not affected — LLM already omits those correctly.
var abilityPrefixRe = regexp.MustCompile(`^(?:wis|str|int|con|dex|cha):\s*`)

// V3Format is the format identifier for esoteric-ebb sidecar v3.
const V3Format = "esoteric-ebb-sidecar.v3"

// V3Sidecar is the top-level structure for translations.json output.
type V3Sidecar struct {
	Format            string    `json:"format"`
	Entries           []V3Entry `json:"entries"`
	ContextualEntries []V3Entry `json:"contextual_entries"`
}

// V3Entry represents a single translation entry in the sidecar.
type V3Entry struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	Target      string `json:"target"`
	SourceFile  string `json:"source_file"`
	TextRole    string `json:"text_role"`
	SpeakerHint string `json:"speaker_hint"`
}

// BuildV3Sidecar converts pipeline done items to V3Sidecar.
// Per D-01: entries[] is deduped by source text (first-seen-wins) for TranslationMap,
// contextual_entries[] contains ALL items with full metadata for ContextualMap.
// Per D-03: passthrough items included with source=target.
func BuildV3Sidecar(items []contracts.V2PipelineItem) V3Sidecar {
	sidecar := V3Sidecar{
		Format:            V3Format,
		Entries:           make([]V3Entry, 0),
		ContextualEntries: make([]V3Entry, 0, len(items)),
	}
	seen := make(map[string]bool)
	for _, item := range items {
		target := item.KOFormatted
		// Strip ability score prefixes that LLM may have included in output.
		// Game engine reads speaker from ink # tags, not text content.
		target = abilityPrefixRe.ReplaceAllString(target, "")

		entry := V3Entry{
			ID:          item.ID,
			Source:      item.SourceRaw,
			Target:      target,
			SourceFile:  item.SourceFile,
			TextRole:    item.ContentType,
			SpeakerHint: item.Speaker,
		}
		sidecar.ContextualEntries = append(sidecar.ContextualEntries, entry)
		if !seen[item.SourceRaw] {
			seen[item.SourceRaw] = true
			sidecar.Entries = append(sidecar.Entries, entry)
		}
	}
	return sidecar
}

// WriteTranslationsJSON writes V3Sidecar as indented JSON to path.
// Uses shared.AtomicWriteFile for crash-safe writes.
func WriteTranslationsJSON(path string, sidecar V3Sidecar) error {
	data, err := json.MarshalIndent(sidecar, "", "  ")
	if err != nil {
		return err
	}
	return shared.AtomicWriteFile(path, data, 0o644)
}
