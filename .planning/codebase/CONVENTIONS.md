# Coding Conventions

**Analysis Date:** 2026-03-22

## Languages and Scope

The codebase has two primary languages with distinct conventions:

- **Go** — all pipeline/workflow logic under `workflow/` and `projects/*/cmd/`
- **Python** — project-level tooling scripts under `projects/*/tools/` and `scripts/`

The `github-docs/` subtree is a separate Next.js/TypeScript project with its own config.

---

## Go Conventions

### Naming Patterns

**Packages:**
- Flat lowercase: `translation`, `evaluation`, `semanticreview`, `fragmentcluster`, `platform`, `shared`
- No underscores, no multi-word compound names split by anything other than directory hierarchy

**Files:**
- `snake_case.go` — e.g. `batch_builder.go`, `checkpoint_writer.go`, `llm_client.go`
- Test files: `<source_file>_test.go` co-located in the same package directory — e.g. `batch_builder_test.go` alongside `batch_builder.go`

**Functions and Methods:**
- Exported: `PascalCase` — e.g. `NewSessionLLMClient`, `ParseModel`, `BuildPrompt`, `NormalizeOutputLines`
- Unexported: `camelCase` — e.g. `buildBatch`, `collectProposals`, `persistResults`, `maskTags`, `restoreTags`

**Types and Structs:**
- Exported: `PascalCase` — e.g. `Config`, `LLMProfile`, `SessionLLMClient`, `TranslationCheckpointItem`
- Unexported: `camelCase` — e.g. `translationRuntime`, `itemMeta`, `textProfile`, `fakeCheckpointStore`

**Variables:**
- Local: `camelCase` — e.g. `pendingIDs`, `sourceStrings`, `runItems`
- Package-level: unexported `camelCase`, exported `PascalCase`

**Constants:**
- Grouped with `const ( ... )` blocks using `camelCase` for unexported, `PascalCase` for exported
- Examples: `statusPending`, `statusPass`, `kindTrans`, `kindEval` (in `workflow/internal/evaluation/types.go`)

**Interfaces:**
- Defined in `workflow/internal/contracts/` and `workflow/pkg/platform/`
- Named for what they do: `TranslationCheckpointStore`, `EvalStore`, `FileStore`, `LLMTraceSink`
- Interface compliance verified with compile-time assertions: `var _ contracts.TranslationCheckpointStore = (*fakeCheckpointStore)(nil)`

### Import Organization

Three groups separated by blank lines in this order:
1. Standard library
2. Third-party packages
3. Internal packages using the module path `localize-agent/workflow/...`

Example from `workflow/internal/translation/pipeline_mock_test.go`:
```go
import (
    "encoding/json"
    "fmt"
    "net/http"
    "net/http/httptest"
    "strings"
    "sync"
    "testing"

    "localize-agent/workflow/internal/contracts"
    "localize-agent/workflow/pkg/platform"
    "localize-agent/workflow/pkg/shared"
)
```

Module path: `localize-agent` (defined in `go.mod`).

### Error Handling

**Pattern:** Errors are returned up the call stack; fatal errors in CLI entry points call `os.Exit(1)` or `os.Exit(2)`. Internal functions return `(T, error)` pairs.

**In tests:** `t.Fatalf(...)` is used exclusively — no `t.Errorf` — meaning the first failure immediately stops the test. Error format strings use `=` not `:` for key-value pairs, e.g. `t.Fatalf("skippedInvalid=%d, want 1", ...)`.

**In production code:** `fmt.Fprintf(os.Stderr, "error ...: %v\n", err)` then `os.Exit(1)` or `return 1` from run functions. Functions that can fail return `error`; callers check immediately.

**Wrapped errors:** Use `fmt.Errorf("context: %w", err)` pattern (implied by stdlib usage).

### Logging

**Go:** `fmt.Printf` and `fmt.Println` for progress output to stdout. `fmt.Fprintf(os.Stderr, ...)` for errors. No logging framework is used.

**Progress pattern** (from `workflow/internal/evaluation/pipeline.go`):
```go
fmt.Printf("Pending: %d items\n", len(pendingIDs))
fmt.Printf("  pass=%-5d revise=%-5d reject=%-5d pending=%-5d\n", ...)
fmt.Printf("Elapsed: %.2fs (%.2fm)\n", elapsed, elapsed/60)
```

