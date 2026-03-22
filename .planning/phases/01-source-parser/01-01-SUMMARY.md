---
phase: 01-source-parser
plan: 01
subsystem: parser
tags: [ink-json, sha256, tdd, dialogue-blocks, tree-walker]

# Dependency graph
requires: []
provides:
  - "inkparse package with Parse/ParseFile functions for ink JSON tree walking"
  - "DialogueBlock and ParseResult types for downstream pipeline use"
  - "SHA-256 source hashing (SourceHash function)"
  - "CLI go-ink-parse for batch parsing 286 TextAsset files"
affects: [01-02-PLAN, 01-03-PLAN, phase-02-translation]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Recursive container walker for ink JSON tree traversal"
    - "Path-based block IDs: KnotName/gate/choice/blk-N"
    - "TDD with inline JSON fixtures matching real ink structure"

key-files:
  created:
    - workflow/internal/inkparse/types.go
    - workflow/internal/inkparse/parser.go
    - workflow/internal/inkparse/parser_test.go
    - workflow/internal/inkparse/hash.go
    - workflow/internal/inkparse/glue.go
    - workflow/cmd/go-ink-parse/main.go
  modified: []

key-decisions:
  - "D-02 resolved: path-based block IDs (KnotName/gate/choice/blk-N) for human readability"
  - "D-04 resolved: parser in workflow/internal/inkparse (shared, reusable for other ink games)"
  - "Speaker tags (wis, str, int, con, dex, cha, reply) classified separately from check tags (DC/FC)"
  - "Within-container glue handled inline; cross-divert glue deferred (only 34 occurrences in 10 files)"

patterns-established:
  - "inkparse package follows project convention: types.go + feature.go + feature_test.go"
  - "CLI pattern: flag parsing -> domain logic, no LoadProjectConfig dependency for parser"
  - "UTF-8 BOM stripping for TextAsset files (existing pattern from go-esoteric-adapt-in)"

requirements-completed: [PREP-01, PARSE-01, PARSE-02, PARSE-03]

# Metrics
duration: 10min
completed: 2026-03-22
---

# Phase 01 Plan 01: Core Ink Parser Summary

**Recursive ink JSON tree walker producing 40,067 dialogue blocks from 286 TextAsset files with SHA-256 hashing, branch structure preservation, and speaker/tag metadata**

## Performance

- **Duration:** 10 min
- **Started:** 2026-03-22T06:46:53Z
- **Completed:** 2026-03-22T06:56:29Z
- **Tasks:** 2
- **Files created:** 6

## Accomplishments
- TDD-driven ink JSON parser that correctly merges consecutive ^text entries into dialogue blocks
- Branch structure preserved: knot/gate(g-N)/choice(c-N) paths with named hub support
- Speaker and metadata tags (OBJ, DC_check, XPGain, etc.) attached to blocks
- CLI parses all 286 TextAsset files: 40,067 blocks extracted with zero parse errors
- SHA-256 source hashing for every block (64-char hex, deterministic)

## Task Commits

Each task was committed atomically:

1. **Task 1 RED: Failing tests** - `d38a793` (test)
2. **Task 1 GREEN: Parser implementation** - `3ad7f7e` (feat)
3. **Task 2: CLI entry point** - `daf3f1f` (feat)

_TDD task had separate RED and GREEN commits._

## Files Created/Modified
- `workflow/internal/inkparse/types.go` - DialogueBlock, ParseResult, BlockMeta type definitions
- `workflow/internal/inkparse/parser.go` - Core recursive container walker, flat content extractor, choice text parser
- `workflow/internal/inkparse/parser_test.go` - 19 TDD tests: hash, block merge, branch structure, metadata, glue, choices, real file
- `workflow/internal/inkparse/hash.go` - SHA-256 source hash function
- `workflow/internal/inkparse/glue.go` - Glue handling documentation and design notes
- `workflow/cmd/go-ink-parse/main.go` - CLI with -single, -assets-dir, -output flags

## Decisions Made
- **Block IDs are path-based** (D-02): `KnotName/gate/choice/blk-N` for human readability and deterministic ordering
- **Parser in workflow/internal/** (D-04): shared location for potential reuse with other ink-based games
- **Speaker classification**: tags like `wis`, `str`, `int`, `con`, `dex`, `cha`, `reply` are speaker-role tags; `DC*`, `FC*`, `OBJ`, `XPGain` are metadata tags
- **Cross-divert glue deferred**: only 34 occurrences in 10 files; within-container glue handled inline
- **"s" key filtering**: choice start-content ("s") sub-containers excluded from named container recursion to prevent duplicate block extraction

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] UTF-8 BOM handling**
- **Found during:** Task 1 (real file integration test)
- **Issue:** TextAsset files have UTF-8 BOM (0xEF 0xBB 0xBF), causing JSON unmarshal to fail
- **Fix:** Added `bytes.TrimPrefix(data, utf8BOM)` at Parse entry point
- **Files modified:** workflow/internal/inkparse/parser.go
- **Committed in:** 3ad7f7e

**2. [Rule 1 - Bug] Internal "s" key treated as named container**
- **Found during:** Task 2 (CLI real-data verification)
- **Issue:** Choice start-content dict key "s" was walked as a named container, producing 14 spurious blocks per choice-heavy file
- **Fix:** Added `isInternalKey()` filter excluding "s", "$" vars, and "^" refs from metadata container recursion
- **Files modified:** workflow/internal/inkparse/parser.go
- **Committed in:** daf3f1f

---

**Total deviations:** 2 auto-fixed (2 bugs)
**Impact on plan:** Both fixes essential for correctness. No scope creep.

## Issues Encountered
- Initial parser design (separate walkKnotContainer/walkGateContainer/walkMetaSubContainers) was too complex for ink's deeply nested structure. Rewrote to single recursive `walkContainer` approach that handles arbitrary nesting depth. This was within the REFACTOR step of TDD.

## User Setup Required
None - no external service configuration required.

## Known Stubs
None - all functionality is fully implemented.

## Next Phase Readiness
- inkparse package ready for Plan 02 (content type classification) and Plan 03 (DB ingestion)
- Parse output (40,067 blocks) can be piped to downstream tools via CLI JSON output
- Block structure (knot/gate/choice paths) enables scene-level clustering for translation

## Self-Check: PASSED

All 6 created files verified present. All 3 commit hashes verified in git log.

---
*Phase: 01-source-parser*
*Completed: 2026-03-22*
