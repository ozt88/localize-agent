# localize-agent

`localize-agent` is a reusable localization workspace for multiple translation projects.

The repository is designed to separate:
- shared translation pipeline code
- project-specific source files, context, outputs, and helper scripts

This lets one workspace support multiple games or apps without hard-coding one title into the whole repository.

## What This Repository Is

This repository contains:
- shared Go-based workflow commands for translation, evaluation, validation, and apply steps
- shared agent and ops context used across projects
- per-project directories under `projects/`

This repository is not:
- a single-game translation dump
- a root-level collection of project-specific release or install scripts
- a place where project-specific terminology should leak into shared defaults

## Repository Layout

```text
workflow/                 shared engine, CLI commands, pipeline logic
workflow/context/         shared agent, ops, and style guidance
projects/<project>/       project-specific profiles, context, source, output, commands
context.md                root workspace index
README.md                 repository overview
```

## Project Model

Each project should live under `projects/<project-name>/` and typically contains:

- `project.json`
  Project profile used by commands such as `go run ./workflow/cmd/go-translate --project <name>`
- `context/`
  Project-specific tone, lore, schema, and rules
- `source/`
  Canonical source files for that project
- `output/`
  Derived artifacts such as adapted inputs, ids, checkpoints, evaluation DBs, and translated output
- `cmd/`
  Project-local helper scripts

Examples currently present in this workspace:
- `projects/esoteric-ebb`
- `projects/rogue-trader`

## Core Workflow

Typical flow for a project:

1. Prepare or adapt project input files if needed.
2. Run translation with `go-translate --project <name>`.
3. Run evaluation with `go-evaluate --project <name>`.
4. Apply translated output back to the project schema.

Example:

```powershell
go run ./workflow/cmd/go-translate --project esoteric-ebb
go run ./workflow/cmd/go-evaluate --project esoteric-ebb
```

Project-local wrapper scripts may exist under `projects/<project>/cmd/` for repeatable tasks.

## Design Rules

- Shared logic belongs in `workflow/`.
- Project-specific files belong in `projects/<project>/`.
- Shared prompts and docs should be project-agnostic unless explicitly marked otherwise.
- Root-level convenience scripts that only fit one project should be avoided or moved into that project directory.

## Documentation

- Root workspace index: `context.md`
- Shared agent guidance: `workflow/context/agent_context.md`
- Shared ops guidance: `workflow/context/ops.md`
- Project layout reference: `projects/README.md`

## Encoding Policy

This workspace is configured to prefer UTF-8 for text files.

Repository-local defaults are defined in:
- `.editorconfig`
- `.gitattributes`
- `.vscode/settings.json`

`.bat` files are the main exception and use CRLF line endings.

## Status

The workspace is being cleaned up from older single-project assumptions into a reusable multi-project localization agent.
