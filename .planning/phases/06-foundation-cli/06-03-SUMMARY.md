---
phase: 06-foundation-cli
plan: 03
subsystem: database, cli
tags: [retranslation, score-threshold, postgresql, sqlite, histogram, batch-reset]

requires:
  - phase: 06-01
    provides: v2pipeline store + contracts infrastructure
  - phase: 06-02
    provides: pipeline items schema and worker patterns
provides:
  - ScoreHistogram, SelectRetranslationBatches, ResetForRetranslation store methods
  - retranslation_gen column + retranslation_snapshots table
  - go-retranslate-select CLI with histogram/dry-run/execute modes
  - RetranslateSelectConfig + RunRetranslateSelect domain logic
affects: [08-retranslation-execution]

tech-stack:
  added: []
  patterns: [batch-level retranslation selection, snapshot-before-reset, generation tracking]

key-files:
  created:
    - workflow/internal/v2pipeline/retranslate.go
    - workflow/internal/v2pipeline/retranslate_test.go
    - workflow/cmd/go-retranslate-select/main.go
  modified:
    - workflow/internal/contracts/v2pipeline.go
    - workflow/internal/v2pipeline/store.go
    - workflow/internal/v2pipeline/postgres_v2_schema.sql
    - workflow/internal/v2pipeline/worker_test.go

key-decisions:
  - "D-10 채택: StatePendingRetranslate 없이 기존 StatePendingTranslate로 리셋하여 기존 worker가 재번역 처리"
  - "batch_id 단위 선택: 개별 라인 재번역 불가, 씬 클러스터 단위 보존"
  - "retranslation_gen으로 세대 추적, retranslation_snapshots로 원본 보존"

patterns-established:
  - "Snapshot-before-reset: ResetForRetranslation은 트랜잭션 내에서 스냅샷 저장 후 상태 리셋"
  - "Score-based batch selection: HAVING MIN(score_final) < threshold로 배치 내 최저 점수 기준 선택"

requirements-completed: [RETRANS-01, RETRANS-02, RETRANS-03]

duration: 9min
completed: 2026-04-06
---

# Phase 06 Plan 03: Retranslation Select Summary

**score_final 기반 재번역 후보 선택 CLI + retranslation_gen/snapshots DB 스키마 + batch 단위 상태 리셋**

## Performance

- **Duration:** 9 min
- **Started:** 2026-04-06T15:45:34Z
- **Completed:** 2026-04-06T15:54:26Z
- **Tasks:** 3
- **Files modified:** 7

## Accomplishments
- DB 스키마에 retranslation_gen 컬럼 + retranslation_snapshots 테이블 + idx_pv2_score 인덱스 추가 (SQLite + Postgres 양쪽)
- contracts에 ScoreBucket, RetranslationCandidate 타입 + 3개 인터페이스 메서드 정의, store에 구현
- RunRetranslateSelect 도메인 로직: histogram/dry-run/execute 모드 지원
- go-retranslate-select CLI: -score-threshold, -dry-run, -histogram, -content-type, -project 플래그

## Task Commits

Each task was committed atomically:

1. **Task 1: DB schema + contracts + store retranslation methods** - `0453614` (feat)
2. **Task 2: RunRetranslateSelect domain logic** - `4e192d5` (feat)
3. **Task 3: go-retranslate-select CLI entrypoint** - `ff5cd1e` (feat)

## Files Created/Modified
- `workflow/internal/contracts/v2pipeline.go` - RetranslationGen field, ScoreBucket/RetranslationCandidate types, 3 interface methods
- `workflow/internal/v2pipeline/store.go` - SQLite schema extension, ScoreHistogram/SelectRetranslationBatches/ResetForRetranslation implementations, column list updates
- `workflow/internal/v2pipeline/postgres_v2_schema.sql` - ALTER TABLE retranslation_gen, CREATE TABLE retranslation_snapshots, idx_pv2_score
- `workflow/internal/v2pipeline/retranslate.go` - RetranslateSelectConfig, RunRetranslateSelect, histogram/candidate/reset logic
- `workflow/internal/v2pipeline/retranslate_test.go` - 11 tests: schema, histogram, batch selection, reset, snapshots, multi-gen, domain logic
- `workflow/cmd/go-retranslate-select/main.go` - CLI entrypoint with flag parsing and project config loading
- `workflow/internal/v2pipeline/worker_test.go` - fakeStore updated with 3 new interface methods

## Decisions Made
- D-10 채택: `StatePendingRetranslate` 상태 상수를 추가하지 않음. 기존 `StatePendingTranslate`로 리셋하여 기존 TranslateWorker가 별도 수정 없이 재번역을 처리. `retranslation_gen > 0`으로 재번역 세대 구분.
- batch_id 단위 선택: 개별 라인이 아닌 배치(씬 클러스터) 단위로만 재번역 선택 가능. 문맥 보존을 위한 의도적 제약.
- ScoreHistogram에서 SQLite FLOOR 미지원: `CAST(score_final / ? AS INTEGER) * ?` 패턴으로 우회.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] fakeStore interface compliance**
- **Found during:** Task 1
- **Issue:** worker_test.go의 fakeStore가 새 인터페이스 메서드 누락으로 컴파일 실패
- **Fix:** ScoreHistogram, SelectRetranslationBatches, ResetForRetranslation stub 메서드 추가
- **Files modified:** workflow/internal/v2pipeline/worker_test.go
- **Verification:** go build + go test 전체 통과
- **Committed in:** 0453614 (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** fakeStore 업데이트는 인터페이스 확장의 필연적 결과. No scope creep.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- 재번역 선별 도구 완성: Phase 08 재번역 실행에서 `go-retranslate-select -score-threshold 7.0 -dry-run=false`로 후보 선별 가능
- Phase 07 A/B 테스트에서 재번역 CLI를 활용하여 품질 비교 가능
- retranslation_snapshots로 롤백 안전장치 확보

---
*Phase: 06-foundation-cli*
*Completed: 2026-04-06*
