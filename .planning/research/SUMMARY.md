# Project Research Summary

**Project:** Esoteric Ebb 한국어 번역 파이프라인 v1.1 — Context-Aware Retranslation
**Domain:** Game localization pipeline quality improvement (Ink-based cRPG, Korean)
**Researched:** 2026-04-06
**Confidence:** HIGH

## Executive Summary

v1.1은 신규 기술 스택이 필요한 프로젝트가 아니다. Go 1.24, PostgreSQL 17, OpenCode LLM 백엔드로 구성된 현재 스택이 그대로 유지되며, 외부 의존성 추가 없이 기존 코드베이스 위에서 데이터 풍부화(data enrichment)와 프롬프트 엔지니어링으로 번역 품질을 개선한다. 핵심 작업은 세 가지다: (1) Python 추출기에서 speaker/branch 데이터를 더 풍부하게 추출하고 DB에 저장, (2) `clustertranslate/prompt.go`의 번역 프롬프트를 계층적 구조로 재설계하여 화자 프로필과 브랜치 컨텍스트를 주입, (3) 점수 기반으로 저품질 항목을 선별하여 개선된 프롬프트로 재번역하는 CLI 도구 추가.

현재 40,067건 중 대부분은 품질 점수 90.7 평균으로 이미 양호하다. 전체 재번역(75시간 LLM 시간)은 불필요하며, 임계값 미만(예: score < 7.0) 항목만 선별적으로 재번역하는 것이 핵심 전략이다. 이를 가능하게 하는 `StatePendingRetranslate` 상태 머신과 `ScoreFinal` 컬럼이 이미 존재하며, 누락된 것은 "점수 기반 재큐잉" CLI 하나뿐이다. 재번역 단위는 반드시 개별 라인이 아닌 전체 `batch_id` 클러스터여야 한다 — 부분 재번역은 씬 내 말투 불일치를 유발하는 핵심 위험이다.

최대 위험 요소는 두 가지다: (1) `isSpeakerTag()` 휴리스틱이 게임 커맨드 태그를 화자로 오인할 수 있어, 화자 허용 목록(allow-list) 수동 검증이 tone consistency 구현 전에 필수다. (2) 브랜치 컨텍스트를 무제한 주입하면 프롬프트가 폭발적으로 커져 품질이 오히려 하락하므로, 즉각 상위 브랜치 1단계로 컨텍스트를 엄격히 제한해야 한다.

## Key Findings

### Recommended Stack

기존 스택 그대로 유지 — `go.mod` 변경 없음. v1.1은 Go 표준 라이브러리(`encoding/json`, `regexp`, `strings`)로 모든 신규 로직을 구현한다. PostgreSQL에는 `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`로 2-3개 컬럼만 추가(마이그레이션 도구 불필요). Python 추출기(`extract_assetripper_textasset.py`)는 speaker 전파 및 branch 인덱스 생성을 위해 소폭 강화되나 신규 패키지는 없다.

신규로 생성되는 것은 코드가 아닌 **데이터 파일 두 개**다: `character_voices.json`(화자별 톤 프로필)과 DB 스키마 컬럼 추가(`retranslation_gen`, `retranslation_reason`). 이외 모든 기능은 기존 코드 경로 수정으로 구현된다.

**Core technologies (변경 없음):**
- Go 1.24.0: 파이프라인 워커, CLI, 프롬프트 빌더 — 신규 외부 의존성 없음
- PostgreSQL 17: 파이프라인 상태 저장소 — 컬럼 2개 추가, 스키마 구조 유지
- OpenCode (gpt-5.4): 번역 LLM — 프롬프트 변경만, 모델 교체 없음
- OpenCode (codex-mini): 태그 복원 — 변경 없음
- Python 3.x: ink JSON 추출기 강화 — 표준 라이브러리만 사용

### Expected Features

**Must have (table stakes — v1.1 품질 개선의 전제 조건):**
- **프롬프트 감사 및 재구조화** — 현재 `defaultStaticRules()`의 24개 플랫 규칙 목록을 계층적 섹션(컨텍스트/음성/태스크/제약)으로 재편. 최고 ROI, 최저 위험. 다른 모든 기능이 프롬프트 구조에 의존함
- **선택적 재번역 MVP** — `ScoreFinal < threshold` 항목 쿼리 + `pending_retranslate` 재큐잉 CLI. 이 없이는 개선된 프롬프트를 효율적으로 적용할 방법이 없음
- **화자 추출 커버리지 감사** — `pipeline_items_v2`에서 `DISTINCT speaker` 수동 검증, allow-list 생성. tone consistency의 데이터 품질 게이트

