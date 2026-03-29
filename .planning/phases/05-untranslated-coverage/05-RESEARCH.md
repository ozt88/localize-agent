# Phase 5: 미번역 커버리지 개선 - Research

**Researched:** 2026-03-29
**Domain:** BepInEx Plugin.cs 런타임 텍스트 매칭 + runtime_lexicon 확장
**Confidence:** HIGH

## Summary

Phase 5는 미번역 497건(+ 오탐 341건)을 해결하여 플레이어 체감 품질을 높이는 작업이다. 핵심은 두 가지: (1) Plugin.cs의 TryTranslate 체인에 렌더링 래퍼 strip 전처리를 추가하고, (2) runtime_lexicon.json에 UI 라벨/passthrough 규칙을 대량 추가하는 것이다.

LLM 재호출 없이, 기존 번역 데이터(TranslationMap 75,204건)를 최대한 활용하는 "매칭 개선" 작업이다. Plugin.cs C# 코드 수정과 runtime_lexicon.json 데이터 추가가 주요 산출물이며, export.go의 lexicon 생성 로직도 새 규칙을 반영해야 한다.

**Primary recommendation:** Plugin.cs TryTranslate 진입부에 렌더링 래퍼 strip 전처리 단계를 추가하고, runtime_lexicon.json에 ~200건의 규칙을 추가하되, export.go에서 자동 생성하는 방식이 아닌 수동 편집(정적 lexicon)으로 관리한다.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01 (렌더링 래퍼 strip):** Plugin.cs TryTranslate에서 color/noparse/line-indent 래퍼를 strip한 뒤 inner text로 lookup, 번역 후 re-wrap
- **D-02 (지역 이름):** 고유지명 원문 유지, 일반명사만 번역. 기존 번역 패턴 일관성 유지
- **D-03 (고유명사/직업명):** 직업명(Cleric, Barbarian 등), NPC 이름 원문 유지
- **D-04 (Passthrough):** 숫자/퍼센트/주사위/템플릿 전량 passthrough (52건)
- **D-05 (UI 라벨):** runtime_lexicon.json에 규칙 추가. 설정 메뉴 ~60건, 게임 메카닉 ~50건, passthrough ~98건, 번역 필요 ~150건
- **D-06 (게임 대사 매칭):** inline 태그 strip 후 lookup. D-01과 동일 패턴 확장
- **D-07 (멀티라인):** TextAsset 오버라이드에 있으면 무시, 없으면 개별 대응
- **D-08 (캡처 로직):** wrapper unwrap 후 hit 판정, hit이면 캡처 제외

### Claude's Discretion
- 렌더링 래퍼 strip 구현 위치 (TryTranslate 내부 vs 별도 전처리 단계)
- runtime_lexicon 규칙 형식 (exact vs substring vs regex 선택)
- 멀티라인 텍스트 처리 우선순위
- 테스트 방법 (유닛 테스트 vs 인게임 검증 비율)

### Deferred Ideas (OUT OF SCOPE)
- 고유명사 음역/의역 정책 개선 (Cleric->성직자 등)
- 폰트 weight 불일치 근본 해결
- 품질 스코어 기반 선택적 재번역
- 패치 빌드 스크립트 자동화
- Tag masking translation, Format skip, OpenCode scaleout
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PLUGIN-03 | 직접 매칭 커버리지 95%+ 달성 확인 | 현재 98.8%. 래퍼 strip(136건) + UI 라벨(248건) + passthrough(52건) + 대사 매칭(41건) 해결 시 미번역 ~20건 이하로 감소 가능. 캡처 오탐 341건 수정으로 untranslated_count도 대폭 감소. |
</phase_requirements>

## Standard Stack

이 Phase는 새 라이브러리 도입 없음. 기존 기술 스택 내에서 수정.

