# Phase 06: Foundation — 프롬프트 재구조화 + 화자 검증 + 재번역 CLI - Context

**Gathered:** 2026-04-06
**Status:** Ready for planning

<domain>
## Phase Boundary

번역 품질 개선의 전제 조건 확립: 프롬프트 구조가 컨텍스트 주입을 수용할 준비가 되고, 화자 데이터가 검증되고, 선별 재번역 도구가 동작.

Requirements: PROMPT-01, PROMPT-02, PROMPT-03, SPEAKER-01, SPEAKER-02, SPEAKER-03, RETRANS-01, RETRANS-02, RETRANS-03

</domain>

<decisions>
## Implementation Decisions

### 프롬프트 구조

- **D-01:** 재번역은 클러스터 번역 경로(`BuildScriptPrompt` + `v2StaticRules`)로 단일화. 개별 번역 프롬프트(`defaultStaticRules`)는 UI/overlay 용도로만 유지, 재구조화 대상 아님.
- **D-02:** `v2StaticRules`(9개)를 컨텍스트/보이스/태스크/제약 4개 섹션으로 계층화. Phase 07에서 voice card, branch context, continuity window가 각 섹션에 삽입될 수 있도록 구조 설계.
- **D-03:** ability-score voice guide는 워밍업에 전체 가이드 포함(`BuildBaseWarmup`) + per-batch에 해당 캐릭터 voice guide만 강조 주입(`BuildScriptPrompt`). 두 레이어 모두 적용.

### 화자 검증

- **D-04:** 2단계 화자 검증: (1) DB에서 `DISTINCT speaker` + 빈도 분포 추출 → (2) 빈도 낮은(1-2회) 의심 항목은 ink JSON 소스와 교차 검증하여 오인식 필터링.
- **D-05:** 검증된 화자 allow-list를 JSON 파일로 관리. `isSpeakerTag` 오인식을 필터링하는 데 사용.
- **D-06:** 화자 커버리지 목표 90%+ (대화 라인 대비). 미달 시 ink 파서의 `#` 태그 파싱 로직 강화.

### 재번역 선별

- **D-07:** ScoreFinal threshold는 score_final 히스토그램 분포 분석 후 자연스러운 cutoff 지점에서 결정. 고정값이 아닌 데이터 기반.
- **D-08:** 재번역 단위는 반드시 batch_id 전체 클러스터. 개별 라인 재번역 금지 (P1 pitfall: 톤 불일치 방지).
- **D-09:** `retranslation_gen` 컬럼을 pipeline_items_v2에 추가. 각 재번역 세대마다 gen+1. 이전 세대 데이터 유지로 롤백 가능.
- **D-10:** 재번역 CLI는 기존 파이프라인 상태 머신과 통합. `StatePendingRetranslate` 상태를 활용하여 기존 worker가 재번역도 처리.

### Claude's Discretion

- 토큰 예산: Phase 06에서는 프로파일링만 수행. 예산 상한 및 우선순위 전략은 Phase 07 discuss에서 결정.
- 프롬프트 계층 구조의 구체적 포맷 (마크다운 헤딩, 구분자 스타일 등)
- DB migration 구체 구현 (retranslation_gen 컬럼 타입, 기본값)
- score_final 히스토그램 시각화 방식

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### 프롬프트 시스템
- `workflow/internal/clustertranslate/prompt.go` — v2StaticRules, BuildBaseWarmup, BuildScriptPrompt, BuildContentSuffix
- `workflow/internal/clustertranslate/types.go` — ClusterTask 구조체 (Batch, PrevGateLines, GlossaryJSON)
- `workflow/internal/translation/skill.go` — defaultStaticRules (24개 규칙), overlayStaticRules
- `projects/esoteric-ebb/context/v2_base_prompt.md` — ability-score voice guide (wis/str/int/cha/dex/con)

### 화자 추출
- `workflow/pkg/segmentchunk/chunker.go` — SpeakerHint 필드, speaker 기반 청크 분할
- `workflow/internal/semanticreview/reader.go` — SpeakerHint 읽기, isSpeakerTag 로직 참조

### 재번역 인프라
- `workflow/internal/contracts/v2pipeline.go` — V2PipelineItem (ScoreFinal 필드), 파이프라인 상태 상수
- `workflow/internal/v2pipeline/worker.go` — translate/format/score worker 루프
- `workflow/internal/v2pipeline/export.go` — BuildV3Sidecar (entries[] dedup 로직)

### 리서치 산출물
- `.planning/research/SUMMARY.md` — v1.1 리서치 종합
- `.planning/research/PITFALLS.md` — P1(클러스터 깨짐), P2(speaker 오탐), P4(sidecar dedup), P6(score 한계)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `BuildBaseWarmup()` — 워밍업 조립 함수, voice guide 통합 포인트
- `BuildScriptPrompt()` — per-batch 프롬프트 빌더, voice/branch/continuity 주입 확장점
- `BuildContentSuffix()` — 콘텐츠 타입별 suffix, 새 섹션 추가 패턴으로 재사용
- `StatePendingRetranslate` — 기존 파이프라인 상태 머신에 이미 정의된 재번역 상태
- `ScoreFinal` — pipeline items에 이미 존재하는 품질 점수 필드
- `lore.go` 패턴 — JSON 로드 → term 매칭 → 프롬프트 주입 (Phase 07 voice card에 재사용)

### Established Patterns
- 프롬프트 빌더: warmup (세션 초기화) + per-batch (배치별 동적 콘텐츠) 2레이어 구조
- DB 상태 머신: lease-based worker claims, 상태 전이로 파이프라인 제어
- Go CLI 패턴: `cmd/go-*/main.go` → flag 파싱 → config 로드 → domain `Run()`

### Integration Points
- `clustertranslate/prompt.go`에 섹션 구조 추가 (v2StaticRules 계층화)
- `pipeline_items_v2` 테이블에 `retranslation_gen` 컬럼 추가
- 새 CLI: `cmd/go-retranslate-select/main.go` (score 기반 재번역 후보 선택)
- `v2_base_prompt.md`에서 ability-score voice 섹션 추출 → per-batch 주입 로직

</code_context>

<specifics>
## Specific Ideas

- 재번역은 클러스터 경로로만 실행 — v1.0에서 40,067건 전량이 클러스터로 처리된 검증된 경로
- ability-score voice guide가 이미 `v2_base_prompt.md`에 상세히 정의되어 있음 (wis: "침착하고 달관한 어조", str: "직선적이고 단순한 문장" 등) — 이를 구조적으로 활용
- 리서치 P1 pitfall: 개별 라인 재번역은 절대 금지, batch_id 단위 필수

</specifics>

<deferred>
## Deferred Ideas

- Score LLM 프롬프트 개선 (맥락 인식 점수) — v2 요구사항 SCORE-01, SCORE-02
- 고유명사 정책 통일 — v2 요구사항 NAMING-01, NAMING-02
- 미번역 154건 해소 — Out of Scope (별도 작업)

</deferred>

---

*Phase: 06-foundation-cli*
*Context gathered: 2026-04-06*
