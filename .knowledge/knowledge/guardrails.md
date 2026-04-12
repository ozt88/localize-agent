# Guardrails

## pipeline_items_v2 상태 리셋
리셋 쿼리에서 `score_final`은 반드시 `-1`로 설정 (NULL 불가 — NOT NULL 제약).
`failure_type`, `last_error`, `claimed_by`도 NULL 불가 → `''`(빈 문자열)로 설정.

## v2pipeline 백엔드 지정
v2pipeline CLI 실행 시 반드시 `--backend postgres`를 명시해야 한다.
project.json의 translation.checkpoint_backend 기본값("sqlite")이 v2pipeline에 상속될 수 있음 — CLI 플래그로 명시적으로 overwrite.

## 스스로 확인 가능한 건 직접 확인
파일 존재, 실행 결과, DB 상태 등 도구(Read, Bash, Glob, Grep)로 확인 가능한 사항을 사용자에게 묻지 않는다.

## EnsureContext(warmup) 실패 시 UpdateRetryState 필수
worker.go의 translate/format/score 3개 워커 모두 warmup(EnsureContext) 실패 시 `UpdateRetryState` 호출이 없으면 items가 lease 만료(최대 400s)까지 working 상태로 묶힘. warmup 에러 핸들러에 반드시 retry 로직 추가.

## 파이프라인 대량 실행 전 10건 샘플 검증 필수
voice cards + RAG context가 실제로 프롬프트에 주입되는지 10건 샘플로 확인 후 특수 말투 캐릭터(고어체 등) 육안 검토. 컴파일 성공 ≠ 품질 보장.

## main.go CLI 플래그 회귀 확인
Phase 완료 커밋 전 `git diff HEAD~1 -- **/main.go`로 CLI 플래그 누락 여부 확인. 코드 복원 후 main.go 플래그가 빠지는 사례 있음 (Phase 08-01 교훈).

## v2pipeline 다계층 코드 복원 순서
worktree 버그 후 v2pipeline 코드 복원 시 컴파일 의존 순서 준수:
`contracts → store → clustertranslate/types → clustertranslate/prompt → scorellm → v2pipeline/types → worker → run → main.go`
중간 계층 미완성 상태에서 `go build ./...` 실행 시 컴파일 에러. 각 계층 완료 후 단계적으로 빌드 확인 필수.
