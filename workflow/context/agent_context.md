# Agent Context

This file is shared agent guidance for the current workspace.

## Scope

- Identify the active project under `projects/`.
- Use shared workflow docs from `workflow/context/`.
- Use project-specific runtime context from `projects/<project>/context/`.

## Shared Translation Rules

1. Preserve JSON structure exactly.
2. Preserve placeholders and markup exactly: `$...`, `{...}`, `<...>`, `\n`.
3. Translate only player-facing text.
4. Do not translate technical identifiers, file paths, GUID-like fragments, asset IDs, or control strings.
5. Keep proper nouns and recurring terms consistent across the project.
6. Prefer natural Korean over literal English word order when meaning is clear.
7. If context is weak, choose the safest wording and avoid speculative lore additions.

## Context Loading

- Shared workflow guidance: `workflow/context/`
- Project runtime context: `projects/<project>/context/*.md`
- Project config: `projects/<project>/project.json`
- Project ops/live stack: `projects/<project>/context/live_stack.md` (if exists)

This file is intentionally generic. Project-specific lore, tone, and schema details belong in the project context files.
