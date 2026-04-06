# Phase 07: Context Enrichment — 톤 프로필 + 분기 맥락 + 연속성 윈도우 - Context

**Gathered:** 2026-04-07
**Status:** Ready for planning

<domain>
## Phase Boundary

씬 단위 번역 일관성 향상: 캐릭터별 말투 유지, 선택지 분기 맥락 전달, 주변 대사 윈도우 확장으로 LLM이 문맥을 충분히 인지. Phase 06에서 확립한 4-tier 프롬프트 구조(Context/Voice/Task/Constraints)에 컨텍스트를 주입하는 단계.

Requirements: TONE-01, TONE-02, BRANCH-01, BRANCH-02, BRANCH-03, CONT-01, CONT-02

</domain>

<decisions>
## Implementation Decisions

### Voice Card 설계
- **D-01:** named 캐릭터용 voice card를 LLM 자동 생성. 게임 대사 DB에서 캐릭터별 샘플을 추출하고, LLM이 말투/존댓말/성격을 분석하여 JSON voice card 자동 생성.
- **D-02:** voice card 필드는 기본 3필드: 말투(화법 스타일), 존댓말 레벨(반말/평어/존대), 성격 키워드. ability-score voice guide와 동일한 구조.
- **D-03:** 상위 15명(100회+ 등장) 캐릭터에 대해서만 voice card 생성. Snell(2663)~Thal(100) 범위. 나머지는 voice card 없이 기존 범용 규칙 적용.

### 분기 맥락 주입
- **D-04:** ink JSON 파서(inkparse) 확장으로 choice container의 부모 선택지 텍스트를 추출. DialogueBlock에 ParentChoiceText 필드 추가. 소스 준비 단계에서 해결.
- **D-05:** 브랜치 깊이 1단계 제한 (로드맵 Success Criteria 준수). 토큰 예산 내 유지.

### 연속성 윈도우 확장
- **D-06:** prev/next 3줄 슬라이딩 윈도우로 확장 (로드맵 기준). 현재 PrevGateLines 3줄에서 양방향으로 확장.
- **D-07:** 재번역 시 prevKO/nextKO를 DB ko_formatted 조회로 채움. prev/next line_id로 해당 아이템의 ko_formatted를 조회. 이미 store.go에 의존성 조회 로직 존재.

### 토큰 예산 + A/B 테스트
- **D-08:** 토큰 예산 초과 시 우선순위: voice card(가장 중요, 마지막 삭제) > branch context > continuity window(가장 먼저 삭제). 낮은 우선순위부터 제거.
- **D-09:** A/B 테스트는 저품질 배치 10개를 컨텍스트 주입 전/후로 번역하여 score 비교. 프롬프트 크기 회귀 없음 확인.

### Claude's Discretion
- voice card JSON 파일 저장 위치 및 로드 방식 (lore.go 패턴 재사용 가능)
- 분기 맥락의 프롬프트 주입 위치 ([CONTEXT] 블록 vs Voice 섹션)
- 토큰 예산 상한값 (프로파일링 결과 기반 결정)
- A/B 테스트 배치 선택 기준 (score_final 분포 기반)
- voice card 생성용 LLM 프롬프트 설계
- continuity window의 next 라인 처리 (최초 번역 시 미번역 상태)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### 프롬프트 시스템
- `workflow/internal/clustertranslate/prompt.go` — v2Sections 4-tier 구조, BuildBaseWarmup, BuildScriptPrompt, buildVoiceSection, abilityScoreVoice 맵
- `workflow/internal/clustertranslate/types.go` — ClusterTask (Batch, PrevGateLines, GlossaryJSON), PromptMeta (EstimatedTokens)
- `projects/esoteric-ebb/context/v2_base_prompt.md` — ability-score voice guide (wis/str/int/cha/dex/con 한국어 설명)

### 화자 데이터
- `projects/esoteric-ebb/context/speaker_allow_list.json` — 검증된 화자 목록 + 빈도 (상위 15명: Snell~Thal)
- `workflow/internal/inkparse/speaker_allowlist.go` — speaker allow-list 필터 로직

### ink 파서 (분기 맥락 확장 대상)
- `workflow/internal/inkparse/types.go` — DialogueBlock 구조체 (Choice 필드 존재, ParentChoiceText 미존재)
- `workflow/internal/inkparse/batcher.go` — Batch 구조, BuildBatches

### DB 연속성
- `workflow/internal/v2pipeline/store.go` — prev_line_id/next_line_id 조회, ko_formatted 의존성 로직
- `workflow/internal/contracts/v2pipeline.go` — V2PipelineItem (ScoreFinal, Speaker 필드)

### 리서치 산출물
- `.planning/research/SUMMARY.md` — v1.1 리서치 종합
- `.planning/research/PITFALLS.md` — P1(클러스터 깨짐), P2(speaker 오탐)

### Phase 06 컨텍스트
- `.planning/phases/06-foundation-cli/06-CONTEXT.md` — D-01~D-10 결정사항, 프롬프트 계층화, 재번역 CLI

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `abilityScoreVoice` 맵 + `buildVoiceSection()`: named 캐릭터 voice card 주입에 동일 패턴 재사용 가능
- `PrevGateLines` [CONTEXT] 블록 주입: continuity window 확장의 기반
- `lore.go` JSON 로드 → term 매칭 → 프롬프트 주입 패턴: voice card 로딩에 재사용
- `estimateTokens()`: 토큰 예산 관리에 활용
- `prev_line_id`/`next_line_id` DB 조회 (store.go): prevKO/nextKO 조회에 활용

### Established Patterns
- 프롬프트 빌더: warmup (세션 초기화) + per-batch (배치별 동적 콘텐츠) 2레이어 구조
- v2Sections 4-tier: Context/Voice/Task/Constraints — 새 컨텍스트가 각 섹션에 삽입
- JSON 파일 기반 컨텍스트 로딩 (glossary, lore, speaker_allow_list)

### Integration Points
- `BuildScriptPrompt()`: voice card, branch context, continuity window 주입 확장점
- `ClusterTask` 구조체: 새 필드 추가 (VoiceCard, ParentChoiceText, PrevKO/NextKO 등)
- `DialogueBlock`: ParentChoiceText 필드 추가 (inkparse 확장)
- `store.go`: ko_formatted 조회 쿼리 추가 (재번역 시 prevKO/nextKO)

</code_context>

<specifics>
## Specific Ideas

- ability-score voice는 이미 v2_base_prompt.md에 상세 설명 + 예시 포함 — named 캐릭터 voice card도 동일한 수준의 한국어 설명 필요
- voice card 자동 생성 시 게임 대사 샘플에서 말투 패턴을 추출하는 것이 핵심 — 캐릭터별 10~20개 대사 샘플이면 충분
- 재번역 시 prevKO/nextKO가 있으면 톤 일관성이 크게 향상됨 — 기존 번역과 어울리도록 LLM이 맥락 인지

</specifics>

<deferred>
## Deferred Ideas

None — 논의가 phase scope 내에서 진행됨

</deferred>

---

*Phase: 07-context-enrichment*
*Context gathered: 2026-04-07*
