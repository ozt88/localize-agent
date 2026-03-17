# LLM Translation Pipeline Onboarding

## 목적
이 문서는 새 게임이나 새 LLM CLI 환경에서 현재 번역 파이프라인을 재사용할 때 필요한 최소 준비사항을 정리한다.

대상 환경 예:
- Ollama
- OpenCode
- Claude CLI 계열
- Codex CLI 계열
- 기타 로컬/원격 LLM 서비스

핵심 원칙:
- vendor를 직접 기준으로 삼지 않는다
- project-local contract와 low/high/score 역할 분리를 먼저 정의한다
- runtime worker는 DB 상태를 source of truth로 사용한다 (PostgreSQL 권장, SQLite fallback)

## 1. 필수 개념

### 1.1 project-local contract
프로젝트마다 고정 규칙은 project 하위 파일로 둔다.

필수 예시:
- `context/<project>_translation_system.md`
- `context/<project>_semantic_review_system.md`

이 파일은 backend별로 다르게 구현되더라도 같은 의미를 유지해야 한다.
- Ollama: baked Modelfile 또는 system/history
- OpenCode: warmup
- 기타 CLI: system prompt / preamble

### 1.2 역할 기반 LLM profile
모델은 vendor 기준이 아니라 역할 기준으로 나눈다.

필수 역할:
- `low_llm`
  - 대량 baseline translation
- `high_llm`
  - 재번역 / uplift
- `score_llm`
  - semantic oddness scoring

### 1.3 response contract
출력 계약은 backend가 아니라 contract로 관리한다.

권장:
- main translation: `plain`
- evaluator / rich metadata가 필요한 review: `json` 또는 `plain score`

### 1.4 DB state machine
checkpoint DB를 source of truth로 사용한다 (PostgreSQL 권장).

현재 최소 state:
- `pending_translate`
- `pending_score`
- `pending_retranslate`
- `done`
- `failed`

## 2. 새 게임 온보딩 체크리스트

### 2.1 source 준비
- raw source를 준비한다
- 필요하면 translator package / chunk package를 만든다
- `source/prepared`에 아래를 만든다
  - `source_*.json`
  - `current_*.json`
  - `ids_*.txt`

확인:
- IDs 개수 확인
- source/current key 일치 확인

### 2.2 project config 작성
`projects/<game>/project.json`에 다음을 정의한다.

필수:
- `translation`
- `evaluation`
- `pipeline`

`pipeline` 필수 항목:
- `stage_batch_size`
- `threshold`
- `max_retries`
- `low_llm`
- `high_llm`
- `score_llm`

각 LLM profile 권장 항목:
- `llm_backend`
- `server_url`
- `model`
- `agent`
- `context_files`
- `translator_response_mode`
- `concurrency`
- `batch_size`
- `timeout_sec`

Ollama라면 추가 가능:
- `ollama_baked_system`
- `ollama_reset_history`
- `ollama_keep_alive`
- `ollama_num_ctx`
- `ollama_temperature`

### 2.3 project-local contract 작성
최소 두 파일을 만든다.

번역용:
- 프로젝트 톤
- register
- lexical guidance
- text-type translation behavior

semantic review용:
- weirdness score 기준
- meaning drift / alignment / referent drift 기준
- 출력 contract 기본 원칙

중요:
- 복원/태그/제어토큰 같은 deterministic 처리 규칙은 시스템이 우선 책임진다
- LLM에게 시스템이 처리 가능한 일을 맡기지 않는다

### 2.4 control token / prefix 규칙 정의
새 게임에 아래 같은 시스템 토큰이 있는지 확인한다.
- choice prefix
- control line
- rich-text tags
- variable tokens

분류:
- pure control passthrough
- prefix strip + restore
- control + quoted tail split
- tag masking + restore

이 규칙은 prompt가 아니라 전처리/후처리/validation 레이어에서 처리한다.

### 2.5 baseline 번역 경로 검증
작은 샘플로 먼저 확인한다.

권장:
- `n=10`
- `n=100`

