# Guardrails

## pipeline_items_v2 상태 리셋
리셋 쿼리에서 `score_final`은 반드시 `-1`로 설정 (NULL 불가 — NOT NULL 제약).
`failure_type`, `last_error`, `claimed_by`도 NULL 불가 → `''`(빈 문자열)로 설정.

## v2pipeline 백엔드 지정
v2pipeline CLI 실행 시 반드시 `--backend postgres`를 명시해야 한다.
project.json의 translation.checkpoint_backend 기본값("sqlite")이 v2pipeline에 상속될 수 있음 — CLI 플래그로 명시적으로 overwrite.

## 스스로 확인 가능한 건 직접 확인
파일 존재, 실행 결과, DB 상태 등 도구(Read, Bash, Glob, Grep)로 확인 가능한 사항을 사용자에게 묻지 않는다.
