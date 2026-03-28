package v2pipeline

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"localize-agent/workflow/internal/contracts"
	"localize-agent/workflow/pkg/shared"
)

// abilityPrefixRe matches ability score prefixes that LLM may have included
// in translation output (e.g., "con: ", "str: "). The game engine reads
// speaker from ink # tags, not from text content. Character names (Braxo, etc.)
// are not affected — LLM already omits those correctly.
var abilityPrefixRe = regexp.MustCompile(`^(?:wis|str|int|con|dex|cha):\s*`)

// dcfcPrefixRe matches DC/FC stat-check prefixes in choice text.
// Example: "DC12 str-" or "FC8 int-" — game system markers, not display text.
var dcfcPrefixRe = regexp.MustCompile(`^[A-Z]{2}\d+\s+\w+-`)

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
		// Fallback: if ko_formatted is empty but ko_raw exists (tag-free items
		// that skipped the formatter stage), use ko_raw directly.
		if target == "" && item.KORaw != "" {
			target = item.KORaw
		}
		target = CleanTarget(target)

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

	// Explode multi-line block entries into per-line entries so that
	// TranslationMap can match individual lines the game renders.
	// Only when source and target line counts match (89%+ of blocks).
	var exploded []V3Entry
	for _, entry := range sidecar.Entries {
		if entry.Target == "" || !strings.Contains(entry.Source, "\n") {
			continue
		}
		srcLines := splitKeepNonEmpty(entry.Source)
		tgtLines := splitKeepNonEmpty(entry.Target)
		if len(srcLines) != len(tgtLines) || len(srcLines) < 2 {
			continue
		}
		for i, src := range srcLines {
			if seen[src] {
				continue
			}
			seen[src] = true
			exploded = append(exploded, V3Entry{
				ID:          fmt.Sprintf("%s/ln-%d", entry.ID, i),
				Source:      src,
				Target:      tgtLines[i],
				SourceFile:  entry.SourceFile,
				TextRole:    entry.TextRole,
				SpeakerHint: entry.SpeakerHint,
			})
		}
	}
	sidecar.Entries = append(sidecar.Entries, exploded...)

	// Add DC/FC-stripped body entries so TranslationMap matches
	// when Plugin.cs strips the prefix before lookup.
	var dcfcBodies []V3Entry
	for _, entry := range sidecar.Entries {
		if entry.Target == "" {
			continue
		}
		bodySource := dcfcPrefixRe.ReplaceAllString(entry.Source, "")
		if bodySource == entry.Source || bodySource == "" {
			continue // no prefix found or empty body
		}
		if seen[bodySource] {
			continue
		}
		seen[bodySource] = true
		dcfcBodies = append(dcfcBodies, V3Entry{
			ID:          entry.ID + "/body",
			Source:      bodySource,
			Target:      entry.Target,
			SourceFile:  entry.SourceFile,
			TextRole:    entry.TextRole,
			SpeakerHint: entry.SpeakerHint,
		})
	}
	sidecar.Entries = append(sidecar.Entries, dcfcBodies...)

	return sidecar
}

// CleanTarget applies all target text cleanup: LLM escape normalization
// followed by ability prefix stripping. Use this for any path that consumes
// KOFormatted from the DB (translations.json AND TextAsset injection).
func CleanTarget(s string) string {
	s = NormalizeLLMEscapes(s)
	s = abilityPrefixRe.ReplaceAllString(s, "")
	// Strip DC/FC stat-check prefixes from translated choice text.
	// The game parses these from the SOURCE; translation should only contain the body.
	s = dcfcPrefixRe.ReplaceAllString(s, "")
	return s
}

// normalizeLLMEscapes fixes common LLM output artifacts where the model
// produces literal JSON escape sequences instead of actual characters.
// Detected in ~3,360 of 35,030 translations (v2 pipeline, 2026-03).
func NormalizeLLMEscapes(s string) string {
	// Strip wrapping double-quotes: LLM sometimes wraps entire output in "..."
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	// Unescape literal \n → actual newline (0x0A)
	s = strings.ReplaceAll(s, `\n`, "\n")
	// Unescape literal \" → actual double-quote
	s = strings.ReplaceAll(s, `\"`, `"`)
	return s
}

// splitKeepNonEmpty splits s by newline and returns only non-empty trimmed lines.
func splitKeepNonEmpty(s string) []string {
	parts := strings.Split(s, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
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
