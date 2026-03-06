# Esoteric Ebb Shared Context

## Scope

- Canonical source file: `projects/esoteric-ebb/source/translation_assetripper_textasset_unique.json`
- Working batch directory: `projects/esoteric-ebb/output/batches/translation_assetripper_textasset_unique`
- The pipeline preserves the original unified schema and applies translated `target` values back into that schema.

## Hard Field Rules

- Never modify metadata fields such as `id`, `source`, `category`, `source_file`, `source_index`, `source_occurrence`, `context_prev`, `context_next`, `speaker_hint`, or `tags`.
- Only write workflow-approved output fields such as `target` and `status` when the schema allows them.

## Preservation Rules

- Preserve `$NAME`, `$ROGUE`, `{...}`, `<...>`, `\n`, and escaped sequences exactly.
- Do not reorder or normalize placeholders, markup, or parser-sensitive content.
- Leave technical or non-player-facing strings untranslated.

## Tone

- Dark fantasy
- Black comedy
- TRPG-style narration
- Natural Korean dialogue
- Concise and readable UI/system text

## Priorities

1. `quest`
2. sentence-like dialogue, narration, choices
3. meaningful UI/system text
4. low-value repetitive text
5. skip technical noise
