---
phase: 02-translation-engine
plan: 01
subsystem: database
tags: [postgresql, sqlite, pipeline, state-machine, lease-based, v2pipeline]

requires:
  - phase: 01-source-parser
    provides: "DialogueBlock, ParseResult, BuildBatches — Phase 1 parser output consumed by ingest CLI"
provides:
  - "V2PipelineStore interface (contracts/v2pipeline.go)"
  - "PostgreSQL/SQLite store implementation (v2pipeline/store.go)"
  - "pipeline_items_v2 DDL with indexes"
  - "v2-ingest CLI for loading Phase 1 JSON into DB"
affects: [02-02, 02-03, 02-04]

tech-stack:
  added: []
  patterns:
    - "v2pipeline Store with rebind() for PostgreSQL/SQLite dual-backend"
    - "ON CONFLICT (source_hash) DO NOTHING for dedup per INFRA-02"
    - "Lease-based ClaimPending with CTE UPDATE RETURNING (postgres) / two-step (sqlite)"
    - "MarkTranslated routes by has_tags: true->pending_format, false->pending_score"
    - "MarkScored routes by failure_type per D-14 (pass/translation/format/both)"

key-files:
  created:
    - workflow/internal/contracts/v2pipeline.go
    - workflow/internal/v2pipeline/types.go
    - workflow/internal/v2pipeline/store.go
    - workflow/internal/v2pipeline/store_test.go
    - workflow/internal/v2pipeline/postgres_v2_schema.sql
    - workflow/cmd/go-v2-ingest/main.go
  modified: []

key-decisions:
  - "source_hash UNIQUE constraint for ON CONFLICT dedup (not just index)"
  - "SQLite schema uses INTEGER for has_tags, TEXT for timestamps/JSONB (PostgreSQL uses native types)"
  - "ClaimPending uses CTE UPDATE RETURNING for PostgreSQL, two-step SELECT+UPDATE for SQLite"
  - "Ingest CLI reads envelope format {results: [...]} matching go-ink-parse output"

patterns-established:
  - "v2pipeline.Store dual-backend pattern: rebind() for ?->$N, timeValue/boolValue for type adaptation"
  - "Pipeline state constants in v2pipeline/types.go, interface in contracts/v2pipeline.go"

requirements-completed: [INFRA-01, INFRA-02, INFRA-03]

duration: 5min
completed: 2026-03-22
---

# Phase 02 Plan 01: V2 Pipeline DB Infrastructure Summary

**Lease-based pipeline state machine with PostgreSQL/SQLite store, source_hash dedup, and Phase 1 JSON ingest CLI**

## Performance

- **Duration:** 5 min
- **Started:** 2026-03-22T15:42:45Z
- **Completed:** 2026-03-22T15:47:57Z
- **Tasks:** 3
- **Files created:** 6

## Accomplishments
- V2PipelineStore interface with 12 methods covering full translate/format/score lifecycle
- PostgreSQL store with embedded DDL, 4 indexes (state, state+lease, source_hash, batch_id)
- Ingest CLI that reads Phase 1 parser JSON, assigns batch IDs via BuildBatches, detects tags, handles passthrough blocks
- 8 tests covering seed dedup, claim/release, has_tags routing, failure_type routing, counts, attempt log, retry state, mark failed

## Task Commits

Each task was committed atomically:

1. **Task 1: Define V2 pipeline contracts and types** - `f426a78` (feat)
2. **Task 2: Implement PostgreSQL store with embedded DDL** - `9225531` (feat)
3. **Task 3: Build ingest CLI loading Phase 1 JSON into pipeline_items_v2** - `7d3f497` (feat)

## Files Created/Modified
- `workflow/internal/contracts/v2pipeline.go` - V2PipelineStore interface and V2PipelineItem struct
- `workflow/internal/v2pipeline/types.go` - 10 state constants and Config struct
- `workflow/internal/v2pipeline/store.go` - Full store implementation with PostgreSQL/SQLite dual-backend
- `workflow/internal/v2pipeline/store_test.go` - 8 tests covering all store operations
- `workflow/internal/v2pipeline/postgres_v2_schema.sql` - DDL for pipeline_items_v2 table
- `workflow/cmd/go-v2-ingest/main.go` - CLI to ingest Phase 1 JSON into DB

## Decisions Made
- Used UNIQUE constraint on source_hash (not just index) to enable ON CONFLICT dedup
- SQLite schema adapts PostgreSQL types: JSONB->TEXT, TIMESTAMPTZ->TEXT, BOOLEAN->INTEGER
- ClaimPending uses CTE UPDATE RETURNING for PostgreSQL atomicity, two-step approach for SQLite
- Ingest CLI reads the envelope format `{results: [...]}` matching go-ink-parse output structure

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Added UNIQUE constraint to source_hash column**
- **Found during:** Task 2 (store implementation)
- **Issue:** SQLite ON CONFLICT (source_hash) DO NOTHING requires a UNIQUE constraint, not just an index
- **Fix:** Added UNIQUE to source_hash column definition in both SQLite and PostgreSQL schemas
- **Files modified:** workflow/internal/v2pipeline/store.go, workflow/internal/v2pipeline/postgres_v2_schema.sql
- **Verification:** All 8 tests pass including TestSeedInsertsAndDeduplicates
- **Committed in:** 9225531 (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Essential fix for dedup correctness. No scope creep.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- V2PipelineStore interface ready for downstream plans (translate, format, score workers)
- Ingest CLI ready to populate pipeline_items_v2 from Phase 1 parser output
- DDL has all columns needed for the full translate->format->score lifecycle

## Self-Check: PASSED

All 6 created files verified present. All 3 task commits verified in git log.

---
*Phase: 02-translation-engine*
*Completed: 2026-03-22*
