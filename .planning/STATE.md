---
gsd_state_version: 1.0
milestone: v1.1
milestone_name: 번역 품질 개선 — 맥락 기반 재번역
status: phase_complete
stopped_at: Phase 08 CLOSED (인프라 완료, 품질 미달 — Phase 09로 이월)
last_updated: "2026-04-12T18:00:00.000Z"
last_activity: 2026-04-12 -- Phase 08 전량 종료. watchdog 수정, retranslate.go 구현. voice card/RAG 누락으로 번역 품질 미달 판정 → Phase 09로 이월.
progress:
  total_phases: 5
  completed_phases: 4
  total_plans: 12
  completed_plans: 12
  percent: 100
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-06)

**Core value:** 게임이 실제로 렌더링하는 대사 블록 단위로 소스를 생성하여, 태그 깨짐 없이 한국어 패치가 동작해야 한다.
**Current focus:** Phase 08 CLOSED → Phase 09 (voice cards + RAG 재통합 + 전량 재번역)

## Current Position

Phase: 08 (retranslation-infrastructure) — CLOSED
Status: 인프라 완료, 품질 전제조건 미충족으로 실번역 Phase 09로 이월

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

- [ ] `go-v2-pipeline --help`에서 `--voice-cards`, `--rag-context` 플래그 확인
- [ ] `voice_cards.json` 존재 확인
- [ ] `rag_batch_context.json` 존재 확인
- [ ] 10건 샘플 번역 후 Kattegatt 등 고어체 캐릭터 육안 검토

## Session Continuity

Last session: 2026-04-12T18:00:00.000Z
Stopped at: Phase 08 CLOSED, ROADMAP + STATE 업데이트 완료
Next action: Phase 09 Plan 01 — voice cards + RAG go-v2-pipeline 재통합, voice_cards.json 생성, 샘플 검증
