# Phase 07: Context Enrichment — 톤 프로필 + 분기 맥락 + 연속성 윈도우 - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-07
**Phase:** 07-context-enrichment
**Areas discussed:** Voice Card 설계, 분기 맥락 주입, 연속성 윈도우 확장, 토큰 예산 + A/B 테스트

---

## Voice Card 설계

### 생성 방식

| Option | Description | Selected |
|--------|-------------|----------|
| LLM 자동 생성 (추천) | 게임 대사 DB에서 캐릭터별 샘플을 추출하고, LLM이 말투/존댓말/성격을 분석하여 JSON voice card 자동 생성 | ✓ |
| 수동 작성 | 주요 캐릭터 10명 정도만 수동 작성, 나머지는 범용 voice guide 적용 | |
| Claude 재량 | voice card 구조와 생성 방식은 Claude가 최적 방안 결정 | |

**User's choice:** LLM 자동 생성

### Card 필드

| Option | Description | Selected |
|--------|-------------|----------|
| 기본 3필드 (추천) | 말투(화법 스타일), 존댓말 레벨(반말/평어/존대), 성격 키워드 | ✓ |
| 확장 5필드 | 말투 + 존댓말 + 성격 + 관계성 + 예시 문장 | |
| Claude 재량 | ability-score 비슷한 구조로 Claude가 설계 | |

**User's choice:** 기본 3필드

### Card 범위

| Option | Description | Selected |
|--------|-------------|----------|
| 상위 15명 (추천) | 100회+ 등장 캐릭터만. Snell(2663)~Thal(100) | ✓ |
| 전체 40+명 | allow-list 전체 | |
| Claude 재량 | 빈도 기준 cutoff를 Claude가 결정 | |

**User's choice:** 상위 15명

### Card 저장

| Option | Description | Selected |
|--------|-------------|----------|
| JSON 파일 (lore.go 패턴) | projects/esoteric-ebb/context/voice_cards.json | |
| DB 저장 | pipeline_items_v2에 voice_card 필드 추가 | |
| Claude 재량 | 저장 방식 Claude 결정 | ✓ |

**User's choice:** Claude 재량

---

## 분기 맥락 주입

### 추출 소스

| Option | Description | Selected |
|--------|-------------|----------|
| ink JSON 파서 확장 (추천) | inkparse에서 파싱 시 choice container의 부모 선택지 텍스트를 추출 → ParentChoiceText 필드 추가 | ✓ |
| DB 조회 | 번역 시점에 pack_json의 choice 정보로 DB에서 부모 선택지 텍스트를 동적 조회 | |
| Claude 재량 | 추출 방식 Claude 결정 | |

**User's choice:** ink JSON 파서 확장

### 주입 위치

| Option | Description | Selected |
|--------|-------------|----------|
| [CONTEXT] 블록에 추가 (추천) | 기존 PrevGateLines처럼 [CONTEXT] 섹션에 "Player chose: X" 형태로 추가 | |
| Voice 섹션에 추가 | v2Sections.Voice에 분기 맥락 포함 | |
| Claude 재량 | 주입 위치 Claude 결정 | ✓ |

**User's choice:** Claude 재량

---

## 연속성 윈도우 확장

### 윈도우 범위

| Option | Description | Selected |
|--------|-------------|----------|
| prev/next 3줄 (로드맵 기준) | 현재 배치 앞뒤로 3줄씩. 재번역 시에는 기존 한국어 번역(prevKO/nextKO)도 함께 주입 | ✓ |
| prev 3줄만 (보수적) | 현재 PrevGateLines 확장. next는 미번역 상태에서 의미 없음 | |
| Claude 재량 | 윈도우 범위 Claude 결정 | |

**User's choice:** prev/next 3줄 (로드맵 기준)

### KO 소스

| Option | Description | Selected |
|--------|-------------|----------|
| DB ko_formatted 조회 (추천) | prev/next line_id로 DB에서 해당 아이템의 ko_formatted를 조회 | ✓ |
| sidecar JSON 조회 | translations.json sidecar에서 조회 | |
| Claude 재량 | 소스 방식 Claude 결정 | |

**User's choice:** DB ko_formatted 조회

---

## 토큰 예산 + A/B 테스트

### 토큰 우선순위

| Option | Description | Selected |
|--------|-------------|----------|
| continuity > branch > voice (추천) | voice card 최우선 유지, 토큰 초과 시 continuity window부터 축소/제거 | ✓ |
| 전체 비례 축소 | 모든 컨텍스트를 비례적으로 축소 | |
| Claude 재량 | 토큰 관리 전략 Claude 결정 | |

**User's choice:** continuity > branch > voice

### A/B 테스트 구성

| Option | Description | Selected |
|--------|-------------|----------|
| 10개 배치 비교 (추천) | 저품질 배치 10개를 컨텍스트 주입 전/후로 번역하고 score 비교 | ✓ |
| 50개 배치 통계 | 더 큰 샘플로 통계적 유의성 확인 | |
| Claude 재량 | A/B 테스트 규모와 방법 Claude 결정 | |

**User's choice:** 10개 배치 비교

---

## Claude's Discretion

- voice card JSON 파일 저장 위치 및 로드 방식
- 분기 맥락의 프롬프트 주입 위치
- 토큰 예산 상한값
- A/B 테스트 배치 선택 기준
- voice card 생성용 LLM 프롬프트 설계
- continuity window의 next 라인 처리 (최초 번역 시)

## Deferred Ideas

None
