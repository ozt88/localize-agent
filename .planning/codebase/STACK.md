# Technology Stack

**Analysis Date:** 2026-03-22

## Languages

**Primary:**
- Go 1.24.0 - Core workflow engine, CLI tools, pipeline orchestration
- Python 3.x - Data extraction, ingestion tooling, RAG scripts, semantic embeddings

**Secondary:**
- PowerShell - Server management scripts (Ollama, OpenCode, PostgreSQL), project command runners

## Runtime

**Environment:**
- Go: native compiled binaries, run via `go run` or pre-built `.exe` in `workflow/.bin/`
- Python: standard CPython interpreter (Windows, UTF-8 mode explicitly set in scripts)

**Package Manager:**
- Go: Go modules (`go.mod` / `go.sum`) - lockfile present
- Python: no `requirements.txt` or `pyproject.toml` detected; dependencies installed ad-hoc
- Node/Bun: `.opencode/bun.lock` + `.opencode/package.json` for OpenCode plugin only

## Frameworks

**Core:**
- No web framework - all Go code is CLI tooling with `flag` standard library
- `database/sql` standard library with custom adapter layer for SQLite and PostgreSQL

**Semantic Similarity:**
- `sentence-transformers` Python library - multilingual embeddings via `paraphrase-multilingual-MiniLM-L12-v2`
- `scikit-learn` - cosine similarity computation in `workflow/internal/semanticreview/scripts/embed_compare.py`

**Testing:**
- Go standard `testing` package - no external test framework detected

**Build/Dev:**
- PowerShell scripts for server lifecycle management in `scripts/`
- Project-specific PowerShell in `projects/<name>/cmd/`

## Key Dependencies

**Go - Critical:**
- `github.com/jackc/pgx/v5 v5.7.6` - PostgreSQL driver (via `pgx/v5/stdlib` for `database/sql` compatibility)
- `modernc.org/sqlite v1.38.2` - Pure-Go SQLite implementation (no CGo, Windows-compatible)

**Go - Infrastructure (indirect):**
- `github.com/google/uuid v1.6.0` - UUID generation
- `github.com/dustin/go-humanize v1.0.1` - Human-readable numbers in output
- `golang.org/x/sync v0.18.0` - Concurrency primitives
- `golang.org/x/crypto v0.44.0` - Cryptography (pgx dependency)
- `golang.org/x/text v0.31.0` - Unicode/text utilities

**Node/Bun:**
- `@opencode-ai/plugin 1.2.26` - OpenCode AI server plugin (`.opencode/package.json`)

**Python (inferred from imports):**
- `sentence-transformers` - semantic embedding model
- `scikit-learn` - ML utilities (cosine similarity)
- `psycopg2` or `psql` CLI - PostgreSQL access in pipeline ingest (uses `psql.exe` subprocess)

## Configuration

**Environment:**
- No `.env` file; configuration loaded from `project.json` files under `projects/<name>/`
- `ProjectConfig` struct defined in `workflow/pkg/shared/project.go`
- Key config fields: `llm_backend` (`opencode` or `ollama`), `server_url`, `model`, `checkpoint_backend` (`sqlite` or `postgres`), `checkpoint_dsn`

**Build:**
- `.editorconfig` enforces UTF-8, LF line endings for `.md`, `.txt`, `.json`, `.go`, `.ps1`
- `.gitattributes` present (likely LF normalization)
- No `Makefile` or `justfile` detected; builds invoked via `go build`/`go run` directly

## Platform Requirements

**Development:**
- Windows 11 (primary dev machine, paths and scripts are Windows-specific)
- PostgreSQL 17 installed at `C:\Program Files\PostgreSQL\17\` (hardcoded in `pipeline_ingest.py`)
- OpenCode server at `C:\Users\DELL\scoop\apps\opencode\current\opencode.exe` (Scoop-managed)
- Ollama server instances on ports 11434, 11435, 11437, 11438 (multiple simultaneous instances)

**Production:**
- Local only - no cloud deployment detected; all services run on local machine
- PostgreSQL data dir: `workflow/output/postgres17_data/` (local to repo)
- OpenCode state dir: `workflow/output/opencode/`

---

*Stack analysis: 2026-03-22*
