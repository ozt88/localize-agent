# Phase 3: 패치 출력 & 전량 실행 - Context

**Gathered:** 2026-03-23
**Status:** Ready for planning

<domain>
## Phase Boundary

v2 파이프라인으로 40,067건+ 전량을 처리하고, BepInEx 호환 패치 아티팩트를 생성한다. 아티팩트: translations.json (v2 블록 ID 포맷), 285개 textassets ink JSON (한국어 역삽입), localizationtexts CSV 8개 파일 전량 번역. 패치 생성은 별도 export CLI로 수행하며, 부분 완료 상태에서도 패치 빌드 가능.

</domain>

<decisions>
## Implementation Decisions

### translations.json 포맷 (PATCH-01)
- **D-01:** v2 전용 포맷 (`esoteric-ebb-sidecar.v3`). 각 엔트리에 block ID, source_file, text_role, speaker_hint 포함.
- **D-02:** 동일 source text라도 block ID가 다르면 별도 엔트리로 전부 포함. ID로 고유 식별.
- **D-03:** passthrough 항목(번역 불필요 문자열)도 source=target으로 포함. 완전한 매핑 보장.
- **D-04:** 독립 CLI (`go-v2-export`)로 생성. v2 파이프라인 DB에서 `state=done` 항목 조회하여 출력.

### TextAsset ink JSON 역삽입 (PATCH-02)
- **D-05:** 전체 JSON 재생성 방식. 원본 ink JSON을 파싱한 후 `"^text"` 노드를 한국어(`ko_formatted`)로 교체하여 새로운 JSON 생성.
- **D-06:** 출력 파일 형식은 `.json` (ink JSON 그대로). Plugin.cs의 TextAssetOverrides가 `.text` getter를 후킹하므로 유효한 ink JSON이어야 함. **주의:** 현재 Plugin.cs의 `LoadTextAssetOverrides()`는 `*.txt` 패턴만 스캔하므로 `.json` 파일은 로드되지 않음. Phase 4(PLUGIN-01)에서 Plugin.cs가 `.json`도 스캔하도록 수정 예정.
- **D-07:** 역삽입 검증 전략은 Claude 재량. 구조 보존, 블록 수 일치 등 적절한 검증 구현.

### 전량 실행 운영 전략 (VERIFY-01)
- **D-08:** 실패률 임계치 설정하여 시스템적 문제 조기 감지. 구체적 임계치는 Claude 재량.
- **D-09:** 모니터링 및 진행 보고 방식은 Claude 재량.
- **D-10:** 부분 완료 상태에서도 done 항목만으로 패치 빌드 가능. export CLI가 done 항목만 조회하여 출력.

### localizationtexts CSV (PATCH-03)
- **D-11:** 8개 CSV 파일 전체 전량 번역 (Feats, ItemTexts, JournalTexts, Popups, QuestPoints, SheetInfo, SpellTexts, UIElements).
- **D-12:** CSV 번역의 파이프라인 통합 vs 별도 처리는 Claude 재량. 콘텐츠 유형별 최적 방식 선택.
- **D-13:** runtime_lexicon.json은 Phase 4로 연기. Phase 3에서는 생성하지 않음.

### Claude's Discretion
- 실패률 임계치 수치 설정
- 모니터링/진행 보고 구현 방식
- TextAsset 역삽입 검증 전략 상세
- CSV 번역의 파이프라인 통합 여부 및 방식
- export CLI 필터링 옵션 (content_type별, source_file별 등)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### v2 파이프라인 (번역 소스)
- `workflow/internal/v2pipeline/store.go` — pipeline_items_v2 DB 스키마, 상태 머신 구현, claim/lease 로직
- `workflow/internal/v2pipeline/run.go` — 3-role worker pool 오케스트레이션, 메트릭, graceful shutdown
- `workflow/internal/contracts/v2pipeline.go` — V2PipelineItem DTO, 상태 상수, 라우팅 규칙
- `workflow/cmd/go-v2-pipeline/main.go` — CLI 진입점, flag 파싱, project.json 통합

### ink 파서 (역삽입 기반)
- `workflow/internal/inkparse/types.go` — DialogueBlock 구조체 (ID, SourceFile, Text, SourceHash 등)
- `workflow/internal/inkparse/parser.go` — ink JSON 트리 워킹, `"^text"` 병합 로직 (역삽입 시 동일 로직 역방향 적용)
- `workflow/cmd/go-ink-parse/main.go` — 286개 TextAsset → 40,067 블록 파싱 CLI

