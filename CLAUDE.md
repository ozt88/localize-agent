<!-- GSD:project-start source:PROJECT.md -->
## Project

**Esoteric Ebb 한국어 번역 파이프라인 v2**

Esoteric Ebb(내러티브 cRPG, Ink 스크립트 기반)의 한국어 번역 파이프라인 v2. v1에서 발견된 근본 문제 — 소스 생성 단위와 게임 렌더링 단위의 불일치 — 를 해결하기 위해 ink JSON 트리를 대사 블록 단위로 파싱하고, 씬 단위 클러스터 번역 + 포맷터 LLM으로 태그를 복원하는 2단계 아키텍처. 40,067건 전량 재번역.

**Core Value:** 게임이 실제로 렌더링하는 대사 블록 단위로 소스를 생성하여, 태그 깨짐 없이 한국어 패치가 동작해야 한다.

### Constraints

- **LLM 백엔드**: OpenCode 서버 (gpt-5.4 번역, codex-mini 포맷팅) — 로컬 인프라, 외부 API 없음
- **게임 버전**: 1.1.3 고정, ink JSON 구조 변경 없음
- **기존 코드**: v1 파이프라인 프레임워크(contracts, platform, shared) 위에 구축
- **DB 규칙**: source_raw 기준 중복 체크 필수, 맹목적 INSERT 금지
- **패치 포맷**: BepInEx TranslationLoader 호환 (translations.json sidecar 방식)
<!-- GSD:project-end -->

<!-- GSD:stack-start source:codebase/STACK.md -->
## Technology Stack