확인할 것:
- completed / invalid / translator_error
- prompt stall 여부
- control token 보존
- prefix exact restore
- viewer 표시 확인

### 2.6 semantic review 경로 검증
권장 기본:
- `direct`
- `score-only`

작은 샘플에서:
- top suspicious line이 실제로 이상한지
- false positive가 과도하지 않은지

### 2.7 pipeline dry run
copy DB에서 먼저 실행한다.

권장:
- `--reset`
- `--seed-limit 3` 또는 `10`
- `--max-retries 1`

확인:
- `pipeline_items` 생성
- state 전이
- score 후 retry queue 이동
- 최종 `done/failed` 전이

### 2.8 본실행
메인 DB에서 실행한다.

권장 순서:
1. baseline translation 일부 적재
2. semantic review
3. threshold 조정
4. pipeline full run

## 3. 새 CLI / 새 backend 온보딩 체크리스트

### 3.1 필요한 최소 capability
새 CLI/backend는 아래를 충족해야 한다.

필수:
- prompt 전송 가능
- timeout 제어 가능
- model 선택 가능
- plain 또는 json 응답 계약 중 하나 지원

있으면 좋은 것:
- system prompt / warmup
- session 유지
- batching
- concurrency

### 3.2 backend 연결 방식
backend는 아래 둘 중 하나면 된다.

- HTTP server style
  - `server_url`
- command/CLI wrapper style
  - 향후 adapter에서 command 실행으로 감쌀 수 있음

현재 공통 인터페이스에서 중요한 것은 vendor 이름이 아니라:
- request 입력
- response contract
- timeout / concurrency / batching

### 3.3 contract 매핑
프로젝트 contract를 backend에 맞게 매핑한다.

예:
- Ollama: Modelfile or baked system
- OpenCode: warmup
- Claude/Codex CLI: system prompt + payload wrapper

중요:
- source contract는 하나
- backend별 adapter만 다르게

### 3.4 response contract 선택
권장:
- translation path: `plain`
- score-only review: `plain`
- detailed evaluator: `json`

질문:
- 이 backend가 strict JSON에서 불안정한가?
- plain output이 더 빠른가?
- batch에서 순서/index 매핑이 가능한가?

## 4. 실전 운영 가이드

### 4.1 translator와 evaluator를 분리
- low_llm은 throughput 우선
- high_llm은 quality uplift
- score_llm은 suspicious candidate 검출

### 4.2 먼저 안정성, 그 다음 속도
확인 순서:
1. 구조 보존
2. invalid / translator_error
3. throughput

### 4.3 작은 샘플에서 stall 패턴을 먼저 확인
특히:
- 짧은 fragment
- quoted dialogue
- control token mixed lines
- aggressive/sarcastic dialogue

### 4.4 profile은 project에 두고 pipeline은 공통으로 유지
다른 게임으로 옮길 때 바뀌는 것은:
- source adapter
- context contract
- control token rules
- profile values

pipeline 코드는 가능하면 공통으로 유지한다.

## 5. 빠른 점검용 체크리스트

새 게임 시작 전에:
- [ ] source/current/ids 준비 완료
- [ ] `project.json`의 `translation/evaluation/pipeline` 작성 완료
- [ ] `low_llm/high_llm/score_llm` 설정 완료
- [ ] translation system contract 작성 완료
- [ ] semantic review system contract 작성 완료
- [ ] control token 규칙 확인 완료
- [ ] `n=10` translation 성공
- [ ] `n=100` translation benchmark 확인
- [ ] semantic review POC 확인
- [ ] pipeline dry run 확인
- [ ] viewer에서 상태 확인 가능

## 6. 현재 Esoteric Ebb 기준 추천
- `low_llm`
  - OpenCode / `openai/gpt-5.2`
  - `plain`
- `high_llm`
  - OpenCode / `openai/gpt-5.4`
  - `plain`
- `score_llm`
  - OpenCode / `openai/gpt-5.4`
  - `direct score-only`

이 조합은 현재 저장소에서 가장 많이 검증된 경로다.
