---
phase: 03-patch-output-full-run
plan: 01
subsystem: pipeline
tags: [go, json, translations, v3-sidecar, export]

requires:
  - phase: 02-translation-engine
    provides: V2PipelineStore interface and Store implementation with pipeline state machine
provides:
  - QueryDone() method on V2PipelineStore interface and Store implementation
  - V3Sidecar/V3Entry types for esoteric-ebb-sidecar.v3 format
  - BuildV3Sidecar function converting done items to sidecar entries
  - WriteTranslationsJSON function for crash-safe JSON output
affects: [03-02, 03-03, patch-build]

tech-stack:
  added: []
  patterns: [v3-sidecar-format, atomic-json-write]

key-files:
  created:
    - workflow/internal/v2pipeline/export.go
    - workflow/internal/v2pipeline/export_test.go
  modified:
    - workflow/internal/contracts/v2pipeline.go
    - workflow/internal/v2pipeline/store.go
    - workflow/internal/v2pipeline/worker_test.go

key-decisions:
  - "V3 format uses item.ContentType as text_role and item.Speaker as speaker_hint (direct mapping, no transformation)"
  - "No dedup in sidecar output (D-02): each pipeline item gets its own entry regardless of source_raw duplicates"

patterns-established:
  - "V3Sidecar export pattern: BuildV3Sidecar(items) -> WriteTranslationsJSON(path, sidecar)"

requirements-completed: [PATCH-01]

duration: 3min
completed: 2026-03-22
---

# Phase 03 Plan 01: Patch Export Domain Logic Summary

**QueryDone() store method and V3Sidecar export producing esoteric-ebb-sidecar.v3 translations.json from done pipeline items**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-22T18:07:55Z
- **Completed:** 2026-03-22T18:10:23Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- Added QueryDone() to V2PipelineStore interface and Store, returning done items ordered by sort_index
- Implemented V3Sidecar/V3Entry types and BuildV3Sidecar function with D-02 (no dedup) and D-03 (passthrough) compliance
- WriteTranslationsJSON uses shared.AtomicWriteFile for crash-safe output
- 5 unit tests covering mixed items, no-dedup, passthrough, empty input, and file write

## Task Commits

Each task was committed atomically:

1. **Task 1: Store.QueryDone() + V2PipelineStore interface extension** - `3418f41` (feat)
2. **Task 2: V3Sidecar types + WriteTranslationsJSON (RED)** - `922a199` (test)
3. **Task 2: V3Sidecar types + WriteTranslationsJSON (GREEN)** - `1828ad6` (feat)

## Files Created/Modified
- `workflow/internal/contracts/v2pipeline.go` - Added QueryDone() to V2PipelineStore interface
- `workflow/internal/v2pipeline/store.go` - QueryDone() implementation querying state=done ORDER BY sort_index
- `workflow/internal/v2pipeline/worker_test.go` - Added QueryDone() to fakeStore for interface compliance
- `workflow/internal/v2pipeline/export.go` - V3Sidecar/V3Entry types, BuildV3Sidecar, WriteTranslationsJSON
- `workflow/internal/v2pipeline/export_test.go` - 5 tests: mixed items, no-dedup, passthrough, empty, file write

## Decisions Made
- V3 format directly maps ContentType -> TextRole and Speaker -> SpeakerHint without transformation
- No dedup in sidecar (D-02): each item produces a separate entry even with identical source_raw

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added QueryDone() to fakeStore in worker_test.go**
- **Found during:** Task 1
- **Issue:** Adding QueryDone to interface broke compile-time check for fakeStore in worker_test.go
- **Fix:** Added stub QueryDone() method to fakeStore
- **Files modified:** workflow/internal/v2pipeline/worker_test.go
- **Verification:** go build and go vet pass
- **Committed in:** 3418f41

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Necessary for interface compliance. No scope creep.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- QueryDone() and V3Sidecar export ready for Plan 03-02 (CLI command) and 03-03 (full run)
- All 20 v2pipeline tests passing

---
*Phase: 03-patch-output-full-run*
*Completed: 2026-03-22*
