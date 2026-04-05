# Anti-Patterns

## Stop Hook prompt 타입으로 콘텐츠 생성 시도

- **하지 마라**: Claude Code Stop Hook `type: "prompt"`로 파일 생성이나 콘텐츠 저장을 시도
- **이유**: prompt 타입은 검증/게이트 모드로만 동작. 요약 생성, 파일 쓰기 불가. 매 턴 피드백 노이즈만 발생
- **대신**: 파일 쓰기가 필요하면 `type: "command"` + 외부 스크립트, 또는 CLAUDE.md 행동 지시 사용

## settings.json Stop Hook에 큰따옴표 중첩

- **하지 마라**: `bash -c "echo "..."  >> file"` 형태의 중첩 큰따옴표
- **이유**: JSON 이스케이프 + bash 이스케이프 충돌로 명령이 잘림. 훅이 조용히 실패하여 디버깅 어려움
- **대신**: `bash -c '...'` 작은따옴표로 감싸기
