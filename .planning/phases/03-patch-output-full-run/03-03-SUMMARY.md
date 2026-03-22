---
phase: 03-patch-output-full-run
plan: 03
subsystem: pipeline
tags: [go, cli, export, csv, textasset, translations-json, bom, ink-json, v3-sidecar]

requires:
  - phase: 03-patch-output-full-run
    provides: V3Sidecar export (03-01), InjectTranslations ink JSON injection (03-02)
provides:
  - go-v2-export CLI generating translations.json + TextAsset .json + localizationtexts CSV
  - CSV domain logic (ReadCSVFile, WriteCSVFile, TranslateCSVRows) with BOM handling
  - Coverage/fail-rate threshold checks for pipeline quality gates
affects: [04-plugin-verify, patch-build]

tech-stack:
  added: []
  patterns: [csv-bom-roundtrip, export-cli-pattern, coverage-gate]

key-files:
  created:
    - workflow/cmd/go-v2-export/main.go
    - workflow/internal/v2pipeline/csvexport.go
    - workflow/internal/v2pipeline/csvexport_test.go
  modified: []

key-decisions:
  - "TextAsset output uses .json extension (D-06); Plugin.cs update deferred to Phase 4 PLUGIN-01"
  - "CSV translation uses high_llm profile for quality parity with main pipeline"
  - "Fail rate thresholds: >5% warning, >20% abort (D-08 discretion)"
  - "min-coverage 0 disables coverage check for partial export (D-10)"

patterns-established:
  - "Export CLI pattern: flag parse -> OpenStore -> coverage check -> generate artifacts"
  - "CSV BOM roundtrip: strip on read, prepend on write for Windows game file compatibility"

requirements-completed: [PATCH-02, PATCH-03, VERIFY-01]

duration: 4min
completed: 2026-03-22
---

# Phase 03 Plan 03: Export CLI + CSV Domain + Full Run Summary

**go-v2-export CLI producing translations.json v3, TextAsset .json ink injection, and localizationtexts CSV translation with BOM roundtrip and pipeline quality gates**

## Performance

- **Duration:** 4 min (multi-session with checkpoint approval)
- **Started:** 2026-03-22T18:22:17Z
- **Completed:** 2026-03-22T18:26:20Z
- **Tasks:** 4 (2 auto + 2 checkpoint:human-verify)
- **Files modified:** 3

## Accomplishments
- go-v2-export CLI consolidates all patch artifact generation: translations.json, TextAsset .json files, localizationtexts CSV
- CSV domain logic with BOM-aware read/write and full re-translation support (D-11)
- Pipeline quality gates: coverage threshold (--min-coverage) and fail rate abort (>20%)
- User approved pipeline full run (VERIFY-01) and export verification checkpoints

## Task Commits

Each task was committed atomically:

1. **Task 2 RED: CSV translation domain logic - failing tests** - `6717c25` (test)
2. **Task 2 GREEN: CSV translation domain logic - implementation** - `0fc429c` (feat)
3. **Task 1: go-v2-export CLI** - `bc9402d` (feat)
4. **Task 3: v2 pipeline full run (VERIFY-01)** - checkpoint:human-verify (approved)
5. **Task 4: Export verification** - checkpoint:human-verify (approved)

**Plan metadata:** (pending)

_Note: Tasks 1-2 executed out of order (TDD task 2 first, then CLI task 1). Tasks 3-4 are user verification checkpoints with no code commits._

## Files Created/Modified
- `workflow/cmd/go-v2-export/main.go` - Export CLI entry point: flags, coverage check, translations.json + TextAsset injection + CSV translation
- `workflow/internal/v2pipeline/csvexport.go` - CSV domain: ReadCSVFile (BOM strip), WriteCSVFile (BOM prepend), TranslateCSVRows (overwrite KOREAN column)
- `workflow/internal/v2pipeline/csvexport_test.go` - 7 tests: read/write BOM, translate rows, overwrite, errors, short columns

## Decisions Made
- TextAsset output uses `.json` extension per D-06; Phase 4 PLUGIN-01 will update Plugin.cs to scan `*.json`
- CSV translation uses `high_llm` profile from project.json for quality parity
- Fail rate thresholds at Claude's discretion (D-08): >5% warning, >20% abort
- `--min-coverage 0` disables coverage check, enabling partial export (D-10)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All Phase 03 plans complete: patch export domain (03-01), ink injection (03-02), export CLI + CSV + verification (03-03)
- Phase 04 can proceed: Plugin.cs needs update to scan `*.json` TextAsset files (PLUGIN-01)
- Full pipeline run approved by user; export CLI verified against real data

## Self-Check: PASSED

- All 3 created files verified on disk
- All 3 task commits verified in git log (6717c25, 0fc429c, bc9402d)

---
*Phase: 03-patch-output-full-run*
*Completed: 2026-03-22*
