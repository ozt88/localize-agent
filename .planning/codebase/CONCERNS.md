# Codebase Concerns

**Analysis Date:** 2026-03-22

## Tech Debt

**Dead exported symbols (documented, not yet removed):**
- Issue: A formal cleanup plan exists (`plans/dead_code_cleanup_20260318.md`) identifying unused exported symbols across three packages. These bloat the API surface and confuse LLMs writing code against this codebase.
- Files:
  - `workflow/internal/semanticreview/scoring.go` — `BuildReportItem`, `BuildDirectScoreReportItem`
  - `workflow/internal/semanticreview/run.go` — `ComputeSemanticSimilarities`, `WriteReports`, `ReviewDirectItems`
  - `workflow/internal/semanticreview/reader.go` — `LoadDoneItems`
  - `workflow/internal/semanticreview/backtranslate.go` — `NewBacktranslator`
  - `workflow/internal/semanticreview/direct_score.go` — `NewDirectScorer`
  - `workflow/pkg/platform/checkpoint_store.go` — `LoadDonePackItems` (no caller outside package)
  - `workflow/pkg/segmentchunk/chunker.go` — 11 exported symbols only used in tests
- Impact: Increases cognitive surface area, increases LLM hallucination risk when navigating the codebase.
- Fix approach: Execute tasks 1–3 in `plans/dead_code_cleanup_20260318.md`.

**`stringField` / `boolField` helpers duplicated across four packages:**
- Issue: The same `stringField(m map[string]any, key string) string` function is copy-pasted in four separate locations.
- Files:
  - `workflow/cmd/go-review/main.go:1432`
  - `workflow/internal/semanticreview/reader.go:308`
  - `workflow/internal/translation/checkpoint_meta.go:91`
  - `workflow/pkg/platform/checkpoint_store.go:351`
  - `boolField` duplicated across `workflow/cmd/go-review/main.go:1440` and `workflow/internal/translation/checkpoint_meta.go:122`
- Impact: Any fix to the pattern must be applied in four places. Divergence risk over time.
- Fix approach: Extract to `workflow/pkg/shared/` as a shared map utility.

**`sourceHash` computed as string length, not a real hash:**
- Issue: In `workflow/internal/translation/result_persister.go` (lines 111 and 184), `sourceHash` is set to `fmt.Sprintf("%x", len(meta.enText))` — which is just the hexadecimal of the text's byte length. This does not detect content changes.
- Files: `workflow/internal/translation/result_persister.go`
- Impact: Two source texts of the same length will be considered identical for checkpoint dedup purposes. Content changes that preserve length will not invalidate checkpoints.
- Fix approach: Replace with `fmt.Sprintf("%x", sha256.Sum256([]byte(meta.enText)))`.

**`skippedTimeout` counter is always zero:**
- Issue: In `workflow/internal/translation/pipeline.go`, `skippedTimeout` is declared (line 33), initialized to 0 (line 51), and logged out (line 139) but is never incremented anywhere in the file. The pipeline reports a timeout count metric that is permanently 0.
- Files: `workflow/internal/translation/pipeline.go`
- Impact: Misleading metrics output. Actual timeout events (from `CallWithRetry` failures) are classified as `skippedTranslatorErr`, not surfaced as timeouts.
- Fix approach: Either remove the counter and metric label, or wire it to count `context.DeadlineExceeded` errors from the retry loop.

**Inline regex compilation inside hot functions:**
- Issue: In `workflow/internal/translation/normalized_input.go` (lines 288–292), four `regexp.MustCompile` calls appear inside `deriveFragmentHints`, a function called per translation item. Same pattern in `workflow/internal/translation/structured_text.go` line 180 inside `stripExistingStatCheckPrefix`.
- Files:
  - `workflow/internal/translation/normalized_input.go:288–292`
  - `workflow/internal/translation/structured_text.go:180`
  - `workflow/internal/translation/glossary.go:78` (compiled per call)
- Impact: Regex compilation is non-trivial CPU work repeated thousands of times per translation run.
- Fix approach: Hoist to package-level `var` declarations like the existing pattern in `postprocess_validation.go`.

