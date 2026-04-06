# Esoteric Ebb 한국어 번역 파이프라인 v2

## What This Is

Esoteric Ebb(내러티브 cRPG, Ink 스크립트 기반)의 한국어 번역 파이프라인 v2. ink JSON 트리를 대사 블록 단위로 파싱하고, 씬 단위 클러스터 번역 + 포맷터 LLM으로 태그를 복원하는 2단계 아키텍처. 40,067건 전량 번역 완료, 게임 내 패치 적용 및 검증됨.

## Core Value

게임이 실제로 렌더링하는 대사 블록 단위로 소스를 생성하여, 태그 깨짐 없이 한국어 패치가 동작해야 한다.

## Current State (v1.0 shipped 2026-03-29)

- **파이프라인:** Go CLI 기반, PostgreSQL 상태 관리, OpenCode LLM 백엔드
- **번역:** 35,030 done items → 75,204 sidecar entries + 77,205 contextual entries
- **패치:** Plugin.cs v2 (3-stage TryTranslate + StripAllTmpTags), runtime_lexicon 320 rules
- **커버리지:** 99.8% (154건 미번역 — DB 누락 대화/주문 설명)
- **빌드:** build_patch_package_unified.ps1 → go-v2-export (v2 블록 단위)

## Requirements

### Validated (v1.0)

- ✓ Go CLI 파이프라인 프레임워크 — v1 existing
- ✓ PostgreSQL 기반 파이프라인 상태 관리 — v1 existing
- ✓ OpenCode LLM 백엔드 연동 (gpt-5.4) — v1 existing
- ✓ BepInEx TranslationLoader 플러그인 (Plugin.cs) — v1 existing → v2 rewrite
- ✓ 패치 출력 포맷 — v1 existing → v2 go-v2-export
- ✓ ink JSON 트리 파서 — Phase 01
- ✓ 씬 단위 클러스터 번역 — Phase 02
- ✓ 포맷터 LLM (codex-mini) 태그 복원 — Phase 02
- ✓ 콘텐츠 유형별 입력 설계 — Phase 02
- ✓ Plugin.cs 3-stage TryTranslate + TMP 태그 strip — Phase 04.1, 04.2, 05
- ✓ 패치 빌드 파이프라인 (go-v2-export) — Phase 05
- ✓ 전량 재번역 실행 (40,067건+) — Phase 03
- ✓ 게임 내 검증 (태그 깨짐 없음, 커버리지 99.8%) — Phase 05

### Active (next milestone)

- [ ] 번역 품질 개선 — 맥락 전달 방식 연구, 프롬프트 구조 변경
- [ ] 고유명사 정책 통일 — Cleric→성직자 vs 원문 유지, 전체 일관성
- [ ] 미번역 154건 해소 — DB 누락 대화/주문 설명 추가 번역
- [ ] 선택적 재번역 — 품질 스코어 기반, 낮은 점수만 재번역

### Out of Scope

- 새로운 게임 버전 대응 — 현재 1.1.3 기준
- 다른 언어 지원 — 한국어 전용
- 웹 UI/대시보드 — CLI 파이프라인 유지

## Context

### v2 아키텍처
```
ink JSON tree
  ↓ [코드] 트리 파싱 → 분기 구조 보존한 스크립트
  ↓ [LLM: gpt-5.4] 스크립트 통째 번역 (태그 없이, 자유 형식)
  ↓ [코드] 라인 분리 → ID 매핑
  ↓ [LLM: codex-mini] 태그 복원 (필요한 라인만)
  ↓ [코드] 최종 검증 (태그 수만 확인)
```

### 기술 환경
- Go 1.24, Python 3.x, C# (BepInEx 플러그인)
- PostgreSQL 5433 (localize_agent), pipeline_items_v2 테이블
- OpenCode 서버 127.0.0.1:4112 (openai/gpt-5.4, codex-mini)
- 게임: E:\SteamLibrary\steamapps\common\Esoteric Ebb (v1.1.3)

## Constraints

- **LLM 백엔드**: OpenCode 서버 — 로컬 인프라, 외부 API 없음
- **게임 버전**: 1.1.3 고정, ink JSON 구조 변경 없음
- **기존 코드**: v1 파이프라인 프레임워크 위에 구축
- **DB 규칙**: source_raw 기준 중복 체크 필수
- **패치 포맷**: BepInEx TranslationLoader 호환

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| 전량 재번역 (v1 결과 폐기) | v1 소스 단위가 근본적으로 잘못됨 | ✓ Good — 75,204 entries |
| 2단계 LLM (번역 + 포맷팅 분리) | LLM이 태그를 변형하는 문제 해결 | ✓ Good |
| 씬 단위 클러스터 번역 | 문맥 보존 + 분기 구조 포함 | ✓ Good |
| Plugin.cs v2 재작성 | v1 레거시 가정으로 인한 연쇄 버그 해결 | ✓ Good — 3,083→1,821 lines |
| 빌드 스크립트 Go export 전환 | Python v1 빌드가 잘못된 데이터 사용 | ✓ Good — go-v2-export |
| StripAllTmpTags + ContainsKorean | 복잡한 re-wrap 대신 단순 strip + passthrough | ✓ Good — 태그 손상 0 |

---
*Last updated: 2026-03-29 after v1.0 milestone completion*
