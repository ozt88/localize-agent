# Phase 1: 소스 준비 & 파서 - Context

**Gathered:** 2026-03-22
**Status:** Ready for planning

<domain>
## Phase Boundary

소스 해시를 SHA-256으로 수정하고, ink JSON 트리를 게임 렌더링 단위(대사 블록)로 정확히 파싱하여, 번역 파이프라인의 올바른 소스를 생성한다. 모든 콘텐츠 유형(ink 대사 + UI + 메뉴 + 시스템)을 분류하고 유형별 배칭 형식을 적용한다. 번역 불필요 문자열은 패스스루 처리한다.

</domain>

<decisions>
## Implementation Decisions

### 파서 출력 형식
- **D-01:** Claude 재량. JSON 파일 → DB 인제스트 2단계 또는 DB 직접 삽입 중 최적 방식 선택.
- **D-02:** Claude 재량. 블록 ID는 경로 기반(knot/gate/choice/blk-N) 또는 해시 기반 중 선택.
- **D-03:** Claude 재량. textassets 역삽입용 블록 위치 기록을 Phase 1에서 같이 할지, Phase 3에서 별도로 할지 선택.
- **D-04:** Claude 재량. 파서 모듈 위치는 `workflow/internal/` (공용, rogue-trader 재사용 가능) 또는 `projects/esoteric-ebb/` (전용) 중 선택. rogue-trader가 ink를 사용하면 공용이 유리.

### 메타데이터 설계 (소비자별 분리)
- **D-05:** 메타데이터는 소비자별로 분리 설계:
  - 번역 LLM용: speaker, 트리 위치/분기 구조, 콘텐츠 유형
  - 포맷 LLM용: 원본 리치텍스트 태그 목록, 태그 위치
  - 파이프라인 코드용: 블록 ID, source_hash, 콘텐츠 유형

### 블록 경계 검증 (런타임 캡처 기반)
- **D-06:** 검증 기준은 translations.json이 아니라 **게임 런타임 캡처 데이터** (순수 영문).
  - Plugin.cs에 전수 캡처 모드 추가 완료 (origin 구분: ink_dialogue, ink_choice, tmp_text, menu_scan)
  - 캡처 데이터 확보 완료: `projects/esoteric-ebb/source/full_text_capture_clean.json` (4,550건)
- **D-07:** 게임 엔진은 ink 텍스트에 렌더링 래퍼를 추가 (`<line-indent>`, `<#hex>`, `<size>`, `<smallcaps>`). 파서는 래퍼 없는 순수 텍스트 블록을 생성해야 함. 래퍼는 게임 엔진 영역.
- **D-08:** 검증 전략 — 결정론적 파싱이므로 패턴별 샘플로 검증:
  - ink 런타임 스펙 기반 유닛 테스트 (TDD): "^" 합치기, 태그 포함 합치기, 게이트/선택지 경계, glue 처리
  - 캡처 데이터의 ink_dialogue/ink_choice 원본과 파서 출력 대조
  - 패턴별로 맞으면 전체 286개 파일에 동일 로직 적용 가능

### 파서 아키텍처 (확정)
- **D-20:** knot 진입부 + 게이트(g-N) + 선택지(c-N) 단위 추출. 커버리지 100% 확인 (163,294개 텍스트 엔트리).
- **D-21:** 각 단위 내부에서 연속된 `"^"` 엔트리를 대사 블록으로 조립. 개별 블록 = 패치 출력 단위 (TranslationMap 키).
- **D-22:** 게이트 = 번역 클러스터 경계 (10~30줄 스크립트). knot 진입부도 별도 클러스터.
- **D-23:** ink 런타임 시뮬레이션(A안) 불필요. divert 추적 없이 배열 순회만으로 충분.
- **D-24:** `ev`/`/ev` (evaluation 스택), `str`/`/str` (문자열 빌딩) 구간은 건너뛰기.
- **D-25:** 컨테이너 마지막 원소가 dict이면 메타데이터(`#f`, `#n`) + 서브컨테이너(g-N, c-N). null이면 빈 메타.
- **D-26:** 선택지 `{"*": "path", "flg": N}` — flg 비트필드로 유형 판별 (0x1=조건, 0x2=시작콘텐츠, 0x4=선택지전용, 0x8=보이지않는기본, 0x10=1회용).