**`translationpipeline/store.go` is a monolith (2062 lines):**
- Issue: All pipeline state-machine transitions, query logic, scoring decisions, job claiming, and repair heuristics are in a single file.
- Files: `workflow/internal/translationpipeline/store.go`
- Impact: High cognitive load; difficult to test individual transitions; any new pipeline state requires editing this one file. The file is already over the context window of some LLM editing sessions.
- Fix approach: Decompose into sub-files by concern: `store_claim.go`, `store_score.go`, `store_repair.go`, `store_seed.go`.

**`workflow/cmd/go-review/main.go` is a monolith (2286 lines):**
- Issue: The review HTTP server, all route handlers, SQL query logic, HTML template, and review UI data formatting are in a single `main.go`.
- Files: `workflow/cmd/go-review/main.go`
- Impact: Any change to the review UI requires editing a large, dense file. Hard to test individual handlers.
- Fix approach: Extract handlers, template rendering, and DB query logic into separate files within the same `main` package.

**`go-review-next.exe` binary committed to `workflow/bin/` without a corresponding `cmd/` source:**
- Issue: `workflow/bin/go-review-next.exe` exists as a tracked binary artifact. No `workflow/cmd/go-review-next/` source directory exists. The `.gitignore` excludes `workflow/bin/` but this file is currently committed.
- Files: `workflow/bin/go-review-next.exe`
- Impact: Repo consumers cannot rebuild this binary from source. Stale binary will diverge from future changes.
- Fix approach: Create `workflow/cmd/go-review-next/` with source, or remove the binary and document it as a local build artifact.

**SQLite busy timeout set differently across two codepaths:**
- Issue: `workflow/pkg/platform/sqlite.go` sets `PRAGMA busy_timeout=5000` (5 second wait). The `openSQLiteStore` in `workflow/internal/translationpipeline/store.go` (lines 101–108) sets the same pragma manually without the shared `openSQLite` helper — so it runs separately, avoiding the shared wrapper. This is a divergent SQLite initialization path.
- Files:
  - `workflow/pkg/platform/sqlite.go`
  - `workflow/internal/translationpipeline/store.go:96–116`
- Impact: Any future change to shared SQLite settings (e.g., WAL checkpoint interval, foreign keys) may be missed in the pipeline store path.
- Fix approach: Use `platform.openSQLite` inside `openSQLiteStore`, or export it properly.

## Known Bugs

**`UpsertItems` `pack_json` COALESCE semantics silently drop pack data on null input:**
- Symptoms: When `KOObj` or `PackObj` is `nil`, `json.Marshal(nil)` returns `"null"` (not empty string). The SQLite `COALESCE(excluded.ko_json, items.ko_json)` checks for SQL NULL, not `"null"` the JSON string. A `"null"` string will overwrite existing data.
- Files: `workflow/pkg/platform/checkpoint_store.go:174–181`
- Trigger: Calling `UpsertItem` with `koObj = nil` or `packObj = nil` on an existing row.
- Workaround: Currently callers always provide non-nil maps for successful translations.

## Security Considerations

**Hardcoded PostgreSQL DSN committed in a project.json:**
- Risk: The DSN `postgres://postgres@127.0.0.1:5433/localize_agent?sslmode=disable` is stored inside `projects/esoteric-ebb/output/batches/canonical_full_retranslate_dual_score_20260311_1/project.json`. The `output/batches/` path is not gitignored. This is a localhost DSN, not a remote credential, but the pattern is fragile — a future DSN with a password would be committed.
- Files: `projects/esoteric-ebb/output/batches/canonical_full_retranslate_dual_score_20260311_1/project.json`
- Current mitigation: DSN uses no password, localhost only.
- Recommendations: Add `projects/*/output/batches/` to `.gitignore` or ensure `checkpoint_dsn` is always sourced from environment variables via `os.Getenv`, not stored in project config files.

**Hardcoded DSN also appears in build report artifacts:**
- Risk: `projects/esoteric-ebb/extract/*/build_report.json` and `translations.json` embed the same DSN string. These are inside `projects/*/extract/` which is gitignored, but artifacts at rest contain the DSN string.
- Files: `projects/esoteric-ebb/extract/1.1.1/ExportedProject/Assets/StreamingAssets/TranslationPatch/build_report.json`
- Current mitigation: `projects/*/extract/` is gitignored.
- Recommendations: Strip DSN from build reports before writing; or document that build reports must not be shared externally.

