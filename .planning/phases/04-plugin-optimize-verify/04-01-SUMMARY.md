---
phase: 04-plugin-optimize-verify
plan: 01
subsystem: export
tags: [v3sidecar, contextual-entries, dedup, json, translations]

# Dependency graph
requires:
  - phase: 03-patch-output
    provides: V3Sidecar struct, BuildV3Sidecar, WriteTranslationsJSON
provides:
  - "V3Sidecar contextual_entries[] field for ContextualMap disambiguation"
  - "Source-text dedup in entries[] (first-seen-wins) for TranslationMap"
affects: [04-02, 04-03, plugin-cs]

# Tech tracking
tech-stack:
  added: []
  patterns: [dual-output-sidecar, first-seen-wins-dedup]

key-files:
  created: []
  modified:
    - workflow/internal/v2pipeline/export.go
    - workflow/internal/v2pipeline/export_test.go

key-decisions:
  - "D-01: entries[] deduped by source text (first-seen-wins), contextual_entries[] contains all items"

patterns-established:
  - "Dual-output sidecar: entries for TranslationMap (deduped), contextual_entries for ContextualMap (full)"

requirements-completed: [PLUGIN-01]

# Metrics
duration: 2min
completed: 2026-03-26
---

# Phase 04 Plan 01: V3Sidecar Contextual Entries Summary

**V3Sidecar dual output: entries[] deduped by source text (first-seen-wins) + contextual_entries[] with all 35K items for ContextualMap disambiguation**

## Performance

- **Duration:** 2 min
- **Started:** 2026-03-26T03:12:17Z
- **Completed:** 2026-03-26T03:14:03Z
- **Tasks:** 1 (TDD: RED + GREEN)
- **Files modified:** 2

## Accomplishments
- V3Sidecar struct gains ContextualEntries field (json:"contextual_entries")
- BuildV3Sidecar dedupes entries[] by source text with first-seen-wins semantics
- contextual_entries[] preserves all items with full per-item metadata (SourceFile, TextRole, SpeakerHint)
- Plugin.cs field name compatibility confirmed (contextual_entries matches AddContextualEntry expectations)

## Task Commits

Each task was committed atomically:

1. **Task 1 RED: Failing tests for contextual_entries** - `83f2c88` (test)
2. **Task 1 GREEN: V3Sidecar contextual_entries implementation** - `372f6d6` (feat)

_TDD task: RED commit (failing tests) followed by GREEN commit (implementation)_

## Files Created/Modified
- `workflow/internal/v2pipeline/export.go` - Added ContextualEntries field to V3Sidecar, rewrote BuildV3Sidecar with dedup logic
- `workflow/internal/v2pipeline/export_test.go` - Added 3 new tests, updated NoDedup->DedupEntries test, updated EmptyItems test

## Decisions Made
- D-01: entries[] uses first-seen-wins dedup by source text for TranslationMap; contextual_entries[] has all items for ContextualMap

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Known Stubs
None - all data paths are fully wired.

## Next Phase Readiness
- contextual_entries[] ready for Plugin.cs ContextualMap consumption
- entries[] dedup reduces TranslationMap size while preserving full data in contextual_entries[]
- Plan 02 (Plugin.cs matching optimization) can proceed

---
*Phase: 04-plugin-optimize-verify*
*Completed: 2026-03-26*