### Configuration Pattern

Each subsystem has a `Config` struct and a `DefaultConfig()` function:
- `Config` structs live in `types.go` within each package
- `DefaultConfig()` returns fully populated defaults
- CLI flags in `cmd/go-*/main.go` start from `DefaultConfig()` and override with parsed flags
- Explicit flag tracking: `fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })` prevents project config from overriding explicitly provided CLI args

Example: `workflow/internal/evaluation/types.go`, `workflow/internal/translation/` (DefaultConfig).

### Struct Initialization

Struct literals are used extensively in tests with named fields. Go struct literal syntax with composite literal initialization is the norm — no builder pattern.

### Concurrency

Mutexes (`sync.Mutex`) used in concurrent code. Pattern: lock immediately before shared state access, defer unlock or unlock explicitly before return. `sync.WaitGroup` for goroutine fans. Thread-safe fakes in tests use `sync.Mutex` to protect captured state.

---

## Python Conventions

### File Organization

Scripts in `projects/*/tools/` and `scripts/` follow a CLI pattern:
- Module-level docstring describing purpose, responsibilities, and usage (as both library and CLI)
- Constants in `UPPER_SNAKE_CASE` at module top
- Classes with `PascalCase`, decorated with `@dataclass` where applicable
- `if __name__ == "__main__":` guard at the bottom

Example from `projects/esoteric-ebb/tools/pipeline_ingest.py`:
```python
#!/usr/bin/env python3
"""
Module docstring with Responsibilities and Usage sections.
"""

PSQL = "C:/Program Files/PostgreSQL/17/bin/psql.exe"
DEFAULT_BATCH_DIR = PROJECT_DIR / "output" / "batches" / "..."

PACK_SCHEMA = [...]    # inline comments explain each field
SKIP_PATTERNS = [...]
```

### Naming Patterns

**Files:** `snake_case.py` — e.g. `pipeline_ingest.py`, `diff_version_source.py`, `build_context_clusters.py`

**Functions:** `snake_case` — e.g. `add_glossary_terms`, `dry_run`, `apply`

**Classes:** `PascalCase` — e.g. `PipelineIngest`

**Constants:** `UPPER_SNAKE_CASE` — e.g. `PSQL`, `DEFAULT_DSN_ARGS`, `PACK_SCHEMA`, `SKIP_PATTERNS`

**Variables:** `snake_case`

### Error Handling (Python)

Scripts use `sys.exit(1)` on fatal error. Errors are printed to stderr via `print(..., file=sys.stderr)`. No exceptions framework or custom exception classes detected.

### Comments

**Go:** Inline comments above logic blocks; no JSDoc-style doc comments on private functions. Public API functions do not consistently have doc comments — convention is to write self-describing names.

**Python:** Module-level triple-quoted docstrings are mandatory. Inline `#` comments label groups of constants and schema fields.

---

## TypeScript Conventions (github-docs subtree only)

The `github-docs/` directory is a standalone Next.js project, distinct from the main codebase.

**Linting:** ESLint with `@typescript-eslint`, `prettier`, `eslint-plugin-github`, `eslint-plugin-import` (config at `github-docs/eslint.config.ts`)

**Formatting:** Prettier (integrated via `eslint-plugin-prettier`)

**Key rules enforced:**
- `@typescript-eslint/no-unused-vars: error`
- `prefer-const: error` (with `destructuring: 'all'`)
- `import/no-extraneous-dependencies: error`
- `import/extensions: error` (json always)
- `camelcase: off` (APIs use underscores)
- `no-console: off`

**TypeScript config:** `strict: true`, `target: ES2022`, `moduleResolution: Bundler`, path alias `@/` maps to `./src/`

---

## Module Design (Go)

**No barrel files.** Each Go package exports only what is needed. Internal packages under `workflow/internal/` are not importable from outside the module.

**Package layout pattern:**
- `types.go` — Config structs, type aliases, constants
- `<feature>.go` — core logic
- `<feature>_test.go` — tests co-located
- `pipeline_mock_test.go` — fake implementations and test helpers (inside the same package, not exported)

**Contracts package:** `workflow/internal/contracts/` holds shared interface definitions (`TranslationCheckpointStore`, `EvalStore`, `FileStore`) and plain data structs. No logic lives here.

---

*Convention analysis: 2026-03-22*