**LLM API calls have no authentication headers:**
- Risk: `SessionLLMClient.postJSON` in `workflow/pkg/platform/llm_client.go` sends plain JSON with no Authorization header. Relies entirely on network-level access control (localhost binding).
- Files: `workflow/pkg/platform/llm_client.go:177–285`
- Current mitigation: OpenCode server is assumed to run on localhost only.
- Recommendations: If OpenCode server is ever exposed on a non-loopback interface, all LLM traffic becomes unauthenticated.

## Performance Bottlenecks

**Glossary matching compiles a regex per call:**
- Problem: `containsGlossaryTerm` in `workflow/internal/translation/glossary.go:77` calls `regexp.MustCompile` for every `(entry, text)` pair evaluation. With a large glossary and many items, this runs O(items × glossary_size) compilations per translation run.
- Files: `workflow/internal/translation/glossary.go:77`
- Cause: No caching of compiled patterns per glossary entry.
- Improvement path: Pre-compile all glossary patterns once in `loadGlossaryEntries` and store them on the `glossaryEntry` struct.

**`store.go` `RouteKnownFailedNoDoneRow` loads all failed rows then filters in Go:**
- Problem: In `workflow/internal/translationpipeline/store.go` (around line 560–620), the method fetches all `StateFailed` rows with a specific `last_error` string and then classifies them in Go. On large datasets this pulls many rows from the DB that are immediately discarded.
- Files: `workflow/internal/translationpipeline/store.go`
- Cause: Classification logic (`classifyKnownFailedNoRowFamily`) depends on parsed JSON pack content, making SQL-side filtering difficult.
- Improvement path: Add a `family` or `routing_hint` column populated during failure recording so classification can be pushed to SQL.

**SQLite single-writer bottleneck for high-concurrency pipeline workers:**
- Problem: `workflow/pkg/platform/sqlite.go` enforces `SetMaxOpenConns(1)` for SQLite (correct), giving a maximum write throughput of one commit at a time. With multiple Go worker goroutines, write lock contention is serialized. The 5-second busy timeout means write-heavy workers can stall.
- Files: `workflow/pkg/platform/sqlite.go`
- Cause: SQLite WAL mode allows one writer; not a bug, but a scaling ceiling.
- Improvement path: Migrate to PostgreSQL backend for multi-worker production runs (the Postgres path already exists).

## Fragile Areas

**`AtomicWriteFile` uses delete-then-rename on Windows:**
- Files: `workflow/pkg/shared/atomic_write.go:38–43`
- Why fragile: On Windows, `os.Rename` over an existing file fails, so the code deletes the target then renames the temp. There is a small window between `os.Remove(path)` and `os.Rename(tmpPath, path)` where the output file does not exist. A crash or power loss in that window destroys the output.
- Safe modification: The current behavior is the best available workaround for Windows; note that true atomicity on Windows requires alternative strategies (e.g., transaction NTFS).
- Test coverage: Not tested for failure-in-window scenario.

**Translation pipeline `persistResults` mutates `meta.curObj` directly:**
- Files: `workflow/internal/translation/result_persister.go:66`
- Why fragile: `base := meta.curObj` followed by `base["Text"] = restored` mutates the shared object from `rt.currentStrings`. Multiple workers operating concurrently share this map structure. The `doneMu` mutex protects only the `done` map, not the underlying `meta.curObj` map entries.
- Safe modification: Deep-copy `meta.curObj` before writing `"Text"` to avoid data races under high concurrency.
- Test coverage: No concurrent mutation test exists.

**Checkpoint schema migration uses `ALTER TABLE … ADD COLUMN` with error swallowing:**
- Files: `workflow/internal/translationpipeline/store.go:164–173`, `workflow/pkg/platform/eval_store.go:58–69`
- Why fragile: Missing columns are added by running `ALTER TABLE` and swallowing errors that contain "duplicate column name". If a real DDL error occurs (e.g., disk full, permissions), it is silently ignored because it's matched by the same string check.
- Safe modification: Check the specific SQLite error code (1) rather than string matching.
- Test coverage: No failure-case test for schema migration.

**`looksNonEnglishPassthroughSource` heuristic is English word-list based:**
- Files: `workflow/internal/translation/structured_text.go:80–107`
- Why fragile: The function identifies non-English source text by absence of common English words and presence of non-ASCII characters. It will misidentify short English technical strings (game IDs, proper nouns) or English text that happens not to contain the listed common words.
- Safe modification: Any change to the word list will silently change which items are skipped as passthroughs.
- Test coverage: Covered in `structured_text_test.go` but only for known-good examples.

