# Phase 4: Plugin Optimize & Verify - Research

**Researched:** 2026-03-26
**Domain:** C# BepInEx plugin optimization, Go sidecar export, game verification
**Confidence:** HIGH

## Summary

Phase 4 involves three distinct work streams: (1) Go-side export.go modification to add `contextual_entries[]` to V3 sidecar JSON, (2) C# Plugin.cs matching chain reduction from 8 to 4 stages plus TextAsset `.json` loading, and (3) game verification with capture-based hit rate analysis. All three are well-scoped with existing code patterns to follow.

The Go-side change is straightforward -- BuildV3Sidecar already produces `entries[]` with deduped sources; `contextual_entries[]` adds the full 35K items with metadata. Plugin.cs already parses `contextual_entries` via LoadEntriesFromJson (line 1166) and AddContextualEntry (line 1414), so no C# parsing changes are needed.

The Plugin.cs matching chain reduction is surgical deletion: remove 4 methods and their call sites from TryTranslate, remove NormalizedMap from the main chain (keep it for CaptureUntranslated diagnostic), and add `*.json` to LoadTextAssetOverrides. The verification loop is iterative: deploy patch, play game, collect capture data, analyze hit rate, fix misses.

**Primary recommendation:** Execute in order: export.go contextual_entries first (enables full ContextualMap), then Plugin.cs chain reduction + TextAsset fix, then deploy and verify.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- D-01: V3 sidecar에 `contextual_entries[]` 추가. `entries[]`는 중복 source 제거 대표 엔트리(TranslationMap용), `contextual_entries[]`는 전체 35K 엔트리 + 메타데이터(ContextualMap용). export.go의 `BuildV3Sidecar()`만 변경, Plugin.cs 코드 변경 없음 — 이미 `contextual_entries` 파싱 지원.
- D-02: 8단계 → 4단계로 축소. 유지: `GeneratedPattern` → `TranslationMap` → `Contextual` → `RuntimeLexicon`. 제거: `Decorated`, `NormalizedMap`, `Embedded`, `TagSeparatedSegments`. 근거: v2에서 ink 대사는 TextAsset override 경로로 처리되어 TryTranslate를 거치지 않음. UI 텍스트는 LocalizationIdOverrides로 처리.
- D-03: 특정 씬 샘플로 1차 검증. `ENABLE_FULL_CAPTURE` 모드로 캡처 데이터 자동 수집. hit rate 99%+ 목표. 발견된 문제는 Phase 4 안에서 즉시 수정.
- D-04: runtime_lexicon.json 범위는 v2 패치 배포 후 실제 miss 데이터 기반으로 결정.
- D-05: LoadTextAssetOverrides()에서 `*.txt` + `*.json` 패턴으로 확장.

### Claude's Discretion
- 검증용 샘플 씬 선정 (다양한 콘텐츠 유형 커버)
- 매칭 체인 제거 시 관련 코드/필드 정리 범위
- 캡처 데이터 분석 자동화 방식
- hit rate 미달 시 수정 전략

### Deferred Ideas (OUT OF SCOPE)
- Decorated/Normalized 복원 — 99% 달성 후에도 특정 텍스트가 miss되면 안전망으로 재추가 검토
- 패치 빌드 스크립트 수정 (BepInEx/doorstop 보존) — 프로젝트 범위 밖
- 품질 스코어 기반 선택적 재번역 — 별도 마일스톤
- 고유명사 음역/의역 정책 개선 — 별도 작업
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PLUGIN-01 | Plugin.cs 매칭 로직을 대사 블록 단위 직접 매칭 우선으로 변경 | D-01 contextual_entries export + D-02 chain reduction. TranslationMap direct match + ContextualMap (full 35K entries) covers dialogue blocks |
| PLUGIN-02 | TryTranslateTagSeparatedSegments 제거 또는 최하위 폴백으로 강등 | D-02: complete removal along with Decorated, NormalizedMap(in chain), Embedded. Lines 864-901 in TryTranslate |
| PLUGIN-03 | 직접 매칭 커버리지 95%+ 달성 확인 | D-03 verification loop: ENABLE_FULL_CAPTURE -> analyze hit/miss -> fix. Target elevated to 99%+ |
| VERIFY-02 | 패치 적용 후 게임 내에서 태그 깨짐 없이 한국어 표시 | D-03 sample scene verification covering intro, dialogue branches, ability screens, combat |
</phase_requirements>

