# Codebase Structure

**Analysis Date:** 2026-03-22

## Directory Layout

```
localize-agent/
├── workflow/                    # Core Go module (module: localize-agent)
│   ├── cmd/                     # Shared CLI entry points (one dir per binary)
│   │   ├── go-translate/        # Translation runner
│   │   ├── go-evaluate/         # Evaluation runner
│   │   ├── go-validate/         # Checkpoint validation
│   │   ├── go-apply/            # Apply eval results to JSON
│   │   ├── go-translation-pipeline/ # DB state-machine orchestrator
│   │   ├── go-semantic-review/  # Semantic oddness scorer
│   │   ├── go-review/           # Review sub-command
│   │   ├── go-sample-ids/       # ID sampling tool
│   │   ├── go-load-pack/        # Load eval pack into DB
│   │   ├── go-migrate-checkpoint/ # Checkpoint DB migration
│   │   ├── go-replay-opencode-request/ # Replay a traced LLM request
│   │   ├── go-fragment-cluster-batch-runner/ # Fragment cluster translation
│   │   └── go-fragment-cluster-retranslate/  # Fragment cluster retranslation
│   ├── internal/                # Domain packages (unexported)
│   │   ├── contracts/           # Shared interfaces and DTOs
│   │   ├── translation/         # Core translation domain
│   │   ├── evaluation/          # Evaluation domain
│   │   ├── translationpipeline/ # DB-driven pipeline orchestration
│   │   ├── semanticreview/      # Semantic scoring domain
│   │   ├── fragmentcluster/     # Fragment cluster grouping
│   │   └── dbmigration/         # DB schema migration helpers
│   ├── pkg/                     # Reusable packages (side-effect implementations)
│   │   ├── platform/            # DB, LLM client, filesystem, tracing
│   │   ├── shared/              # Project config, metrics, retry, IO utils
│   │   └── segmentchunk/        # Segment chunking logic
│   ├── context/                 # Shared LLM context/agent files
│   ├── bin/                     # Compiled binary output (gitignored)
│   └── output/                  # Run output directory (DBs, batch results)
├── projects/                    # Per-game configuration and tooling
│   ├── esoteric-ebb/            # Esoteric Ebb localization project
│   │   ├── project.json         # Canonical project config
│   │   ├── cmd/                 # Project-specific Go binaries
│   │   │   ├── go-esoteric-adapt-in/
│   │   │   ├── go-esoteric-apply-out/
│   │   │   └── go-esoteric-build-translator-chunks/
│   │   ├── context/             # LLM system prompts and rules for this game
│   │   ├── source/              # Game source data
│   │   │   └── prepared/        # Normalized source_*.json, current_*.json, ids_*.txt
│   │   ├── extract/             # Raw game asset extracts (by version)
│   │   ├── output/              # Translation outputs, evaluation DBs
│   │   ├── patch/               # Patch build artifacts
│   │   ├── tools/               # Python utility scripts
│   │   ├── rag/                 # RAG context data
│   │   ├── ollama/              # Ollama model config (Modelfiles)
│   │   └── release/             # Release build artifacts
│   └── rogue-trader/            # Rogue Trader localization project
│       ├── project.json         # Canonical project config
│       ├── cmd/                 # Project-specific PowerShell build scripts
│       ├── source/              # Game source JSON and derived exports
│       ├── extract/             # Raw asset extracts
│       ├── output/              # Translation outputs
│       └── tools/               # Python extraction/build scripts
├── plans/                       # Implementation plan documents (Markdown)
├── scripts/                     # Operational scripts (PowerShell, Python)
│   ├── *.ps1                    # Pipeline management, Ollama/OpenCode serve wrappers
│   └── *.py                     # Analysis, fixture export, autosuggest tools
├── .planning/                   # GSD planning workspace
│   └── codebase/                # Codebase analysis documents
├── .github/                     # GitHub Actions workflows
├── .opencode/                   # OpenCode agent configuration
├── .sisyphus/                   # Run continuation state
├── go.mod                       # Go module definition (module: localize-agent)
├── go.sum                       # Dependency lockfile
├── ARCHITECTURE.md              # Architecture guide (Korean)
├── ONBOARDING_LLM_PROJECTS.md  # Onboarding guide for new games/backends
├── AGENTS.md                    # Agent invocation documentation
└── README.md                    # Project overview
```

## Directory Purposes

**`workflow/cmd/`:**
- Purpose: One directory per CLI binary; each contains exactly one `main.go`
- Contains: Flag parsing, project config loading, `Run(Config)` invocation
- Key files: `go-translate/main.go`, `go-translation-pipeline/main.go`, `go-evaluate/main.go`, `go-apply/main.go`

**`workflow/internal/contracts/`:**
- Purpose: Shared interfaces preventing domain packages from importing platform implementations
- Contains: `files.go` (FileStore), `translation.go` (TranslationCheckpointStore), `evaluation.go` (EvalStore, EvalPackItem, EvalResult)
- Key files: `workflow/internal/contracts/translation.go`, `workflow/internal/contracts/evaluation.go`

