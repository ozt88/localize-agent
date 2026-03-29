---
phase: 05-untranslated-coverage
plan: 02
subsystem: localization
tags: [runtime-lexicon, ui-labels, passthrough, regex, game-translation]

# Dependency graph
requires:
  - phase: 04.2-source-cleanup-reexport
    provides: Clean DB sources, 3-stage TryTranslate, runtime_lexicon.json v2 format
provides:
  - Expanded runtime_lexicon.json with 281 rules (218 exact, 16 substring, 47 regex)
  - UI settings labels translated (Ambient Volume, Fullscreen, Resolution, etc.)
  - Game mechanics terms translated (Hit Dice, Inventory, Spellbook, etc.)
  - Passthrough rules for proper nouns, numbers, dice, percentages
  - Regex rules for templates (Level X reached, Day N, VSYNC, dice notation)
affects: [05-01-PLAN, 05-03-PLAN, plugin-verification]

# Tech tracking
tech-stack:
  added: []
  patterns: [lexicon-passthrough-pattern, regex-template-pattern]

key-files:
  created: []
  modified:
    - E:/SteamLibrary/steamapps/common/Esoteric Ebb/Esoteric Ebb_Data/StreamingAssets/TranslationPatch/runtime_lexicon.json

key-decisions:
  - "Passthrough proper nouns (Cleric, Tolstad, etc.) use find==replace exact rules per D-02/D-03"
  - "All numeric/dice/percentage entries covered by both exact rules (observed values) and regex fallbacks (patterns)"
  - "Template strings (Level X reached, Day N, VSYNC) use regex with Korean translations"
  - "Font names (Averia Serif, Roboto) treated as passthrough"

patterns-established:
  - "Passthrough pattern: exact_replacement with find==replace suppresses untranslated capture"
  - "Template regex: named rules with capture groups for variable parts"

requirements-completed: [PLUGIN-03]

# Metrics
duration: 4min
completed: 2026-03-29
---

# Phase 05 Plan 02: Runtime Lexicon Expansion Summary

**Expanded runtime_lexicon.json from 42 to 281 rules covering UI labels, game mechanics, passthrough proper nouns/numbers, and template regex patterns**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-29T03:47:03Z
- **Completed:** 2026-03-29T03:51:00Z
- **Tasks:** 1
- **Files modified:** 1

## Accomplishments
- Expanded runtime_lexicon.json from 42 to 281 total rules (569% increase)
- Added 210 new exact_replacements: ~48 UI settings translations, ~42 game mechanics translations, ~55 passthrough proper nouns (places, classes, NPCs), ~65 passthrough numbers/dice/percentages
- Added 29 new regex_rules: level_reached, day_number, round_number, cast_spell, dice_notation, percentage, resolution, vsync_mode, pure_number, signed_number, xp_fraction, and more
- All 42 original rules preserved intact (8 exact, 16 substring, 18 regex)
- Zero duplicate find values across all exact_replacements

## Task Commits

Each task was committed atomically:

1. **Task 1: Analyze untranslated_capture.json and classify all 497 items into lexicon rules** - deployed to game directory (outside git repo)

Note: runtime_lexicon.json lives in the game's TranslationPatch directory (E: drive), which is outside the git repository. The repo's `projects/esoteric-ebb/patch/` directory is gitignored. The file is deployed directly to the game installation.

## Files Created/Modified
- `E:/.../TranslationPatch/runtime_lexicon.json` - Expanded from 42 to 281 rules (218 exact, 16 substring, 47 regex)

## Decisions Made
- Passthrough proper nouns (Cleric, Tolstad, Goblin Garden, etc.) use find==replace exact rules per D-02/D-03 decisions
- All numeric/dice/percentage entries covered by exact rules for observed values AND regex fallbacks for patterns
- Font names (Averia Serif, Roboto) treated as passthrough since they're font identifiers
- Template strings like "Level X Cleric" and "Level X Mercenary" use regex passthrough (keep English per D-03)
- Game dates (22nd Gorgoni, 10 PE; March 22nd, 27 PE) use ordinal_date regex passthrough
- "DC" prefix and bracket patterns ([DC 15]) get passthrough regex rules
- Short strings ("is", "vs", "Lv", "DC", "NAT 20") treated as passthrough

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] File outside git repository**
- **Found during:** Task 1 commit
- **Issue:** runtime_lexicon.json is in E:/SteamLibrary/... (game directory), not in the git repo. The repo's patch/ directory is gitignored.
- **Fix:** Deployed file directly to game directory. Also copied to projects/esoteric-ebb/patch/input/runtime_lexicon.json (gitignored). Commit will be docs-only with SUMMARY.md.
- **Impact:** No code commit for the actual file change, but the deployed artifact is correct.

**2. [Rule 2 - Missing Critical] Added regex fallback patterns for numeric/template categories**
- **Found during:** Task 1 analysis
- **Issue:** Plan listed specific numeric values (1%, 10%, etc.) as exact rules, but game can produce any percentage. Exact rules alone would miss unseen values.
- **Fix:** Added regex fallbacks: `percentage` (^\d+%$), `pure_number` (^\d{1,5}$), `signed_number` (^[+-]\d+$), `fraction` (^\d+/\d+$), `xp_fraction`, `dc_number`, `dc_template`, `dc_bracket`, `ordinal_date`, `exh_count`, `ordinal_suffix`, `xx_pipe_separated`, `date_template`
- **Verification:** Both exact and regex rules verified present

---

**Total deviations:** 2 (1 blocking, 1 missing critical)
**Impact on plan:** Both necessary for correctness. No scope creep.

## Issues Encountered
- Windows cp949 encoding issue when printing Korean characters to stdout - solved with PYTHONIOENCODING=utf-8

## Known Stubs
None - all rules contain actual translations or explicit passthrough values.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Lexicon expansion complete, ready for in-game verification
- Plan 01 (Plugin.cs wrapper strip) and Plan 03 (verification) can proceed
- Remaining untranslated items are: rendering-wrapped (Plan 01), inline-tagged dialogue (Plan 01), and multiline text (deferred per D-07)

---
*Phase: 05-untranslated-coverage*
*Completed: 2026-03-29*