## Standard Stack

### Core (no new dependencies -- all existing)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| BepInEx | 5.x | Unity mod framework | Already installed, Plugin.cs built against it |
| Harmony | v2 (via BepInEx) | Runtime method patching | Already used for all TMP/ink intercepts |
| System.Text.Json | .NET built-in | JSON parsing in Plugin.cs | Already used for translations.json loading |
| Go 1.24 | 1.24.0 | export.go modification | Project standard |
| Go testing | stdlib | export_test.go | Project standard |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| encoding/json | Go stdlib | V3Sidecar JSON marshaling | Already used in export.go |
| shared.AtomicWriteFile | project pkg | Crash-safe file writes | Already used by WriteTranslationsJSON |

### Alternatives Considered
None -- all tools are already in the project. No new dependencies needed.

## Architecture Patterns

### V3 Sidecar Structure (Current vs Target)

Current `translations.json`:
```json
{
  "format": "esoteric-ebb-sidecar.v3",
  "entries": [
    {"id": "...", "source": "...", "target": "...", "source_file": "...", "text_role": "...", "speaker_hint": "..."}
  ]
}
```

Target (add `contextual_entries`):
```json
{
  "format": "esoteric-ebb-sidecar.v3",
  "entries": [ /* deduped by source -- TranslationMap */ ],
  "contextual_entries": [
    {
      "id": "KnotName/g-0/blk-0",
      "source": "Hello, traveler.",
      "target": "안녕, 여행자.",
      "source_file": "TS_Intro.json",
      "text_role": "dialogue",
      "speaker_hint": "Braxo",
      "context_en": ""
    }
  ]
}
```

### Pattern 1: entries[] vs contextual_entries[] Distinction

**What:** `entries[]` contains one entry per unique source text (first-seen wins). Plugin.cs loads these into TranslationMap for direct lookup. `contextual_entries[]` contains ALL 35K items with full metadata. Plugin.cs loads these into ContextualMap for disambiguating duplicate source texts by source_file and context scoring.

**When to use:** entries[] for O(1) direct lookup; contextual_entries[] when same English text appears in multiple source files with potentially different Korean translations.

**Implementation in export.go:**
```go
// BuildV3Sidecar with contextual_entries
type V3Sidecar struct {
    Format             string    `json:"format"`
    Entries            []V3Entry `json:"entries"`
    ContextualEntries  []V3Entry `json:"contextual_entries"`
}

func BuildV3Sidecar(items []contracts.V2PipelineItem) V3Sidecar {
    sidecar := V3Sidecar{
        Format:            V3Format,
        Entries:           make([]V3Entry, 0),
        ContextualEntries: make([]V3Entry, 0, len(items)),
    }

    seen := make(map[string]bool)
    for _, item := range items {
        target := abilityPrefixRe.ReplaceAllString(item.KOFormatted, "")
        entry := V3Entry{
            ID:          item.ID,
            Source:      item.SourceRaw,
            Target:      target,
            SourceFile:  item.SourceFile,
            TextRole:    item.ContentType,
            SpeakerHint: item.Speaker,
        }

        // contextual_entries: ALL items
        sidecar.ContextualEntries = append(sidecar.ContextualEntries, entry)

        // entries: first-seen per source text (dedup)
        if !seen[item.SourceRaw] {
            seen[item.SourceRaw] = true
            sidecar.Entries = append(sidecar.Entries, entry)
        }
    }
    return sidecar
}
```

### Pattern 2: TryTranslate Chain Reduction

**What:** Remove 4 matching stages from TryTranslate, keeping only 4.

**Current chain (8 stages, lines 839-915):**
1. GeneratedPattern (regex patterns for dynamic UI text)
2. TranslationMap (direct dictionary lookup)
3. Decorated (tag stripping + NormalizedMap lookup) -- REMOVE
4. NormalizedMap (all-tag-stripped lookup) -- REMOVE
5. Contextual (source_file + history scoring)
6. RuntimeLexicon (substring/regex replacements)
7. Embedded (fragment extraction + NormalizedMap) -- REMOVE
8. TagSeparatedSegments (segment-by-segment) -- REMOVE

