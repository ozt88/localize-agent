---
created: 2026-03-25T03:34:24.960Z
title: Review dashboard v2 support
area: tooling
files:
  - workflow/cmd/go-review/main.go
---

## Problem

Review 대시보드(http://127.0.0.1:8094)가 v1 `pipeline_items` 테이블만 지원.
v2 `pipeline_items_v2` 테이블의 진행 상황을 웹에서 모니터링할 수 없음.
현재는 psql 직접 쿼리로만 확인 가능.

## Solution

go-review에 pipeline_items_v2 테이블 지원 추가:
- 테이블 존재 여부 자동 감지 (hasTableForBackend 재사용)
- v2 상태 카운트 대시보드
- v2 항목 검색/필터/상세 보기
