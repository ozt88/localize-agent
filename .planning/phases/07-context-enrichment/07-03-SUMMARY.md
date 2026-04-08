---
phase: 07-context-enrichment
plan: 03
subsystem: clustertranslate, v2pipeline
tags: [prompt-injection, voice-card, branch-context, continuity-window, token-budget, tdd]
dependency_graph:
  requires:
    - phase: 07-context-enrichment
      plan: 01
      provides: VoiceCard type, LoadVoiceCards, BuildNamedVoiceSection
    - phase: 07-context-enrichment
      plan: 02
      provides: ParentChoiceText, GetNextLines, GetAdjacentKO, ClusterTask extensions
  provides: [extended-BuildScriptPrompt, context-budget-trimming, worker-context-assembly, voice-cards-cli]
  affects: [translation-quality, retranslation-pipeline]
tech_stack:
  added: []
  patterns: [token-budget-trimming, context-priority-shedding, tdd-red-green]
key_files:
  created: []
  modified:
    - workflow/internal/clustertranslate/prompt.go
    - workflow/internal/clustertranslate/prompt_test.go
    - workflow/internal/v2pipeline/types.go
    - workflow/internal/v2pipeline/worker.go
    - workflow/cmd/go-v2-pipeline/main.go
decisions:
  - "contextBudgetTokens = 4000 (gpt-5.4 context window 50% target, base ~2000 + max ~900 context + margin)"
  - "Token budget priority D-08: continuity (first removed) -> branch -> voice card (last removed)"
  - "CLI flag added to go-v2-pipeline (correct v2 CLI) instead of go-translation-pipeline (v1 CLI)"
  - "Voice cards loaded once at TranslateWorker startup, not per-batch"
metrics:
  duration: 7min
  completed: "2026-04-07T16:44:00Z"
  tasks: 2/3 (checkpoint pending)
  files: 5
status: checkpoint-pending
---

# Phase 07 Plan 03: Prompt Integration + Token Budget + A/B Test Summary

BuildScriptPrompt 5-type context injection (voice card, branch, next lines, prevKO/nextKO) + trimContextForBudget D-08 priority shedding + worker translateBatch full context assembly + CLI voice-cards flag

## Status: CHECKPOINT PENDING (Task 3)

Tasks 1-2 completed. Task 3 (A/B test human-verify) awaiting user verification.

## Performance

- **Duration:** 7 min (Tasks 1-2)
- **Started:** 2026-04-07T16:37:31Z
- **Tasks:** 2/3 complete (Task 3 is checkpoint:human-verify)
- **Files modified:** 5

## Accomplishments

### Task 1: BuildScriptPrompt Extension (TDD)
- Refactored core logic into `buildScriptPromptCore` for budget trimming testability
- Added 5 context types to [CONTEXT] block: PrevGateLines, ParentChoiceText, NextLines, PrevKO, NextKO
- Added Named Character Voice Guide section from VoiceCards map
- Implemented `trimContextForBudget` with D-08 priority: continuity -> branch -> voice card
- Added `contextBudgetTokens = 4000` constant
- 8 new tests (TDD RED -> GREEN), 56 total tests passing

### Task 2: Config + Worker + CLI Integration
- Added `VoiceCardsPath` and `VoiceCards` fields to `v2pipeline.Config`
- Extended `translateBatch` with GetNextLines, GetAdjacentKO, ParentChoiceText, VoiceCards
- Voice cards loaded once at `TranslateWorker` startup (not per-batch)
- Added `-voice-cards` CLI flag to `go-v2-pipeline`
- Full build passes, all tests pass (except pre-existing TestSelectRetranslationBatches)

## Task Commits

1. **Task 1 RED:** `e5f6701` - test(07-03): add failing tests for BuildScriptPrompt context enrichment
2. **Task 1 GREEN:** `d7b8971` - feat(07-03): extend BuildScriptPrompt with 5-type context injection + token budget
3. **Task 2:** `549fe6c` - feat(07-03): add Config.VoiceCardsPath + extend translateBatch with 5-type context assembly

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] CLI target correction**
- **Found during:** Task 2
- **Issue:** Plan specified `workflow/cmd/go-translation-pipeline/main.go` but that CLI uses `translationpipeline.Config` (v1 pipeline). The v2 pipeline CLI is `workflow/cmd/go-v2-pipeline/main.go`.
- **Fix:** Added `-voice-cards` flag to `go-v2-pipeline/main.go` instead (correct v2 CLI).
- **Files modified:** `workflow/cmd/go-v2-pipeline/main.go`

## Verification

- `go test ./workflow/internal/clustertranslate/` -- all 56 tests pass
- `go build ./workflow/...` -- full build passes
- `go test ./workflow/internal/v2pipeline/` -- all tests pass except pre-existing TestSelectRetranslationBatches

## Self-Check: PENDING

Will complete after Task 3 checkpoint resolution.

---
*Phase: 07-context-enrichment*
*Status: checkpoint-pending (Task 3: A/B test human-verify)*