**Target chain (4 stages):**
```csharp
internal static bool TryTranslate(ref string value, string origin = "unknown")
{
    if (string.IsNullOrEmpty(value)) return false;
    CaptureAllText(value, origin);
    var originalValue = value;
    bool found = false;

    // Stage 1: Generated patterns (dynamic UI)
    if (!found && TryTranslateGeneratedPattern(ref value))
        found = true;

    // Stage 2: Direct dictionary lookup
    if (!found && TranslationMap.TryGetValue(value, out var translated) && !string.IsNullOrEmpty(translated))
    {
        translated = RestoreChoicePrefix(originalValue, translated);
        RecordTranslationHit(originalValue, translated);
        value = translated;
        found = true;
    }

    // Stage 3: Contextual (disambiguation by source_file + history)
    if (!found)
    {
        var normalized = NormalizeKey(value);
        if (TryTranslateContextual(ref value, originalValue, normalized))
            found = true;
    }

    // Stage 4: Runtime lexicon (substring/regex)
    if (!found && TryTranslateRuntimeLexicon(ref value))
        found = true;

    RememberContext(originalValue);
    if (found)
    {
        value = StripQuotationMarks(value);
        value = CleanOrphanBoldTags(value);
        return true;
    }
    RecordTranslationMiss(originalValue);
    CaptureUntranslated(originalValue);
    return false;
}
```

### Pattern 3: TextAsset Override .json Extension

**What:** Add `*.json` file pattern to LoadTextAssetOverrides.

**Current (line 1227):**
```csharp
foreach (var path in Directory.EnumerateFiles(dir, "*.txt", SearchOption.AllDirectories))
```

**Target:**
```csharp
var patterns = new[] { "*.txt", "*.json" };
foreach (var pattern in patterns)
{
    foreach (var path in Directory.EnumerateFiles(dir, pattern, SearchOption.AllDirectories))
    {
        var name = Path.GetFileNameWithoutExtension(path);
        var text = File.ReadAllText(path);
        if (string.IsNullOrWhiteSpace(name) || string.IsNullOrWhiteSpace(text))
            continue;
        TextAssetOverrides[name] = text;
    }
}
```

**Critical detail:** `TextAssetOverrides` is keyed by filename without extension. For ink JSON textassets, the game requests them by asset name (e.g., "TS_Intro"), and the plugin returns the full JSON content as replacement. The `.json` files produced by Phase 3 have the same name pattern, so `GetFileNameWithoutExtension` correctly strips `.json` to get the asset name.

### Anti-Patterns to Avoid
- **Removing NormalizeKey function:** NormalizeKey is used by ContextualMap (for key normalization), RememberContext, CaptureUntranslated, and other remaining code. Only remove the NormalizedMap DICTIONARY usage from the TryTranslate chain, not the NormalizeKey function itself.
- **Removing NormalizedMap dictionary entirely:** CaptureUntranslated (line 2680) and state dump (line 2998) still reference it. Keep the dictionary and its population in AddEntry, but remove it from the matching chain. Alternatively, if removing the dictionary, update all remaining references.
- **Breaking contextual_entries JSON field naming:** Plugin.cs AddContextualEntry expects specific field names: `id`, `source`, `target`, `context_en`, `speaker_hint`, `text_role`, `translation_lane`, `source_file`. The V3Entry struct must match.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Source text deduplication | Custom dedup logic | `map[string]bool` seen set in BuildV3Sidecar | Simple first-seen-wins pattern, consistent with how TranslationMap works |
| JSON atomic writes | Manual temp-file + rename | `shared.AtomicWriteFile` | Already handles cross-platform atomic writes |
| Capture data analysis | Manual game-log parsing | Plugin's existing `full_text_capture.json` + `translation_loader_state.json` | Already structured JSON with origin tags |
| Hit rate calculation | Custom metric code | Parse `translation_loader_state.json` counters | Plugin already tracks hit/miss/contextual counts |

**Key insight:** The plugin already has comprehensive capture and diagnostics infrastructure. The verification loop should leverage existing `ENABLE_FULL_CAPTURE`, `full_text_capture.json`, `untranslated_capture.json`, and `translation_loader_state.json` rather than building new instrumentation.

## Common Pitfalls

