# Phase 1: 소스 준비 & 파서 - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-22
**Phase:** 01-source-parser
**Areas discussed:** 파서 출력 형식, 블록 경계 검증, 콘텐츠 유형 분류, v1 데이터 처리

---

## 파서 출력 형식

| Option | Description | Selected |
|--------|-------------|----------|
| JSON 파일 → DB | v1처럼 source_*.json으로 출력 후 인제스트 도구로 DB 삽입 | |
| DB 직접 삽입 | 파서가 직접 PostgreSQL에 넣음 | |
| Claude 재량 | Claude가 최적 방식 선택 | ✓ |

**User's choice:** Claude 재량
**Notes:** 블록 ID 생성, textassets 역삽입 위치, 모듈 위치도 모두 Claude 재량으로 위임.

### 메타데이터 설계
**User's insight:** "메타데이터 활용목적은? 번역 품질을 위한 맥락은 번역 LLM에게 전달되어야하고, 안전한 포맷팅을 위한 정보는 포맷 LLM에게 전달해야해"
→ 소비자별 메타데이터 분리 설계로 결정.

---

## 블록 경계 검증

### 검증 기준 탐색
1. translations.json 매칭 제안 → 사용자 의문 제기
2. 분석 결과: translations.json은 v1 산출물이라 검증 기준으로 부적합
3. 사용자 제안: "파서 출력이 게임에서 의미 있는 결과인지 보는 것"
4. Plugin.cs 분석: `TranslationMap[source]`가 게임이 보내는 실제 문자열과 매칭
5. 결론: 진짜 검증 기준은 **게임 런타임이 보내는 실제 문자열**

### Plugin.cs 전수 캡처 모드 구현
- `TryTranslate`에 origin 파라미터 추가
- 6개 훅에서 출처 전달: tmp_text, ui_text, ui_elements, ink_dialogue, ink_choice, menu_scan
- `ENABLE_FULL_CAPTURE` 파일로 활성화
- 빌드 성공, 경고/오류 0개

### 캡처 실행
1. v1 패치 포함 상태로 1차 플레이 → 70% 한국어 (검증 부적합)
2. 패치 제거 후 2차 플레이 → 100% 영문 순수 캡처 (4,550건)

### 핵심 발견
- 게임 엔진이 렌더링 래퍼 추가: `<line-indent>`, `<#hex>`, `<size>`, `<smallcaps>`
- 래퍼 벗기면 translations.json source 키와 매칭 가능
- 결정론적 파싱이므로 패턴별 샘플로 전체 검증 가능

---

## 콘텐츠 유형 분류

| Option | Description | Selected |
|--------|-------------|----------|
| ink 전용 | Phase 1은 ink JSON 파싱만 | |
| 전체 분류 | ink + UI + 메뉴 + 시스템 다 분류 | ✓ |

**User's choice:** 전체 분류
**Notes:** ink 내부 분류 기준은 Claude 재량.

---

## v1 데이터 처리

| Option | Description | Selected |
|--------|-------------|----------|
| 새 테이블 | 기존 items 유지 + v2용 새 테이블 | ✓ |
| 초기화 | 기존 items 비우고 재사용 | |
| 별도 DB | 새 PostgreSQL DB | |

**User's choice:** 새 테이블
**v1 참조:** 참조 안 함 — 완전 독립 재번역

### 용어집 논의
- v1은 wiki에서 취합했으나 결과 아쉬움
- 게임 내장 GlossaryTerms.txt 발견 (54+ 용어, 카테고리/DC 포함)
- 4개 소스 결합: GlossaryTerms.txt > localizationtexts > wiki > 파서 부산물

---

## Claude's Discretion

파서 출력 형식, 블록 ID 생성, textassets 위치 기록, 모듈 위치, ink 콘텐츠 분류 기준

## Deferred Ideas

- 용어집 구축 자체는 Phase 2
- 패치 빌드 스크립트 수정은 프로젝트 범위 밖
