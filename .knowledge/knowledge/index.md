# Knowledge Index

Last compiled: 2026-04-11
Total entries: 9

## Quick Reference

| Topic | File | Key Items |
|-------|------|-----------|
| RAG 구현 방식 | decisions.md | PageIndex 거절 이유, enriched termbank 채택 |
| applyProjectDefaults 버그 | decisions.md | sqlite 기본값 v2pipeline 상속 버그 수정 |
| v2pipeline lore 통합 | decisions.md | ragcontext 패키지 신규 생성 |
| DB 리셋 쿼리 제약 | guardrails.md | score_final=-1, failure_type/last_error/claimed_by='' |
| v2pipeline 백엔드 | guardrails.md | --backend postgres 명시 필수 |
| total=0 버그 | troubleshooting.md | SQLite 연결 오인식 |
| A/B 테스트 미완료 | troubleshooting.md | max_passes=15, concurrency=2 부족 |
| psql NULL 에러 | troubleshooting.md | score_final NOT NULL 제약 |
| 경로 중복 | troubleshooting.md | PROJECT_ROOT vs PROJECT_DIR |

## Summary

Phase 07.1(RAG 통합) 실행 중 축적된 지식.
핵심: PageIndex는 비실용적이라 enriched termbank + regex 매칭 채택.
v2pipeline에서 project.json의 sqlite 기본값 상속 버그가 가장 치명적 함정 (total=0).
A/B 테스트는 concurrency/passes 설정이 배치 크기에 비해 부족하면 대부분 미처리됨.

## Keywords

- PageIndex, VectifyAI → decisions.md
- enriched termbank, rag_batch_context → decisions.md
- applyProjectDefaults, checkpoint_backend, sqlite → decisions.md, troubleshooting.md
- total=0, v2pipeline → troubleshooting.md
- NOT NULL, score_final, failure_type → guardrails.md, troubleshooting.md
- A/B 테스트, max_passes, concurrency → troubleshooting.md