**`workflow/internal/translation/`:**
- Purpose: Core translation business logic: batch building, LLM calls, result persistence, pre/post-processing
- Contains: `run.go`, `pipeline.go`, `batch_builder.go`, `proposal_collector.go`, `result_persister.go`, `skill.go`, `prompts.go`, `tags.go`, `postprocess_validation.go`, `proposal_validation.go`, `structured_text.go`, `text_kind.go`, `glossary.go`, `lore.go`
- Key files: `workflow/internal/translation/run.go`, `workflow/internal/translation/types.go`

**`workflow/internal/evaluation/`:**
- Purpose: Evaluation pipeline: load packs, run translate+evaluate loop, persist scores
- Contains: `run.go`, `modes.go`, `pipeline.go`, `work_builder.go`, `worker_runner.go`, `item_runner.go`, `result_persister.go`, `skill.go`, `prompts.go`, `client.go`, `logic.go`, `file_logic.go`
- Key files: `workflow/internal/evaluation/run.go`, `workflow/internal/evaluation/work_builder.go`

**`workflow/internal/translationpipeline/`:**
- Purpose: DB-state-machine orchestrator managing three worker roles (translate, score, retranslate)
- Contains: `run.go`, `store.go`, `types.go`, `postgres_pipeline_schema.sql`
- Key files: `workflow/internal/translationpipeline/types.go` (state constants + Config + PipelineItem)

**`workflow/internal/semanticreview/`:**
- Purpose: Score translated strings for semantic oddness; produce ranked reports
- Contains: `run.go`, `api.go`, `backtranslate.go`, `direct_score.go`, `embedding.go`, `scoring.go`, `reader.go`, `report.go`, `skill.go`, `client.go`

**`workflow/internal/fragmentcluster/`:**
- Purpose: Group related fragments and translate as a coherent cluster
- Contains: `cluster.go`

**`workflow/pkg/platform/`:**
- Purpose: All side-effect implementations: DB access, LLM HTTP clients, file I/O, tracing
- Contains: `llm_client.go` (OpenCode SessionLLMClient), `ollama_client.go`, `checkpoint_store.go`, `checkpoint_db.go`, `eval_store.go`, `filestore.go`, `postgres.go`, `sqlite.go`, `db_backend.go`, `llm_backend.go`, `http_client.go`, `trace_path.go`, `trace_sink.go`
- Key files: `workflow/pkg/platform/llm_client.go`, `workflow/pkg/platform/checkpoint_store.go`

**`workflow/pkg/shared/`:**
- Purpose: Cross-cutting utilities safe to import from any layer
- Contains: `project.go` (ProjectConfig loader), `metrics.go`, `flags.go` (MultiFlag), `retry.go`, `atomic_write.go`, `io.go`, `json.go`
- Key files: `workflow/pkg/shared/project.go`

**`projects/<name>/`:**
- Purpose: All per-game artifacts: config, source data, context prompts, tooling, outputs
- Contains: `project.json`, `cmd/`, `context/`, `source/prepared/`, `output/`, `tools/`
- Key files: `projects/esoteric-ebb/project.json`, `projects/rogue-trader/project.json`

**`projects/<name>/source/prepared/`:**
- Purpose: Normalized pipeline inputs consumed by `go-translate` and the pipeline
- Contains: `source_*.json` (EN originals), `current_*.json` (current KO state), `ids_*.txt` (ID selection lists)

**`projects/<name>/context/`:**
- Purpose: Game-specific LLM system prompts and rules injected as context files
- Contains: `*_context.md` (translation system prompt), `*_semantic_review_system.md`, `*_rules.md`

**`projects/<name>/tools/`:**
- Purpose: Python scripts for data extraction, ingestion, analysis specific to that game
- Contains: `pipeline_ingest.py`, `diff_version_source.py`, `extract_scene_texts.py`, `inject_scene_items.py` (esoteric-ebb)

**`scripts/`:**
- Purpose: Operational scripts for managing running services and analysis across projects
- Contains: PowerShell scripts for Ollama/OpenCode server management, pipeline monitoring, Python analysis tools

**`plans/`:**
- Purpose: Implementation plan markdown documents for features and migrations

## Key File Locations

**Entry Points:**
- `workflow/cmd/go-translate/main.go`: Translation run entry
- `workflow/cmd/go-translation-pipeline/main.go`: Pipeline orchestration entry
- `workflow/cmd/go-evaluate/main.go`: Evaluation run entry
- `workflow/cmd/go-apply/main.go`: Apply results entry
- `workflow/cmd/go-semantic-review/main.go`: Semantic review entry

**Configuration:**
- `go.mod`: Module definition (`localize-agent`), Go 1.24.0, dependencies
- `projects/esoteric-ebb/project.json`: Esoteric Ebb game config (LLM profiles, paths)
- `projects/rogue-trader/project.json`: Rogue Trader game config
- `workflow/pkg/shared/project.go`: `ProjectConfig` struct and `LoadProjectConfig` loader

