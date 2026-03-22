---
phase: 01-source-parser
plan: 02
subsystem: parser
tags: [ink-json, classifier, passthrough, batcher, content-type, tdd]

# Dependency graph
requires:
  - "01-01: inkparse package with DialogueBlock, ParseResult types"
provides:
  - "Content type classification (Classify) for 5 types: dialogue, spell, ui, item, system"
  - "Passthrough detection (IsPassthrough) for non-translatable strings"
  - "Content-type-aware batch builder (BuildBatches) with gate-boundary grouping"
  - "Batch struct with format metadata (script, card, dictionary, document)"
affects: [01-03-PLAN, phase-02-translation]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "File prefix-based content classification (TS_, AR_, TU_, etc.)"
    - "Gate boundary as dialogue cluster edge (D-22)"
    - "Small gate merging for optimal batch sizes"

key-files:
  created:
    - workflow/internal/inkparse/classifier.go
    - workflow/internal/inkparse/classifier_test.go
    - workflow/internal/inkparse/passthrough.go
    - workflow/internal/inkparse/passthrough_test.go
    - workflow/internal/inkparse/batcher.go
    - workflow/internal/inkparse/batcher_test.go
  modified:
    - workflow/internal/inkparse/types.go

key-decisions:
  - "D-10 resolved: file prefix is primary classifier signal (TS_/AR_/CB_/EP_ -> dialogue, TU_/TE_ -> system), with tag and structural signals as secondary"
  - "Adapted plan's filename patterns to actual game data (no *Spell*/*Item* files exist; used tag-based detection instead)"
  - "Small adjacent gates in same knot are merged to meet 10-block minimum batch size"

patterns-established:
  - "Classifier: file prefix -> tags -> structural signals -> default dialogue"
  - "Batcher: group by content type, split/merge by size limits per format"

requirements-completed: [PARSE-04, PARSE-05, PARSE-06]

# Metrics
duration: 6min
completed: 2026-03-22
---

# Phase 01 Plan 02: Content Classification and Batching Summary

**Content type classifier (5 types via file prefix + tag signals), passthrough detector (ink control/variables/templates), and gate-boundary-aware batch builder with format-specific size limits**

## Performance

- **Duration:** 6 min
- **Started:** 2026-03-22T06:58:51Z
- **Completed:** 2026-03-22T07:04:34Z
- **Tasks:** 2
- **Files created:** 6, modified: 1

## Accomplishments
- TDD-driven classifier assigns one of 5 content types based on file prefix patterns (28 known prefixes), tag metadata (spell/ability/OBJ), and structural signals (speaker, text length)
- Passthrough detector identifies non-translatable strings: ink control words (end/done/DONE), v1 control regex, variable refs ($var), template strings ({expr}), punctuation-only, empty
- Batch builder groups blocks by content type with gate boundaries as cluster edges: dialogue (script, 10-30), spell/item (card, 5-10), UI (dictionary, 50-100), system (document, full section)
- All 56 inkparse tests pass (Plan 01 + Plan 02 + Plan 03), go vet clean

## Task Commits

Each task was committed atomically:

1. **Task 1 RED: Failing tests** - `916a84f` (test)
2. **Task 1 GREEN: Classifier + passthrough implementation** - `084b18b` (feat)
3. **Task 2: Batch builder** - `438049c` (feat)

_TDD task had separate RED and GREEN commits._

## Files Created/Modified
- `workflow/internal/inkparse/types.go` - Added ContentType constants, ContentType and IsPassthrough fields to DialogueBlock
- `workflow/internal/inkparse/classifier.go` - Classify() with file prefix matching, tag detection, structural signal fallback
- `workflow/internal/inkparse/classifier_test.go` - 11 tests covering all 5 content types and edge cases
- `workflow/internal/inkparse/passthrough.go` - IsPassthrough() extending v1 patterns with ink control words, variables, templates
- `workflow/internal/inkparse/passthrough_test.go` - 13 tests for passthrough and non-passthrough cases
- `workflow/internal/inkparse/batcher.go` - BuildBatches() with Batch struct, gate grouping, size splitting/merging
- `workflow/internal/inkparse/batcher_test.go` - 8 tests for batch building, splitting, merging, format assignment, passthrough exclusion

## Decisions Made
- **D-10 resolved (file prefix as primary signal):** Actual game data has no files matching plan's suggested "*Spell*", "*Item*" patterns. Instead, all 286 TextAsset files use 2-3 letter prefixes (TS_, AR_, CB_, etc.). Classifier maps these prefixes to content types directly. Spell/item detection uses tag metadata (spell, OBJ tags) as secondary signal.
- **Small gate merging:** Adjacent gates in the same knot with <10 blocks each are merged to avoid undersized batches, improving translation context quality.
- **Removed parallel Plan 01-03 stubs:** classifier_stub.go and passthrough_stub.go were created by the parallel agent for compilation; replaced with real implementations.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Removed parallel execution stub files**
- **Found during:** Task 1 (GREEN phase)
- **Issue:** Plan 01-03 parallel agent created classifier_stub.go and passthrough_stub.go with duplicate constant/function declarations that prevented compilation
- **Fix:** Deleted stub files, placed real constants in types.go and implementations in classifier.go/passthrough.go
- **Files modified:** Removed classifier_stub.go, passthrough_stub.go
- **Committed in:** 084b18b

**2. [Rule 1 - Bug] Fixed validate.go compilation error from parallel agent**
- **Found during:** Task 1 (GREEN phase verification)
- **Issue:** Parallel Plan 01-03 agent renamed reClosingColor regex variable to reColorOpen but left a reference to the old name on line 70, causing build failure
- **Fix:** Waited for parallel agent to self-correct (they fixed it in a subsequent edit)
- **Files modified:** None (parallel agent's fix)
- **Committed in:** N/A (fixed by other agent)

---

**Total deviations:** 1 auto-fixed (1 blocking from parallel execution)
**Impact on plan:** Stub removal necessary for compilation. No scope creep.

## Issues Encountered
- Plan's content type file patterns (e.g., files containing "Spell", "Item", "Tutorial") did not match actual game data. All TextAsset files use short prefixes (TS_, AR_, CB_, TU_, etc.). Adapted classifier to use prefix-based matching instead, which is more reliable for this game's file naming convention.

## User Setup Required
None - no external service configuration required.

## Known Stubs
None - all functionality is fully implemented.

## Next Phase Readiness
- Classifier, passthrough detector, and batcher ready for Plan 03 (DB ingestion) and Phase 2 (translation)
- BuildBatches can be called on ParseResult output to produce translation-ready batches
- ContentType and IsPassthrough fields on DialogueBlock enable downstream filtering

## Self-Check: PASSED

All 7 created/modified files verified present. All 3 commit hashes verified in git log.

---
*Phase: 01-source-parser*
*Completed: 2026-03-22*