## Languages
- Go 1.24.0 - Core workflow engine, CLI tools, pipeline orchestration
- Python 3.x - Data extraction, ingestion tooling, RAG scripts, semantic embeddings
- PowerShell - Server management scripts (Ollama, OpenCode, PostgreSQL), project command runners
## Runtime
- Go: native compiled binaries, run via `go run` or pre-built `.exe` in `workflow/.bin/`
- Python: standard CPython interpreter (Windows, UTF-8 mode explicitly set in scripts)
- Go: Go modules (`go.mod` / `go.sum`) - lockfile present
- Python: no `requirements.txt` or `pyproject.toml` detected; dependencies installed ad-hoc
- Node/Bun: `.opencode/bun.lock` + `.opencode/package.json` for OpenCode plugin only
## Frameworks
- No web framework - all Go code is CLI tooling with `flag` standard library
- `database/sql` standard library with custom adapter layer for SQLite and PostgreSQL
- `sentence-transformers` Python library - multilingual embeddings via `paraphrase-multilingual-MiniLM-L12-v2`
- `scikit-learn` - cosine similarity computation in `workflow/internal/semanticreview/scripts/embed_compare.py`
- Go standard `testing` package - no external test framework detected
- PowerShell scripts for server lifecycle management in `scripts/`
- Project-specific PowerShell in `projects/<name>/cmd/`
## Key Dependencies
- `github.com/jackc/pgx/v5 v5.7.6` - PostgreSQL driver (via `pgx/v5/stdlib` for `database/sql` compatibility)
- `modernc.org/sqlite v1.38.2` - Pure-Go SQLite implementation (no CGo, Windows-compatible)
- `github.com/google/uuid v1.6.0` - UUID generation
- `github.com/dustin/go-humanize v1.0.1` - Human-readable numbers in output
- `golang.org/x/sync v0.18.0` - Concurrency primitives
- `golang.org/x/crypto v0.44.0` - Cryptography (pgx dependency)
- `golang.org/x/text v0.31.0` - Unicode/text utilities
- `@opencode-ai/plugin 1.2.26` - OpenCode AI server plugin (`.opencode/package.json`)
- `sentence-transformers` - semantic embedding model
- `scikit-learn` - ML utilities (cosine similarity)
- `psycopg2` or `psql` CLI - PostgreSQL access in pipeline ingest (uses `psql.exe` subprocess)
## Configuration
- No `.env` file; configuration loaded from `project.json` files under `projects/<name>/`
- `ProjectConfig` struct defined in `workflow/pkg/shared/project.go`
- Key config fields: `llm_backend` (`opencode` or `ollama`), `server_url`, `model`, `checkpoint_backend` (`sqlite` or `postgres`), `checkpoint_dsn`
- `.editorconfig` enforces UTF-8, LF line endings for `.md`, `.txt`, `.json`, `.go`, `.ps1`
- `.gitattributes` present (likely LF normalization)
- No `Makefile` or `justfile` detected; builds invoked via `go build`/`go run` directly
## Platform Requirements
- Windows 11 (primary dev machine, paths and scripts are Windows-specific)
- PostgreSQL 17 installed at `C:\Program Files\PostgreSQL\17\` (hardcoded in `pipeline_ingest.py`)
- OpenCode server at `C:\Users\DELL\scoop\apps\opencode\current\opencode.exe` (Scoop-managed)
- Ollama server instances on ports 11434, 11435, 11437, 11438 (multiple simultaneous instances)
- Local only - no cloud deployment detected; all services run on local machine
- PostgreSQL data dir: `workflow/output/postgres17_data/` (local to repo)
- OpenCode state dir: `workflow/output/opencode/`
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## Conventions

## Languages and Scope
- **Go** — all pipeline/workflow logic under `workflow/` and `projects/*/cmd/`
- **Python** — project-level tooling scripts under `projects/*/tools/` and `scripts/`
## Go Conventions
### Naming Patterns
- Flat lowercase: `translation`, `evaluation`, `semanticreview`, `fragmentcluster`, `platform`, `shared`
- No underscores, no multi-word compound names split by anything other than directory hierarchy
- `snake_case.go` — e.g. `batch_builder.go`, `checkpoint_writer.go`, `llm_client.go`
- Test files: `<source_file>_test.go` co-located in the same package directory — e.g. `batch_builder_test.go` alongside `batch_builder.go`
- Exported: `PascalCase` — e.g. `NewSessionLLMClient`, `ParseModel`, `BuildPrompt`, `NormalizeOutputLines`
- Unexported: `camelCase` — e.g. `buildBatch`, `collectProposals`, `persistResults`, `maskTags`, `restoreTags`
- Exported: `PascalCase` — e.g. `Config`, `LLMProfile`, `SessionLLMClient`, `TranslationCheckpointItem`
- Unexported: `camelCase` — e.g. `translationRuntime`, `itemMeta`, `textProfile`, `fakeCheckpointStore`
- Local: `camelCase` — e.g. `pendingIDs`, `sourceStrings`, `runItems`
- Package-level: unexported `camelCase`, exported `PascalCase`
- Grouped with `const ( ... )` blocks using `camelCase` for unexported, `PascalCase` for exported
- Examples: `statusPending`, `statusPass`, `kindTrans`, `kindEval` (in `workflow/internal/evaluation/types.go`)
- Defined in `workflow/internal/contracts/` and `workflow/pkg/platform/`
- Named for what they do: `TranslationCheckpointStore`, `EvalStore`, `FileStore`, `LLMTraceSink`
- Interface compliance verified with compile-time assertions: `var _ contracts.TranslationCheckpointStore = (*fakeCheckpointStore)(nil)`
### Import Organization
### Error Handling
### Logging
### Configuration Pattern
- `Config` structs live in `types.go` within each package
- `DefaultConfig()` returns fully populated defaults
- CLI flags in `cmd/go-*/main.go` start from `DefaultConfig()` and override with parsed flags
- Explicit flag tracking: `fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })` prevents project config from overriding explicitly provided CLI args
### Struct Initialization
### Concurrency
## Python Conventions
### File Organization
- Module-level docstring describing purpose, responsibilities, and usage (as both library and CLI)
- Constants in `UPPER_SNAKE_CASE` at module top
- Classes with `PascalCase`, decorated with `@dataclass` where applicable
- `if __name__ == "__main__":` guard at the bottom
#!/usr/bin/env python3
### Naming Patterns
### Error Handling (Python)
### Comments
## TypeScript Conventions (github-docs subtree only)
- `@typescript-eslint/no-unused-vars: error`
- `prefer-const: error` (with `destructuring: 'all'`)
- `import/no-extraneous-dependencies: error`
- `import/extensions: error` (json always)
- `camelcase: off` (APIs use underscores)
- `no-console: off`
## Module Design (Go)
- `types.go` — Config structs, type aliases, constants
- `<feature>.go` — core logic
- `<feature>_test.go` — tests co-located
- `pipeline_mock_test.go` — fake implementations and test helpers (inside the same package, not exported)
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->
## Architecture

## Pattern Overview
- Strict separation between CLI entry points (`cmd`), domain logic (`internal`), and side-effect implementations (`pkg/platform`)
- All cross-domain dependencies flow through interfaces defined in `workflow/internal/contracts`
- DB is the source of truth for pipeline state; translation checkpoints and pipeline items are persisted before in-memory aggregation
- LLM backends are abstracted behind a single client interface supporting two backends: OpenCode (HTTP API) and Ollama (local model server)
- Per-project configuration via `projects/<name>/project.json` drives all paths, model profiles, and pipeline settings
## Layers
- Purpose: Parse flags, load project config, call domain `Run(Config)`
- Location: `workflow/cmd/go-*/main.go` and `projects/<name>/cmd/go-*/main.go`
- Contains: `flag` parsing, `shared.LoadProjectConfig`, config override merging, `os.Exit` handling
- Depends on: domain packages (`translation`, `evaluation`, `translationpipeline`), `workflow/pkg/shared`
- Used by: Shell scripts and PowerShell wrappers
- Purpose: Orchestrate business pipelines; no direct I/O or LLM SDK calls
- Location: `workflow/internal/translation/`, `workflow/internal/evaluation/`, `workflow/internal/translationpipeline/`, `workflow/internal/semanticreview/`, `workflow/internal/fragmentcluster/`
- Contains: `Run(Config)` functions, batch builders, proposal collectors, result persisters, worker runners, skill/prompt builders
- Depends on: `workflow/internal/contracts` (interfaces), `workflow/pkg/platform` (implementations at init time only), `workflow/pkg/shared`
- Used by: `cmd` entry points
- Purpose: Define shared interfaces and DTOs so domain does not import platform implementations
- Location: `workflow/internal/contracts/`
- Contains: `FileStore`, `TranslationCheckpointStore`, `EvalStore`, `EvalPackItem`, `EvalResult`, `TranslationCheckpointItem`
- Depends on: nothing (pure Go interfaces and structs)
- Used by: `internal/translation`, `internal/evaluation`, `pkg/platform`
- Purpose: Implement contracts with real I/O: SQLite, PostgreSQL, OS filesystem, HTTP to LLM servers
- Location: `workflow/pkg/platform/`
- Contains: `SessionLLMClient`, `OllamaClient`, `sqliteCheckpointStore`, `postgresCheckpointStore`, `EvalStore`, `OSFileStore`, `JSONLTraceSink`
- Depends on: `modernc.org/sqlite`, `github.com/jackc/pgx/v5`, standard `net/http`
- Used by: domain packages (injected at startup via `Run`)
- Purpose: Cross-cutting helpers not tied to any domain
- Location: `workflow/pkg/shared/`
- Contains: `LoadProjectConfig`, `MetricCollector`, `MultiFlag`, atomic write, retry helpers, JSON/IO utilities
- Depends on: standard library only
- Used by: all layers
- Purpose: Per-game settings: LLM profiles, source paths, DB paths, context files
- Location: `projects/<name>/project.json`
- Contains: `translation`, `evaluation`, `pipeline` sections; three named LLM profiles (`low_llm`, `high_llm`, `score_llm`)
- Depends on: nothing (JSON loaded by `shared.LoadProjectConfig`)
- Used by: CLI entry points and `translationpipeline.Run`
## Data Flow
- Translation checkpoint: per-item status persisted to SQLite/PostgreSQL `items` table; resumable via `--resume` flag
- Pipeline state: `pipeline_items` table with lease-based worker claims; stale claims reclaimed via `--cleanup-stale-claims`
- Evaluation state: `eval_items` table with status column (`pending`, `evaluating`, `pass`, `fail`, `applied`)
## Key Abstractions
- Purpose: Durable record of per-string translation outcomes (status, source hash, KO JSON)
- Examples: `workflow/pkg/platform/checkpoint_store.go` (SQLite), postgres variant in same file
- Pattern: Interface with two concrete implementations selected by `NormalizeDBBackend`
- Purpose: Load eval packs, track item state, save scored results
- Examples: `workflow/pkg/platform/eval_store.go`
- Pattern: Interface methods map 1:1 to SQLite operations; `SaveResult` writes `final_ko`
- Purpose: Send prompts to LLM backends; maintain session/context across concurrent slots
- Examples: `SessionLLMClient` (OpenCode HTTP), `OllamaClient` (Ollama HTTP)
- Pattern: Each concurrent worker slot has a named session key; warmup/context injected once per session
- Purpose: Construct translation prompts from `translationTask`; parse raw LLM output into `proposal`
- Pattern: Stateless; receives context strings and rules at construction; called once per batch slot
- Purpose: Typed representation of `project.json`; all paths resolved relative to project directory
- Pattern: Loaded once at startup by `shared.LoadProjectConfig`; passed to domain `Run` for LLM profile extraction
- Purpose: Named constants for all pipeline states (`pending_translate`, `working_translate`, `pending_score`, etc.)
- Pattern: State is stored in DB; workers claim by updating `state` + `claimed_by` + `lease_until` atomically
## Entry Points
- Location: `workflow/cmd/go-translate/main.go`
- Triggers: `go run ./workflow/cmd/go-translate` or compiled binary
- Responsibilities: Flag parsing, project config merge, invoke `translation.Run`
- Location: `workflow/cmd/go-evaluate/main.go`
- Triggers: `go run ./workflow/cmd/go-evaluate`
- Responsibilities: Flag parsing, project config merge, invoke `evaluation.Run`
- Location: `workflow/cmd/go-validate/main.go`
- Triggers: `go run ./workflow/cmd/go-validate`
- Responsibilities: Validate checkpoint DB entries for structural correctness
- Location: `workflow/cmd/go-apply/main.go`
- Triggers: `go run ./workflow/cmd/go-apply`
- Responsibilities: Apply evaluated translations from eval DB back to localization JSON
- Location: `workflow/cmd/go-translation-pipeline/main.go`
- Triggers: `go run ./workflow/cmd/go-translation-pipeline`
- Responsibilities: Full DB-driven pipeline orchestration with three-role worker pools
- Location: `workflow/cmd/go-semantic-review/main.go`
- Triggers: `go run ./workflow/cmd/go-semantic-review`
- Responsibilities: Score translated strings for semantic oddness; output ranked report
- Location: `workflow/cmd/go-fragment-cluster-batch-runner/main.go`
- Triggers: `go run ./workflow/cmd/go-fragment-cluster-batch-runner`
- Responsibilities: Run translation for grouped fragment clusters
- `projects/esoteric-ebb/cmd/go-esoteric-adapt-in/` - ingest game source into pipeline format
- `projects/esoteric-ebb/cmd/go-esoteric-apply-out/` - apply translations back to game format
- `projects/esoteric-ebb/cmd/go-esoteric-build-translator-chunks/` - build chunk packages
## Error Handling
- Configuration/IO errors: `fmt.Fprintf(os.Stderr, ...) return 1/2` pattern in all `Run` functions
- Per-item LLM failures: recorded to checkpoint DB as `status=failed` or `status=translator_error`; skipped with `skippedTranslatorErr` counter
- Timeout: items skipped with `skippedTimeout` counter when `cfg.SkipTimeout=true`
- Invalid output: post-processing validation rejects malformed LLM responses; recovery attempted up to `cfg.PlaceholderRecoveryAttempts`
- Pipeline lease expiry: stale `working_*` claims reclaimed by `--cleanup-stale-claims` flag on next run
## Cross-Cutting Concerns
<!-- GSD:architecture-end -->

<!-- GSD:workflow-start source:GSD defaults -->
## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd:quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd:debug` for investigation and bug fixing
- `/gsd:execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->

## Knowledge Compiler — 턴 수집

매 응답의 마지막 단계로, 이번 턴에서 일어난 일을 `.knowledge/raw/` 에 기록한다.

**규칙:**
1. 응답 완료 직전에 Write 도구로 `.knowledge/raw/{YYYY-MM-DD}.md` 파일에 append (당일 파일이 없으면 생성)
2. 형식: `### {HH:MM} — {한줄 제목}\n{2-3줄 요약}\n` (한국어)
3. 요약에 포함할 것: 무엇을 했는지, 핵심 발견/결정, 변경된 파일
4. 요약에 포함하지 않을 것: 코드 전문, 사용자 개인정보, 도구 호출 세부사항
5. 사용자가 `/collect-off` 또는 "수집 중지"라고 하면 해당 세션 동안 수집 중단
6. 이 지시는 GSD 워크플로 안팎 모두에서 항상 적용

**예시:**
```markdown
### 22:04 — Stop Hook 발동 테스트
- Stop Hook command 타입: 따옴표 이스케이프 수정 후 정상 발동 확인
- Stop Hook prompt 타입: 턴 컨텍스트 접근 가능하나 검증 모드로 동작, 생성 불가
- 결론: prompt 훅으로 자동 요약 수집 불가 → CLAUDE.md 행동 지시 방식으로 전환
```


<!-- GSD:profile-start -->
## Developer Profile

> Profile not yet configured. Run `/gsd:profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
