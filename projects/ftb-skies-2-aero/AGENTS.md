# FTB Skies 2: Aero

- 등록 정보는 `project.json`, 정책은 `context/translation-policy.json`, 현재 릴리즈는 `releases/1.13.0-ko-v1.json`을 먼저 읽는다.
- 이 프로젝트는 `tools/run_pack_translation.py`와 공용 `workflow/cmd/structured-job`을 사용한다. 기존 DB 기반 `go-v2-pipeline`의 프로젝트 전환만으로 실행하지 않는다.
- 번역과 독립 통합 검수 모두 `gpt-6-sol`, `medium`을 사용한다. 서로 다른 그룹은 최대 3개 병렬, 같은 그룹은 순서를 지킨다.
- 현재 1.13.0 영어 및 설치 데이터가 사실 기준이다. 과거 번역 문장은 재사용하지 않는다. 용어는 현재 아이템·개념 ID 및 출처를 확인한다.
- `output/admission-current.json`에 편입된 항목은 다시 번역하지 않는다. 실패·보류 항목만 근거를 보완한다. 불확실한 유료 호출을 자동 재발행하지 않는다.
- `output/`은 로컬 실행 상태이며 Git에 포함하지 않는다. 깨끗한 clone에는 기존 검수 영수증과 원본 스냅샷이 없으므로 준비·편입 데이터를 확보하기 전 번역을 시작하지 않는다. 날짜가 고정된 기존 도구는 완료한 1.13.0 실행의 재개 도구이며 새 버전용 범용 실행기가 아니다.
- `FTB_AERO_INSTANCE`로 설치 경로를 지정한다. 생략하면 사용자 홈의 `curseforge/minecraft/Instances/FTB Skies 2 Aero`를 사용한다. 주 실행기의 명시적 `--instance`를 우선한다.
- 실제 인스턴스 설치는 별도 사용자 요청으로만 한다. `tools/install_local_client.py`는 종료 상태·원본 해시를 확인하고 변경 파일을 백업한다. 설치 후 원본 스냅샷과 달라진 스크립트는 정상이며 원본으로 임의 덮어쓰지 않는다.
- Git에는 코드, 정책, 상대 경로 기반 용어 출처, 공개 릴리즈 요약만 보관한다. 번역 ZIP은 GitHub Release 자산으로 제공한다. 개인 경로, 인증정보, 실행 로그와 로컬 백업을 커밋하지 않는다.