### Pitfall 1: contextual_entries Missing context_en Field
**What goes wrong:** Plugin.cs AddContextualEntry reads `context_en` for contextual scoring. If export.go V3Entry struct does not include this field, ContextualMap entries have empty context, reducing disambiguation quality.
**Why it happens:** Current V3Entry struct has no `context_en` field. The existing entries[] path didn't need it.
**How to avoid:** Add `ContextEN string` field to V3Entry. Populate from surrounding items' SourceRaw in the same knot/gate (or leave empty for now -- ContextualMap primarily uses source_file matching which doesn't need context_en).
**Warning signs:** ContextualMap hit rate is low despite having candidates -- check if context_en is populated.

**Decision point:** The current V3Entry struct has `Source`, `Target`, `SourceFile`, `TextRole`, `SpeakerHint`. AddContextualEntry also reads `context_en` and `translation_lane`. The `context_en` field enables history-based scoring in TryTranslateContextual. For the initial implementation, leaving `context_en` empty is acceptable because source_file matching (line 2286-2302) is the primary disambiguation path and handles the 2,654 ambiguous sources. The history-based fallback (lines 2310+) is secondary.

### Pitfall 2: NormalizedMap Removal Scope Confusion
**What goes wrong:** Removing NormalizedMap from TryTranslate chain but forgetting that NormalizedMap is referenced in CaptureUntranslated (line 2680) and state dump (line 2998).
**Why it happens:** grep shows 15+ NormalizedMap references -- easy to miss non-chain usages.
**How to avoid:** Two options: (A) Keep NormalizedMap populated in AddEntry but remove from matching chain only, or (B) Remove entirely and update CaptureUntranslated + state dump. Option A is safer and simpler.
**Warning signs:** Compilation errors after removal.

### Pitfall 3: TextAsset Override Collision Between .txt and .json
**What goes wrong:** If both `TS_Intro.txt` and `TS_Intro.json` exist in the textassets directory, one overwrites the other in TextAssetOverrides (keyed by filename without extension).
**Why it happens:** Phase 3 outputs `.json` files, but old `.txt` files might still exist in the patch directory.
**How to avoid:** Process `.json` files AFTER `.txt` files so `.json` takes precedence (v2 format). Or clean old `.txt` textasset files before deploying v2 patch.
**Warning signs:** Unexpected English text in game despite having Korean textassets.

### Pitfall 4: entries[] Dedup Losing Best Translation
**What goes wrong:** When deduplicating entries[] by source text, the first-seen item's translation is used. If the first occurrence has a lower quality score than later ones, TranslationMap gets a suboptimal translation.
**Why it happens:** QueryDone returns items ordered by sort_index, not by quality score.
**How to avoid:** For entries[] dedup, this is acceptable because: (a) TranslationMap is only the first-pass lookup, (b) ContextualMap has all 35K entries for disambiguation, (c) most duplicate sources have identical translations anyway.
**Warning signs:** Known high-ambiguity terms showing wrong translation in non-contextual path.

### Pitfall 5: Patch Deployment Forgetting BepInEx/doorstop
**What goes wrong:** Rebuilding the patch package destroys BepInEx core files and doorstop.
**Why it happens:** build_patch_package_unified.ps1 behavior (documented in feedback_patch_build_checklist.md).
**How to avoid:** After running build script, manually verify BepInEx/, doorstop.dll, and fonts are intact. Or deploy translations.json and textassets manually without running the full build script.
**Warning signs:** Game crashes on launch or no translation loading.

### Pitfall 6: translations_loaded=0 in Capture Data
**What goes wrong:** Analyzing untranslated_capture.json from a session where translations weren't loaded gives misleading miss data.
**Why it happens:** Previous capture (1,040 entries) was taken with translations_loaded=0.
**How to avoid:** Always check translation_loader_state.json for translations_loaded > 0 before trusting capture data. Re-capture after v2 patch deployment.
**Warning signs:** translation_loader_state.json shows translations_loaded=0.

## Code Examples

### Example 1: BuildV3Sidecar with contextual_entries (Go)

