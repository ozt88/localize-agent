---
phase: 02-translation-engine
plan: 03
subsystem: translation
tags: [tagformat, scorellm, codex-spark, tag-restoration, quality-gating]

requires:
  - phase: 02-01
    provides: "V2PipelineItem struct and pipeline state constants"
provides:
  - "Tag extraction, detection, stripping (ExtractTags, HasRichTags, StripTags, CountTags)"
  - "Tag validation via frequency map comparison (ValidateTagMatch, order-independent per D-07)"
  - "Formatter prompt for codex-spark (BuildFormatWarmup, BuildFormatPrompt, ParseFormatResponse)"
  - "Score LLM types and routing (ScoreResult.TargetState per D-14)"
  - "Score LLM response parser (ParseScoreResponse with code fence handling)"
affects: [02-04, 03-pipeline-orchestration]

tech-stack:
  added: []
  patterns: [frequency-map-tag-validation, code-fence-json-extraction, html-safe-json-encoding]

key-files:
  created:
    - workflow/internal/tagformat/types.go
    - workflow/internal/tagformat/tags.go
    - workflow/internal/tagformat/tags_test.go
    - workflow/internal/tagformat/validate.go
    - workflow/internal/tagformat/validate_test.go
    - workflow/internal/tagformat/prompt.go
    - workflow/internal/tagformat/prompt_test.go
    - workflow/internal/scorellm/types.go
    - workflow/internal/scorellm/prompt.go
    - workflow/internal/scorellm/parser.go
    - workflow/internal/scorellm/parser_test.go
  modified: []

key-decisions:
  - "Tag order ignored in validation per D-07 -- frequency map comparison, not positional"
  - "JSON encoding uses SetEscapeHTML(false) to preserve raw tags in prompt output"
  - "Code fence extraction shared pattern between tagformat and scorellm parsers"

patterns-established:
  - "Frequency map validation: build map[string]int for EN and KO, compare counts per unique tag"
  - "Code fence JSON extraction: regex to unwrap ```json ... ``` before parsing"
  - "LLM prompt separation: warmup (system) + prompt (user) as separate functions"

requirements-completed: [TRANS-05, TRANS-06, TRANS-08]

duration: 5min
completed: 2026-03-22
---

# Phase 02 Plan 03: Tag Format + Score LLM Summary

**Tag extraction with frequency-map validation (7 tag types, order-independent per D-07), codex-spark formatter prompt with EN+KO pairs, and Score LLM parser with failure_type routing to pipeline states**

## Performance

- **Duration:** 5 min
- **Started:** 2026-03-22T15:50:19Z
- **Completed:** 2026-03-22T15:55:16Z
- **Tasks:** 2
- **Files modified:** 11

## Accomplishments
- Tag extraction handles all 7 rich-text tag types (i, b, shake, wiggle, u, size=N, s) via single regex
- ValidateTagMatch uses frequency map comparison -- order ignored per D-07, exact count+string match required
- Formatter prompt sends EN+KO pairs as JSON per D-05, with HTML-safe encoding preserving raw tags
- Score LLM parser validates failure_type routing (pass/translation/format/both) per D-14
- Both parsers handle markdown code fence wrapping (Pitfall 5)

## Task Commits

Each task was committed atomically:

1. **Task 1: Tag extraction, detection, and exact-match validation** - `80b5551` (feat)
2. **Task 2: Formatter prompt and Score LLM response parser** - `ec3d442` (feat)

_Note: TDD tasks -- RED/GREEN phases executed for each task_

## Files Created/Modified
- `workflow/internal/tagformat/types.go` - FormatTask, FormatResult, TagValidationError types
- `workflow/internal/tagformat/tags.go` - ExtractTags, HasRichTags, StripTags, CountTags
- `workflow/internal/tagformat/tags_test.go` - 8 tests covering all tag types and edge cases
- `workflow/internal/tagformat/validate.go` - ValidateTagMatch with frequency map comparison
- `workflow/internal/tagformat/validate_test.go` - 6 tests: pass, count mismatch, reorder, missing, attribute mismatch
- `workflow/internal/tagformat/prompt.go` - BuildFormatWarmup, BuildFormatPrompt, ParseFormatResponse
- `workflow/internal/tagformat/prompt_test.go` - 7 tests: warmup, single/multi prompt, parse valid/invalid/code fence
- `workflow/internal/scorellm/types.go` - ScoreTask, ScoreResult with TargetState routing
- `workflow/internal/scorellm/prompt.go` - BuildScoreWarmup, BuildScorePrompt
- `workflow/internal/scorellm/parser.go` - ParseScoreResponse with validation and code fence handling
- `workflow/internal/scorellm/parser_test.go` - 8 tests: valid, failure, invalid JSON, missing fields, out of range, code fence, TargetState

## Decisions Made
- Tag order ignored in validation per D-07 update -- Korean word order differs from English, so tag position naturally changes. Semantic correctness delegated to Score LLM (D-14).
- Used `json.NewEncoder` with `SetEscapeHTML(false)` to preserve raw `<b>`, `<i>` tags in formatter prompt JSON output (Go's default `json.Marshal` escapes angle brackets).
- Code fence extraction pattern (```` ```json ... ``` ````) shared between tagformat and scorellm parsers for robust LLM output handling.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] ValidateTagMatch count-only short-circuit removed**
- **Found during:** Task 1 (tag validation)
- **Issue:** Quick count check returned generic "count mismatch" error without listing which specific tags were missing/extra
- **Fix:** Removed early return on count mismatch; always compute frequency maps for detailed error messages including specific missing/extra tags
- **Files modified:** workflow/internal/tagformat/validate.go
- **Verification:** TestValidateTagMatch_MissingTag now correctly reports `<i>` and `</i>` as missing
- **Committed in:** 80b5551

**2. [Rule 1 - Bug] JSON HTML entity escaping in BuildFormatPrompt**
- **Found during:** Task 2 (formatter prompt)
- **Issue:** `json.Marshal` encodes `<` and `>` as `\u003c` and `\u003e`, breaking tag content in LLM prompts
- **Fix:** Switched to `json.NewEncoder` with `SetEscapeHTML(false)`
- **Files modified:** workflow/internal/tagformat/prompt.go
- **Verification:** TestBuildFormatPrompt_Single now finds raw `<b>Watch</b>` in output
- **Committed in:** ec3d442

---

**Total deviations:** 2 auto-fixed (2 bugs)
**Impact on plan:** Both auto-fixes necessary for correctness. No scope creep.

## Issues Encountered
None beyond the auto-fixed items above.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- tagformat and scorellm packages ready for integration in pipeline orchestrator (Plan 04)
- Formatter prompt tested for codex-spark tag restoration workflow
- Score LLM routing tested for all 4 failure_type values
- ValidateTagMatch ready to gate Stage 2 output before scoring

## Self-Check: PASSED

- All 11 files FOUND
- Commit 80b5551 FOUND
- Commit ec3d442 FOUND

---
*Phase: 02-translation-engine*
*Completed: 2026-03-22*