### v1 translations.json 포맷 (참고)
- `projects/esoteric-ebb/output/batches/canonical_full_retranslate_dual_score_20260311_1/translations_v1_deploy.json` — v1 sidecar 포맷 (`esoteric-ebb-sidecar.v2`, source/target/text_role/speaker_hint)

### Plugin.cs 로딩 메커니즘
- `projects/esoteric-ebb/patch/mod-loader/EsotericEbb.TranslationLoader/Plugin.cs` — TranslationMap 로딩(LoadEntriesFromJson), TextAssetOverrides 로딩(LoadTextAssetOverrides), 매칭 체인(exact→decorated→normalized→contextual→lexicon)

### localizationtexts CSV
- `projects/esoteric-ebb/extract/1.1.3/ExportedProject/Assets/StreamingAssets/TranslationPatch/localizationtexts/` — 8개 CSV 파일 (ID,ENGLISH,KOREAN 형식)

### 패치 빌드 인프라
- `projects/esoteric-ebb/patch/tools/build_patch_package_unified.ps1` — 패치 패키지 빌드 스크립트 (dist/dist_full 생성)

### 프로젝트 설정
- `projects/esoteric-ebb/project.json` — LLM 프로파일, 서버 URL, 배치/동시성 설정

### 원본 게임 에셋
- `projects/esoteric-ebb/extract/1.1.3/ExportedProject/Assets/` — 285개 TextAsset ink JSON 원본 파일

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `v2pipeline/store.go` QueryByState: done 항목 조회 — export CLI의 데이터 소스
- `inkparse/parser.go` 트리 워킹 로직: 역삽입 시 동일 경로로 `"^text"` 노드 위치 역추적 가능
- `go-esoteric-apply-out/main.go` tokenCompatible() 검증: 태그/placeholder 보존 확인 패턴 참고
- `build_patch_package_unified.ps1`: 패치 디렉토리 구조 (`textassets/`, `localizationtexts/`, `translations.json`)

### Established Patterns
- CLI: `cmd/go-*/main.go` → flag → LoadProjectConfig → domain.Run(Config)
- DB: PostgreSQL via pgx/v5, SQLite via modernc.org/sqlite — 동일 인터페이스 양쪽 지원
- Export: v1 apply-out 패턴 — DB 조회 → 검증 → 파일 쓰기

### Integration Points
- 새 CLI: `workflow/cmd/go-v2-export/main.go` — translations.json + textassets + CSV 생성
- 입력: `pipeline_items_v2` 테이블 (state=done, ko_formatted)
- 출력 디렉토리: `projects/esoteric-ebb/output/v2/` 또는 직접 패치 디렉토리
- textassets 출력: `textassets/{AssetName}.json` (ink JSON 형식)
- CSV 출력: `localizationtexts/{SheetName}.txt` (ID,ENGLISH,KOREAN 형식 유지)

</code_context>

<specifics>
## Specific Ideas

- v2 translations.json 포맷 `esoteric-ebb-sidecar.v3`은 v1의 `esoteric-ebb-sidecar.v2`와 호환되지 않음. Plugin.cs가 v3 포맷을 인식하도록 Phase 4에서 수정 필요.
- TextAsset 역삽입 시 파서의 트리 워킹 로직을 역방향으로 재사용: 동일 경로(`KnotName/g-N/c-N`)를 따라가면서 `"^text"` 노드를 찾아 교체.
- 부분 완료 패치: export CLI에 `--min-coverage` 플래그로 최소 커버리지 확인 가능하게 하면 유용.
- localizationtexts CSV의 KOREAN 칼럼이 이미 부분 채워져 있는 파일이 있음 (Feats 등). 전량 번역이므로 기존 값도 덮어쓰기.
- 77K건 중 passthrough(번역 불필요)는 이미 인제스트 시 state=done으로 처리됨. 실제 LLM 처리 대상은 더 적을 수 있음.

</specifics>

<deferred>
## Deferred Ideas

- runtime_lexicon.json 생성 — Phase 4 게임 검증에서 동적 텍스트 패턴 확인 후 처리
- Plugin.cs v3 포맷 인식 로직 — Phase 4 (PLUGIN-01)
- 패치 빌드 스크립트 수정 (BepInEx/doorstop 보존) — 프로젝트 범위 밖
- 고유명사 음역/의역 정책 개선 — 별도 작업
- 품질 스코어 기반 선택적 재번역 — Phase 3 전량 실행 이후 분석

</deferred>

---

*Phase: 03-patch-output-full-run*
*Context gathered: 2026-03-23*