```go
// Target implementation for export.go
type V3Sidecar struct {
    Format            string    `json:"format"`
    Entries           []V3Entry `json:"entries"`
    ContextualEntries []V3Entry `json:"contextual_entries"`
}

func BuildV3Sidecar(items []contracts.V2PipelineItem) V3Sidecar {
    sidecar := V3Sidecar{
        Format:            V3Format,
        Entries:           make([]V3Entry, 0),
        ContextualEntries: make([]V3Entry, 0, len(items)),
    }
    seen := make(map[string]bool)
    for _, item := range items {
        target := abilityPrefixRe.ReplaceAllString(item.KOFormatted, "")
        entry := V3Entry{
            ID:          item.ID,
            Source:      item.SourceRaw,
            Target:      target,
            SourceFile:  item.SourceFile,
            TextRole:    item.ContentType,
            SpeakerHint: item.Speaker,
        }
        sidecar.ContextualEntries = append(sidecar.ContextualEntries, entry)
        if !seen[item.SourceRaw] {
            seen[item.SourceRaw] = true
            sidecar.Entries = append(sidecar.Entries, entry)
        }
    }
    return sidecar
}
```

### Example 2: Simplified TryTranslate Chain (C#)

```csharp
internal static bool TryTranslate(ref string value, string origin = "unknown")
{
    if (string.IsNullOrEmpty(value)) return false;
    CaptureAllText(value, origin);
    var originalValue = value;
    bool found = false;

    if (!found && TryTranslateGeneratedPattern(ref value))
        found = true;

    if (!found && TranslationMap.TryGetValue(value, out var translated) && !string.IsNullOrEmpty(translated))
    {
        translated = RestoreChoicePrefix(originalValue, translated);
        RecordTranslationHit(originalValue, translated);
        value = translated;
        found = true;
    }

    if (!found)
    {
        var normalized = NormalizeKey(value);
        if (TryTranslateContextual(ref value, originalValue, normalized))
            found = true;
    }

    if (!found && TryTranslateRuntimeLexicon(ref value))
        found = true;

    RememberContext(originalValue);
    if (found)
    {
        value = StripQuotationMarks(value);
        value = CleanOrphanBoldTags(value);
        return true;
    }
    RecordTranslationMiss(originalValue);
    CaptureUntranslated(originalValue);
    return false;
}
```

### Example 3: TextAsset Override with .json (C#)

```csharp
private void LoadTextAssetOverrides()
{
    // ... candidateDirs setup unchanged ...

    try
    {
        var patterns = new[] { "*.txt", "*.json" };
        foreach (var dir in dirs)
        {
            foreach (var pattern in patterns)
            {
                foreach (var path in Directory.EnumerateFiles(dir, pattern, SearchOption.AllDirectories))
                {
                    var name = Path.GetFileNameWithoutExtension(path);
                    var text = File.ReadAllText(path);
                    if (string.IsNullOrWhiteSpace(name) || string.IsNullOrWhiteSpace(text))
                        continue;
                    TextAssetOverrides[name] = text;
                }
            }
        }
        _textAssetOverrideCount = TextAssetOverrides.Count;
        Log.LogInfo($"Loaded text asset overrides from {dirs.Length} directories ({_textAssetOverrideCount} files)");
    }
    catch (Exception ex)
    {
        Log.LogWarning($"Failed to load text asset overrides: {ex.Message}");
    }
}
```

## Code Cleanup Inventory

Methods and code blocks to remove from Plugin.cs when removing 4 matching stages:

### Methods to Remove
| Method | Lines | Dependencies |
|--------|-------|-------------|
| `TryTranslateDecorated` | 2212-2242 | Uses ExtractDecoratedParts, NormalizedMap |
| `TryTranslateEmbedded` | 2244-2277 | Uses ExtractEmbeddedCandidates, NormalizedMap |
| `TryTranslateTagSeparatedSegments` | 2352-2416 | Uses ExtractTagSeparatedSegments, LookupTranslatedSegment, NormalizedMap |
| `ExtractDecoratedParts` | 3076-3140 | Helper for TryTranslateDecorated |
| `DecoratedParts` class | 3065-3075 | Data class for ExtractDecoratedParts |
| `ExtractEmbeddedCandidates` | 3181-3215 | Helper for TryTranslateEmbedded |
| `ExtractTagSeparatedSegments` | 3216-3250+ | Helper for TryTranslateTagSeparatedSegments |
| `LookupTranslatedSegment` | 3240-3260+ | Helper for TryTranslateTagSeparatedSegments |

### TryTranslate Call Sites to Remove (lines 864-901)
- Lines 864-867: `TryTranslateDecorated` call
- Lines 869-880: `NormalizedMap` lookup block
- Lines 893-896: `TryTranslateEmbedded` call
- Lines 898-901: `TryTranslateTagSeparatedSegments` call

