# Phase 4: 플러그인 최적화 & 게임 검증 - Context

**Gathered:** 2026-03-26
**Status:** Ready for planning

<domain>
## Phase Boundary

Plugin.cs 매칭 로직을 대사 블록 단위에 최적화하고, 게임 내에서 태그 깨짐 없이 한국어가 표시됨을 확인한다. 매칭 체인을 최소화하고, v2 패치 배포 후 캡처 기반 검증으로 99%+ hit rate를 달성한다.

</domain>

<decisions>
## Implementation Decisions

### V3 sidecar 메타데이터 활용 (PLUGIN-01)
- **D-01:** V3 sidecar에 `contextual_entries[]` 추가. `entries[]`는 중복 source 제거 대표 엔트리(TranslationMap용), `contextual_entries[]`는 전체 35K 엔트리 + 메타데이터(ContextualMap용). export.go의 `BuildV3Sidecar()`만 변경, Plugin.cs 코드 변경 없음 — 이미 `contextual_entries` 파싱 지원.

### 매칭 체인 최소화 (PLUGIN-02)
- **D-02:** 8단계 → 4단계로 축소.
  - **유지:** `GeneratedPattern` → `TranslationMap` → `Contextual` → `RuntimeLexicon`
  - **제거:** `Decorated`, `NormalizedMap`, `Embedded`, `TagSeparatedSegments`
  - **근거:** v2에서 ink 대사는 TextAsset override 경로로 처리되어 TryTranslate를 거치지 않음. UI 텍스트는 LocalizationIdOverrides로 처리. 제거 대상 단계들은 v2 아키텍처에서 활용되는 구체적 근거 없음.

### 게임 검증 전략 (VERIFY-02)
- **D-03:** 특정 씬 샘플로 1차 검증 (인트로, 대화 분기, 능력치 화면, 전투 등). `ENABLE_FULL_CAPTURE` 모드로 캡처 데이터 자동 수집. hit rate 99%+ 목표. 발견된 문제는 Phase 4 안에서 즉시 수정.

### runtime_lexicon.json (PLUGIN-01 연관)
- **D-04:** 게임 검증 캡처 결과를 보고 결정. v2 패치 배포 후 실제 miss 데이터 기반으로 패턴 작성. 고유명사는 glossary에서 처리. 기존 untranslated_capture (1,040건)는 translations_loaded=0 상태 캡처이므로 신뢰 불가 — v2 패치 적용 후 재캡처 필요.

### TextAsset 로딩 확장 (Phase 3 D-06 이행)
- **D-05:** Plugin.cs의 `LoadTextAssetOverrides()`에서 `*.txt` 패턴을 `*.txt` + `*.json`으로 확장. Phase 3에서 textasset 출력이 `.json` 확장자로 결정됨.

### Claude's Discretion
- 검증용 샘플 씬 선정 (다양한 콘텐츠 유형 커버)
- 매칭 체인 제거 시 관련 코드/필드 정리 범위
- 캡처 데이터 분석 자동화 방식
- hit rate 미달 시 수정 전략

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Plugin.cs (수정 대상)
- `projects/esoteric-ebb/patch/mod-loader/EsotericEbb.TranslationLoader/Plugin.cs` — 전체 3,388줄. 매칭 체인(lines 839-915), AddEntry/AddContextualEntry(lines 1180-1455), LoadTextAssetOverrides(lines 1207-1245), TryTranslateDecorated(lines 2212-2242), TryTranslateEmbedded(lines 2244-2277), TryTranslateTagSeparatedSegments, NormalizedMap 관련 코드

### V3 sidecar export (수정 대상)
- `workflow/internal/v2pipeline/export.go` — BuildV3Sidecar(), V3Sidecar/V3Entry 구조체. contextual_entries 추가 필요
- `workflow/internal/v2pipeline/export_test.go` — 기존 테스트, contextual_entries 테스트 추가 필요

