# Localize Agent

`localize-agent` is a reusable translation workspace for multiple game or app localization projects.

It is not a single-project repository. Shared pipeline code lives once in `workflow/`, and each localization target is isolated under `projects/<project-name>/`.

## Workspace Model

- `workflow/`
  Shared engine, CLI entrypoints, pipeline logic, validators, checkpoint handling, and shared context.
- `projects/<project-name>/`
  Project-local profile, context, source files, output artifacts, and helper commands.

This separation is the main rule of the repository:
- shared logic belongs in `workflow/`
- project-specific data and convenience scripts belong in `projects/<project-name>/`

## Project Layout

Each project directory should contain:

- `project.json`
  Project profile used by commands such as `go-translate --project <name>`
- `context/`
  Project-specific lore, style, schema, and rules
- `source/`
  Original or canonical input files for that project
- `output/`
  Derived files such as adapted source, ids, checkpoints, evaluation DBs, and translated output
- `cmd/`
  Project-local helper scripts for repeatable tasks

## How To Work In This Repo

1. Identify the active project under `projects/`.
2. Use shared workflow commands from `workflow/`.
3. Load shared guidance from `workflow/context/`.
4. Load project-specific context from `projects/<project-name>/context/`.
5. Keep any new project-specific automation inside that project directory instead of the repository root.

## Shared Documents

- Root overview: `context.md`
- Shared agent guidance: `workflow/context/agent_context.md`
- Shared ops guidance: `workflow/context/ops.md`
- Shared project-agnostic code/style references: `workflow/context/*`
- Project layout reference: `projects/README.md`

## Current Projects

- `projects/esoteric-ebb`
- `projects/rogue-trader`

These are examples of projects hosted by the same workspace. Their source files, terminology, output names, and install targets should not be treated as global defaults unless a document explicitly says so.

## Current Active Example

At the moment, the most recently aligned active example is `projects/esoteric-ebb`.

Example source:
- `projects/esoteric-ebb/source/translation_assetripper_textasset_unique.json`

Example batch output directory:
- `projects/esoteric-ebb/output/batches/translation_assetripper_textasset_unique`

This is an example of current usage, not the definition of the repository itself.
