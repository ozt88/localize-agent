---
phase: 03-patch-output-full-run
plan: 02
subsystem: inkparse
tags: [ink-json, injection, tree-walking, source-hash, bom, korean-patch]

# Dependency graph
requires:
  - phase: 01-source-parser
    provides: "inkparse parser (walkContainer/walkFlatContent pattern, SourceHash, DialogueBlock)"
provides:
  - "InjectTranslations function for ink JSON Korean text insertion"
  - "InjectReport struct for tracking replacement statistics"
affects: [03-patch-output-full-run, patch-build]

# Tech tracking
tech-stack:
  added: []
  patterns: ["injector struct mirroring walker pattern for tree modification"]

key-files:
  created:
    - workflow/internal/inkparse/inject.go
  modified:
    - workflow/internal/inkparse/inject_test.go

key-decisions:
  - "Injector mirrors parser walkContainer/walkFlatContent structure exactly for hash consistency"
  - "First ^text node gets full Korean text, remaining nodes get empty ^ to preserve node count"
  - "Output always includes BOM to match original ink JSON file format"

patterns-established:
  - "Mirror pattern: new functionality (inject) mirrors existing traversal (parse) for consistency"
  - "In-place modification: modify []any slices from json.Unmarshal directly, then re-marshal"

requirements-completed: [PATCH-02]

# Metrics
duration: 3min
completed: 2026-03-22
---

# Phase 03 Plan 02: Ink JSON Injection Summary

**InjectTranslations function mirroring parser tree-walk to replace ^text nodes with Korean translations, with 11 tests covering unit and integration scenarios**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-22T18:07:56Z
- **Completed:** 2026-03-22T18:10:45Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- InjectTranslations walks ink JSON tree identically to Parse(), computing SourceHash for block matching
- Multi-node blocks handled correctly: first node gets full Korean, remaining get empty "^"
- 11 tests: 8 unit tests + 3 integration tests (round-trip, structure preservation, partial counts)

## Task Commits

Each task was committed atomically:

1. **Task 1: InjectTranslations core function with tree walking (TDD)** - `ffd43f3` (feat)
2. **Task 2: Integration test with real ink JSON structure** - `8403302` (test)

## Files Created/Modified
- `workflow/internal/inkparse/inject.go` - InjectTranslations function, InjectReport struct, injector tree walker
- `workflow/internal/inkparse/inject_test.go` - 11 test functions covering single/multi-node blocks, missing translations, BOM, structure preservation, round-trip

## Decisions Made
- Injector mirrors parser walkContainer/walkFlatContent structure exactly to ensure hash computation consistency
- First ^text node of a replaced block gets "^" + full Korean text; remaining nodes get "^" (empty) to preserve JSON node count
- Output always includes UTF-8 BOM to match original ink JSON file format
- Round-trip test uses ID set comparison (not index) due to map iteration order non-determinism

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed round-trip test map iteration order assumption**
- **Found during:** Task 2 (TestInjectRoundTrip)
- **Issue:** Test compared blocks by index, but Go map iteration order is non-deterministic
- **Fix:** Changed to set-based ID comparison instead of positional matching
- **Files modified:** workflow/internal/inkparse/inject_test.go
- **Verification:** Test passes consistently
- **Committed in:** 8403302 (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Test correctness fix. No scope creep.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- InjectTranslations ready for use by patch output CLI (03-03 plan)
- Function signature: `InjectTranslations(data []byte, sourceFile string, translations map[string]string) ([]byte, *InjectReport, error)`
- Provides InjectReport with Total/Replaced/Missing counts for progress tracking

---
*Phase: 03-patch-output-full-run*
*Completed: 2026-03-22*
