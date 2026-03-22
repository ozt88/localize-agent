---
phase: 02-translation-engine
plan: 04
subsystem: pipeline
tags: [go, postgresql, worker-pool, llm, translate, format, score, retry, goroutines]

# Dependency graph
requires:
  - phase: 02-translation-engine
    plan: 01
    provides: "v2pipeline store, DB schema, contracts.V2PipelineStore, state machine constants"
  - phase: 02-translation-engine
    plan: 02
    provides: "clustertranslate: BuildScriptPrompt, ParseNumberedOutput, MapLinesToIDs, glossary loader"
  - phase: 02-translation-engine
    plan: 03
    provides: "tagformat: BuildFormatPrompt, ParseFormatResponse, ValidateTagMatch; scorellm: BuildScorePrompt, ParseScoreResponse, ScoreFinal()"

provides:
  - "TranslateWorker, FormatWorker, ScoreWorker — 3-role concurrent worker implementations in v2pipeline package"
  - "Run(Config) int — pipeline orchestrator launching worker pools for all 3 roles"
  - "go-v2-pipeline CLI entry point with full flag interface"
  - "v2_base_prompt.md, v2_format_prompt.md, v2_score_prompt.md — LLM system prompts for each role"

affects: [03-patch-build, 04-verify-plugin]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Role-prefixed session keys (v2-translate-, v2-format-, v2-score-) prevent session contamination across LLM roles (Pitfall 3)"
    - "D-15 retry escalation: attempts < 2 same model with hint, attempt >= 2 escalate to high_llm, >= 3 MarkFailed"
    - "D-16 attempt log: AppendAttemptLog on every attempt (success or failure) for audit trail"
    - "D-14 score routing: MarkScored routes to pending_translate/pending_format/done based on FailureType"
    - "context.Context-driven goroutine lifecycle with SIGINT/SIGTERM cancellation"

key-files:
  created:
    - workflow/internal/v2pipeline/worker.go
    - workflow/internal/v2pipeline/worker_test.go
    - workflow/internal/v2pipeline/run.go
    - workflow/cmd/go-v2-pipeline/main.go
    - projects/esoteric-ebb/context/v2_base_prompt.md
    - projects/esoteric-ebb/context/v2_format_prompt.md
    - projects/esoteric-ebb/context/v2_score_prompt.md
  modified: []

key-decisions:
  - "scorellm/types.go uses string literals instead of v2pipeline constants to break import cycle — cross-package constants avoided"
  - "Code review additions: speaker/choice/gate columns in worker struct, PrevGateLines for scene context, -once flag for single-batch test runs"
  - "Excluded blocks (content_type=excluded) skipped in translate worker before LLM call"
  - "ScoreFinal() method on ScoreResult computes weighted final score (vs inline float math in worker)"

patterns-established:
  - "Worker loop: ClaimPending -> process -> MarkResult -> AppendAttemptLog, sleep on empty queue"
  - "FormatWorker skips items with has_tags=false (routed directly to pending_score by MarkTranslated)"
  - "stderr logging for worker errors (non-fatal item failures), os.Exit only for config/init failures"

requirements-completed: [TRANS-01, TRANS-02, TRANS-03, TRANS-04, TRANS-05, TRANS-06, TRANS-07, TRANS-08, INFRA-01, INFRA-02, INFRA-03]

# Metrics
duration: multi-session (tasks 1-2 prior session, review fix post-checkpoint)
completed: 2026-03-23
---

# Phase 02 Plan 04: Pipeline Orchestrator and Worker Pool Summary

**3-role concurrent worker pool (translate/format/score) with D-15 retry escalation, D-16 attempt logging, and go-v2-pipeline CLI wiring all domain packages into a runnable pipeline**

## Performance

- **Duration:** Multi-session (tasks committed in prior session, code review fix applied post-checkpoint)
- **Started:** prior session
- **Completed:** 2026-03-23
- **Tasks:** 3 (Task 1: workers, Task 2: orchestrator + CLI, Task 3: verification with code review)
- **Files modified:** 7

## Accomplishments

- TranslateWorker, FormatWorker, ScoreWorker implement full pipeline stages with role-prefixed session keys (Pitfall 3 compliance)
- Run() orchestrator launches configurable-concurrency worker pools, handles SIGINT/SIGTERM gracefully
- CLI (go-v2-pipeline) provides full flag interface: -role, -once, -cleanup-stale-claims, all LLM profile overrides
- 15 v2pipeline tests + all domain tests pass; both go-v2-ingest and go-v2-pipeline binaries compile

## Task Commits

