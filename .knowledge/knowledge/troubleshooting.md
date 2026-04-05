# Troubleshooting

## Stop Hook이 발동하지 않음

- **증상**: settings.json에 Stop hook 설정했으나 로그 파일 미생성
- **원인**: bash -c 명령의 따옴표 이스케이프 오류 — JSON 내 큰따옴표 중첩으로 명령 파싱 실패
- **해결**: 작은따옴표로 bash -c 인자 감싸기: `bash -c 'echo ... >> file'`
- **확인**: hook 대상 파일이 생성/갱신되는지 다음 턴에서 직접 확인

## Stop Hook prompt 피드백이 매 턴 반복됨

- **증상**: "Stop hook feedback: [prompt text]: ..." 메시지가 매 응답마다 출력
- **원인**: prompt 타입 훅이 매 Stop 이벤트마다 검증 결과를 피드백으로 반환
- **해결**: 테스트 완료 후 `"Stop": []`로 비활성화, 또는 필요한 훅으로 교체
