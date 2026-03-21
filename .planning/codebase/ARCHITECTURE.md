# Architecture

**Analysis Date:** 2026-03-22

## Pattern Overview

**Overall:** Layered CLI application with contract-driven dependency inversion

**Key Characteristics:**
- Strict separation between CLI entry points (`cmd`), domain logic (`internal`), and side-effect implementations (`pkg/platform`)
- All cross-domain dependencies flow through interfaces defined in `workflow/internal/contracts`
- DB is the source of truth for pipeline state; translation checkpoints and pipeline items are persisted before in-memory aggregation
- LLM backends are abstracted behind a single client interface supporting two backends: OpenCode (HTTP API) and Ollama (local model server)
- Per-project configuration via `projects/<name>/project.json` drives all paths, model profiles, and pipeline settings

## Layers

**CLI Entry Points:**
- Purpose: Parse flags, load project config, call domain `Run(Config)`
- Location: `workflow/cmd/go-*/main.go` and `projects/<name>/cmd/go-*/main.go`
- Contains: `flag` parsing, `shared.LoadProjectConfig`, config override merging, `os.Exit` handling
- Depends on: domain packages (`translation`, `evaluation`, `translationpipeline`), `workflow/pkg/shared`
- Used by: Shell scripts and PowerShell wrappers

**Domain Logic:**
- Purpose: Orchestrate business pipelines; no direct I/O or LLM SDK calls
- Location: `workflow/internal/translation/`, `workflow/internal/evaluation/`, `workflow/internal/translationpipeline/`, `workflow/internal/semanticreview/`, `workflow/internal/fragmentcluster/`
- Contains: `Run(Config)` functions, batch builders, proposal collectors, result persisters, worker runners, skill/prompt builders
- Depends on: `workflow/internal/contracts` (interfaces), `workflow/pkg/platform` (implementations at init time only), `workflow/pkg/shared`
- Used by: `cmd` entry points

**Contracts:**
- Purpose: Define shared interfaces and DTOs so domain does not import platform implementations
- Location: `workflow/internal/contracts/`
- Contains: `FileStore`, `TranslationCheckpointStore`, `EvalStore`, `EvalPackItem`, `EvalResult`, `TranslationCheckpointItem`
- Depends on: nothing (pure Go interfaces and structs)
- Used by: `internal/translation`, `internal/evaluation`, `pkg/platform`

**Platform (Side Effects):**
- Purpose: Implement contracts with real I/O: SQLite, PostgreSQL, OS filesystem, HTTP to LLM servers
- Location: `workflow/pkg/platform/`
- Contains: `SessionLLMClient`, `OllamaClient`, `sqliteCheckpointStore`, `postgresCheckpointStore`, `EvalStore`, `OSFileStore`, `JSONLTraceSink`
- Depends on: `modernc.org/sqlite`, `github.com/jackc/pgx/v5`, standard `net/http`
- Used by: domain packages (injected at startup via `Run`)

**Shared Utilities:**
- Purpose: Cross-cutting helpers not tied to any domain
- Location: `workflow/pkg/shared/`
- Contains: `LoadProjectConfig`, `MetricCollector`, `MultiFlag`, atomic write, retry helpers, JSON/IO utilities
- Depends on: standard library only
- Used by: all layers

**Project Configuration:**
- Purpose: Per-game settings: LLM profiles, source paths, DB paths, context files
- Location: `projects/<name>/project.json`
- Contains: `translation`, `evaluation`, `pipeline` sections; three named LLM profiles (`low_llm`, `high_llm`, `score_llm`)
- Depends on: nothing (JSON loaded by `shared.LoadProjectConfig`)
- Used by: CLI entry points and `translationpipeline.Run`

## Data Flow

**Standard Translation Flow:**

1. `cmd/go-translate/main.go` parses flags and merges `project.json` overrides
2. `translation.Run(cfg)` opens `platform.TranslationCheckpointStore` (SQLite or Postgres)
3. Source JSON (`source_*.json`), current JSON (`current_*.json`), and ID list (`ids_*.txt`) are read from disk
4. `translateSkill` builds prompts; `newServerClientWithConfig` selects `SessionLLMClient` or `OllamaClient`
5. `runPipeline` fans out work to concurrent goroutine slots (`cfg.Concurrency`)
6. Per slot: `buildBatch` -> `collectProposals` (LLM call) -> `persistResults` (checkpoint DB upsert)
7. Metrics summary printed; exit code returned to shell

**Pipeline Orchestration Flow (DB state machine):**

1. `cmd/go-translation-pipeline/main.go` invokes `translationpipeline.Run(cfg)`
2. `project.json` is loaded; `pipeline_items` table seeded from IDs file
3. Pipeline states cycle: `pending_translate` -> `working_translate` -> `pending_score` -> `working_score` -> `pending_retranslate` | `done` | `failed`
4. Three role-differentiated worker pools (`low_llm`, `score_llm`, `high_llm`) claim items by lease, process, and commit state transitions
5. Score threshold (`cfg.Threshold`) determines whether an item advances to `done` or `pending_retranslate`
6. Worker heartbeats written to `run_logs/pipeline_heartbeats/*.jsonl`

**Evaluation Flow:**

1. `cmd/go-evaluate/main.go` invokes `evaluation.Run(cfg)`
2. `EvalStore` (SQLite) loaded from `evaluation.db` path
3. `work_builder.go` prepares work queue from DB `pending` items
4. Workers call `item_runner.go` which loops: translate -> evaluate -> persist result
5. `modes.go` handles status/export/reset sub-commands without running the worker pipeline

**Apply Flow:**

1. `cmd/go-apply/main.go` reads `pass`-status items from evaluation DB
2. Loads current localization JSON, applies `final_ko` values
3. Writes output JSON; updates `eval_items.status` to `applied`

