# Decisions

## [active] 턴 수집은 CLAUDE.md 행동 지시 방식

- **맥락**: 지식 컴파일러 Stage 1 — 매 턴 요약을 raw/에 자동 저장하는 메커니즘 선택
- **시도**: Stop Hook `type: "command"` → 발동 O, 컨텍스트 접근 X. Stop Hook `type: "prompt"` → 컨텍스트 접근 O, 검증 모드로 동작하여 생성 불가
- **결정**: CLAUDE.md에 행동 지시 추가 — 매 응답 마지막에 `.knowledge/raw/{YYYY-MM-DD}.md`에 턴 요약 append
- **근거**: 메인 에이전트는 컨텍스트 접근 + 파일 쓰기 모두 가능. 가끔 빼먹을 수 있으나 MVP 허용 범위
- **출처**: raw/2026-04-05.md

## [rejected] Stop Hook prompt 타입으로 턴 요약 자동 생성

- **맥락**: Stop Hook의 `type: "prompt"`가 턴 요약을 생성하여 파일에 저장할 수 있는지 테스트
- **시도**: prompt에 "턴 요약을 한국어로 2-3줄 생성하라" 지시
- **결과**: prompt가 생성이 아닌 검증/게이트 모드로 동작 — 응답이 조건에 부합하는지 평가만 함. 파일 쓰기 도구 접근도 불가
- **결정**: 기각. prompt 훅은 출력 검증용이지 콘텐츠 생성용이 아님
- **원칙**: Claude Code Stop Hook의 prompt 타입은 response validation 용도
- **출처**: raw/2026-04-05.md

## [active] 컴파일은 GSD Phase 경계에서 자동 실행

- **맥락**: raw/ → knowledge/ 컴파일 트리거를 수동(/compile)이 아닌 자동으로
- **결정**: gsd-phase-researcher Step 0 (incremental) + gsd-verifier Step 10b (full reconcile)
- **근거**: 사용자가 수동 실행을 빼먹을 가능성 높음. Phase 경계는 자연스러운 트리거 지점
- **출처**: raw/2026-04-05.md