Each task was committed atomically:

1. **Task 1: Worker implementations for 3 LLM roles with retry logic** - `7536fc2` (feat)
2. **Task 2: Pipeline orchestrator Run function and CLI entry point** - `152e49b` (feat)
3. **Task 3: Code review fixes (speaker/choice/gate, PrevGateLines, -once, excluded blocks)** - `e2c9236` (fix)

## Files Created/Modified

- `workflow/internal/v2pipeline/worker.go` — TranslateWorker, FormatWorker, ScoreWorker with D-15 retry and D-16 attempt logging
- `workflow/internal/v2pipeline/worker_test.go` — 7 unit tests using fake store and fake LLM client
- `workflow/internal/v2pipeline/run.go` — Run(Config) int orchestrator: store open, glossary load, 3-role worker pools, SIGINT handling, CountByState reporting
- `workflow/cmd/go-v2-pipeline/main.go` — CLI entry point: -project, -dsn, -role, -once, -cleanup-stale-claims, all LLM profile flags
- `projects/esoteric-ebb/context/v2_base_prompt.md` — Translation system prompt (numbered line output, speaker/CHOICE preservation, proper noun rules)
- `projects/esoteric-ebb/context/v2_format_prompt.md` — Tag restoration system prompt for codex-spark formatter
- `projects/esoteric-ebb/context/v2_score_prompt.md` — Quality evaluation system prompt for score LLM

## Decisions Made

- **Import cycle resolution:** scorellm/types.go uses string literals for FailureType constants instead of importing v2pipeline constants. Cross-package constant sharing avoided by keeping state constants local to v2pipeline.
- **ScoreFinal() method:** Code review added `ScoreFinal()` method to ScoreResult instead of inline float arithmetic in worker, cleaner abstraction boundary.
- **Excluded blocks skipped early:** TranslateWorker checks content_type before LLM call to avoid wasting API calls on excluded items.
- **-once flag:** Single-batch-and-exit mode enables dry-run testing without needing full DB population or infinite loop behavior.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Import cycle between v2pipeline and scorellm packages**
- **Found during:** Task 1 (worker implementation)
- **Issue:** scorellm/types.go needed FailureType constants, v2pipeline/types.go defined them — circular import
- **Fix:** scorellm/types.go defines FailureType values as string literals; v2pipeline worker compares against those literals
- **Files modified:** workflow/internal/scorellm/types.go
- **Verification:** go build ./... passes with no import cycle errors
- **Committed in:** 7536fc2 (Task 1 commit)

**2. [Rule 2 - Code Review] Added speaker/choice/gate columns, PrevGateLines, -once flag, excluded block guard**
- **Found during:** Post-Task 2 code review (checkpoint)
- **Issue:** Worker struct missing speaker/choice/gate fields needed for ClusterTask construction; PrevGateLines absent for scene context; no -once test mode; excluded blocks not filtered before LLM call
- **Fix:** Added fields to worker item struct, PrevGateLines lookup from store, -once flag to CLI and Run(), content_type guard in TranslateWorker
- **Files modified:** workflow/internal/v2pipeline/worker.go, workflow/cmd/go-v2-pipeline/main.go, workflow/internal/v2pipeline/run.go
- **Verification:** go build and go test ./workflow/internal/v2pipeline/... all pass post-fix
- **Committed in:** e2c9236 (code review fix commit)

---

**Total deviations:** 2 auto-fixed (1 import cycle bug, 1 code review enhancement batch)
**Impact on plan:** Import cycle fix required for compilation. Code review fixes improve correctness and testability. No scope creep.

## Issues Encountered

- FormatWorker batch size: plan specified 3-5 items per D-06; final implementation uses cfg.FormatBatchSize (configurable), defaulting to 5 — plan intent preserved.
- ScoreWorker processes one item at a time per RESEARCH.md Open Question 2 resolution; single-item scoring avoids context contamination between different block types.

## User Setup Required

None — no external service configuration required. PostgreSQL DSN provided at runtime via -dsn flag.

## Next Phase Readiness

- Full v2 pipeline runnable: `go run ./workflow/cmd/go-v2-pipeline/ -project esoteric-ebb -dsn "..." -role all`
- All domain packages wired: clustertranslate, tagformat, scorellm, glossary, v2pipeline store
- Phase 03 (patch build) can begin: translations flow to done state in pipeline_items_v2, ready for apply-out extraction
- Remaining concern: tag registry corpus enumeration (~50 unique patterns estimate) may surface edge cases during first full run

---
*Phase: 02-translation-engine*
*Completed: 2026-03-23*