**Core Domain Logic:**
- `workflow/internal/translation/run.go`: Translation orchestration
- `workflow/internal/translation/pipeline.go`: Concurrent worker pipeline
- `workflow/internal/translation/batch_builder.go`: Batch input construction
- `workflow/internal/translation/proposal_collector.go`: LLM proposal collection
- `workflow/internal/translation/result_persister.go`: Checkpoint write logic
- `workflow/internal/translationpipeline/types.go`: Pipeline state constants and Config
- `workflow/internal/translationpipeline/store.go`: `pipeline_items` DB store

**Contracts/Interfaces:**
- `workflow/internal/contracts/files.go`: FileStore interface
- `workflow/internal/contracts/translation.go`: TranslationCheckpointStore interface
- `workflow/internal/contracts/evaluation.go`: EvalStore interface and DTOs

**Platform Implementations:**
- `workflow/pkg/platform/llm_client.go`: OpenCode SessionLLMClient
- `workflow/pkg/platform/ollama_client.go`: Ollama HTTP client
- `workflow/pkg/platform/checkpoint_store.go`: SQLite/Postgres checkpoint store
- `workflow/pkg/platform/eval_store.go`: SQLite evaluation store
- `workflow/pkg/platform/db_backend.go`: DB backend selector (`sqlite` | `postgres`)
- `workflow/pkg/platform/llm_backend.go`: LLM backend selector (`opencode` | `ollama`)

**Testing:**
- Co-located with source: `workflow/internal/translation/*_test.go`
- `workflow/internal/translation/testdata/` - test fixtures

## Naming Conventions

**Files:**
- Go source: `snake_case.go` (e.g., `batch_builder.go`, `result_persister.go`)
- Test files: `<filename>_test.go` co-located with source
- SQL schemas: `<purpose>_schema.sql` embedded via `//go:embed`
- Config: `project.json` (fixed name, one per project directory)
- Context/system prompts: `<game>_context.md`, `<game>_rules.md`, `<game>_semantic_review_system.md`

**Directories:**
- CLI binaries: `go-<verb>` or `go-<game>-<verb>` (e.g., `go-translate`, `go-esoteric-adapt-in`)
- Internal packages: descriptive noun (`translation`, `evaluation`, `translationpipeline`)
- Projects: kebab-case game name (`esoteric-ebb`, `rogue-trader`)
- Source data sets: `ids_<label>.txt`, `source_<label>.json`, `current_<label>.json`

**Go packages:**
- Package name matches directory name (e.g., `package translation`, `package platform`, `package shared`)
- Module path prefix: `localize-agent/workflow/...`

## Where to Add New Code

**New CLI command (shared):**
- Create `workflow/cmd/go-<verb>/main.go`
- Import the relevant domain package; call `domain.Run(cfg)`
- Follow the flag-parse then `shared.LoadProjectConfig` then override pattern from `go-translate/main.go`

**New CLI command (project-specific):**
- Create `projects/<name>/cmd/go-<game>-<verb>/main.go`
- Same pattern as shared commands

**New domain feature:**
- Business logic goes in `workflow/internal/<domain>/` as a new `.go` file
- If it needs a new contract, add the interface to `workflow/internal/contracts/`
- If it needs a new platform implementation, add to `workflow/pkg/platform/`

**New platform implementation:**
- Add `workflow/pkg/platform/<name>.go`
- Implement the relevant `contracts` interface
- Wire up in the factory function pattern used by `NewTranslationCheckpointStore`

**New utility:**
- Shared across domains: `workflow/pkg/shared/<name>.go`
- Domain-specific: keep in the domain package

**New project:**
- Create `projects/<game>/project.json` with `translation`, `evaluation`, `pipeline` sections
- Create `projects/<game>/context/<game>_context.md` and `<game>_semantic_review_system.md`
- Create `projects/<game>/source/prepared/` with `source_*.json`, `current_*.json`, `ids_*.txt`
- Optionally add project-specific `cmd/` binaries and `tools/` Python scripts

**New test:**
- Place `<file>_test.go` next to the file under test
- Test fixtures in `testdata/` subdirectory within the package

## Special Directories

**`workflow/output/`:**
- Purpose: Runtime outputs (checkpoint DBs, eval DBs, batch result JSON)
- Generated: Yes
- Committed: No (gitignored)

**`workflow/bin/`:**
- Purpose: Compiled binary output from `build_translate_exe.ps1` and similar
- Generated: Yes
- Committed: No

**`projects/esoteric-ebb/extract/`:**
- Purpose: Raw extracted game assets organized by version (`1.1.1/`, etc.)
- Generated: Yes (from game extraction tools)
- Committed: Partially (structure only; large binary assets gitignored)

**`projects/esoteric-ebb/patch/`:**
- Purpose: Patch build outputs; restored BepInEx/doorstop/font files go here
- Generated: Yes
- Committed: Untracked (listed in git status)

**`.planning/codebase/`:**
- Purpose: GSD codebase analysis documents
- Generated: Yes (by GSD map-codebase)
- Committed: Yes

**`.sisyphus/run-continuation/`:**
- Purpose: Run continuation state for orchestrated pipeline sessions
- Generated: Yes
- Committed: No

---

*Structure analysis: 2026-03-22*