**Should have (차별화 품질 향상):**
- **브랜치 컨텍스트 주입** — "Player chose: X" 프롬프트 컨텍스트. 현재 `ChoiceBlock` 필드 존재하나 데이터 미인입. 복잡성 높음(ink 트리 순회 필요)
- **톤 일관성 프로필** — `character_voices.json` 로드 후 해당 배치의 화자에 한해 per-batch 프롬프트에 주입. `lore.go` 패턴 재사용
- **연속성 프롬프트 윈도우 확장** — prev/next 1줄 → 3줄. 재번역 시 기존 한국어 번역을 컨텍스트로 제공. 인프라 거의 완비

**Defer (v1.2+):**
- Score LLM 프롬프트 개선 (변경 시 기존 ScoreFinal 값 무효화)
- 글로서리 퍼지 매칭 확장
- 시맨틱 리뷰 파이프라인과 재번역 자동 연계

### Architecture Approach

v1.1 아키텍처는 기존 v2 파이프라인의 **수정(modify)**이며 신규 파이프라인이 아니다. 주요 변경은 `clustertranslate/prompt.go`의 `BuildScriptPrompt()` 강화와 `contracts/v2pipeline.go`에 메서드 2-3개 추가로 집중된다. 새로운 상태 머신 단계나 워커 역할은 추가되지 않는다 — 재번역은 기존 `pending_translate` 상태로 리셋하고 기존 TranslateWorker가 처리한다.

**Major components:**
1. `inkparse/` (변경 없음) — ink JSON 파싱, 화자 추출, 배치 구성
2. `clustertranslate/prompt.go` (수정) — 번역 프롬프트 빌더. 브랜치 컨텍스트, 화자 프로필, 씬 헤더 주입
3. `clustertranslate/types.go` (수정) — `ClusterTask`에 `BranchContext`, `ActiveSpeakers`, `SceneHeader` 필드 추가
4. `contracts/v2pipeline.go` (수정) — `QueryByScoreRange()`, `ResetForRetranslation()`, `GetBranchContext()` 인터페이스 추가
5. `v2pipeline/store.go` (수정) — 신규 쿼리/리셋 메서드 구현, 스키마 컬럼 추가
6. `cmd/go-retranslate-select/` (신규) — 점수/화자/content-type 기반 재번역 후보 선택 CLI
7. `projects/esoteric-ebb/context/character_voices.json` (신규 데이터 파일) — 화자별 성격 프로필

### Critical Pitfalls

1. **개별 라인 재번역으로 씬 일관성 파괴** — 재번역 단위는 반드시 `batch_id` 전체 클러스터. 개별 라인 재번역은 동일 씬 내 말투 불일치를 만든다. Phase 1 설계에서 규칙 확립 필수.

2. **isSpeakerTag 휴리스틱 오인식** — 게임 커맨드 태그("Minor", "Crowns")가 화자로 잘못 추출될 수 있음. tone consistency 구현 전에 `DISTINCT speaker` 수동 검증 + allow-list 생성 필수.

3. **브랜치 컨텍스트 폭발** — ink DAG 구조의 재귀 탐색 시 프롬프트 크기가 지수적으로 증가. 즉각 상위 브랜치 1단계로 엄격히 제한, 현재 기준 최대 20% 토큰 증가 예산.

4. **사이드카 중복 제거 비일관성** — 재번역 후 `BuildV3Sidecar()`의 first-seen 로직이 구버전 번역을 선택할 수 있음. `entries[]` dedup을 first-seen → highest-score 방식으로 수정 필요, 재번역 실행 전에.

5. **프롬프트 크기 회귀** — 화자 프로필 + 브랜치 컨텍스트 + 글로서리 중첩 시 LLM 어텐션 분산으로 단순 라인 품질 하락. content_type별 컨텍스트 게이팅(dialogue만 전체 컨텍스트), 100개 항목 A/B 테스트로 회귀 감지.

## Implications for Roadmap

연구 전반에 걸쳐 공통된 의존성 순서가 명확하다: 프롬프트 구조 → 화자 데이터 → 컨텍스트 주입 → 재번역 실행. 이 순서를 역행하면 비효율적이거나 실제 품질 저하를 초래한다.

### Phase 1: Foundation — 프롬프트 재구조화 + 화자 검증 + 선택적 재번역 CLI

**Rationale:** 다른 모든 기능의 전제 조건. 프롬프트 구조가 확립되지 않으면 컨텍스트 주입이 오히려 노이즈가 된다. 화자 allow-list 없이 tone profiles를 만들면 잘못된 화자에 규칙이 적용된다. 재번역 CLI 없이 개선된 프롬프트를 실제 적용할 방법이 없다.

