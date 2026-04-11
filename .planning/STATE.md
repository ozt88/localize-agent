---
gsd_state_version: 1.0
milestone: v1.1
milestone_name: 번역 품질 개선 — 맥락 기반 재번역
status: executing
stopped_at: Phase 07.1 Plan 04 A/B 테스트 No RAG 조건 스코어링 진행 중
last_updated: "2026-04-11T19:50:00.000Z"
last_activity: 2026-04-11 -- score-worker sub-batch 수정, dead code 8개 삭제, No RAG 스코어링 백그라운드 진행 중
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
- `go-v2-pipeline --role score --score-concurrency 2 --score-timeout-sec 180 --lease-sec 400` (PID 불명, 백그라운드)

## 다음 세션에서 할 일

### Step 1: 스코어링 완료 확인
```sql
SELECT state, count(*) FROM pipeline_items_v2
WHERE batch_id IN (...10개 배치 ID...)
GROUP BY state;
```
모두 `done` 또는 `pending_translate`(재번역 실패)이면 완료.

### Step 2: No RAG 스코어 수집
```python
# ab_test_rag.py의 get_scores() 함수 참조
# 또는 직접 psql:
SELECT batch_id, AVG(score_final)
FROM pipeline_items_v2
WHERE batch_id IN (...) AND score_final > 0
GROUP BY batch_id;
```

### Step 3: With RAG 재실행
No RAG 스코어 확보 후 동일 10개 배치를 RAG로 재번역+스코어링.
`python projects/esoteric-ebb/context/ab_test_rag.py` 실행 시
기존 results.json에서 batch_id 읽어 With RAG만 실행하도록 수정 필요
(현재 v3 스크립트는 A/B 양쪽 모두 재실행 — No RAG는 이미 있으므로 B만 필요)

### Step 4: 비교 및 Plan 04 SUMMARY 작성

## 이번 세션 완료 작업 (2026-04-11)

### 버그 수정
- `score-worker`: 20개 단일 LLM 호출 → 5개씩 sub-batch 분리 (lease 만료 문제 해결)
- `ab_test_rag.py v3`: cwd=PROJECT_ROOT 추가 (project.json 탐색 실패 수정)
- `ab_test_rag.py v3`: A/B 양쪽 모두 현재 스코어러로 재번역 (공정성 확보)

### Dead code 삭제 (커밋 e8ac2f2)
- `scorellm.BuildScoreWarmup` — warmup이 .md 파일에서 로드됨
- `tagformat.BuildFormatWarmup` — 동일
- `tagformat.HasRichTags`, `StripTags`, `CountTags` — 테스트 전용
- `clustertranslate.BuildNamedVoiceSection` — 테스트 전용
- `platform.LoadDonePackItems` — 호출처 없음
- `shared.StripCodeFence` — 테스트 전용

### 커밋 목록
- `e5c3ff3` fix(ab-test): v3 full A/B rerun
- `a72e03c` fix(ab-test): subprocess cwd=PROJECT_ROOT
- `113cb6f` fix(score-worker): sub-batch 5개씩 처리
- `c0dc317` refactor(score): scoreItem/BuildScorePrompt dead code 삭제
- `e8ac2f2` refactor: dead code 전량 삭제

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