## Scaling Limits

**SQLite-backed pipeline store:**
- Current capacity: Handles thousands of items comfortably in single-writer mode.
- Limit: Performance degrades under concurrent multi-worker write load at scale (>10 writers). The `busy_timeout=5000ms` is a hard ceiling before writes return errors.
- Scaling path: Use the already-implemented PostgreSQL backend (`workflow/internal/translationpipeline/store.go:118–137`).

**PostgreSQL connection pool fixed at 16:**
- Current capacity: `workflow/pkg/platform/postgres.go` hardcodes `SetMaxOpenConns(16)` and `SetMaxIdleConns(16)`.
- Limit: Not configurable from project config or CLI flags.
- Scaling path: Expose as a config parameter in `ProjectConfig`.

## Dependencies at Risk

**`modernc.org/sqlite` (pure-Go SQLite driver):**
- Risk: Pure-Go SQLite implementation is less battle-tested than `mattn/go-sqlite3` (CGo). Potential subtle behavioral differences vs. the C SQLite library, particularly around WAL mode, PRAGMA semantics, and concurrent access edge cases.
- Impact: Data corruption edge cases may be harder to debug.
- Migration plan: Switching to `mattn/go-sqlite3` requires CGo build toolchain on Windows, which is a setup burden. Current driver is appropriate given the Windows-primary development environment.

**OpenCode server dependency with undocumented API:**
- Risk: `SessionLLMClient` in `workflow/pkg/platform/llm_client.go` depends on a `/session` and `/session/{id}/message` API. The OpenCode server's API contract is not documented in this repo. Breaking changes to the OpenCode API would fail silently at runtime.
- Impact: Pipeline fails at first LLM call with HTTP error; no compile-time safety.
- Migration plan: Add integration tests against a mock OpenCode server (a partial mock exists in `workflow/internal/translation/pipeline_mock_test.go`).

## Missing Critical Features

**No retry budget for checkpoint write failures:**
- Problem: In `result_persister.go`, a checkpoint write failure immediately sets `abortWorker = true`, stopping the entire worker goroutine. There is no retry for transient write errors (e.g., SQLite lock contention under high concurrency).
- Blocks: Cannot safely run many translation workers sharing one SQLite checkpoint DB.
- Files: `workflow/internal/translation/result_persister.go:119–133`

**No health check or circuit breaker for LLM server:**
- Problem: If the OpenCode or Ollama server goes down mid-pipeline, each item will exhaust its full `MaxAttempts × BackoffSec` retry budget before failing. There is no global server health check to pause work until the server recovers.
- Blocks: Long pipeline runs on flaky servers waste time and tokens on exhaustive per-item retries.
- Files: `workflow/pkg/platform/llm_client.go`, `workflow/pkg/shared/retry.go`

## Test Coverage Gaps

**`translationpipeline/run.go` orchestration logic:**
- What's not tested: The full pipeline orchestration in `Run()` — worker spawning, scoring loop, score-decision commit — has no integration test. Only the store's SQL operations are tested in `store_test.go`.
- Files: `workflow/internal/translationpipeline/run.go`
- Risk: Regression in worker coordination, concurrency ordering, or scoring threshold logic goes undetected.
- Priority: High

**`result_persister.go` concurrent mutation of `curObj`:**
- What's not tested: No test exercises concurrent goroutines writing through `persistResults` to verify there is no data race on the `meta.curObj` map.
- Files: `workflow/internal/translation/result_persister.go`
- Risk: Data race under concurrent load; may surface as silent data corruption rather than a panic.
- Priority: High

**`AtomicWriteFile` Windows edge cases:**
- What's not tested: The delete-then-rename path on Windows (triggered when normal rename-over fails) has no test.
- Files: `workflow/pkg/shared/atomic_write.go`
- Risk: Output file loss on crash in the rename window.
- Priority: Medium

**Python pipeline tools (`projects/esoteric-ebb/tools/`, `projects/esoteric-ebb/patch/tools/`):**
- What's not tested: The 30+ Python scripts have no test files whatsoever. They include database writes, file transformations, and SQL generation.
- Files: `projects/esoteric-ebb/tools/*.py`, `projects/esoteric-ebb/patch/tools/*.py`
- Risk: Regressions in data preparation, patch build, or SQL generation go undetected.
- Priority: Medium

---

*Concerns audit: 2026-03-22*
