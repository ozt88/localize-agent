---
created: 2026-03-25T03:34:24.960Z
title: Format skip when tags preserved
area: pipeline
files:
  - workflow/internal/v2pipeline/store.go:328
  - workflow/internal/v2pipeline/worker.go
---

## Problem

번역 LLM이 태그를 이미 정확히 보존한 경우에도 format LLM을 호출함.
format LLM이 할 일이 없어서 JSON 파싱 에러 발생 (189건).
불필요한 LLM 호출 + 에러 유발.

## Solution

`MarkTranslated`에서 ko_raw와 source_raw의 태그를 비교:
- 태그 수/종류가 일치하면 → ko_formatted = ko_raw, pending_score로 직행
- 불일치하면 → 기존대로 pending_format으로 라우팅
tag masking 구현 시 이 로직은 자연스럽게 대체됨.