**Delivers:** 측정 가능한 품질 개선의 최소 기반. 완료 후 A/B 점수 비교로 Phase 2 진행 가치를 검증 가능.

**Addresses:**
- 프롬프트 감사 및 재구조화 (P1 feature)
- 화자 추출 커버리지 감사 + allow-list (P1 feature)
- 선택적 재번역 MVP: `go-retranslate-select` CLI (P1 feature)
- DB 스키마: `retranslation_gen`, `retranslation_reason`, `idx_pv2_score` 추가

**Avoids:**
- Pitfall 2 (화자 오인식) — allow-list 생성이 이 Phase의 명시적 산출물
- Pitfall 6 (잘못된 재번역 후보 선택) — 컨텍스트 인식 점수 방법론을 이 Phase에서 확립

**Research flag:** 표준 패턴 — 기존 코드 수정 위주, 추가 연구 불필요

---

### Phase 2: Context Enrichment — 톤 프로필 + 브랜치 컨텍스트 + 연속성 윈도우

**Rationale:** Phase 1의 프롬프트 구조와 화자 데이터 위에서만 의미가 있다. 화자 allow-list 없이 tone profiles를 주입하면 잘못된 라인에 적용된다. 브랜치 컨텍스트는 명확한 프롬프트 섹션이 있어야 "착지"한다.

**Delivers:** 개별 라인 품질을 넘어 씬 단위 일관성 향상. 특히 선택지가 많은 장면(tunnels, hubs)에서 효과.

**Addresses:**
- 톤 일관성 프로필: `character_voices.json` 데이터 파일 + `lore.go` 패턴 재사용
- 브랜치 컨텍스트 주입: `GetBranchContext()` + `ClusterTask.BranchContext`
- 연속성 프롬프트 윈도우: prev/next 1줄 → 3줄

**Avoids:**
- Pitfall 3 (브랜치 컨텍스트 폭발) — 토큰 예산 설정 + 1단계 상위 브랜치만 포함
- Pitfall 5 (단조로운 캐릭터 목소리) — 10개 라인 다양성 테스트 후 bulk 적용
- Pitfall 7 (프롬프트 크기 회귀) — content_type 기반 컨텍스트 게이팅, A/B 테스트

**Research flag:** 브랜치 컨텍스트 SQL 쿼리 설계에 주의 필요 (knot-gate-choice 계층 구조 쿼리). `GetBranchContext()` 구현 전 DB 스키마와 기존 쿼리 패턴 재검토 권장.

---

### Phase 3: Retranslation Execution — 재번역 실행 + 사이드카 수정 + 검증

**Rationale:** Phase 1+2가 완료된 후에만 실행. 개선되지 않은 프롬프트로 재번역하면 동일한 품질의 결과만 나온다. 사이드카 dedup 로직은 재번역 전에 수정해야 한다.

**Delivers:** 실제 게임 패치 품질 향상. 저품질 항목 재번역 + 새 `translations.json` 익스포트.

**Addresses:**
- `BuildV3Sidecar()` dedup 로직 수정 (first-seen → highest-score)
- `go-retranslate-select`로 후보 선택 (score < 7.0 기준)
- 전체 batch_id 단위 재번역 실행
- 인게임 검증

**Avoids:**
- Pitfall 1 (클러스터 일관성 파괴) — batch_id 단위 선택 강제
- Pitfall 4 (사이드카 dedup 비일관성) — 익스포트 로직 먼저 수정 후 재번역

**Research flag:** 표준 패턴 — 기존 파이프라인 실행 + 검증. 추가 연구 불필요.

---

### Phase Ordering Rationale

- **프롬프트 구조가 먼저인 이유:** `BuildScriptPrompt()`의 구조가 브랜치 컨텍스트와 화자 프로필이 "착지"할 섹션을 제공. 구조 없이 데이터 주입 시 LLM이 무시하거나 혼동.
- **화자 검증이 톤 프로필 전에 오는 이유:** `character_voices.json`을 만들어도 `isSpeakerTag` 오인식으로 잘못된 화자에 규칙이 적용되면 역효과. Allow-list가 데이터 품질 게이트.
- **사이드카 수정이 재번역 전에 오는 이유:** 재번역 후 dedup 수정하면 일부 항목의 구버전 번역이 `entries[]`에 남는 상태가 발생. 수정이 먼저여야 한다.
- **각 Phase가 검증 가능한 단위인 이유:** Phase 1 완료 → 소규모 재번역 A/B 테스트로 효과 측정 → Phase 2 진행 여부 결정. 이 검증 없이 Phase 2+3은 가치가 검증되지 않은 상태.

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 2 (브랜치 컨텍스트):** ink knot-gate-choice 계층 구조에서 parent choice 텍스트 추출 SQL이 비자명함. `pipeline_items_v2` 스키마에서 실제로 gate와 choice가 어떻게 저장되었는지 재검토 필요. `GetPrevGateLines()` 기존 구현 방식 참고.

