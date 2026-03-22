# Esoteric Ebb 한국어 번역 파이프라인 v2

## What This Is

Esoteric Ebb(내러티브 cRPG, Ink 스크립트 기반)의 한국어 번역 파이프라인 v2. v1에서 발견된 근본 문제 — 소스 생성 단위와 게임 렌더링 단위의 불일치 — 를 해결하기 위해 ink JSON 트리를 대사 블록 단위로 파싱하고, 씬 단위 클러스터 번역 + 포맷터 LLM으로 태그를 복원하는 2단계 아키텍처. 40,067건 전량 재번역.

## Core Value

게임이 실제로 렌더링하는 대사 블록 단위로 소스를 생성하여, 태그 깨짐 없이 한국어 패치가 동작해야 한다.

## Requirements

### Validated

- ✓ Go CLI 파이프라인 프레임워크 (cmd/internal/contracts/platform 레이어) — v1 existing
- ✓ PostgreSQL 기반 파이프라인 상태 관리 — v1 existing
- ✓ OpenCode LLM 백엔드 연동 (gpt-5.4) — v1 existing
- ✓ BepInEx TranslationLoader 플러그인 (Plugin.cs) — v1 existing
- ✓ 패치 출력 포맷 (translations.json, textassets/, localizationtexts/, runtime_lexicon.json) — v1 existing
- ✓ 프로젝트별 설정 (project.json) — v1 existing
- ✓ ink JSON 트리 파서 — 대사 블록 단위로 소스 생성, 분기 구조 보존. Validated in Phase 01: source-parser

### Active
- [ ] 씬 단위 클러스터 번역 — 태그 없이 스크립트 형식으로 번역
- [ ] 포맷터 LLM (codex-mini) — 번역 결과에 태그 복원
- [ ] 콘텐츠 유형별 입력 설계 — 대사/주문/UI/아이템/시스템 각각 최적 형식
- [ ] Plugin.cs 매칭 로직 최적화 — 대사 블록 단위 매칭, 불필요한 폴백 제거
- [ ] 패치 출력 생성 — v2 소스 형식에 맞는 translations.json + textassets 생성
- [ ] 전량 재번역 실행 — 40,067건+ v2 파이프라인 통과
- [ ] 게임 내 검증 — 패치 적용 후 태그 깨짐 없이 한국어 표시

### Out of Scope

- 패치 빌드 스크립트 수정 (BepInEx/doorstop/fonts 보존 문제) — v2 이후 별도 처리
- 새로운 게임 버전 대응 — 현재 1.1.3 기준
- 다른 언어 지원 — 한국어 전용
- 웹 UI/대시보드 — CLI 파이프라인 유지

## Context

### v1 근본 문제
ink JSON의 `"^text"` 엔트리를 개별 번역 아이템으로 분리했으나, 게임 엔진은 여러 `"^"` 엔트리를 합쳐서 하나의 대사 블록으로 렌더링. translations.json에 분리된 형태로 저장 → 게임이 보내는 합쳐진 형태와 매칭 실패 → 플러그인이 세그먼트별 부분 치환 시도 → 태그가 엉뚱한 위치에 남아 bold 누출.

### v2 실험 결과 (검증 완료)
1. **클러스터 번역**: 8줄 씬 스크립트 → 8/8 완벽 매핑
2. **분기 번역**: Snell_Companion 16줄 분기 포함 → BRANCH/OPTION 구조 보존, 톤 일관
3. **포맷터 LLM**: codex-mini 4개 항목 태그 복원 → 4/4 태그 수 완벽 일치
4. **태그 복원 품질**: `<b><color=#5782FD>INTELLIGENCE</color></b>` → `<b><color=#5782FD>지능</color></b>` 정확

### v2 아키텍처
```
ink JSON tree
  ↓ [코드] 트리 파싱 → 분기 구조 보존한 스크립트
  ↓ [LLM: gpt-5.4] 스크립트 통째 번역 (태그 없이, 자유 형식)
  ↓ [코드] 라인 분리 → ID 매핑
  ↓ [LLM: codex-mini] 태그 복원 (필요한 라인만)
  ↓ [코드] 최종 검증 (태그 수만 확인)
```

### ink JSON 구조
- `root[-1]`에 named knots (씬 단위)
- 각 knot: `g-N` (게이트), `c-N` (선택지 분기)
- 텍스트: `"^text"` 엔트리, 태그: `"#"` 뒤에 speaker/DC 체크
- TextAsset 파일 286개, contextual_entries 71,787개

### 콘텐츠 유형별 입력 설계
| 유형 | 묶는 기준 | 배치 크기 | 형식 |
|---|---|---|---|
| 대사 (dialogue, narration, reaction, choice) | 씬/트리 분기 | 10~30줄 | 스크립트 |
| 주문/tooltip | 카테고리 (주문 레벨, 학파) | 5~10개 | 구조화 카드 |
| UI 라벨 | 메뉴/화면 단위 | 50~100개 | 사전 |
| 아이템/퀘스트 설명 | 아이템 종류 | 5~10개 | 카드+맥락 |
| 시스템/튜토리얼 | 화면/섹션 | 전체 | 문서 |

### 플러그인 매칭 체인 (v1)
1. TranslationMap 직접 매칭
2. TryTranslateDecorated (장식 태그 제거 후 매칭)
3. NormalizedMap (모든 태그 제거, 공백 정규화)
4. TryTranslateContextual (문맥 기반)
5. TryTranslateRuntimeLexicon (substring/regex 치환)
6. TryTranslateEmbedded (임베디드 텍스트)
7. TryTranslateTagSeparatedSegments ← **v1 문제의 원인, v2에서 제거/교체**

### 기술 환경
- Go 1.24, Python 3.x, C# (BepInEx 플러그인)
- PostgreSQL 5433 (localize_agent)
- OpenCode 서버 127.0.0.1:4112 (openai/gpt-5.4, codex-mini)
- 게임: E:\SteamLibrary\steamapps\common\Esoteric Ebb

## Constraints

- **LLM 백엔드**: OpenCode 서버 (gpt-5.4 번역, codex-mini 포맷팅) — 로컬 인프라, 외부 API 없음
- **게임 버전**: 1.1.3 고정, ink JSON 구조 변경 없음
- **기존 코드**: v1 파이프라인 프레임워크(contracts, platform, shared) 위에 구축
- **DB 규칙**: source_raw 기준 중복 체크 필수, 맹목적 INSERT 금지
- **패치 포맷**: BepInEx TranslationLoader 호환 (translations.json sidecar 방식)

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| 전량 재번역 (v1 결과 폐기) | v1 소스 단위가 근본적으로 잘못됨, 부분 패치 불가 | — Pending |
| 2단계 LLM (번역 + 포맷팅 분리) | LLM이 태그를 변형하는 문제 해결, 실험으로 검증 완료 | ✓ Good |
| 씬 단위 클러스터 번역 | 문맥 보존 + 분기 구조 포함, 실험으로 검증 완료 | ✓ Good |
| 빌드 스크립트 수정은 v2 밖 | 파이프라인 집중, 빌드 문제는 별도 마일스톤 | — Pending |
| Plugin.cs 매칭 로직 최적화 포함 | 대사 블록 단위 소스에 맞게 폴백 체인 정리 필요 | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd:transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd:complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-03-23 after Phase 02 completion — translation engine (cluster translate, tag format, score LLM, pipeline orchestrator) complete*