### Glue 메커닉 (ink 컴파일 JSON)
- **D-16:** 컴파일된 ink JSON에서 glue는 문자열 `"<>"`로 표현됨.
- **D-17:** glue는 현재 텍스트와 divert 대상의 다음 텍스트를 줄바꿈 없이 하나로 합친다.
  - 예: `"^You answer. "` + `<>` + divert → `"^Another question..."` = `"You answer. Another question from the darkness."`
  - 캡처 데이터로 확인 완료.
- **D-18:** 286개 파일 중 10개만 glue 사용 (총 34개 마커). 빈도는 낮지만 무시 불가.
- **D-19:** 파서가 divert를 따라가면서 glue로 연결된 텍스트를 추적해야 함 — 단순 배열 순회가 아니라 divert 해석 필요. 파서 복잡도 증가 요인.

### 콘텐츠 유형 분류
- **D-09:** Phase 1에서 ink + UI + 메뉴 + 시스템 전체 분류. ink 전용이 아님.
- **D-10:** Claude 재량. ink 내부 콘텐츠 유형 분류 기준 (파일명/구조 패턴/태그 기반) 선택.

### v1 데이터 처리
- **D-11:** 기존 items 테이블 유지 + v2용 새 테이블 생성 (items_v2 등). v1 데이터 보존.
- **D-12:** v2 번역 시 v1 번역 결과 참조 안 함. 완전 독립 재번역.

### 용어집 구축 (Phase 2 연계)
- **D-13:** 용어집 소스 4개 결합:
  1. `GlossaryTerms.txt` (게임 내장, 54+ 용어, 카테고리/DC 포함) — **1순위**
  2. `localizationtexts/` (SheetInfo, Feats, QuestPoints 등)
  3. wiki 크롤링 데이터 (`rag/esoteric_ebb_lore_termbank.json`)
  4. 파서 부산물 (speaker 이름, 고빈도 고유명사 자동 추출)
- **D-14:** Phase 1 파서가 lore 정의 항목 ("term - description" 패턴)과 speaker 이름을 수집해두면 Phase 2에서 활용.

### 설계 원칙
- **D-15:** 코드 = 결정론적 작업 (파싱, 구조 분석, ID 매핑, 검증). LLM은 파서에 관여하지 않음.

### Claude's Discretion
- 파서 출력 형식 (JSON vs DB 직접)
- 블록 ID 생성 방식 (경로 vs 해시)
- textassets 역삽입 위치 기록 시점
- 파서 모듈 위치 (공용 vs 전용)
- ink 내부 콘텐츠 분류 기준

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### ink JSON 구조
- `projects/esoteric-ebb/extract/1.1.3/ExportedProject/Assets/` — 게임 에셋 (TextAsset 286개 포함)
- `.planning/research/ARCHITECTURE.md` — v2 아키텍처 설계, 데이터 흐름

### 런타임 캡처 데이터
- `projects/esoteric-ebb/source/full_text_capture_clean.json` — 순수 영문 캡처 (4,550건, origin 구분)
- `projects/esoteric-ebb/source/full_text_capture.json` — v1 패치 포함 캡처 (참고용)

### 게임 내장 용어집
- `projects/esoteric-ebb/extract/1.1.1/ExportedProject/Assets/Resources/glossaryterms/GlossaryTerms.txt` — 게임 용어 정의 (54+ 항목)

### v1 번역 데이터 (참고용)
- `projects/esoteric-ebb/output/batches/canonical_full_retranslate_dual_score_20260311_1/translations_v1_deploy.json` — v1 translations.json (60,949 entries)

