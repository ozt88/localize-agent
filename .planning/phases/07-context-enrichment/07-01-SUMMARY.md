---
phase: 07-context-enrichment
plan: 01
subsystem: translation
tags: [voice-card, llm, go, json, character-profile]

requires:
  - phase: 05-untranslated-coverage
    provides: pipeline_items_v2 with speaker data
provides:
  - VoiceCard type and LoadVoiceCards/BuildNamedVoiceSection functions
  - go-generate-voice-cards CLI for LLM-based voice card generation
  - voice_cards.json with 15 named character voice profiles
affects: [07-context-enrichment, prompt-injection, retranslation]

tech-stack:
  added: []
  patterns: [voice-card-json-load, named-character-voice-section]

key-files:
  created:
    - workflow/internal/clustertranslate/voice_card.go
    - workflow/internal/clustertranslate/voice_card_test.go
    - workflow/cmd/go-generate-voice-cards/main.go
    - projects/esoteric-ebb/context/voice_cards.json
  modified: []

key-decisions:
  - "Top 15 named characters selected by frequency (184+): Snell through Arn"
  - "VoiceCard uses 3 fields: speech_style, honorific, personality -- matching plan D-01/D-02/D-03"
  - "CLI is idempotent: loads existing voice_cards.json and only generates missing entries"

patterns-established:
  - "Voice card JSON pattern: flat map keyed by character name with speech_style/honorific/personality"
  - "BuildNamedVoiceSection: per-batch voice guide injection for named characters (complements ability-score buildVoiceSection)"

requirements-completed: [TONE-01]

duration: 5min
completed: 2026-04-07
---

# Phase 07 Plan 01: Voice Card Infrastructure Summary

**VoiceCard type + LoadVoiceCards/BuildNamedVoiceSection Go 함수, LLM 기반 voice card 생성 CLI, 15명 named character voice_cards.json 데이터**

## Performance

- **Duration:** 5 min
- **Started:** 2026-04-07T00:23:57Z
- **Completed:** 2026-04-07T00:29:10Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- VoiceCard 타입과 LoadVoiceCards/BuildNamedVoiceSection 함수 구현 (7개 테스트 통과)
- go-generate-voice-cards CLI: DB 대사 샘플링 + LLM 프롬프트로 voice card 자동 생성
- 15명 named character (Snell~Arn) voice_cards.json 데이터 생성

## Task Commits

Each task was committed atomically:

1. **Task 1: voiceCard type + LoadVoiceCards + BuildNamedVoiceSection** - `82989cb` (test: RED), `f565ba9` (feat: GREEN)
2. **Task 2: go-generate-voice-cards CLI + voice_cards.json** - `d1ceb58` (feat)

_Note: Task 1 followed TDD (RED -> GREEN)_

## Files Created/Modified
- `workflow/internal/clustertranslate/voice_card.go` - VoiceCard struct, LoadVoiceCards, BuildNamedVoiceSection
- `workflow/internal/clustertranslate/voice_card_test.go` - 7 test cases for voice card functions
- `workflow/cmd/go-generate-voice-cards/main.go` - LLM-based voice card generation CLI
- `projects/esoteric-ebb/context/voice_cards.json` - 15 named character voice profiles

## Decisions Made
- Top 15 named characters selected (frequency >= 184, Snell through Arn) -- plan context specified these 15
- CLI uses pipeline low_llm profile for voice card generation (OpenCode gpt-5.2)
- voice_cards.json is a data asset under version control (not generated at build time)
- CLI is idempotent: existing entries preserved, only missing speakers generated

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- voice_card.go exports ready for Plan 03 prompt injection integration
- voice_cards.json data available for LoadVoiceCards in runtime
- BuildNamedVoiceSection complements existing buildVoiceSection (ability-score speakers)

## Self-Check: PASSED

All 5 files verified present. All 3 commits verified in git log.

---
*Phase: 07-context-enrichment*
*Completed: 2026-04-07*