### 파이프라인 계약
- `workflow/internal/contracts/v2pipeline.go` — V2PipelineItem DTO (ID, SourceFile, ContentType, Speaker, SourceRaw, KOFormatted)

### 런타임 캡처 (검증 기준 데이터)
- `E:\SteamLibrary\steamapps\common\Esoteric Ebb\BepInEx\full_text_capture.json` — 4,550건 캡처 (origin: tmp_text 2936, menu_scan 984, ink_dialogue 435, ink_choice 195)
- `E:\SteamLibrary\steamapps\common\Esoteric Ebb\BepInEx\untranslated_capture.json` — 1,040건 (translations_loaded=0 상태, v2 배포 후 재캡처 필요)
- `E:\SteamLibrary\steamapps\common\Esoteric Ebb\BepInEx\translation_loader_state.json` — hit/miss 카운터

### 패치 빌드
- `projects/esoteric-ebb/patch/tools/build_patch_package_unified.ps1` — 패치 패키지 빌드
- `E:\SteamLibrary\steamapps\common\Esoteric Ebb\Esoteric Ebb_Data\StreamingAssets\TranslationPatch\` — 패치 배포 위치

### Phase 3 실행 결과
- `.planning/phases/03-patch-output-full-run/03-EXECUTION-LOG.md` — 35,036/35,036 done, 파서 버그/validation 전략 변경 등 교훈
- `.planning/phases/03-patch-output-full-run/03-VERIFICATION.md` — 9/10 truths verified, human verification pending

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `Plugin.cs` LoadEntriesFromJson(): 이미 `contextual_entries[]` 파싱 지원 — AddContextualEntry()로 ContextualMap에 로딩
- `Plugin.cs` TryTranslateContextual(): source_file 기반 후보 선택 + 최근 히스토리 8건 기반 스코어링
- `Plugin.cs` ENABLE_FULL_CAPTURE: 파일 마커 기반 전체 캡처 모드 — 검증에 직접 활용
- `export.go` BuildV3Sidecar(): V2PipelineItem → V3Entry 변환. contextual_entries 생성 로직 추가 위치

### Established Patterns
- CLI: `cmd/go-*/main.go` → flag → LoadProjectConfig → domain.Run(Config)
- Plugin.cs: Harmony v2 prefix/postfix 패치, static dictionary 기반 매칭
- 캡처: JSON 파일로 런타임 데이터 수집 → 분석

### Integration Points
- `export.go` 변경 → translations.json 재생성 → 패치 디렉토리 배포
- `Plugin.cs` 변경 → 빌드 → BepInEx/plugins/ 배포
- `LoadTextAssetOverrides()` — `*.json` 패턴 추가로 v2 textasset 파일 로딩
- 검증 루프: 패치 배포 → 게임 실행 → 캡처 수집 → 분석 → 수정 → 반복

</code_context>

<specifics>
## Specific Ideas

- 게임 캡처 분석: full_text_capture에서 origin별 태그 분포 확인됨 — ink_dialogue/ink_choice는 전부 렌더링 래핑 태그 포함, TextAsset override 경로로 처리
- tmp_text 2,633건 중 2,430건이 렌더링+ink 태그 혼합 — 이것도 TextAsset override 경로이므로 TryTranslate 불필요
- build_report.json에 ambiguous_source_count: 2,654 — contextual_entries가 이 동음이의 케이스를 해소
- Plugin.cs에 per-stage hit counter가 없음 (Decorated, Embedded 등). 제거 후에도 디버깅 영향 없음

</specifics>

<deferred>
## Deferred Ideas

- Decorated/Normalized 복원 — 99% 달성 후에도 특정 텍스트가 miss되면 안전망으로 재추가 검토
- 패치 빌드 스크립트 수정 (BepInEx/doorstop 보존) — 프로젝트 범위 밖
- 품질 스코어 기반 선택적 재번역 — 별도 마일스톤
- 고유명사 음역/의역 정책 개선 — 별도 작업

</deferred>

---

*Phase: 04-plugin-optimize-verify*
*Context gathered: 2026-03-26*
