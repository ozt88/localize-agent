---
gsd_state_version: 1.0
milestone: v1.1
milestone_name: 번역 품질 개선 — 맥락 기반 재번역
status: executing
stopped_at: Completed 09-01-PLAN.md
last_updated: "2026-04-12T15:22:24.857Z"
progress:
  total_phases: 5
  completed_phases: 2
  total_plans: 9
  completed_plans: 7
  percent: 78
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-06)

**Core value:** 게임이 실제로 렌더링하는 대사 블록 단위로 소스를 생성하여, 태그 깨짐 없이 한국어 패치가 동작해야 한다.
**Current focus:** Phase 09 — retranslation-execution

## Current Position

Phase: 09 (retranslation-execution) — EXECUTING
Plan: 2 of 3
Status: Ready to execute

## Phase 08 종료 사유

**판정: 파이프라인 번역물 전량 폐기**

worktree 버그(08-01) 이후 복원된 코드에서 `go-v2-pipeline/main.go`의 `--voice-cards`, `--rag-context` 플래그가 누락된 채 3,412건이 번역됨.

AR_Kattegatt 씬 비교에서 확인:

- gen=0 (구버전): `그대/~도다/~노라` 고어체 → 캐릭터 정체성 유지
- gen=1 (신버전): `너/~다` 현대 반말로 평탄화 → 캐릭터 고유 말투 소실

Phase 08 목표("voice card + RAG 포함 프롬프트로 재번역")가 실제로는 달성되지 않음.

## Phase 08 완료된 인프라 (유효)

- `BuildV3Sidecar` highest-gen dedup — 동일 source_raw에서 최신 gen 항목만 export
- `ResetAllForRetranslation` — 전체 pending_translate 리셋 CLI
- `watchdog` false-kill 수정 — deepProbe → probeServer (단순 HTTP GET)
- `retranslate.go` 구현 — SelectRetranslationBatches, ScoreHistogram, ResetForRetranslation
- `retranslation_snapshots` 테이블 — 리셋 전 이전 번역 보존

## Phase 09 진입 전 필수 체크리스트

- [x] `go-v2-pipeline --help`에서 `--voice-cards`, `--rag-context` 플래그 확인
- [x] `voice_cards.json` 존재 확인 (27개 캐릭터)
- [x] `rag_batch_context.json` 존재 확인
- [x] 10건 샘플 번역 후 VL_Visken Visken 격식체 voice card 효과 확인

## Phase 09-01 완료 결정 사항

- **quoteForPrompt 도입**: `%q` 대신 실제 개행 보존 — 멀티라인 LLM 번역 파괴 방지
- **[NN] 청크 분할 파서 재작성**: 멀티라인 대화 블록을 단일 번역 항목으로 처리
- **voice_cards.json 27개 캐릭터 + relationships**: 관계별 어조 변화 주입 가능

## Session Continuity

Last session: 2026-04-12T15:22:24.853Z
Stopped at: Completed 09-01-PLAN.md
Next action: Phase 09 Plan 02 — 전체 40,067건 재번역 실행
