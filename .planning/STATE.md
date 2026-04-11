---
gsd_state_version: 1.0
milestone: v1.1
milestone_name: 번역 품질 개선 — 맥락 기반 재번역
status: executing
stopped_at: Phase 07.1 Plan 04 A/B 테스트 No RAG 조건 스코어링 진행 중
last_updated: "2026-04-12T01:30:00.000Z"
last_activity: 2026-04-12 -- v2pipeline 성능/안정성 수정 커밋(b22f492), score worker 재가동
progress:
  total_phases: 4
  completed_phases: 2
  total_plans: 10
  completed_plans: 9
  percent: 90
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-06)

**Core value:** 게임이 실제로 렌더링하는 대사 블록 단위로 소스를 생성하여, 태그 깨짐 없이 한국어 패치가 동작해야 한다.
**Current focus:** Phase 07.1 — Plan 04 A/B 테스트 재실행 (No RAG baseline 현재 스코어러로 재측정)

## Current Position

Phase: 07.1 (rag-mcp-pageindex-mcp) — EXECUTING
Plan: 4 of 4 (A/B 테스트 재실행 중 — No RAG 조건 스코어링 진행 중)
Status: No RAG 번역 완료, 스코어링 백그라운드 진행 중 (done=119/217, pending_translate=75 재번역 대기)
Last activity: 2026-04-11 -- score-worker 수정 + dead code 정리 후 스코어링 재개

Progress: [█████████░] 90%

## 현재 DB 상태 (2026-04-11 19:50 기준)

10개 테스트 배치 (217 items) 상태:
- done: 119
- working_score: 20 (진행 중)
- pending_translate: 75 (낮은 점수로 재번역 대기)
- pending_format: 2

**진행 중인 백그라운드 프로세스:**
- `go-v2-pipeline --role score --lease-sec 400` (PID 1372, 백그라운드, 2026-04-12 01:30 기준)
- pending_score=55, working_score=20 처리 중

## 다음 세션에서 할 일

### Step 0: 스코어링 진행 확인 (working_score가 0이 될 때까지)
```sql
SELECT state, count(*) FROM pipeline_items_v2
WHERE batch_id IN (
  'Snell_Companion/Snell_Companion/g-17+g-21+afterSleep+g-2/batch-1084',
  'CB_Lake/CB_Lake/g-4+offer+g-6+hub/batch-170',
  'AR_Viira_Ivcx/AR_Viira_Ivcx/gift+g-1/batch-51',
  'TS_Nearly/Nearly_TS/bardHub/batch-1210',
  'Enc_Final/MASKED/Ragn_Round_3_Hub+g-29+Masked_Round_4+Ragn_Round_4+g-43+endingChoice+g-15+Ragn_Round_2/batch-564',
  'DS_Olzis/Olzis/hub+g-12+g-19/batch-347',
  'DN_Darrow/Darrow/QuestionsHub/batch-303',
  'CB_Moongore/CB_Moongore/g-12+g-17+g-24+HWBTHub+g-29+g-32+caught_end/batch-191',
  'TS_Kull/Kull/intro+g-23+Snell_Round_2+Kull_Round_2+round_3_intro+g-28+g-29+Snell_Round_3+Kull_Round_3+g-35+g-36/batch-1195',
  'CB_Kraaid/CB_Kraaid/g-1+kraaidInfoHub+g-33+yesIam/batch-152'
) GROUP BY state;
```

### Step 1: No RAG 점수 수집 후 With RAG 재실행
- `pending_translate` 항목 처리 방침 결정 필요 (세션 중 미결 — 이전 논의 참조)
- With RAG: `python projects/esoteric-ebb/context/ab_test_rag.py` (B 조건만 실행 필요)

### Step 2: 비교 및 Plan 04 SUMMARY 작성

## 이번 세션 완료 작업 (2026-04-12)

### v2pipeline 성능/안정성 수정 (커밋 b22f492)
- **버그**: translate/format/score warmup 실패 시 items 미해제 → 5시간 stuck 원인
- **수정**: EnsureContext 에러 핸들러에 retry 로직 추가 (3곳)
- **watchdog**: 감지 6분→2분, 재시작 후 stale 자동 reclaim
- **deepProbe**: LLM ping 제거 (gpt-5.4 느려서 false positive → 멀쩡한 서버 kill 반복)
- **score**: sub-batch 5→10, timeout 120→180s

### 커밋 목록
- `b22f492` fix(v2pipeline): watchdog 안정화 + warmup 실패 시 items 즉시 release

## Accumulated Context

### Roadmap Evolution
- Phase 07.1 inserted after Phase 07: RAG+MCP 세계관 맥락 주입

### Pending Todos
- Phase 07.1 Plan 04 A/B 테스트 완료 후 결과 분석
- No RAG baseline 현재 스코어러로 재측정 → With RAG와 공정 비교
- RAG 힌트 품질 재검토 (이전 결과: avg delta -0.81, 10배치 중 9개 하락)

### Blockers/Concerns
- **[ACTIVE]** A/B 테스트 미완료 — No RAG 스코어링 진행 중
- **[ACTIVE]** score-worker timeout: p95=120s, OpenCode 서버 과부하 시 crash (concurrency=2로 완화)
- Phase 08: 재번역 후보 규모 확인 필요

## Session Continuity

Last session: 2026-04-11T19:50:00.000Z
Stopped at: No RAG 스코어링 백그라운드 진행 중, /clear 전 state 저장
Resume file: .planning/phases/07.1-rag-mcp-pageindex-mcp/07.1-04-PLAN.md
Next action: 스코어링 완료 확인 → No RAG 점수 수집 → With RAG 재실행 → 비교 → SUMMARY 작성
</content>
</invoke>