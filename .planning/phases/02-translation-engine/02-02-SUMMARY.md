---
phase: 02-translation-engine
plan: 02
subsystem: translation
tags: [glossary, prompt-builder, parser, validator, cluster-translate, tdd, numbered-line]

requires:
  - phase: 01-source-parser
    provides: "DialogueBlock, Batch, ContentType constants -- consumed by clustertranslate prompt builder"
  - phase: 02-translation-engine
    plan: 01
    provides: "V2PipelineStore, pipeline_items_v2 -- downstream consumer of translated output"
provides:
  - "glossary.LoadGlossary -- unified glossary from 3 sources with dedup"
  - "glossary.WarmupTerms, FilterForBatch, FormatJSON -- per-batch glossary injection"
  - "clustertranslate.BuildBaseWarmup, BuildScriptPrompt -- prompt construction"
  - "clustertranslate.ParseNumberedOutput, MapLinesToIDs -- output parsing"
  - "clustertranslate.ValidateTranslation -- line count + degenerate validation"
affects: [02-03, 02-04]

tech-stack:
  added: []
  patterns:
    - "Numbered-line scene script format: [NN] Speaker: \"text\" with [CHOICE] and [CONTEXT] blocks"
    - "Glossary cascade: GlossaryTerms.txt + localizationtexts CSVs + speaker names -> deduplicated GlossarySet"
    - "Term extraction: text before ' - ' separator in CSV ENGLISH column"
    - "BOM-stripping CSV reader for Windows-originated game files"

key-files:
  created:
    - workflow/internal/glossary/types.go
    - workflow/internal/glossary/loader.go
    - workflow/internal/glossary/loader_test.go
    - workflow/internal/clustertranslate/types.go
    - workflow/internal/clustertranslate/prompt.go
    - workflow/internal/clustertranslate/prompt_test.go
    - workflow/internal/clustertranslate/parser.go
    - workflow/internal/clustertranslate/parser_test.go
    - workflow/internal/clustertranslate/validate.go
    - workflow/internal/clustertranslate/validate_test.go
  modified: []

key-decisions:
  - "Localization texts use comma-delimited CSV (ID,ENGLISH,KOREAN) not pipe/tab -- detected from actual files"
  - "BOM stripping via io.MultiReader pattern for Windows UTF-8 BOM in game CSVs"
  - "Punctuation-only check duplicated locally in clustertranslate (avoids cross-package import of translation package)"
  - "Speaker regex restricted to ASCII names (game uses English character names only)"

patterns-established:
  - "glossary.GlossarySet: deduplicated term store with case-insensitive index and warmup/filter/format API"
  - "clustertranslate prompt format: [NN] Speaker: \"text\" with [CONTEXT] preamble and content-type suffix"
  - "clustertranslate parser: regex-based [NN] extraction, handles quoted/unquoted, speaker/choice variants"

requirements-completed: [TRANS-01, TRANS-02, TRANS-03, TRANS-04, TRANS-07]

duration: 6min
completed: 2026-03-22
---

# Phase 02 Plan 02: Glossary & Cluster Translation Domain Summary

**Glossary loader from 3 game sources with warmup/filter API, numbered-line scene script prompt builder, regex parser with line-to-ID mapping, and line count + degenerate validation**

## Performance

- **Duration:** 6 min
- **Started:** 2026-03-22T15:50:31Z
- **Completed:** 2026-03-22T15:56:05Z
- **Tasks:** 3
- **Files created:** 10

## Accomplishments
- Glossary package loading from GlossaryTerms.txt (85+ game terms), localizationtexts (8 CSV files), and speaker names with case-insensitive dedup
- Cluster translation prompt builder producing numbered-line scene scripts with speaker labels (D-01), choice markers (D-02), previous gate context (D-03), content-type suffixes (D-04), and per-batch glossary (D-11)
- Translation output parser with [NN] regex extraction, speaker/choice metadata, and block ID mapping (TRANS-03)
- Translation validator enforcing line count match (TRANS-04) and degenerate output rejection (D-13)
- 31 total tests across both packages (9 glossary + 22 clustertranslate), all passing

## Task Commits

Each task was committed atomically:

1. **Task 1: Glossary loader from 3 sources** - `263677c` (feat)
2. **Task 2: Cluster translation prompt builder** - `a1ea908` (feat)
3. **Task 3: Translation output parser and validator** - `7086e1e` (feat)

## Files Created/Modified
- `workflow/internal/glossary/types.go` - Term and GlossarySet type definitions
- `workflow/internal/glossary/loader.go` - LoadGlossary, LoadGlossaryTerms, LoadLocalizationTexts, LoadSpeakers, WarmupTerms, FilterForBatch, FormatJSON
- `workflow/internal/glossary/loader_test.go` - 9 tests for all glossary functions
- `workflow/internal/clustertranslate/types.go` - ClusterTask, ClusterResult, TranslatedLine, PromptMeta
- `workflow/internal/clustertranslate/prompt.go` - BuildBaseWarmup, BuildScriptPrompt, BuildContentSuffix
- `workflow/internal/clustertranslate/prompt_test.go` - 11 tests for prompt builder
- `workflow/internal/clustertranslate/parser.go` - ParseNumberedOutput, MapLinesToIDs
- `workflow/internal/clustertranslate/parser_test.go` - 6 tests for parser and mapping
- `workflow/internal/clustertranslate/validate.go` - ValidateTranslation, ValidateLineCount
- `workflow/internal/clustertranslate/validate_test.go` - 5 tests for validation

## Decisions Made
- Detected actual CSV format of localizationtexts files: comma-delimited (ID,ENGLISH,KOREAN), not pipe or tab
- Added BOM stripping via io.MultiReader to handle Windows UTF-8 BOM in game-exported CSV files
- Duplicated punctuation-only check in clustertranslate package to avoid importing translation package (separate concern)
- Speaker regex limited to ASCII character names since game uses English-only character names

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Glossary package ready for integration with cluster translation worker (Plan 03)
- Prompt builder ready to construct LLM requests from Batch + glossary input
- Parser and validator ready to process and validate LLM responses
- All types (ClusterTask, ClusterResult, PromptMeta) ready for pipeline orchestration

## Self-Check: PASSED

All 10 created files verified present. All 3 task commits verified in git log.

---
*Phase: 02-translation-engine*
*Completed: 2026-03-22*
