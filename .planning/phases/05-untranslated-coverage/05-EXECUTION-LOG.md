# Phase 05 Execution Log

## Timeline

### 2026-03-29 — Wave 1 실행 (Plan 01 + 02 병렬)

**05-01 (Plugin.cs 래퍼 strip):** 완료
- RenderingWrapper struct, StripRenderingWrapper 메서드 추가
- Regex 3개: NoparseEmptyRegex, ColorWrapperRegex, InlineTagRegex
- TryTranslate → StripRenderingWrapper → TryTranslateCore 리팩터링
- Commits: e2b8c90, e044619, 5d6fbed

**05-02 (Runtime lexicon 확장):** 완료
- 42 → 281 규칙 (exact 218, substring 16, regex 47)
- Commit: 3bd6cae

### 2026-03-29 — Wave 2 시작 (Plan 03 빌드/배포/검증)

**05-03 Task 1 (빌드/배포):** 완료 — 이후 심각한 회귀 발견

#### Bug 1: translations_loaded = 0 (format mismatch)
- **발견:** 게임 실행 후 번역 전혀 안됨, translations_loaded=0
- **원인:** build_korean_patch_from_checkpoint.py가 format `esoteric-ebb-sidecar.v2`로 생성, Plugin.cs는 `v3` 기대
- **수정:** Python 스크립트 format string v2→v3 수정 + 게임 파일 직접 수정
- Commit: ca9f03c

#### Bug 2: v1 번역 데이터 적용 (근본 문제)
- **발견:** format 수정 후 번역 로드되었으나, 번역 내용이 v1 품질. 공통 source 3,106개뿐
- **근본 원인:** build_patch_package_unified.ps1이 Python 스크립트 호출 → Postgres `items` 테이블 (v1 line-hash ID, 라인 단위) 사용. 정상 빌드는 Go `go-v2-export` → `pipeline_items_v2` 테이블 (블록 단위 ID) 사용.
  - Python 빌드: 64,940 entries, line-hash ID (`line-xxxx`), 라인 단위 source
  - Go 빌드: 75,204 entries, path-based ID (`AR_CoastMap/.../blk-0`), 블록 단위 source
  - 게임은 블록 단위로 렌더링하므로 Go 빌드만 매칭됨
- **수정:** build_patch_package_unified.ps1 전면 재작성 — Python 스크립트 대신 `go-v2-export` 호출
- Commit: e711c41
- 재배포 후 translations_loaded=75,204 확인 (정상)

#### Bug 3: StripRenderingWrapper 불완전 (미해결)
- **발견:** 692건 미번역 중 419건이 래퍼 strip 실패. 스크린샷에서 `</color>` 태그 누출 확인
- **원인:** StripRenderingWrapper가 `<line-indent>`, `<link>`, `<smallcaps>` 미처리. ColorWrapperRegex에 `^...$` 앵커 있어 부분 color 래핑 미매칭.
- 실제 데이터의 65%가 `<#hex><line-indent><link>텍스트</link></line-indent></color>` 형태의 다중 중첩 래퍼
- **상태:** 수정 필요

#### Bug 4: Lexicon 규칙 추가 누락 (미해결)
- **발견:** 146건 짧은 텍스트/고유명사/단일단어가 lexicon에 없음
- **상태:** 규칙 추가 필요

## 현재 상태 (Bug 3/4 수정 전)

| 항목 | 이전 (Phase 4 완료) | 현재 | 목표 |
|------|---------------------|------|------|
| translations_loaded | 75,204 | 75,204 | 75,204 |
| untranslated_count | 838 | 692 | < 100 |
| hits_exact | 46 | 135 | ↑ |
| hits_lexicon | 206 | 3,739 | ↑ |

## 미번역 692건 분류

### Phase 5 내 수정 가능 — 565건 (82%)

| 문제 | 건수 | 수정 위치 |
|------|------|-----------|
| P1: line-indent+smallcaps+color 복합 래퍼 미strip | 308 | Plugin.cs |
| P2: line-indent+link 선택지 래퍼 | 68 | Plugin.cs |
| P3: 기타 복합 래퍼 조합 | 43 | Plugin.cs |
| P4: Lexicon 규칙 누락 | 146 | runtime_lexicon.json |

### Phase 5 범위 밖 — 127건 (18%)

| 문제 | 건수 | 이유 |
|------|------|------|
| X1: DB source_raw 미존재 (대화/선택지) | 81 | 재번역 필요 |
| X2: 긴 텍스트 (주문/능력 설명) | 39 | TextAsset/D-07 scope |
| X3: 번역 품질 (혼합 텍스트) | 4 | 재번역 필요 |
| X4: 특수 color 포맷 (stat 테이블) | 3 | 별도 처리 |

## 남은 작업

1. **P1-P3 수정:** StripRenderingWrapper 재설계 — 다중 중첩 래퍼 순차 strip
2. **P4 수정:** runtime_lexicon.json에 146건 규칙 추가
3. **재빌드/재배포:** build_patch_package_unified.ps1 실행
4. **재검증:** 인게임 untranslated_count 확인
5. **05-03 Task 2 완료:** SUMMARY.md 작성, STATE/ROADMAP 업데이트