**State Management:**
- Translation checkpoint: per-item status persisted to SQLite/PostgreSQL `items` table; resumable via `--resume` flag
- Pipeline state: `pipeline_items` table with lease-based worker claims; stale claims reclaimed via `--cleanup-stale-claims`
- Evaluation state: `eval_items` table with status column (`pending`, `evaluating`, `pass`, `fail`, `applied`)

## Key Abstractions

**TranslationCheckpointStore (`contracts/translation.go`):**
- Purpose: Durable record of per-string translation outcomes (status, source hash, KO JSON)
- Examples: `workflow/pkg/platform/checkpoint_store.go` (SQLite), postgres variant in same file
- Pattern: Interface with two concrete implementations selected by `NormalizeDBBackend`

**EvalStore (`contracts/evaluation.go`):**
- Purpose: Load eval packs, track item state, save scored results
- Examples: `workflow/pkg/platform/eval_store.go`
- Pattern: Interface methods map 1:1 to SQLite operations; `SaveResult` writes `final_ko`

**LLM Client (`workflow/pkg/platform/llm_client.go`, `ollama_client.go`):**
- Purpose: Send prompts to LLM backends; maintain session/context across concurrent slots
- Examples: `SessionLLMClient` (OpenCode HTTP), `OllamaClient` (Ollama HTTP)
- Pattern: Each concurrent worker slot has a named session key; warmup/context injected once per session

**translateSkill (`workflow/internal/translation/skill.go`):**
- Purpose: Construct translation prompts from `translationTask`; parse raw LLM output into `proposal`
- Pattern: Stateless; receives context strings and rules at construction; called once per batch slot

**ProjectConfig (`workflow/pkg/shared/project.go`):**
- Purpose: Typed representation of `project.json`; all paths resolved relative to project directory
- Pattern: Loaded once at startup by `shared.LoadProjectConfig`; passed to domain `Run` for LLM profile extraction

**PipelineItem state machine (`workflow/internal/translationpipeline/types.go`):**
- Purpose: Named constants for all pipeline states (`pending_translate`, `working_translate`, `pending_score`, etc.)
- Pattern: State is stored in DB; workers claim by updating `state` + `claimed_by` + `lease_until` atomically

## Entry Points

**go-translate:**
- Location: `workflow/cmd/go-translate/main.go`
- Triggers: `go run ./workflow/cmd/go-translate` or compiled binary
- Responsibilities: Flag parsing, project config merge, invoke `translation.Run`

**go-evaluate:**
- Location: `workflow/cmd/go-evaluate/main.go`
- Triggers: `go run ./workflow/cmd/go-evaluate`
- Responsibilities: Flag parsing, project config merge, invoke `evaluation.Run`

**go-validate:**
- Location: `workflow/cmd/go-validate/main.go`
- Triggers: `go run ./workflow/cmd/go-validate`
- Responsibilities: Validate checkpoint DB entries for structural correctness

**go-apply:**
- Location: `workflow/cmd/go-apply/main.go`
- Triggers: `go run ./workflow/cmd/go-apply`
- Responsibilities: Apply evaluated translations from eval DB back to localization JSON

**go-translation-pipeline:**
- Location: `workflow/cmd/go-translation-pipeline/main.go`
- Triggers: `go run ./workflow/cmd/go-translation-pipeline`
- Responsibilities: Full DB-driven pipeline orchestration with three-role worker pools

**go-semantic-review:**
- Location: `workflow/cmd/go-semantic-review/main.go`
- Triggers: `go run ./workflow/cmd/go-semantic-review`
- Responsibilities: Score translated strings for semantic oddness; output ranked report

**go-fragment-cluster-batch-runner:**
- Location: `workflow/cmd/go-fragment-cluster-batch-runner/main.go`
- Triggers: `go run ./workflow/cmd/go-fragment-cluster-batch-runner`
- Responsibilities: Run translation for grouped fragment clusters

**Project-specific entry points (esoteric-ebb):**
- `projects/esoteric-ebb/cmd/go-esoteric-adapt-in/` - ingest game source into pipeline format
- `projects/esoteric-ebb/cmd/go-esoteric-apply-out/` - apply translations back to game format
- `projects/esoteric-ebb/cmd/go-esoteric-build-translator-chunks/` - build chunk packages

## Error Handling

**Strategy:** Fail-fast at startup for configuration errors; skip-and-count for per-item LLM errors during pipeline runs

**Patterns:**
- Configuration/IO errors: `fmt.Fprintf(os.Stderr, ...) return 1/2` pattern in all `Run` functions
- Per-item LLM failures: recorded to checkpoint DB as `status=failed` or `status=translator_error`; skipped with `skippedTranslatorErr` counter
- Timeout: items skipped with `skippedTimeout` counter when `cfg.SkipTimeout=true`
- Invalid output: post-processing validation rejects malformed LLM responses; recovery attempted up to `cfg.PlaceholderRecoveryAttempts`
- Pipeline lease expiry: stale `working_*` claims reclaimed by `--cleanup-stale-claims` flag on next run

## Cross-Cutting Concerns

**Logging:** `fmt.Printf` to stdout for progress; `fmt.Fprintf(os.Stderr)` for errors; structured JSONL trace via `platform.JSONLTraceSink` when `--trace-out` is set

**Validation:** Post-processing layer in `workflow/internal/translation/postprocess_validation.go` and `proposal_validation.go` validates tag preservation, length constraints, and structural correctness before checkpoint write

**Authentication:** No application-level auth; LLM backends accessed via HTTP to localhost (`127.0.0.1`) with server managed externally (OpenCode or Ollama processes)

---

*Architecture analysis: 2026-03-22*
