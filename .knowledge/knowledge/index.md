# Knowledge Index

Last compiled: 2026-04-12
Total entries: 18

## Quick Reference

| Topic | File | Key Items |
|-------|------|-----------|
| RAG 구현 방식 | decisions.md | PageIndex 거절 이유, enriched termbank 채택 |
| applyProjectDefaults 버그 | decisions.md | sqlite 기본값 v2pipeline 상속 버그 수정 |
| v2pipeline lore 통합 | decisions.md | ragcontext 패키지 신규 생성 |
| voice card 재설계 | decisions.md | wiki/RAG 기반 + relationships 필드 + 어미 예시 |
| watchdog probe 방식 | decisions.md | deepProbe → probeServer (HTTP GET) |
| DB 리셋 쿼리 제약 | guardrails.md | score_final=-1, failure_type/last_error/claimed_by='' |
| v2pipeline 백엔드 | guardrails.md | --backend postgres 명시 필수 |
| warmup 실패 처리 | guardrails.md | UpdateRetryState 필수 (3개 워커 모두) |
| 대량 실행 전 샘플 검증 | guardrails.md | 10건 샘플로 voice/RAG 주입 확인 + 특수 말투 육안 검토 |
| main.go 플래그 회귀 | guardrails.md | Phase 완료 커밋 전 git diff 확인 |
| v2pipeline 복원 순서 | guardrails.md | contracts→store→clustertranslate→scorellm→v2pipeline→main (9계층) |
| total=0 버그 | troubleshooting.md | SQLite 연결 오인식 |
| A/B 테스트 미완료 | troubleshooting.md | max_passes=15, concurrency=2 부족 |
| psql NULL 에러 | troubleshooting.md | score_final NOT NULL 제약 |
| 경로 중복 | troubleshooting.md | PROJECT_ROOT vs PROJECT_DIR |
| watchdog false-kill | troubleshooting.md | deepProbe timeout → probeServer로 교체 |
| OpenCode hang + stale | troubleshooting.md | --cleanup-stale-claims + warmup UpdateRetryState |
| voice/RAG 플래그 누락 | troubleshooting.md | main.go 플래그 회귀, 샘플 검토 선행 필수 |

## Summary

Phase 07.1(RAG 통합)~Phase 08(재번역 인프라) 실행 중 축적된 지식.
핵심 함정 3가지:
1. **total=0**: project.json sqlite 기본값이 v2pipeline에 상속됨 → `--backend postgres` 명시 필수
2. **watchdog false-kill**: deepProbe(LLM 요청)는 서버 바쁠 때 timeout → probeServer(HTTP GET)로 교체
3. **voice/RAG 플래그 누락**: worktree 버그 후 main.go 미복원 → 35K건 번역 품질 실패 (Phase 08 핵심 교훈)

Phase 09 plans 완성 (2026-04-12): 09-01(코드 복원+voice card 재생성+샘플 검증) → 09-02(35,009건 전량 재번역) → 09-03(export+인게임 검증). checker PASS. rag_batch_context.json Phase 07.1에서 이미 존재 확인.

## Keywords

- PageIndex, VectifyAI → decisions.md
- enriched termbank, rag_batch_context → decisions.md
- applyProjectDefaults, checkpoint_backend, sqlite → decisions.md, troubleshooting.md
- total=0, v2pipeline → troubleshooting.md
- NOT NULL, score_final, failure_type → guardrails.md, troubleshooting.md
- A/B 테스트, max_passes, concurrency → troubleshooting.md
- watchdog, deepProbe, probeServer, false-kill → decisions.md, troubleshooting.md
- EnsureContext, warmup, UpdateRetryState → guardrails.md, troubleshooting.md
- voice card, relationships, Kattegatt, 고어체 → decisions.md, troubleshooting.md
- main.go, CLI 플래그, 회귀 → guardrails.md, troubleshooting.md
- VL_Visken, Ôst, kobold, 샘플 검증 → decisions.md
