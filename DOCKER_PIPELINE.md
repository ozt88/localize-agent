# Docker Compose Pipeline

이 구성은 `translate / score / retranslate` worker를 별도 컨테이너로 실행합니다.

전제:
- SQLite DB와 프로젝트 파일은 bind mount로 공유합니다.
- Ollama와 OpenCode는 우선 컨테이너 밖에서 실행 중이라고 가정합니다.
- 컨테이너에서는 `host.docker.internal`로 외부 LLM 서버를 연결합니다.

기본 주소:
- low LLM: `http://127.0.0.1:11438`
- high / score LLM: `http://127.0.0.1:4112`

## 1. 초기화(init)

`pipeline-init`는 one-shot 작업입니다.
- `pipeline_items` reset
- seed
- 종료

```powershell
docker compose -f docker-compose.pipeline.yml run --rm pipeline-init
```

## 2. worker 실행

worker는 reset 없이 장기 실행합니다.

```powershell
docker compose -f docker-compose.pipeline.yml up --build -d translate-worker score-worker retranslate-worker
```

## 3. 중지

```powershell
docker compose -f docker-compose.pipeline.yml down
```

## 4. 로그

```powershell
docker compose -f docker-compose.pipeline.yml logs -f translate-worker
docker compose -f docker-compose.pipeline.yml logs -f score-worker
docker compose -f docker-compose.pipeline.yml logs -f retranslate-worker
```

## 주의

- reset/seed는 `pipeline-init`에서 먼저 수행하고, worker는 reset 없이 실행하는 흐름을 권장합니다.
- SQLite를 여러 worker가 공유하므로 DB 상태가 source of truth입니다.
- claim/lease는 transaction-safe하게 처리되지만, live DB를 단순 파일 복사하면 WAL 상태 때문에 손상처럼 보일 수 있습니다.
- 서버 주소가 다르면 `docker-compose.pipeline.yml`의 `--*-server-url` 값을 바꾸면 됩니다.