### 기존 코드 패턴
- `workflow/internal/translationpipeline/` — DB 파이프라인 상태 머신
- `workflow/internal/translation/proposal_validation.go` — `isLiteralPassthroughSource()`, `passthroughControlRe`
- `workflow/internal/fragmentcluster/` — v1 씬 단위 그룹핑 (참고)
- `workflow/pkg/platform/checkpoint_store.go` — `UpsertItem()`, source_hash 스키마

### 플러그인
- `projects/esoteric-ebb/patch/mod-loader/EsotericEbb.TranslationLoader/Plugin.cs` — 매칭 체인, 캡처 로직

### 로컬라이제이션 데이터
- `projects/esoteric-ebb/extract/1.1.3/ExportedProject/Assets/StreamingAssets/TranslationPatch/localizationtexts/` — SheetInfo, Feats, Spells, QuestPoints 등

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `isLiteralPassthroughSource()` + `passthroughControlRe`: 패스스루 감지 로직 — v2에서 확장 가능
- `UpsertItem(entryID, status, sourceHash, ...)`: DB 인제스트 인터페이스 — v2 테이블에도 동일 패턴 적용
- `LoadProjectConfig`: 프로젝트별 설정 로딩 — v2 파서 설정도 project.json에 추가
- `fragmentcluster/`: v1의 씬 그룹핑 로직 — 참고용 (v2 파서와 접근이 다름)
- `go-esoteric-adapt-in/`: 기존 소스 인제스트 CLI — v2용 CLI 진입점 패턴 참고

### Established Patterns
- CLI: `cmd/go-*/main.go` → `flag` 파싱 → `shared.LoadProjectConfig` → `domain.Run(Config)`
- 계약: `internal/contracts/` 인터페이스 → `pkg/platform/` 구현
- DB: PostgreSQL via pgx/v5, SQLite via modernc.org/sqlite
- LLM: `SessionLLMClient` (OpenCode), `OllamaClient` — Phase 1에서는 불필요

### Integration Points
- 새 CLI: `workflow/cmd/go-ink-parser/main.go` 또는 `projects/esoteric-ebb/cmd/go-ink-parser/main.go`
- 새 DB 테이블: `items_v2` (기존 `items` 보존)
- 새 계약: 파서 출력 DTO + 스토어 인터페이스 (contracts에 추가)
- 프로젝트 설정: `projects/esoteric-ebb/project.json`에 ink 파서 경로/설정 추가

</code_context>

<specifics>
## Specific Ideas

- 파서 블록 = "게임 엔진이 플러그인에 보내는 문자열에서 렌더링 래퍼를 벗긴 것". 래퍼 (`<line-indent>`, `<#hex>`, `<size>`, `<smallcaps>`)는 게임 엔진이 런타임에 추가.
- TDD 적용: ink 런타임 스펙의 "^" 합치기 규칙을 테스트 케이스로 먼저 작성, 캡처 데이터로 검증.
- v1에서 `<b>COLLECTION</b>` 같은 태그가 분리되어 `"LECTION</b>"` 같은 조각이 된 문제 — v2 파서는 이걸 하나의 블록으로 조립해야 함.
- 캡처 데이터 origin 필드로 "ink 경유 vs UI 경유" 구분 가능 — 콘텐츠 분류의 1차 기준.

</specifics>

<deferred>
## Deferred Ideas

- 용어집 구축 자체는 Phase 2 (TRANS-07) 범위. Phase 1에서는 소스 수집만.
- 패치 빌드 스크립트 수정 (BepInEx/doorstop 보존) — 프로젝트 범위 밖.
- Plugin.cs 전수 캡처 모드 개선 (현재는 v2 검증용으로 충분) — 필요 시 Phase 4.

</deferred>

---

*Phase: 01-source-parser*
*Context gathered: 2026-03-22*
