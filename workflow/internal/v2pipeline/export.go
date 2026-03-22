package v2pipeline

import (
	"encoding/json"

	"localize-agent/workflow/internal/contracts"
	"localize-agent/workflow/pkg/shared"
)

// V3Format is the format identifier for esoteric-ebb sidecar v3.
const V3Format = "esoteric-ebb-sidecar.v3"

// V3Sidecar is the top-level structure for translations.json output.
type V3Sidecar struct {
	Format  string    `json:"format"`
	Entries []V3Entry `json:"entries"`
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
// Per D-02: each item gets its own entry (no dedup by source text).
// Per D-03: passthrough items included with source=target.
func BuildV3Sidecar(items []contracts.V2PipelineItem) V3Sidecar {
	sidecar := V3Sidecar{
		Format:  V3Format,
		Entries: make([]V3Entry, 0, len(items)),
	}
	for _, item := range items {
		sidecar.Entries = append(sidecar.Entries, V3Entry{
			ID:          item.ID,
			Source:      item.SourceRaw,
			Target:      item.KOFormatted,
			SourceFile:  item.SourceFile,
			TextRole:    item.ContentType,
			SpeakerHint: item.Speaker,
		})
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
