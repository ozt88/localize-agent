# External Integrations

**Analysis Date:** 2026-03-22

## APIs & External Services

**LLM Inference - OpenCode (primary AI backend):**
- OpenCode AI server - session-based LLM orchestration for translation and evaluation
  - SDK/Client: custom HTTP client in `workflow/pkg/platform/llm_client.go` (`SessionLLMClient`)
  - API base: `http://127.0.0.1:4112` (default; configured per project in `project.json`)
  - Protocol: REST JSON, session-scoped (`/session`, `/session/{id}/message`)
  - Models accessed via `provider/model` format (e.g., `openai/gpt-5.2`, `openai/gpt-5.4`)
  - Managed by: `scripts/manage-opencode-serve.ps1`, `scripts/run-opencode-serve-wrapper.ps1`
  - Plugin: `@opencode-ai/plugin 1.2.26` installed in `.opencode/`
  - Agents defined in `.opencode/agents/` (e.g., `rt-ko-translate-primary.md`, `rt-ko-eval-primary.md`)

**LLM Inference - Ollama (local model backend):**
- Ollama - local LLM serving for translation and evaluation with custom fine-tuned models
  - SDK/Client: custom HTTP client in `workflow/pkg/platform/ollama_client.go` (`OllamaLLMClient`)
  - API base: configurable per project (e.g., `http://127.0.0.1:11434`, `11435`, `11437`, `11438`)
  - Protocol: REST JSON at `/api/chat` (Ollama chat completion format)
  - Models: custom `TranslateGemma:latest`, `TranslateGemma-fast:latest` (project-specific fine-tunes)
  - Managed by: `scripts/manage-ollama-serve.ps1`, `scripts/run-ollama-serve-wrapper.ps1`
  - Multiple simultaneous Ollama instances supported (different ports per project)

**LLM Backend Selection:**
- Backend is selected via `llm_backend` field in `project.json` or `--llm-backend` CLI flag
- Normalized in `workflow/pkg/platform/llm_backend.go`: values `opencode` or `ollama`
- Translation pipeline supports dual-lane LLM (low-quality fast + high-quality slow): `pipeline.low_llm`, `pipeline.high_llm`, `pipeline.score_llm` in `project.json`

**Wiki Scraping (esoteric-ebb RAG):**
- `esotericebb.wiki.gg` MediaWiki API - game wiki scraping for lore glossary building
  - Client: stdlib `urllib.request` in `projects/esoteric-ebb/rag/scrape_wiki.py`
  - Endpoint: `https://esotericebb.wiki.gg/api.php`
  - Rate-limited: 0.5s between requests
  - Output: JSON files in `projects/esoteric-ebb/rag/wiki_pages/`

## Data Storage

**Databases:**
- SQLite (primary/default checkpoint store)
  - Driver: `modernc.org/sqlite` (pure Go, no CGo)
  - Client: `database/sql` via `workflow/pkg/platform/sqlite.go`
  - Used for: translation checkpoints (`items` table), evaluation results (`eval_items` table)
  - File locations: `projects/<name>/output/batches/<batch>/translation_checkpoint.db`, `projects/<name>/output/evaluation_unified.db`
  - Pragma settings: WAL journal mode, synchronous=FULL, foreign keys ON, busy_timeout=5000ms
  - Max connections: 1 (serialized access for write safety)

- PostgreSQL 17 (pipeline store / optional checkpoint backend)
  - Driver: `github.com/jackc/pgx/v5` (via stdlib adapter)
  - Client: `database/sql` via `workflow/pkg/platform/postgres.go`
  - Connection: `postgres://postgres@127.0.0.1:5433/localize_agent?sslmode=disable`
  - Database name: `localize_agent`
  - Port: 5433 (non-default, avoids conflict with system PostgreSQL)
  - Data directory: `workflow/output/postgres17_data/`
  - Used for: `items` and `pipeline_items` tables (translation pipeline orchestration)
  - Max connections: 16 open / 16 idle
  - Managed by: `scripts/manage-postgres5433.ps1`
  - Schema embedded in Go binary: `workflow/pkg/platform/postgres_translation_pipeline_schema.sql` (via `//go:embed`)

**Database Backend Selection:**
- Configured via `checkpoint_backend` in `project.json` or `--checkpoint-backend` CLI flag
- Values: `sqlite` (default) or `postgres` / `postgresql`
- Migration tool: `workflow/cmd/go-migrate-checkpoint/` - migrates SQLite checkpoint to PostgreSQL
- Normalization: `workflow/pkg/platform/db_backend.go`

**File Storage:**
- Local filesystem only
  - Source strings: JSON files (e.g., `source_esoteric.json`, `enGB_original.json`)
  - IDs files: plain text (e.g., `ids_esoteric.txt`)
  - Context/rules: Markdown files in `projects/<name>/context/`
  - Trace output: JSONL files (optional, `--trace-out` flag)
  - Embeddings I/O: temp JSON files in working directory (`semantic_review_embed_input.json`, `semantic_review_embed_output.json`)

**Caching:**
- None - no Redis or in-memory cache layer; SQLite checkpoint stores serve as result persistence

## Authentication & Identity

**Auth Provider:**
- None - all services run locally with no authentication
- PostgreSQL: password-less connection (`postgres` user, localhost only)
- OpenCode: no auth tokens (local HTTP server)
- Ollama: no auth tokens (local HTTP server)

## Monitoring & Observability

**Error Tracking:**
- None - no Sentry or external error service

**Logs:**
- Structured JSONL trace logs written to disk via `workflow/pkg/platform/trace_sink.go` and `trace_path.go`
- Trace events include: `warmup`, `prompt`, `prompt_error`, `request`, `response_error`, `session_create_error`
- Trace enabled with `--trace-out <path>` flag on CLI commands
- Pipeline heartbeat logs: JSONL files in `<batch_dir>/run_logs/pipeline_heartbeats/`
- PostgreSQL log: `workflow/output/postgres17.log`
- Console output: Go `fmt.Printf` for progress metrics and summaries

## CI/CD & Deployment

**Hosting:**
- Local workstation only (Windows 11)
- No cloud hosting, containers, or remote deployment

**CI Pipeline:**
- None detected - no GitHub Actions, no CI config

## Webhooks & Callbacks

**Incoming:**
- None

**Outgoing:**
- None

## Semantic Embedding (Sub-process Integration)

**Python sub-process:**
- Go calls `python embed_compare.py` as a subprocess for semantic similarity scoring
  - Caller: `workflow/internal/semanticreview/embedding.go` (`computeSemanticSimilarities`)
  - Script: `workflow/internal/semanticreview/scripts/embed_compare.py`
  - Model: `sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2` (HuggingFace, downloaded on first use)
  - Interface: JSON files written/read from working directory (temp files cleaned up after use)

## Pipeline Ingest (PostgreSQL via psql CLI)

**psql subprocess:**
- Python ingestion tools call `psql.exe` as a subprocess for bulk DB operations
  - Caller: `projects/esoteric-ebb/tools/pipeline_ingest.py`
  - Executable: `C:/Program Files/PostgreSQL/17/bin/psql.exe` (hardcoded)
  - Connection: `127.0.0.1:5433` / database `localize_agent` / user `postgres`

---

*Integration audit: 2026-03-22*