Phases with standard patterns (skip research-phase):
- **Phase 1 (프롬프트 재구조화 + CLI):** 기존 `lore.go`, `skill.go` 패턴을 직접 따름. 화자 allow-list는 SQL 한 줄. 재번역 CLI는 기존 store 메서드 조합.
- **Phase 3 (재번역 실행):** 기존 파이프라인 워커 그대로 실행. 사이드카 수정은 정렬 키 변경으로 간단.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | 직접 코드 검사 기반. `go.mod` 확인, 신규 의존성 불필요 확인. 버전 호환성 검증 완료 |
| Features | HIGH | 기존 코드베이스 직접 분석. 기존 필드/상태/인프라 80% 확인 완료. 우선순위 명확 |
| Architecture | HIGH | 모든 관련 패키지 직접 코드 분석. 인터페이스 경계, 데이터 플로우 검증 완료 |
| Pitfalls | HIGH | 코드 분석 + v1.0 사후 분석 + ink 포맷 지식 기반. 7개 구체적 함정 식별 |

**Overall confidence:** HIGH

### Gaps to Address

- **화자 커버리지 실제 규모:** `pipeline_items_v2`에서 `DISTINCT speaker` 실행하기 전까지 실제 coverage gap과 오인식 규모를 정확히 알 수 없음. Phase 1 착수 시 첫 번째 작업으로 이 쿼리 실행 필수.
- **브랜치 구조 실제 분포:** ink 브랜치가 얼마나 깊게 중첩되는지(최대 depth)를 쿼리로 확인해야 컨텍스트 예산 설정 가능. `gate`, `choice` 컬럼 값 분포 분석 필요.
- **재번역 후보 규모:** `score_final < 7.0` 조건의 실제 항목 수를 Phase 1 전에 확인해야 Phase 3 예상 LLM 시간 산정 가능.
- **컨텍스트 인식 점수 방법론:** 현재 `BuildScorePrompt`는 격리 점수만 제공. 재번역 후보 선택의 정확도를 높이려면 "씬 컨텍스트 포함 채점" 방식이 필요하나, 이것이 기존 점수 체계와 얼마나 다른 결과를 낼지는 실험 전까지 불확실.

## Sources

### Primary (HIGH confidence — 직접 코드 분석)

- `workflow/internal/translation/` (skill.go, prompts.go, batch_builder.go, normalized_input.go, types.go, lore.go) — 프롬프트 빌더, 기존 enrichment 패턴
- `workflow/internal/clustertranslate/` (prompt.go, types.go, parser.go) — BuildScriptPrompt, ClusterTask
- `workflow/internal/v2pipeline/` (worker.go, store.go, postgres_v2_schema.sql) — 파이프라인 상태 머신
- `workflow/internal/contracts/v2pipeline.go` — V2PipelineStore 인터페이스
- `workflow/internal/inkparse/parser.go` — isSpeakerTag 휴리스틱
- `workflow/internal/v2pipeline/export.go` — BuildV3Sidecar dedup 로직
- `workflow/internal/scorellm/prompt.go` — BuildScorePrompt 격리 채점
- `projects/esoteric-ebb/context/v2_base_prompt.md` — 현재 프롬프트 구조, ability score 음성 가이드
- `projects/esoteric-ebb/patch/tools/extract_assetripper_textasset.py` — infer_speaker_hint, flush_segment
- Memory: `project_translation_quality.md` — 컨텍스트 품질 이슈 근원 기록

### Secondary (MEDIUM confidence — 외부 자료)

- [Dink: A Dialogue Pipeline for Ink](https://wildwinter.medium.com/dink-a-dialogue-pipeline-for-ink-5020894752ee) — Ink speaker 메타데이터 추출 패턴
- [Localizing Ink with Unity](https://johnnemann.medium.com/localizing-ink-with-unity-42a4cf3590f3) — Ink 로컬라이제이션 과제
- [Ink WritingWithInk documentation](https://github.com/inkle/ink/blob/master/Documentation/WritingWithInk.md) — 공식 태그 문법, 브랜치 구조
- [Game Localization QA Guide](https://www.transphere.com/guide-to-game-localization-quality-assurance/) — 품질 임계값 업계 관행
- [AI Prompt Engineering for Localization 2024](https://custom.mt/ai-prompt-engineering-for-localization-2024-techniques/) — 로컬라이제이션 특화 프롬프트 패턴

---
*Research completed: 2026-04-06*
*Ready for roadmap: yes*
