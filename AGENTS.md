# Agent Instructions

## Repository Structure

- `workflow/` — shared engine, CLI commands, pipeline logic, pkg (public API)
- `workflow/internal/` — internal packages (not importable outside workflow/)
- `workflow/pkg/` — public packages (importable from anywhere in the module)
- `projects/<name>/` — project-specific profiles, context, source, output, commands
- `scripts/` — shared management scripts (PostgreSQL, OpenCode, Review)

## Rules

1. Shared logic belongs in `workflow/`. Project-specific code belongs in `projects/<project>/`.
2. Do not put project-specific commands in `workflow/cmd/`. They go in `projects/<project>/cmd/`.
3. `workflow/pkg/` is the public API boundary. Code outside `workflow/` cannot import `workflow/internal/`.
4. Do not treat SQLite checkpoint as operational truth when PostgreSQL is available.
5. Do not modify live pipeline state (pipeline_items, checkpoint items) without explicit confirmation.
6. Preserve JSON structure, placeholders, and markup exactly during translation operations.
7. Do not translate technical identifiers, file paths, GUIDs, asset IDs, or control strings.

## Project Context Loading

- Shared workflow guidance: `workflow/context/`
- Project runtime context: `projects/<project>/context/`
- Project config: `projects/<project>/project.json`

## Current Projects

- `esoteric-ebb` — active, primary project
- `rogue-trader` — secondary project

## Pipeline State Machine

States flow: `pending_translate` → `working_translate` → `pending_score` → `working_score` → `done`

Retry loop: `pending_score` → `pending_retranslate` → `working_retranslate` → `pending_score`

Special lanes: `pending_overlay_translate`, `pending_failed_translate`

Terminal states: `done`, `failed`

## Do Not

- Add exports to `workflow/internal/` that are needed outside `workflow/` — use `workflow/pkg/` instead.
- Create documentation files unless explicitly requested.
- Commit operational secrets (DSN strings, credentials) to tracked files.
- Run destructive DB operations (TRUNCATE, DROP, DELETE without WHERE) without confirmation.
