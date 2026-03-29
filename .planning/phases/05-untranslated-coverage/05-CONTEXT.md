# Phase 5: 미번역 커버리지 개선 - Context

**Gathered:** 2026-03-29
**Status:** Ready for planning

<domain>
## Phase Boundary

렌더링 래퍼 strip, UI 라벨 번역, passthrough 확장으로 미번역 497건을 해결하여 플레이어 체감 품질을 높인다. 캡처 로직 오탐(341건) 수정도 포함. 기존 번역 재사용, LLM 재호출 없음 (runtime_lexicon + Plugin.cs 수정 중심).

</domain>

<decisions>
## Implementation Decisions

### 렌더링 래퍼 strip (D-01)
- **D-01:** Plugin.cs TryTranslate에서 게임 엔진이 런타임에 추가하는 렌더링 태그를 strip한 뒤 inner text로 lookup → 번역 후 re-wrap.
  - Color 래퍼: `<#DB5B2CFF>text</color>` → strip → lookup "text" → re-wrap (77건)
  - Noparse: `<noparse></noparse>text` → strip 빈 noparse 쌍 → lookup "text" (30건)
  - Line-indent 미매칭: 기존 ChoiceWrapperRegex가 커버 못 하는 29건 조사 → DB 미존재 확인 후 대응
  - Noparse+Color 복합: `<noparse></noparse><#hex>text</color>` → 순차 strip (54건은 color 77건에 포함)

### 지역 이름 정책 (D-02)
- **D-02:** 기존 번역 패턴 유지. 고유지명(Tolstad, Goblin Garden, Temple of Urth 등)은 원문 유지, 일반명사(Guard Tower→경비탑 등)만 번역. 기존 2,303건+ 번역과 일관성 유지.

### 고유명사/직업명 (D-03)
- **D-03:** 직업명(Cleric, Barbarian, Wizard 등)과 NPC 이름 원문 유지. 기존 번역에서 Cleric 2,303건, Arcanist 295건 등 원문 유지 패턴 확인됨. UI 라벨도 동일하게 passthrough.

### Passthrough 범위 (D-04)
- **D-04:** 순수 숫자/퍼센트/주사위/템플릿 전량 passthrough. 부분 번역 없음.
  - passthrough 대상: `+N`, `-N`, `N%`, `NdN`, `v1.1.3`, `2560 x 1440`, `00:00`, `0/300xp`
  - 템플릿도 passthrough: `+X Attribute`, `Level X reached`, `- Cast SpellName -`, `Day XX - XX:XX`
  - 총 52건 해결

### UI 라벨 번역 (D-05)
- **D-05:** runtime_lexicon.json에 규칙 추가하여 게임 설정/메카닉 UI 텍스트 번역.
  - 설정 메뉴: Ambient Volume→배경음 볼륨, Fullscreen→전체 화면, Resolution→해상도 등 (~60건)
  - 게임 메카닉: Hit Dice→히트 다이스, Spellbook→마법서, Inventory→인벤토리 등 (~50건)
  - 고유지명/직업명: passthrough (D-02, D-03 적용) (~40건)
  - 나머지 짧은 텍스트: 개별 분류 후 exact/substring/regex 규칙 배정
  - 총 248건 중 번역 필요 ~150건, passthrough ~98건

### 게임 대사 매칭 실패 (D-06)
- **D-06:** inline 태그(`<color=X>`, `<size=N>`, `<b>`) 포함 대사 41건 조사.
  - DB에 source_raw가 존재하면: Plugin.cs에서 inline 태그 strip 후 lookup
  - DB에 없으면: intro/메타 텍스트로 분류, lexicon 추가 또는 무시
  - 방식은 D-01 렌더링 래퍼 strip과 동일 패턴 확장

### 멀티라인 텍스트 (D-07)
- **D-07:** 주문 설명, 능력치 설명, 시 인용 등 20건. TextAsset 오버라이드로 이미 처리되는지 확인 후, 미처리 건만 대응.
  - TextAsset 오버라이드에 이미 있으면: 무시 (별도 경로로 번역됨)
  - 없으면: 개별 조사 후 TextAsset 추가 또는 lexicon 추가

### 캡처 로직 수정 (D-08)
- **D-08:** Plugin.cs의 miss 캡처 로직 수정. 렌더링 래퍼 strip 후 inner text가 번역 성공한 경우 untranslated 캡처에서 제외.
  - 현재: wrapper 포함 전체 텍스트가 miss로 기록 (341건 오탐)
  - 수정: TryTranslate 내부에서 wrapper unwrap → hit 판정 → hit이면 캡처하지 않음
  - D-01 래퍼 strip 작업과 동시에 수정하면 자연스러움

