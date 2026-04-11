<decisions>
## Implementation Decisions

### 재번역 대상 범위
- **D-01:** 전체 재번역 — score 커트라인 없음, 35,009건 전량 재번역
  - 기존 `go-retranslate-select --score-threshold` 방식 대신 전체 배치를 직접 `pending_translate`로 리셋
  - `retranslation_gen`을 0 → 1로 증가시켜 신 번역 구분
  - unscored 464건도 포함 (재번역 대상)

### score-aware dedup (BuildV3Sidecar)
- **D-02:** 항상 최신 `retranslation_gen` 우선 — score 비교 없이 단순히 gen이 높은 항목이 entries[]에 선택됨
  - 현재 "first-seen-wins" 방식에서 "highest-gen-wins" 방식으로 수정
  - 구현: items를 sort by (source_raw, retranslation_gen DESC) 후 first-seen으로 dedup하면 자연스럽게 최신 gen 우선

### 패치 적용 방식
- **D-03:** 재번역 전량 완료 후 `go-v2-export` 1회 실행 → 기존 `translations.json` 완전 교체 (백업 없음)
  - 중간 export 없음 (진행 중 덮어쓰기 방지)
  - 롤백 필요 시 git으로 복구

### 인게임 검증
- **D-04:** 이전에 테스트한 초반 진입 씬(고정 리스트)으로 회귀 검증
  - 매 릴리즈마다 동일한 씬을 플레이하여 태그 깨짐 및 대사 흐름 확인
  - 게임 초반 진입 씬 = 번역 품질 체감이 가장 높고, 이전 테스트로 기준이 이미 잡혀 있음
- **D-05:** before/after diff 먼저 생성 → 변경된 번역 중 이상해 보이는 항목만 게임에서 확인
  - diff 도구: 기존 translations.json vs 신 translations.json, source 기준 매칭 후 target diff
  - diff에서 이상 징후 없으면 게임 검증 최소화

### Claude's Discretion
- 전체 배치 리셋 방식: `ResetForRetranslation` 배치 단위 루프 vs SQL 전체 UPDATE — 구현 효율 고려하여 결정
- diff 도구: 별도 Go CLI 작성 vs Python 스크립트 vs 기존 도구 활용 — 빠른 쪽으로 결정
- 전량 35,009건 파이프라인 실행 중 watchdog/lease 설정값 조정 — 기존 v2pipeline 안정화 경험 반영
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### 핵심 코드
- `workflow/internal/v2pipeline/export.go` — `BuildV3Sidecar` 현재 구현 (first-seen-wins, D-02 수정 대상)
- `workflow/internal/v2pipeline/retranslate.go` — `RunRetranslateSelect`, `executeReset`, `ResetForRetranslation` — 재번역 리셋 패턴
- `workflow/internal/v2pipeline/store.go` — `pipeline_items_v2` 스키마, `retranslation_gen` 컬럼, `SelectRetranslationBatches`
- `workflow/cmd/go-retranslate-select/main.go` — 재번역 후보 선택 CLI (threshold 기반 — 이번엔 전체 리셋이라 직접 사용 안 할 수 있음)
- `workflow/cmd/go-v2-export/main.go` — 패치 export CLI (`-out-dir`, `-backend`, `-dsn` 플래그)
- `workflow/cmd/go-v2-pipeline/main.go` — 파이프라인 실행 플래그 전체 (translate/format/score 역할 분리)

### 참조 데이터
- `projects/esoteric-ebb/context/ab_test_no_rag_scores.json` — No RAG baseline avg 8.431 (재번역 후 비교 기준)
- `projects/esoteric-ebb/rag/rag_batch_context.json` — RAG 배치 컨텍스트 (재번역 시 `-rag-context` 플래그로 주입)

### Phase 07.1 인계
- `.planning/phases/07.1-rag-mcp-pageindex-mcp/07.1-04-SUMMARY.md` — No RAG baseline, v2pipeline 안정성 수정 내용
- `.planning/phases/07.1-rag-mcp-pageindex-mcp/07.1-03-SUMMARY.md` — RAG 파이프라인 통합 (ragcontext 패키지, -rag-context 플래그)
</canonical_refs>

<specifics>
## Specific Context

### 현재 DB 상태 (2026-04-12 기준)
- done: 35,009건 (전량 재번역 대상)
- score 분포:
  - 0-5: 1,091건 / 44 배치 (최저 품질)
  - 5-7: 17건
  - 7-8: 494건
  - 8-9: 9,018건
  - 9-10: 23,925건
  - unscored: 464건
- No RAG baseline: avg 8.431 (10배치 216 items 기준)

### 전체 리셋 접근법
- `go-retranslate-select`는 threshold 기반 선택 CLI임 — 전체 리셋에는 부적합
- store에서 직접 모든 done items를 `pending_translate`로 리셋하고 `retranslation_gen = 1` 설정 필요
- 기존 `ResetForRetranslation(batchID, nextGen)` 패턴을 배치 단위로 반복하거나, 전체 UPDATE SQL로 처리

### BuildV3Sidecar 수정 방향
- 현재: `seen[item.SourceRaw]` first-seen-wins
- 변경: items를 store에서 조회할 때 `ORDER BY retranslation_gen DESC` 보장 후 first-seen으로 dedup
  - 또는 BuildV3Sidecar 내부에서 같은 source_raw가 여러 gen으로 들어올 경우 highest gen 선택 로직 추가

### v2pipeline 안정성 (2026-04-12 수정 후 현재 설정)
- watchdog: 2분 감지, 재시작 후 stale reclaim 자동
- warmup 실패 시 items 즉시 release
- score sub-batch: 10, timeout: 180s
- translate concurrency: project.json 기본값 사용 (gpt-5.4 concurrency=2)
</specifics>

<deferred>
## Deferred Ideas

- score가 현저히 낮아진 경우(gen=1 < gen=0 - 2.0) 구 버전 fallback — 복잡도 대비 효과 불명확, 전체 재번역 후 결과 보고 판단
- 미번역 154건(DB 누락 대화/주문 설명) 해소 — Out of scope for Phase 08, 별도 Phase 필요
</deferred>

---

*Phase: 08-retranslation-execution*
*Context gathered: 2026-04-12*
