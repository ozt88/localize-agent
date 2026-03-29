---
phase: 05
plan: 03
status: complete
started: 2026-03-29T13:00:00
completed: 2026-03-29T19:30:00
---

## Summary

패치 빌드, 배포, 인게임 검증 완료. 4건의 심각한 버그 발견 및 수정. untranslated 838→154 (81% 감소).

## Tasks

| # | Task | Status | Commit |
|---|------|--------|--------|
| 1 | Build + Deploy | ✓ | e711c41 (build script rewrite) |
| 2 | In-game verification | ✓ | 8c1edcb, 14ece0f (strip fixes) |

## Key Files

### Created
- (none in repo)

### Modified
- `projects/esoteric-ebb/patch/tools/build_patch_package_unified.ps1` — Python→Go export 전환
- `projects/esoteric-ebb/patch/tools/build_korean_patch_from_checkpoint.py` — format v2→v3 (후에 불필요해짐)
- `projects/esoteric-ebb/patch/mod-loader/EsotericEbb.TranslationLoader/Plugin.cs` — StripAllTmpTags 재설계

### External
- `E:\...\BepInEx\plugins\EsotericEbbTranslationLoader\EsotericEbb.TranslationLoader.dll` — 배포됨
- `E:\...\TranslationPatch\runtime_lexicon.json` — 281→320 규칙
- `E:\...\TranslationPatch\translations.json` — 75,204 entries (Go v2 export)

## Bugs Found & Fixed

### Bug 1: translations_loaded=0 (format mismatch)
- Python 스크립트가 `esoteric-ebb-sidecar.v2` 생성, Plugin.cs는 `v3` 기대
- Fix: ca9f03c

### Bug 2: v1 번역 데이터 적용 (근본 원인)
- build_patch_package_unified.ps1이 Python 스크립트 호출 → `items` 테이블(v1 line-hash ID) 사용
- 정상 빌드는 `go-v2-export` → `pipeline_items_v2`(블록 단위 ID)
- Fix: e711c41 — 빌드 스크립트 전면 재작성

### Bug 3: 태그 손상 (ReplacePlainText)
- 문자 단위 태그 매핑이 한/영 길이 차이로 태그 위치 밀림 → color 번짐, `</color>` 누출
- Fix: 8c1edcb, 14ece0f — ContainsKorean 우선 체크, ReplacePlainText 삭제

### Bug 4: `<shake>` 태그 미처리
- AllTmpTagRegex에 shake 누락 → 태그 노출
- Fix: 14ece0f

## Verification Results

| 항목 | Before | After |
|------|--------|-------|
| translations_loaded | 75,204 | 75,204 |
| untranslated_count | 838 | 154 |
| total_misses | 2,747 | 520 |
| hits_lexicon | 206 | 2,556 |
| coverage | 98.9% | 99.8% |

## Deviations

- 빌드 스크립트 재작성은 원래 플랜에 없었으나, v1/v2 데이터 불일치 근본 원인 해결을 위해 필수
- ReplacePlainText 삭제 — 원래 설계(태그 보존 재삽입)가 근본적으로 불가능하여 단순화
- Lexicon 39건 추가 — 원래 Plan 02 범위였으나 검증 중 누락 발견
