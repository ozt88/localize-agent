# Phase 06: Foundation — 프롬프트 재구조화 + 화자 검증 + 재번역 CLI - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-06
**Phase:** 06-foundation-cli
**Areas discussed:** 프롬프트 구조, 화자 검증 전략, 재번역 선별 기준, 토큰 예산 관리

---

## 프롬프트 구조

### 프롬프트 재구조화 범위

| Option | Description | Selected |
|--------|-------------|----------|
| 클러스터 프롬프트만 | BuildScriptPrompt + v2StaticRules만 개선 (v1.0에서 실제 사용된 경로) | |
| 둘 다 통합 | 클러스터 + 개별 번역 프롬프트를 둘 다 계층 구조로 재구성 | |
| 통합 후 클러스터로 단일화 | 재번역도 클러스터 경로로만 실행. 개별 번역 프롬프트는 UI/overlay용으로만 유지 | ✓ |

**User's choice:** 통합 후 클러스터로 단일화
**Notes:** v1.0에서 40,067건이 클러스터로 처리된 검증된 경로

### ability-score voice guide 주입 방식

| Option | Description | Selected |
|--------|-------------|----------|
| 워밍업에 포함 | 현재처럼 v2_base_prompt.md 전체를 워밍업에 넣음 | |
| per-batch 동적 주입 | 배치의 speaker_hint를 확인하고 해당 캐릭터 voice guide만 주입 | |
| 둘 다 | 워밍업에 전체 가이드 + per-batch에 해당 캐릭터 강조 | ✓ |

**User's choice:** 둘 다
**Notes:** 토큰 더 쓰지만 가장 견고

---

## 화자 검증 전략

### 화자 allow-list 생성 방식

| Option | Description | Selected |
|--------|-------------|----------|
| DB 쿼리로 자동 생성 | DISTINCT speaker 추출 → 빈도 분포 → 상위 N개를 allow-list | |
| ink 소스 재파싱 | ink JSON의 # 태그에서 화자 정보를 다시 추출 | |
| 두 단계 | DB에서 현황 파악 → 오인식 의심 항목은 ink 소스와 교차 검증 | ✓ |

**User's choice:** 두 단계
**Notes:** DB 빈도 + ink 소스 교차 검증으로 정확도 확보

---

## 재번역 선별 기준

### ScoreFinal threshold 결정

| Option | Description | Selected |
|--------|-------------|----------|
| 고정값 시작 | ScoreFinal < 7.0 같은 고정 임계값으로 시작, 결과 보고 조정 | |
| 분포 분석 후 결정 | score_final 히스토그램 분포 분석 후 자연스러운 cutoff 결정 | ✓ |
| CLI에서 선택하도록 | threshold를 CLI 플래그로 두고 실행 시 지정 | |

**User's choice:** 분포 분석 후 결정
**Notes:** 데이터 기반 접근

### 원본 번역 스냅샷 보존 방식

| Option | Description | Selected |
|--------|-------------|----------|
| DB 컬럼 추가 | ko_formatted_backup 컬럼 추가 | |
| JSON 덤프 파일 | 대상 항목을 JSON 파일로 덤프 | |
| retranslation_gen 컬럼 | 세대 번호 컬럼 추가, 각 재번역마다 gen+1 | ✓ |

**User's choice:** retranslation_gen 컬럼
**Notes:** 이전 세대 데이터 유지로 롤백 가능

---

## 토큰 예산 관리

### 토큰 예산 전략

| Option | Description | Selected |
|--------|-------------|----------|
| 프로파일링만 | 현재 토큰수 측정, Phase 07에서 추가 시 조절 | |
| 토큰 상한 및 우선순위 | 모델 컨텍스트 창 대비 상한선 + 우선순위 정의 | |
| Claude 재량 | Phase 06에서는 프로파일링만, 예산 전략은 Phase 07 discuss에서 | ✓ |

**User's choice:** Claude 재량
**Notes:** Phase 06은 측정, Phase 07에서 결정

---

## Claude's Discretion

- 프롬프트 계층 구조의 구체적 포맷
- DB migration 구현 세부사항
- score_final 히스토그램 시각화 방식

## Deferred Ideas

- Score LLM 프롬프트 개선 — v2 요구사항
- 고유명사 정책 통일 — v2 요구사항
- 미번역 154건 — 별도 작업
