---
phase: 06-foundation-cli
plan: 02
subsystem: inkparse
tags: [speaker-verification, allow-list, parser-enhancement, coverage-audit]

# Dependency graph
requires:
  - phase: 01-source-parser
    provides: inkparse package with isSpeakerTag heuristic
provides:
  - speaker_allow_list.json (73 verified speakers, 32 rejected tags)
  - SpeakerAllowList Go type with LoadSpeakerAllowList/IsAllowed
  - globalAllowList + SetSpeakerAllowList for isSpeakerTag priority-1 check
affects: [06-03-retranslate-cli, 07-context-injection]

# Tech tracking
tech-stack:
  added: []
  patterns: [allow-list priority check in parser, global package variable with setter]

key-files:
  created:
    - projects/esoteric-ebb/context/speaker_allow_list.json
    - workflow/internal/inkparse/speaker_allowlist.go
    - workflow/internal/inkparse/speaker_allowlist_test.go
  modified:
    - workflow/internal/inkparse/parser.go
    - workflow/internal/inkparse/edge_cases_test.go

key-decisions:
  - "allow-list를 isSpeakerTag 최우선 체크로 통합 (기존 휴리스틱보다 우선)"
  - "globalAllowList 패키지 변수 + SetSpeakerAllowList() setter 패턴"
  - "커버리지 47.6% — 90% 미달로 Task 3 파서 강화 실행"
---

# Plan 06-02 Summary

## Objective
pipeline_items_v2의 speaker 커버리지를 감사하고, 빈도 기반 + ink 교차 검증으로 화자 allow-list를 생성하며, allow-list 기반 isSpeakerTag 파서 강화를 구현.

## Task Results

### Task 1: DB 화자 커버리지 감사 + allow-list JSON + Go 필터 함수
- **Status:** ✓ Complete
- **Commit:** 29730a5
- **Results:**
  - total_dialogue: 32,370 / with_speaker: 15,414 / coverage_pct: 47.6%
  - 73 verified speakers, 32 rejected tags
  - speaker_allow_list.json 생성
  - SpeakerAllowList Go type + LoadSpeakerAllowList + IsAllowed 구현
  - 116줄 테스트 파일

### Task 2: Human Verification Checkpoint
- **Status:** ✓ Approved
- **User verified:** allow-list 내용 확인, 47.6% → Task 3 실행 승인

### Task 3: isSpeakerTag 파서 강화 (조건부 — 커버리지 < 90%)
- **Status:** ✓ Complete
- **Commit:** bf261ec
- **Changes:**
  - globalAllowList 패키지 변수 + SetSpeakerAllowList() setter 추가
  - isSpeakerTag()에 allow-list 최우선 체크 통합
  - edge_cases_test.go에 4개 allow-list 통합 테스트 추가

## Deviations
- 없음. Plan대로 실행.

## Self-Check: PASSED
- [x] speaker_allow_list.json 존재 (73 speakers)
- [x] SpeakerAllowList.IsAllowed 함수 존재
- [x] globalAllowList + SetSpeakerAllowList 존재
- [x] isSpeakerTag에 allow-list 우선 체크 통합
- [x] 테스트 파일 존재 (speaker_allowlist_test.go + edge_cases_test.go)