### Claude's Discretion
- 렌더링 래퍼 strip 구현 위치 (TryTranslate 내부 vs 별도 전처리 단계)
- runtime_lexicon 규칙 형식 (exact vs substring vs regex 선택)
- 멀티라인 텍스트 처리 우선순위
- 테스트 방법 (유닛 테스트 vs 인게임 검증 비율)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Plugin.cs (주요 수정 대상)
- `projects/esoteric-ebb/patch/mod-loader/EsotericEbb.TranslationLoader/Plugin.cs` — TryTranslate 3단계, ChoiceWrapperRegex, miss 캡처 로직, runtime lexicon 매칭
- `.planning/phases/04.1-plugin-cs-v2-inserted/04.1-CONTEXT.md` — Plugin.cs v2 설계: D-05 TryTranslate 4→3단계, D-14 선택지 처리

### Runtime Lexicon
- `E:\SteamLibrary\steamapps\common\Esoteric Ebb\Esoteric Ebb_Data\StreamingAssets\TranslationPatch\runtime_lexicon.json` — 현재 lexicon (8 exact, 16 substrings, 18 regex)
- `workflow/internal/v2pipeline/export.go` — BuildV3Sidecar() lexicon 생성 로직

### 미번역 데이터 (분석 기준)
- `.planning/phases/phase5-prd.md` — 미번역 497건 상세 분류, 패턴별 샘플, 해결 방향
- `E:\SteamLibrary\steamapps\common\Esoteric Ebb\BepInEx\untranslated_capture.json` — 런타임 캡처 원본 (838건, 2026-03-29T11:51)
- `E:\SteamLibrary\steamapps\common\Esoteric Ebb\BepInEx\translation_loader_state.json` — 현재 metrics

### 패치 배포
- `E:\SteamLibrary\steamapps\common\Esoteric Ebb\Esoteric Ebb_Data\StreamingAssets\TranslationPatch\` — 배포 위치
- `E:\SteamLibrary\steamapps\common\Esoteric Ebb\BepInEx\translation_loader_state.json` — 검증 기준 데이터

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `ChoiceWrapperRegex` (Plugin.cs:135-139): 기존 line-indent/link wrapper strip 패턴. color 래퍼 strip에 동일 패턴 확장 가능
- `DialogAddChoiceTextPrefix()` (Plugin.cs:1354-1380): wrapper unwrap → translate → re-wrap 패턴 참조
- `runtime_lexicon.json`: 기존 exact/substring/regex 3단계 구조. 규칙 추가만으로 확장 가능
- `export.go` BuildV3Sidecar(): lexicon 생성 로직. 새 규칙 추가 시 여기서 생성

### Established Patterns
- TryTranslate 3단계: TranslationMap → Contextual → RuntimeLexicon (Phase 04.2에서 확립)
- Miss 캡처: TryTranslate 실패 시 untranslated_capture.json에 기록
- Wrapper 처리 패턴: strip → lookup → re-wrap (ChoiceWrapperRegex에서 확립)

### Integration Points
- Plugin.cs TryTranslate: 래퍼 strip 로직 추가 위치
- Plugin.cs miss 캡처: 오탐 제외 로직 추가 위치
- runtime_lexicon.json: UI 라벨 규칙 추가
- export.go: lexicon 생성 시 새 규칙 포함
- build_patch_package_unified.ps1: 패치 빌드 후 배포

</code_context>

<specifics>
## Specific Ideas

- 현재 translations_loaded=75,204 / hits_exact=46 / hits_lexicon=206 / total_misses=2,747 / untranslated=838 (341 오탐)
- PLUGIN-03 기준: 커버리지 = (hits) / (hits + misses) >= 95%. 현재 98.8%
- 렌더링 래퍼 136건 + UI 라벨 ~150건 해결 시 미번역 ~200건 이하 가능
- 직업명/지역명 ~98건은 passthrough 처리로 "미번역"에서 제외 가능
- 게임 초반부(캐릭터 생성, 인트로) 미번역이 집중됨 — 첫인상 개선 효과 큼

</specifics>

<deferred>
## Deferred Ideas

- 고유명사 음역/의역 정책 개선 (Cleric→성직자 등) — 기존 번역 전체 수정 필요, 별도 마일스톤
- 폰트 weight 불일치 근본 해결 — 별도 작업
- 품질 스코어 기반 선택적 재번역 — 별도 마일스톤
- 패치 빌드 스크립트 자동화 — 범위 밖
- Tag masking translation (todo) — 재번역 시 적용, 현재 Phase와 무관
- Format skip when tags preserved (todo) — 재번역 시 적용
- OpenCode scaleout (todo) — 대량 재번역 시 필요

</deferred>

---

*Phase: 05-untranslated-coverage*
*Context gathered: 2026-03-29*
