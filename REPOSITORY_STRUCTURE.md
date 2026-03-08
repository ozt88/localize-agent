# Repository Structure Policy

## Principle
- `workflow/` contains shared engine code, shared tooling, and shared non-project guidance only.
- Project-specific assets must live under `projects/<project>/`.

## Allowed in `workflow/`
- reusable commands under `workflow/cmd/`
- reusable libraries under `workflow/internal/`
- shared guidance under `workflow/context/`
- shared defaults that are not tied to one game/project

## Not allowed in `workflow/`
- project-specific prompts, rules, or context
- project-specific prepared inputs
- project-specific outputs, traces, or checkpoints
- project-specific source adapters or one-off assets unless they are promoted to shared reusable code

## Required project layout
- `projects/<project>/project.json`
- `projects/<project>/context/`
- `projects/<project>/source/`
- `projects/<project>/output/`
- `projects/<project>/cmd/` if the project needs helper scripts

## Operational rule
- If a file mentions a specific game or project by name, it should normally live under `projects/<project>/`.
- If a file is needed by multiple projects, move it to `workflow/` only after removing project-specific assumptions.

## Current cleanup direction
- Shared agent/ops/style guidance stays in `workflow/context/`
- Esoteric Ebb-specific context stays in `projects/esoteric-ebb/context/`
- CLI help and docs should refer to `projects/<name>/`, not `workflow/projects/<name>/`
