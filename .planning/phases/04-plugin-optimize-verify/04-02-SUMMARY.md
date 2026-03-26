---
phase: 04-plugin-optimize-verify
plan: 02
subsystem: plugin
tags: [csharp, bepinex, tryTranslate, textasset]

requires:
  - phase: 03-patch-output-full-run
    provides: Plugin.cs with 8-stage TryTranslate and TextAsset loading
provides:
  - 4-stage TryTranslate chain (GeneratedPattern, TranslationMap, Contextual, RuntimeLexicon)
  - .json TextAsset loading with v2 precedence
affects: [04-03-deploy-verify]

tech-stack:
  added: []
  patterns: [simplified-matching-chain, dual-pattern-textasset]

key-files:
  created: []
  modified:
    - projects/esoteric-ebb/patch/mod-loader/EsotericEbb.TranslationLoader/Plugin.cs

key-decisions:
  - "NormalizedMap dictionary kept for diagnostics (CaptureUntranslated, state dump) but removed from matching chain"
  - "*.json processed after *.txt in LoadTextAssetOverrides so v2 format takes precedence"

patterns-established:
  - "4-stage matching: GeneratedPattern → TranslationMap → Contextual → RuntimeLexicon"

requirements-completed: [PLUGIN-02, PLUGIN-03]

duration: 8min
completed: 2026-03-26
---

# Plan 04-02: Plugin.cs Simplification Summary

**TryTranslate reduced from 8 to 4 stages, removed Decorated/Embedded/TagSeparatedSegments methods, added .json TextAsset loading**

## Performance

- **Duration:** ~8 min
- **Tasks:** 2
- **Files modified:** 1

## Accomplishments
- TryTranslate chain simplified to 4 stages: GeneratedPattern, TranslationMap, Contextual, RuntimeLexicon
- Removed 4 matching methods and their helper classes (~300 lines deleted)
- LoadTextAssetOverrides now scans both *.txt and *.json files with v2 precedence

## Task Commits

1. **Task 1: Simplify TryTranslate chain to 4 stages** - `d169145` (refactor)
2. **Task 2: Add .json pattern to LoadTextAssetOverrides** - `134fcbd` (feat)

## Files Created/Modified
- `projects/esoteric-ebb/patch/mod-loader/EsotericEbb.TranslationLoader/Plugin.cs` - 4-stage TryTranslate, removed methods, dual-pattern TextAsset loading

## Decisions Made
- NormalizedMap dictionary retained for diagnostic purposes only (removed from chain)
- NormalizeKey function retained as it's used by TryTranslateContextual

## Deviations from Plan
None - plan executed as specified

## Issues Encountered
- Plugin.cs path is in .gitignore; required `git add -f` to commit

## Next Phase Readiness
- Plugin.cs ready for deployment with simplified matching chain
- Combined with 04-01 contextual_entries, the v2 patch is complete for testing

---
*Phase: 04-plugin-optimize-verify*
*Completed: 2026-03-26*
