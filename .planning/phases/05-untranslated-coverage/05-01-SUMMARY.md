---
phase: 05-untranslated-coverage
plan: 01
subsystem: plugin
tags: [csharp, bepinex, regex, rendering-wrapper, trytranslate]

# Dependency graph
requires:
  - phase: 04.2-source-cleanup-reexport
    provides: "3-stage TryTranslate chain, clean DB sources without DC/FC prefixes"
provides:
  - "RenderingWrapper struct with strip/rewrap for color, noparse, inline tags"
  - "StripRenderingWrapper method called at TryTranslate entry"
  - "TryTranslateCore extracted method for 3-stage chain"
  - "D-08 fix: miss capture records stripped inner text, not wrapped original"
affects: [05-02, 05-03, plugin-verification]

# Tech tracking
tech-stack:
  added: []
  patterns: ["strip-translate-rewrap pre-processing at TryTranslate entry"]

key-files:
  created: []
  modified:
    - "projects/esoteric-ebb/patch/mod-loader/EsotericEbb.TranslationLoader/Plugin.cs"

key-decisions:
  - "TryTranslateCore extracted as separate method to avoid duplicating 3-stage chain for wrapper/non-wrapper paths"
  - "Inline tags stripped but NOT re-wrapped (game adds them fresh at render time)"

patterns-established:
  - "Wrapper pre-processing: StripRenderingWrapper at TryTranslate entry, before any stage"
  - "Miss capture uses stripped text for accurate untranslated tracking"

requirements-completed: [PLUGIN-03]

# Metrics
duration: 2min
completed: 2026-03-29
---

# Phase 5 Plan 1: Rendering Wrapper Strip Summary

**StripRenderingWrapper pre-processing in TryTranslate for color/noparse/inline tag wrappers, resolving 136 wrapper mismatches (D-01, D-06) and 341 capture false positives (D-08)**

## Performance

- **Duration:** 2 min
- **Started:** 2026-03-29T03:46:09Z
- **Completed:** 2026-03-29T03:48:14Z
- **Tasks:** 2
- **Files modified:** 1

## Accomplishments
- Added 3 compiled regex patterns (NoparseEmptyRegex, ColorWrapperRegex, InlineTagRegex) for runtime rendering wrapper detection
- Added RenderingWrapper readonly struct with Rewrap method that reverses strip order (color first, noparse last)
- Integrated StripRenderingWrapper call at TryTranslate entry point, before any translation stage
- Extracted TryTranslateCore to avoid code duplication between wrapper and non-wrapper paths
- Fixed D-08: miss capture now records stripped inner text instead of wrapped original (eliminates 341 false positives)

## Task Commits

Each task was committed atomically:

1. **Task 1: Add RenderingWrapper struct and StripRenderingWrapper method** - `e2b8c90` (feat)
2. **Task 2: Integrate wrapper strip into TryTranslate and fix capture logic** - `e044619` (feat)

## Files Created/Modified
- `projects/esoteric-ebb/patch/mod-loader/EsotericEbb.TranslationLoader/Plugin.cs` - Added 3 regex fields, RenderingWrapper struct, StripRenderingWrapper method, refactored TryTranslate with TryTranslateCore extraction

## Decisions Made
- Extracted TryTranslateCore as a private static method rather than inlining the 3-stage chain twice, keeping the code DRY
- Inline tags (i, b, size) are stripped but NOT re-wrapped in the RenderingWrapper struct because the game engine adds them fresh at render time

## Deviations from Plan
None - plan executed exactly as written.

## Issues Encountered
- Plugin.cs is in .gitignore (patch directory excluded); used `git add -f` to force-track the file. This is consistent with prior phases.

## Known Stubs
None - all code is fully functional, no placeholders.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Plugin.cs wrapper strip is ready; game verification will occur in Plan 03
- Plans 02 (runtime_lexicon rules) and 03 (patch build + in-game verify) can proceed

---
*Phase: 05-untranslated-coverage*
*Completed: 2026-03-29*
