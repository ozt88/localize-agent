# Agent Context

This file is shared agent guidance for the current workspace.

## Scope

- Default active project is `projects/esoteric-ebb`.
- Use shared workflow docs from `workflow/context/`.
- Use project-specific runtime context from `projects/esoteric-ebb/context/`.

## Shared Translation Rules

1. Preserve JSON structure exactly.
2. Preserve placeholders and markup exactly: `$...`, `{...}`, `<...>`, `\n`.
3. Translate only player-facing text.
4. Do not translate technical identifiers, file paths, GUID-like fragments, asset IDs, or control strings.
5. Keep proper nouns and recurring terms consistent across the project.
6. Prefer natural Korean over literal English word order when meaning is clear.
7. If context is weak, choose the safest wording and avoid speculative lore additions.

## Context Loading

- Shared workflow behavior: `workflow/context/ops.md`
- Shared Esoteric Ebb guidance: `workflow/context/esoteric_ebb_context.md`
- Project runtime context: `projects/esoteric-ebb/context/esoteric_ebb_context.md`
- Project rules: `projects/esoteric-ebb/context/esoteric_ebb_rules.md`

## Active File References

- Canonical source: `projects/esoteric-ebb/source/translation_assetripper_textasset_unique.json`
- Active batch directory: `projects/esoteric-ebb/output/batches/translation_assetripper_textasset_unique`

This file is intentionally generic. Project-specific lore, tone, and schema details belong in the project context files.