### Core
| Component | Version | Purpose | 비고 |
|-----------|---------|---------|------|
| Plugin.cs (C#) | v2.0.0 | BepInEx 플러그인, TryTranslate 체인 | 1,859줄, 주요 수정 대상 |
| runtime_lexicon.json | v2 format | 런타임 용어집 (exact/substring/regex) | 현재 42 규칙 → ~240 규칙으로 확장 |
| export.go | - | BuildV3Sidecar, lexicon 생성 | 새 lexicon 규칙 반영 |
| build_patch_package_unified.ps1 | - | 패치 빌드 & 배포 | 변경 없음, 실행만 |

### Supporting
| Tool | Purpose | When to Use |
|------|---------|-------------|
| untranslated_capture.json | 미번역 838건 원본 데이터 | 규칙 추가 시 참조, 검증 시 비교 |
| translation_loader_state.json | 런타임 메트릭 | 변경 전후 비교 검증 |
| ENABLE_FULL_CAPTURE flag | 전체 hit 로그 활성화 | 디버깅 시 BepInEx/ 폴더에 빈 파일 생성 |

## Architecture Patterns

### 래퍼 Strip 구현 패턴 (D-01 Discretion 해결)

**권장: TryTranslate 진입부 전처리 방식**

TryTranslate 메서드 최상단에서 렌더링 래퍼를 strip하고, 3-stage 체인 실행 후 hit이면 re-wrap하는 방식. 이유:

1. 기존 3-stage (TranslationMap -> Contextual -> Lexicon) 구조를 깨지 않음
2. 모든 origin (tmp_text, ink_choice, ui_text, property_scan)에서 자동 적용
3. CaptureUntranslated도 자연스럽게 inner text 기준으로 동작 (D-08 동시 해결)
4. DialogAddChoiceTextPrefix의 ChoiceWrapperRegex 패턴과 동일한 strip->translate->re-wrap 구조

```csharp
// TryTranslate 진입부 pseudo-code
internal static bool TryTranslate(ref string value, string origin = "unknown")
{
    if (string.IsNullOrEmpty(value)) return false;

    // Phase 5: Strip rendering wrappers before lookup
    var (stripped, wrapper) = StripRenderingWrapper(value);
    if (wrapper != null)
    {
        var inner = stripped;
        if (TryTranslateCore(ref inner, origin))
        {
            value = wrapper.Rewrap(inner);
            return true;
        }
        // Wrapper stripped but inner not found — capture inner text, not wrapped
        CaptureUntranslated(stripped, origin);
        return false;
    }

    return TryTranslateCore(ref value, origin);
}
```

### Strip 순서 (중요)

래퍼가 중첩될 수 있으므로 순차 strip 필요:
1. `<noparse></noparse>` 빈 쌍 제거 (가장 바깥)
2. `<#hexFF>...</color>` 또는 `<color=#hex>...</color>` color 래퍼 제거
3. Inline tags (`<i>`, `<b>`, `<size=N>`) 제거 (D-06 대사 매칭용)

각 단계에서 제거한 래퍼 정보를 보존하여 re-wrap 시 복원.

### Regex 패턴 설계

```csharp
// Noparse: 빈 쌍만 strip (내용 있는 noparse는 건드리지 않음)
private static readonly Regex NoparseEmptyRegex = new(
    @"<noparse>\s*</noparse>",
    RegexOptions.Compiled);

// Color wrapper: hex 또는 named color
private static readonly Regex ColorWrapperRegex = new(
    @"^(<(?:#[0-9A-Fa-f]{6,8}|color=[^>]+)>)(.*?)(</color>)$",
    RegexOptions.Compiled | RegexOptions.Singleline);

// Inline tags: <i>, </i>, <b>, </b>, <size=N>, </size>
private static readonly Regex InlineTagRegex = new(
    @"</?(?:i|b|size(?:=[^>]*)?)>",
    RegexOptions.Compiled);
```

### Runtime Lexicon 규칙 형식 결정 (Discretion 해결)

| 유형 | 형식 | 사용 조건 |
|------|------|-----------|
| 1:1 완전 일치 | exact_replacements | UI 라벨, 메뉴 항목, 게임 용어 (Settings, Inventory 등) |
| 부분 문자열 | substring_replacements | 복합 문자열 내 번역 (Gained Item: X 등) |
| 패턴 매칭 | regex_rules | 템플릿/변수 포함 (Level X reached, +X Attribute 등) |
| Passthrough | exact_replacements (find=replace) | 직업명, 지역명, 숫자/주사위 |

**Passthrough 처리:** exact_replacements에 `{ "find": "Cleric", "replace": "Cleric" }` 형태로 추가. TryTranslateRuntimeLexicon이 true를 반환하므로 miss 캡처에서 제외됨.

### Anti-Patterns to Avoid
- **Strip 로직을 각 Stage마다 중복 구현하지 말 것:** TryTranslate 진입부 한 곳에서 처리
- **Regex를 매번 new로 생성하지 말 것:** static readonly Regex로 컴파일
- **Passthrough를 무시(skip) 방식으로 구현하지 말 것:** 반드시 TryTranslate가 true를 반환해야 miss에서 제외됨

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| TMP 태그 파싱 | 커스텀 XML 파서 | Regex 패턴 매칭 | TMP 태그는 닫힘 보장 안 됨, `<noparse></noparse>` 빈 쌍도 있음 |
| Lexicon 규칙 관리 | DB 기반 규칙 엔진 | JSON 파일 직접 편집 | 규칙 수 ~240건, 복잡한 관리 불필요 |
| 캡처 결과 분석 | 별도 분석 도구 | 기존 untranslated_capture.json + PowerShell/Python 스크립트 | 838건은 수동 검토 가능한 규모 |

## Common Pitfalls

### Pitfall 1: Re-wrap 시 태그 순서 역전
**What goes wrong:** strip 순서와 re-wrap 순서가 일치하지 않으면 `</color><noparse></noparse>` 같은 잘못된 태그 생성
**Why it happens:** noparse를 먼저 strip하고 color를 나중에 strip하면, re-wrap 시 color가 먼저 감싸이고 noparse가 바깥에 와야 하는데 순서가 뒤바뀜
**How to avoid:** Strip은 바깥->안쪽 순서, re-wrap은 안쪽->바깥 순서. 구조체에 strip 순서를 기록하고 역순으로 복원
**Warning signs:** 인게임에서 색상이 적용되지 않거나 `<noparse>` 텍스트가 보임

### Pitfall 2: ChoiceWrapperRegex와 ColorWrapperRegex 중복 매칭
**What goes wrong:** 선택지 텍스트가 `<#FFFFFFFF><line-indent...>` 래퍼와 `<#DB5B2CFF>` color 래퍼를 동시에 가질 수 있음
**Why it happens:** DialogAddChoiceTextPrefix에서 ChoiceWrapperRegex로 이미 처리된 텍스트가 TryTranslate에서 다시 strip될 수 있음
**How to avoid:** DialogAddChoiceTextPrefix는 body를 추출한 후 TryTranslate를 호출하므로, TryTranslate에 도착하는 텍스트는 이미 body만 남은 상태. 하지만 body 자체에 color 래퍼가 있을 수 있으므로 두 단계 모두 필요
**Warning signs:** 선택지 번역이 2중으로 적용되거나 태그가 남음

### Pitfall 3: Passthrough가 실제로 캡처에서 제외되지 않음
**What goes wrong:** exact_replacements에 passthrough 추가했는데 여전히 untranslated_capture에 기록됨
**Why it happens:** TryTranslateRuntimeLexicon의 exact 체크에서 find==replace이면 `value`가 변경되지 않았다고 판단할 수 있음
**How to avoid:** 현재 코드 확인 결과, exact 매칭은 `TryGetValue` 성공 시 무조건 `return true`이므로 문제없음. 하지만 substring 매칭에서는 find==replace면 `value.Contains`는 true이지만 Replace 후 `modified=true`로 정상 동작
**Warning signs:** passthrough 항목이 캡처 파일에 남아있음

### Pitfall 4: Lexicon 규칙 충돌 (substring이 exact보다 먼저 매칭)
**What goes wrong:** "Hit Points"를 exact로 "체력"으로 번역하려는데, substring "Hit"이 먼저 매칭되어 잘못된 번역 생성
**Why it happens:** TryTranslateRuntimeLexicon은 exact -> substring -> regex 순서로 실행되므로 실제로는 exact가 우선. 하지만 substring 규칙끼리의 충돌은 가능
**How to avoid:** substring 규칙은 longest-match-first로 정렬됨 (코드에서 Sort 확인). 짧은 공통 부분 문자열은 `\b` word boundary가 있는 regex로 옮기는 것이 안전
**Warning signs:** 부분 번역이 잘못 적용됨

### Pitfall 5: 29건 line-indent 미매칭 원인 오판
**What goes wrong:** ChoiceWrapperRegex 버그로 판단하고 regex를 수정하지만 실제로는 DB에 해당 source_raw가 없음
**Why it happens:** 캡처된 텍스트의 inner body가 DB TranslationMap에 존재하지 않아 TryTranslate가 실패한 것이지, regex 매칭 실패가 아닐 수 있음
**How to avoid:** 29건 각각의 inner body를 DB에서 조회하여 존재 여부 확인 후 대응 방식 결정
**Warning signs:** regex 수정 후에도 29건이 그대로 남음

## Code Examples

### 래퍼 Strip 구조체 패턴 (권장)

```csharp
// Source: Plugin.cs 기존 ChoiceWrapperRegex 패턴 확장
private readonly struct RenderingWrapper
{
    public readonly string? NoparsePrefix;   // stripped "<noparse></noparse>" if present
    public readonly string? ColorOpen;       // e.g. "<#DB5B2CFF>"
    public readonly string? ColorClose;      // e.g. "</color>"

    public string Rewrap(string inner)
    {
        var sb = new System.Text.StringBuilder();
        if (NoparsePrefix != null) sb.Append(NoparsePrefix);
        if (ColorOpen != null) sb.Append(ColorOpen);
        sb.Append(inner);
        if (ColorClose != null) sb.Append(ColorClose);
        return sb.ToString();
    }
}
```

### Lexicon 규칙 추가 패턴 (runtime_lexicon.json)

```json
{
  "exact_replacements": [
    { "find": "Settings", "replace": "설정" },
    { "find": "Ambient Volume", "replace": "배경음 볼륨" },
    { "find": "Inventory", "replace": "인벤토리" },
    { "find": "Cleric", "replace": "Cleric" },
    { "find": "Barbarian", "replace": "Barbarian" },
    { "find": "Tolstad", "replace": "Tolstad" }
  ],
  "regex_rules": [
    {
      "name": "level_reached",
      "pattern": "^Level (\\d+) reached$",
      "replace": "레벨 $1 도달",
      "ignore_case": false
    },
    {
      "name": "plus_attribute",
      "pattern": "^([+-]\\d+)\\s+(.+)$",
      "replace": "$1 $2",
      "ignore_case": false
    },
    {
      "name": "day_template",
      "pattern": "^Day (\\d+)$",
      "replace": "$1일차",
      "ignore_case": false
    }
  ]
}
```

### 캡처 로직 수정 (D-08)

```csharp
// 현재: TryTranslate 실패 시 무조건 캡처
Interlocked.Increment(ref _misses);
CaptureUntranslated(original, origin);
return false;

// 수정: wrapper strip 버전에서는 inner text가 hit이면 캡처 안 함
// → TryTranslate 진입부에서 strip 후 TryTranslateCore 호출하므로
//   자연스럽게 해결됨. strip된 inner text로 매칭 성공하면 true 반환,
//   CaptureUntranslated 호출 안 됨.
```

## State of the Art

| 현재 상태 | 목표 상태 | 변경 사항 |
|-----------|-----------|-----------|
| TryTranslate 3-stage (exact/contextual/lexicon) | 3-stage + 전처리 strip | strip 전처리 단계 추가 |
| runtime_lexicon 42 규칙 | ~240 규칙 | ~200 규칙 추가 |
| untranslated_capture 838건 (341 오탐) | ~50건 이하 | 래퍼 strip + lexicon으로 대부분 해결 |
| 캡처 로직: wrapper 포함 텍스트 기록 | inner text 기준 기록 | strip 전처리와 연동 |

## Open Questions

1. **29건 line-indent 미매칭의 정확한 원인**
   - What we know: ChoiceWrapperRegex가 `^((?:<[^>]+>)*\d+\.\s+)(.*?)(</link>.*)$` 패턴
   - What's unclear: 29건이 regex 매칭 실패인지, inner body가 DB에 없는 것인지
   - Recommendation: 29건의 inner body를 DB에서 조회하여 확인 후 대응 (조사 태스크 선행)

2. **멀티라인 20건의 TextAsset 오버라이드 커버리지**
   - What we know: TextAsset 오버라이드 294개 로드됨, 18건 hit
   - What's unclear: 20건 중 몇 건이 TextAsset으로 이미 커버되는지
   - Recommendation: 20건 각각 조사 후 미커버 건만 대응 (우선순위 낮음)

3. **export.go lexicon 생성과 수동 편집의 관계**
   - What we know: 현재 runtime_lexicon.json은 수동 관리 (export.go에 lexicon 생성 로직 없음)
   - What's unclear: 향후 패치 재빌드 시 lexicon이 덮어써지는지
   - Recommendation: build_patch_package_unified.ps1에서 lexicon 파일을 덮어쓰지 않는지 확인

## Sources

### Primary (HIGH confidence)
- Plugin.cs (1,859줄) — TryTranslate 체인, ChoiceWrapperRegex, LoadRuntimeLexicon, CaptureUntranslated 직접 분석
- runtime_lexicon.json — 현재 42 규칙 구조 (8 exact, 16 substring, 18 regex) 직접 확인
- export.go — BuildV3Sidecar, CleanTarget 직접 분석
- untranslated_capture.json — 838건 런타임 캡처 데이터 직접 확인
- translation_loader_state.json — 현재 메트릭 (75,204 loaded, 838 untranslated) 직접 확인
- phase5-prd.md — 497건 상세 분류 및 해결 방향

### Secondary (MEDIUM confidence)
- CONTEXT.md 05-CONTEXT.md — discuss-phase에서 확정된 결정사항 8건

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - 기존 코드베이스 직접 분석, 새 도입 없음
- Architecture: HIGH - 기존 패턴(ChoiceWrapperRegex strip->translate->re-wrap) 확장, 코드 직접 확인
- Pitfalls: HIGH - 실제 코드 흐름 분석 기반, TMP 태그 중첩/순서 이슈는 Phase 04에서 경험

**Research date:** 2026-03-29
**Valid until:** 2026-04-28 (게임 버전 1.1.3 고정, 변동 없음)
