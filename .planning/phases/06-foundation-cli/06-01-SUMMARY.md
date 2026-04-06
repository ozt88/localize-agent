---
phase: 06-foundation-cli
plan: 01
subsystem: translation
tags: [prompt-engineering, voice-guide, token-profiling, clustertranslate]

# Dependency graph
requires:
  - phase: 02-translation-engine
    provides: clustertranslate package with v2StaticRules, BuildBaseWarmup, BuildScriptPrompt
provides:
  - v2PromptSections 4-tier prompt structure (Context/Voice/Task/Constraints)
  - Per-batch ability-score voice guide injection
  - PromptMeta.EstimatedTokens token budget profiling
affects: [07-context-injection, clustertranslate]

# Tech tracking
tech-stack:
  added: []
  patterns: [4-tier prompt sectioning, per-batch voice reminder, token estimation heuristic]

key-files:
  created: []
  modified:
    - workflow/internal/clustertranslate/prompt.go
    - workflow/internal/clustertranslate/types.go
    - workflow/internal/clustertranslate/prompt_test.go

key-decisions:
  - "Voice guide는 워밍업이 아닌 per-batch 리마인더로 주입 (Pitfall 4 방지)"
  - "토큰 추정: 영문 4chars/token, 한국어 2runes/token 근사치 사용"

patterns-established:
  - "4-tier prompt sections: Context/Voice/Task/Constraints 구조"
  - "sectionsToRules() 호환 헬퍼로 기존 코드 호환성 유지"

requirements-completed: [PROMPT-01, PROMPT-02, PROMPT-03]

# Metrics
duration: 3min
completed: 2026-04-06
---

# Phase 06 Plan 01: Prompt Hierarchy Summary

**v2StaticRules 9개 flat 규칙을 4-tier 섹션으로 계층화하고, per-batch ability-score voice guide 주입 + 토큰 프로파일링 추가**

## Performance

- **Duration:** 3 min
- **Started:** 2026-04-06T04:16:31Z
- **Completed:** 2026-04-06T04:19:47Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- v2StaticRules를 v2PromptSections 구조체로 교체, 4개 섹션 헤딩(### Context/Voice/Task/Constraints)으로 프롬프트 조립
- 6개 ability-score speaker (wis/str/int/cha/dex/con)에 대한 per-batch voice guide 리마인더 주입
- estimateTokens 함수로 프롬프트 토큰 예산 근사치를 PromptMeta.EstimatedTokens에 기록

## Task Commits

Each task was committed atomically:

1. **Task 1: v2StaticRules를 v2PromptSections 4-tier 구조로 계층화** - `dafa6c6` (feat)
2. **Task 2: Per-batch ability-score voice guide 주입 + 토큰 프로파일링** - `2586c65` (feat)

_Note: TDD tasks — RED (failing tests) then GREEN (implementation) in single commits_

## Files Created/Modified
- `workflow/internal/clustertranslate/prompt.go` - v2PromptSections 구조체, 4-tier BuildBaseWarmup, abilityScoreVoice map, buildVoiceSection, estimateTokens
- `workflow/internal/clustertranslate/types.go` - PromptMeta에 EstimatedTokens 필드 추가
- `workflow/internal/clustertranslate/prompt_test.go` - 14개 신규 테스트 추가 (섹션 구조, 헤더 순서, voice 주입, 토큰 추정)

## Decisions Made
- Voice guide는 워밍업(warmup)이 아닌 per-batch 리마인더로 짧게 주입 — 워밍업에는 v2_base_prompt.md 전체 가이드가 이미 포함되므로 중복 방지
- 토큰 추정 공식: 영문 chars/4 + 한국어 runes/2 — Phase 07에서 토큰 예산 상한 결정 시 활용

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- 4-tier 프롬프트 구조가 Phase 07 컨텍스트 주입(voice card, branch context, continuity window)의 기반으로 준비됨
- EstimatedTokens로 프롬프트 토큰 예산 모니터링 가능

---
## Self-Check: PASSED

All files exist, all commits verified.

*Phase: 06-foundation-cli*
*Completed: 2026-04-06*