### NormalizedMap -- Keep or Remove Decision
**Recommendation: Keep NormalizedMap populated but remove from matching chain.**
- Keep: `NormalizedMap` dictionary declaration (line 22)
- Keep: Population in `AddEntry` (lines 1199-1203)
- Keep: Reference in `CaptureUntranslated` (line 2680) -- diagnostic
- Keep: Reference in state dump (line 2998) -- diagnostic
- Remove: Lookup in TryTranslate main chain (lines 869-880)
- Remove: All usage in removed methods (TryTranslateDecorated, TryTranslateEmbedded, TryTranslateTagSeparatedSegments)

This preserves diagnostic capability while removing the matching path.

## Verification Strategy

### Verification Loop
```
1. Deploy v2 patch (translations.json + textassets + Plugin.dll)
2. Create ENABLE_FULL_CAPTURE marker file in BepInEx/
3. Launch game, play through sample scenes
4. Quit game
5. Analyze: full_text_capture.json + translation_loader_state.json
6. Calculate hit rate: hits / (hits + misses)
7. If < 99%: analyze misses, fix, redeploy, repeat from step 2
```

### Sample Scenes to Verify
Cover all content types:
- **Intro sequence** -- dialogue, narration, ability text
- **Character creation** -- UI labels, stat names, generated patterns
- **Dialogue with branches** -- ink choices, speaker attribution
- **Ability check scene** -- DC checks, success/fail markers
- **Spell/inventory screen** -- spell descriptions, item names
- **Combat** -- combat log, tooltips, status effects

### Capture Data Analysis
Plugin already outputs structured JSON:
- `full_text_capture.json`: every text string with origin (tmp_text, menu_scan, ink_dialogue, ink_choice)
- `translation_loader_state.json`: counters (translations_loaded, hits, misses, contextual_hits)
- `untranslated_capture.json`: missed strings

### Hit Rate Calculation
```
hit_rate = translation_hit_count / (translation_hit_count + translation_miss_count)
```
From `translation_loader_state.json` counters.

## Open Questions

1. **context_en field population**
   - What we know: AddContextualEntry reads `context_en` for history-based scoring. V3Entry currently has no such field.
   - What's unclear: Whether source_file matching alone achieves 99%+ hit rate, or if context_en is needed for the remaining ambiguous cases.
   - Recommendation: Start without context_en (leave empty). If hit rate is below 99%, add context_en by including neighboring SourceRaw texts from same knot/gate.

2. **translation_lane field**
   - What we know: AddContextualEntry reads `translation_lane`. Not clear what values are expected.
   - What's unclear: Whether this field is used in scoring. Examining TryTranslateContextual, it doesn't reference translation_lane in scoring.
   - Recommendation: Omit from V3Entry or set to empty string. No impact on matching.

3. **NormalizedMap complete removal feasibility**
   - What we know: After chain reduction, NormalizedMap is only used in CaptureUntranslated and state dump.
   - What's unclear: Whether NormalizedMap memory (~35K entries) is worth keeping for diagnostics.
   - Recommendation: Keep for now -- diagnostic value outweighs small memory cost. Revisit after verification confirms 99%+.

## Sources

### Primary (HIGH confidence)
- Plugin.cs source code (3,388 lines) -- direct code inspection
- export.go source code -- direct code inspection
- export_test.go -- existing test patterns
- contracts/v2pipeline.go -- V2PipelineItem DTO fields
- 04-CONTEXT.md -- locked decisions D-01 through D-05
- 03-EXECUTION-LOG.md -- Phase 3 lessons learned

### Secondary (MEDIUM confidence)
- Runtime capture data analysis from discuss-phase (4,550 entries breakdown)
- build_report.json ambiguous_source_count: 2,654

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - all existing libraries, no new dependencies
- Architecture: HIGH - direct code inspection of both Go and C# codebases, existing patterns to follow
- Pitfalls: HIGH - based on actual code analysis showing specific line numbers and dependencies
- Verification: MEDIUM - verification loop is iterative by nature, outcomes depend on actual game behavior

**Research date:** 2026-03-26
**Valid until:** 2026-04-26 (game version 1.1.3 fixed, no upstream changes expected)
