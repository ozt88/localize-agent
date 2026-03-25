---
created: 2026-03-25T03:34:24.960Z
title: Tag masking translation
area: pipeline
files:
  - workflow/internal/clustertranslate/prompt.go
  - workflow/internal/v2pipeline/worker.go
---

## Problem

번역 LLM에 `<b>`, `<i>`, `<shake>` 등 HTML 태그가 포함된 source_raw가 그대로 전달됨.
현재는 번역 LLM이 태그를 잘 보존하고 있어서 문제가 안 되지만, 구조적으로 분리하면:
- format 단계가 불필요해짐 (189건 format parse 에러의 근본 원인)
- 번역 품질에 태그가 영향을 주지 않음
- 토큰 절약 (태그 문자열 제외)

## Solution

1. 번역 전 태그를 플레이스홀더로 마스킹: `<i>text</i>` → `[T1]text[/T1]`
2. 번역 후 플레이스홀더를 원본 태그로 복원
3. format 단계 스킵 가능 — LLM 호출 없이 기계적 복원
4. Phase 3 실행 중 발견 (03-EXECUTION-LOG.md 참조)
