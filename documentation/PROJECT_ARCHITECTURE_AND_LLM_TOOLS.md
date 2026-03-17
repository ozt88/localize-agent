# Localize-Agent 프로젝트 아키텍처 및 LLM 도구 문서

## 개요

`localize-agent`는 다중 게임/앱 로컬라이제이션을 위한 재사용 가능한 워크스페이스입니다. 공유 번역 파이프라인 코드와 프로젝트별 소스 파일, 컨텍스트, 출력물, 헬퍼 스크립트를 분리하여 설계되었습니다.

---

## 1. 전체 프로젝트 구조

### 1.1 디렉토리 레이아웃

```
localize-agent/
├── workflow/                 # 공유 엔진, CLI 명령, 파이프라인 로직
│   ├── cmd/               # Go 기반 워크플로우 명령
│   ├── internal/           # 내부 패키지 (플랫폼, LLM, 번역 등)
│   └── context/            # 공유 에이전트, ops, 스타일 가이드
├── projects/               # 프로젝트별 디렉토리
│   ├── esoteric-ebb/      # Esoteric Ebb 프로젝트
│   └── rogue-trader/      # Rogue Trader 프로젝트
├── scripts/               # 관리 스크립트 (PostgreSQL, OpenCode, Review 등)
├── AGENTS.md             # 운영 메모 및 참조 경로
└── README.md             # 리포지토리 개요
```

### 1.2 프로젝트별 구조

각 프로젝트(`projects/<name>/`)는 다음을 포함합니다:

- **project.json**: 프로젝트 프로필 (LLM 설정, 경로 등)
- **context/**: 프로젝트별 프롬프트, 규칙, 시스템 계약
- **source/**: 소스 파일 (번역 대상 텍스트)
- **output/**: 파생 아티팩트 (체크포인트, 번역 결과 등)
- **cmd/**: 프로젝트별 래퍼 스크립트

---

## 2. LLM 도구 아키텍처

### 2.1 지원되는 LLM 백엔드

이 프로젝트는 두 가지 LLM 백엔드를 지원합니다:

| 백엔드 | 용도 | 특징 |
|---------|------|------|
| **OpenCode** | 고성능 번역, 평가, 점수 매기기 | - OpenAI 호환 API 인터페이스<br>- 세션 기반 컨텍스트 관리<br>- 웜업 지원<br>- 높은 품질과 성능 |
| **Ollama** | 로컬 오픈소스 모델 실행 | - 로컬 실행 (비용 없음)<br>- TranslateGemma 등 다양한 모델<br>- 간단한 API<br>- 낮은 네트워크 지연 |

### 2.2 LLM 클라이언트 구조

#### 2.2.1 공용 인터페이스

두 백엔드는 공통 인터페이스를 구현합니다:

```go
type LLMProfile struct {
    ProviderID     string  // "opencode" 또는 "ollama"
    ModelID        string  // "openai/gpt-5.4" 또는 "TranslateGemma:latest"
    Agent          string  // OpenCode 에이전트 이름 (선택 사항)
    Warmup         string  // 웜업 프롬프트 (선택 사항)
    KeepAlive      string  // 세션 유지 시간 (Ollama)
    ResponseFormat any    // 응답 형식 (Ollama)
    Options        map[string]any  // 추가 옵션
    ResetHistory   bool   // 기록 초기화 여부
}

type LLMClient interface {
    EnsureContext(key string, profile LLMProfile) error
    SendPrompt(key string, profile LLMProfile, prompt string) (string, error)
}
```

#### 2.2.2 OpenCode 클라이언트 (`llm_client.go`)

**특징:**
- 세션 기반 컨텍스트 관리
- 웜업 프롬프트 지원
- 에이전트별 세션 격리
- OpenAI 호환 API (`/session`, `/session/{sid}/message`)

**사용 예시:**
```go
client := NewSessionLLMClient("http://127.0.0.1:4112", 120, metrics, traceSink)
profile := LLMProfile{
    ProviderID: "opencode",
    ModelID: "openai/gpt-5.4",
    Agent: "",  // 선택 사항
    Warmup: "You are a Korean translator...",
}
response, err := client.SendPrompt("my-session-key", profile, prompt)
```

**API 엔드포인트:**
- `POST /session` - 새 세션 생성
- `POST /session/{sid}/message` - 메시지 전송

#### 2.2.3 Ollama 클라이언트 (`ollama_client.go`)

**특징:**
- 로컬 모델 실행 (Ollama 서버)
- 메시지 히스토리 관리
- `keep_alive` 지원
- 응답 형식 커스터마이징

**사용 예시:**
```go
client := NewOllamaLLMClient("http://127.0.0.1:11434", 120, metrics, traceSink)
profile := LLMProfile{
    ProviderID: "ollama",
    ModelID: "TranslateGemma:latest",
    Warmup: "You are a Korean translator...",
    KeepAlive: "12h",
}
response, err := client.SendPrompt("my-session-key", profile, prompt)
```

**API 엔드포인트:**
- `POST /api/chat` - 채팅 요청

---

## 3. 파이프라인 아키텍처

### 3.1 파이프라인 단계

```
source/ → translation → evaluation → review → patch
```

### 3.2 단계별 상세

#### 3.2.1 번역 단계 (`go-translation-pipeline`)

**역할:** 소스 텍스트를 한국어로 번역

**주요 구성 요소:**
- `translation/pipeline.go` - 메인 파이프라인 로직
- `translation/prompts.go` - 번역 프롬프트 빌더
- `translation/batch_builder.go` - 배치 빌더
- `translation/proposal_collector.go` - 번역 제안 수집기
- `translation/postprocess_validation.go` - 후처리 검증

**처리 단계:**
1. **소스 준비**: `source_esoteric.json` 로드
2. **컨텍스트 준비**: 주변 라인 컨텍스트 수집
3. **배치 빌딩**: `chunkBatches`로 ID 목록 분할
4. **번역 요청**: LLM에 프롬프트 전송
5. **제안 수집**: 복수 번역 제안 수집 (동시/순차)
6. **검증**: 토큰/자리/플레이스홀더 일치 여부 확인
7. **저장**: 체크포인트에 결과 저장

**프롬프트 전략:**
```
[일반 번역 규칙]
- 불완전/절단된 조각 수리 금지
- 보이는 조각만 자연스럽게 번역
- 게임플레이 접두사, 액션 마커, 내레이션 큐 보존
- 비영어 구절/주문/외국어 조각 보존
- 용어집(glossary)는 강제 용어로 적용
- [PLAYER OPTION] 앵커는 복사 금지

[구조 패턴별 처리]
- fragment_pattern: action_cue_en, spoken_fragment_en 구조 참조
- structure_pattern: lead_term_en, definition_body_en 구조 참조
- expository_entry: 한국어 설명문으로 렌더링
- long_discourse: 유창한 대화/내레이션 유지
```

#### 3.2.2 평가 단계 (파이프라인 내부)

**역할:** 번역 품질 평가

**처리 단계:**
- 평가 기능은 통합 파이프라인 `go-translation-pipeline` 내부에서 수행됩니다
- 체크포인트에서 번역 결과 로드
- 역번역(back-translation) 수행
- 품질 점수 계산

#### 3.2.3 점수 매기기 단계 (`go-semantic-review`)

**역할:** 시맨틱 유사도와 품질 점수 계산

**주요 구성 요소:**
- `semanticreview/scoring.go` - 점수 계산 로직

**점수 항목:**
```go
type ScoreMetrics struct {
    ScoreSemantic      float64  // 시맨틱 유사도 (0-1)
    ScoreLexical       float64  // 어휘적 유사도 (0-1)
    ScorePrevAlignment float64  // 이전 줄 정렬 패널티
    ScoreNextAlignment float64  // 다음 줄 정렬 패널티
    ScoreFormat       float64  // 포맷 잔여물 패널티
    ScoreFinal        float64  // 최종 점수 (가중평균)
}
```

**점수 계산:**
```go
final = scoreSemantic*0.45 + scoreLexical*0.25 + 
         scorePrev*0.15 + scoreNext*0.15 + scoreFormat
```

**리뷰 태그:**
- `semantic_drift`: 시맨틱 의미 이탈
- `lexical_drift`: 어휘적 이탈
- `closer_to_prev`: 이전 줄에 지나치게 가까움
- `closer_to_next`: 다음 줄에 지나치게 가까움
- `format_residue`: 포맷 잔여물 (prev_ko, next_ko 등)

#### 3.2.4 리뷰 단계 (`go-review`)

**역할:** 웹 기반 리뷰 대시보드 제공

**특징:**
- HTTP 서버 (기본 포트 8094)
- PostgreSQL 또는 SQLite 체크포인트 연동
- 상태 관리 (pending → working → done → failed)
- 점수 기반 필터링 및 정렬

**API 엔드포인트:**
- `GET /` - 대시보드 메인
- `POST /api/items/{id}` - 아이템 업데이트
- `POST /api/run/{name}/status` - 런 상태 변경
- `POST /api/run/{name}/delete` - 런 삭제

#### 3.2.5 패치 적용 단계 (`patch/tools`)

**역할:** 번역 결과를 게임 패치로 변환

**주요 스크립트:**
- `build_korean_patch_from_checkpoint.py` - 체크포인트에서 패치 빌드
- `build_patch_from_latest_batch.ps1` - 최신 배치에서 패치 빌드
- `build_full_patch_distribution.ps1` - 전체 배포 패키지 생성

**출력물:**
- `translations.json` - 런타임 사이드카
- `translation_contextual.json` - 컨텍스트 인식 엔트리
- `textassets/*.txt` - 정적 UI 텍스트 오버라이드

---

## 4. 데이터 흐름

### 4.1 데이터 형식

| 단계 | 입력 | 출력 | 저장소 |
|------|------|------|------|
| **번역** | `source_esoteric.json` | `translation_checkpoint.db` | PostgreSQL 5433 |
| **평가** | `translation_checkpoint.db` | `evaluation_unified.db` | SQLite |
| **점수 매기기** | `translation_checkpoint.db` | `semantic_review/` | PostgreSQL 5433 |
| **리뷰** | PostgreSQL | 웹 대시보드 | PostgreSQL 5433 |
| **패치** | PostgreSQL | `translations.json` | 게임 폴더 |

### 4.2 PostgreSQL 체크포인트 스키마

**주요 테이블:**
- `items` - 번역 아이템
- `pipeline_items` - 파이프라인 상태 관리
- `semantic_scores` - 시맨틱 점수

**주요 필드 (`items`):**
```sql
id                  TEXT PRIMARY KEY
pack_json           JSONB  -- 번역 팩 (target, risk, notes 등)
updated_at          TIMESTAMP
```

**주요 필드 (`pipeline_items`):**
```sql
id      TEXT PRIMARY KEY
state   TEXT  -- pending, working, done, failed
```

---

## 5. LLM 도구 사용 패턴

### 5.1 프로젝트별 설정

#### 5.1.1 Esoteric Ebb (기본 프로젝트)

```json
{
  "translation": {
    "llm_backend": "ollama",
    "server_url": "http://127.0.0.1:11438",
    "model": "TranslateGemma-fast:latest",
    "ollama_baked_system": true,
    "ollama_reset_history": true,
    "ollama_keep_alive": "12h",
    "ollama_num_ctx": 8192,
    "ollama_temperature": 0
  },
  "evaluation": {
    "llm_backend": "ollama",
    "server_url": "http://127.0.0.1:11434",
    "trans_model": "TranslateGemma:latest",
    "eval_model": "TranslateGemma:latest"
  },
  "pipeline": {
    "low_llm": {
      "llm_backend": "opencode",
      "server_url": "http://127.0.0.1:4112",
      "model": "openai/gpt-5.2",
      "concurrency": 8,
      "batch_size": 10,
      "timeout_sec": 120
    },
    "high_llm": {
      "llm_backend": "opencode",
      "server_url": "http://127.0.0.1:4112",
      "model": "openai/gpt-5.4",
      "concurrency": 2,
      "batch_size": 10,
      "timeout_sec": 120
    },
    "score_llm": {
      "llm_backend": "opencode",
      "server_url": "http://127.0.0.1:4112",
      "model": "openai/gpt-5.4",
      "concurrency": 4,
      "batch_size": 20,
      "timeout_sec": 120,
      "prompt_variant": "ultra"
    }
  }
}
```

#### 5.1.2 Live Batch (최신 설정)

```json
{
  "translation": {
    "llm_backend": "opencode",
    "server_url": "http://127.0.0.1:4112",
    "model": "openai/gpt-5.4",
    "checkpoint_backend": "postgres",
    "checkpoint_dsn": "postgres://postgres@127.0.0.1:5433/localize_agent?sslmode=disable"
  },
  "evaluation": {
    "llm_backend": "ollama",
    "server_url": "http://127.0.0.1:11434",
    "trans_model": "TranslateGemma:latest",
    "eval_model": "TranslateGemma:latest"
  },
  "pipeline": {
    "low_llm": {
      "llm_backend": "opencode",
      "server_url": "http://127.0.0.1:4112",
      "model": "openai/gpt-5.4",
      "concurrency": 4,
      "batch_size": 8,
      "timeout_sec": 120
    },
    "high_llm": {
      "llm_backend": "opencode",
      "server_url": "http://127.0.0.1:4112",
      "model": "openai/gpt-5.4",
      "concurrency": 2,
      "batch_size": 10,
      "timeout_sec": 120
    },
    "score_llm": {
      "llm_backend": "opencode",
      "server_url": "http://127.0.0.1:4112",
      "model": "openai/gpt-5.4",
      "concurrency": 1,
      "batch_size": 8,
      "timeout_sec": 120,
      "prompt_variant": "ultra"
    }
  }
}
```

### 5.2 유틸티 및 데이터 관리 커맨드

### 5.2.1 번역 적용 커맨드

**go-esoteric-apply-out**
- 역할: 체크포인트에서 완료된 번역을 게임 소스에 적용
- 사용: `go run ./workflow/cmd/go-esoteric-apply-out --in --out --checkpoint-db`

### 5.2.2 패키지 청크 빌드 커맨드

**go-esoteric-build-translator-chunks**
- 역할: 번역 패키지를 청크로 분할
- 사용: `go run ./workflow/cmd/go-esoteric-build-translator-chunks --in --out`

### 5.2.3 프래그먼트 적용 커맨드

**go-apply-fragment-report**
- 역할: 프래그먼트 리포트를 게임 텍스트에 적용
- 사용: `go run ./workflow/cmd/go-apply-fragment-report --project-dir --report-path --backup-path`

### 5.2.4 체크포인트 마이그레이션 커맨드

**go-migrate-checkpoint**
- 역할: SQLite 체크포인트를 PostgreSQL로 이전
- 사용: `go run ./workflow/cmd/go-migrate-checkpoint --source-sqlite --dest-dsn`

---

## 6. 오케스트레이션

```json
{
  "translation": {
    "llm_backend": "opencode",
    "server_url": "http://127.0.0.1:4112",
    "model": "openai/gpt-5.4",
    "checkpoint_backend": "postgres",
    "checkpoint_dsn": "postgres://postgres@127.0.0.1:5433/localize_agent?sslmode=disable"
  },
  "evaluation": {
    "llm_backend": "ollama",
    "server_url": "http://127.0.0.1:11434",
    "trans_model": "TranslateGemma:latest",
    "eval_model": "TranslateGemma:latest"
  },
  "pipeline": {
    "low_llm": {
      "llm_backend": "opencode",
      "server_url": "http://127.0.0.1:4112",
      "model": "openai/gpt-5.4",
      "concurrency": 4,
      "batch_size": 8,
      "timeout_sec": 120
    },
    "high_llm": {
      "llm_backend": "opencode",
      "server_url": "http://127.0.0.1:4112",
      "model": "openai/gpt-5.4",
      "concurrency": 2,
      "batch_size": 10,
      "timeout_sec": 120
    },
    "score_llm": {
      "llm_backend": "opencode",
      "server_url": "http://127.0.0.1:4112",
      "model": "openai/gpt-5.4",
      "concurrency": 1,
      "batch_size": 8,
      "timeout_sec": 120,
      "prompt_variant": "ultra"
    }
  }
}
```

### 5.2 도구 선택 기준

| 작업 | OpenCode | Ollama | 이유 |
|------|----------|---------|------|
| **초기 번역 (low_llm)** | ✅ | ❌ | 빠른 처리, 높은 동시성 (8) |
| **고품질 번역 (high_llm)** | ✅ | ❌ | 더 나은 모델 (gpt-5.4), 낮은 동시성 (2) |
| **점수 매기기 (score_llm)** | ✅ | ❌ | 복잡한 평가, 특수 프롬프트 (ultra) |
| **평가/역번역** | ✅ | ✅ | 양쪽 모두 가능 (기본은 Ollama) |
| **리뷰** | ✅ | ✅ | 데이터 읽기만 (LLM 사용 안 함) |

### 5.3 동시성 및 배치 크기 설정

| 역할 | 동시성 | 배치 크기 | 타임아웃 | 설명 |
|------|--------|----------|---------|------|
| **low_llm** | 8 | 10 | 120초 | 빠른 초기 번역 |
| **high_llm** | 2 | 10 | 120초 | 고품질 번역 |
| **score_llm** | 1 | 8 | 120초 | 정확한 점수 계산 |
| **semantic_review** | 2 | 20 | 120초 | 시맨틱 분석 |

**설계 원칙:**
- 동시성이 높을수록 처리 속도는 빠르지만 메모리 사용량 증가
- 배치 크기는 각 요청의 토큰 수를 고려하여 조정
- 타임아웃은 LLM 응답 시간을 고려하여 설정

---

## 6. 오케스트레이션

### 6.1 파이프라인 오케스트레이터

**파일:** `projects/esoteric-ebb/cmd/run_pipeline_orchestrated.ps1`

**역할:** 여러 워커 프로세스를 조정하여 파이프라인 실행

**지원 액션:**
- `start`, `restart`, `stop`, `status` - 파이프라인 제어
- `route-no-row` - 처리되지 않은 행 라우팅
- `route-overlay-ui` - 오버레이/UI 라인 라우팅
- `repair-blocked-translate` - 차단된 번역 복구
- `maintain-failed` - 실패한 항목 유지

**프로필:**
- `custom` - 사용자 정의 설정
- `balanced` - 균형 설정
- `score-heavy` - 점수 중심
- `retranslate-heavy` - 재번역 중심

**워커 타입:**
- `translate` - 번역 워커
- `failed_translate` - 실패한 번역 워커
- `overlay_translate` - 오버레이 번역 워커
- `score` - 점수 매기기 워커
- `retranslate` - 재번역 워커

### 6.2 관리 스크립트

#### 6.2.1 PostgreSQL 관리

**파일:** `scripts/manage-postgres5433.ps1`

**명령:**
```powershell
# 상태 확인
powershell -ExecutionPolicy Bypass -File scripts\manage-postgres5433.ps1 -Action status

# 시작
powershell -ExecutionPolicy Bypass -File scripts\manage-postgres5433.ps1 -Action start

# 재시작
powershell -ExecutionPolicy Bypass -File scripts\manage-postgres5433.ps1 -Action restart

# 중지
powershell -ExecutionPolicy Bypass -File scripts\manage-postgres5433.ps1 -Action stop
```

#### 6.2.2 OpenCode 관리

**파일:** `scripts/manage-opencode-serve.ps1`

**명령:**
```powershell
# 상태 확인
powershell -ExecutionPolicy Bypass -File scripts\manage-opencode-serve.ps1 -Action status

# 시작
powershell -ExecutionPolicy Bypass -File scripts\manage-opencode-serve.ps1 -Action start

# 재시작
powershell -ExecutionPolicy Bypass -File scripts\manage-opencode-serve.ps1 -Action restart
```

#### 6.2.3 Review 관리

**파일:** `scripts/manage-review.ps1`

**명령:**
```powershell
# 상태 확인
powershell -ExecutionPolicy Bypass -File scripts\manage-review.ps1 -Action status

# 시작
powershell -ExecutionPolicy Bypass -File scripts\manage-review.ps1 -Action start

# 재시작
powershell -ExecutionPolicy Bypass -File scripts\manage-review.ps1 -Action restart
```

#### 6.2.4 전체 스택 복구

**파일:** `scripts/recover-live-stack.ps1`

**명령:**
```powershell
# 상태 확인
powershell -ExecutionPolicy Bypass -File scripts\recover-live-stack.ps1 -Action status -Profile custom

# 전체 복구
powershell -ExecutionPolicy Bypass -File scripts\recover-live-stack.ps1 -Action recover -Profile custom
```

---

## 7. 트레이싱 및 메트릭

### 7.1 LLM 트레이싱

**이벤트 타입:**
- `warmup` - 웜업 완료
- `prompt` - 프롬프트 전송 및 응답 수신
- `prompt_error` - 프롬프트 오류
- `request` - HTTP 요청
- `request_error` - 요청 오류
- `response_error` - 응답 오류
- `response_empty` - 빈 응답
- `response_parse_error` - 응답 파싱 오류

**출력:** JSONL 형식 (구조화된 로그)

### 7.2 메트릭 수집

```go
type MetricCollector struct {
    // 성공/실패 카운터
    // 지연 시간 (밀리초)
}
```

**지표:**
- 요청 성공률
- 평균 응답 시간
- 실패 요청 수

---

## 8. 프롬프트 전략

### 8.1 번역 프롬프트

**핵심 규칙:**
1. 불완전/절단된 텍스트 수리 금지
2. 보이는 조각만 자연스럽게 번역
3. 게임플레이 요소 보존 (접두사, 마커, 큐)
4. 용어집 강제 적용
5. 비영어 구절/주문 보존
6. 구조 패턴별 처리

**구조 패턴:**
- `fragment_pattern`: `action_cue_en`, `spoken_fragment_en` 참조
- `structure_pattern`: `lead_term_en`, `definition_body_en` 참조
- `expository_entry`: 한국어 설명문
- `long_discourse`: 유창한 대화/내레이션

### 8.2 점수 매기기 프롬프트

**평가 기준:**
- 시맨틱 유사도 (45%)
- 어휘적 유사도 (25%)
- 이전 줄 정렬 (15%)
- 다음 줄 정렬 (15%)
- 포맷 잔여물 (0% 또는 1%)

**프롬프트 변형:**
- `ultra` - 더 엄격한 평가 기준

---

## 9. 오류 처리 및 재시도

### 9.1 재시도 정책

| 오류 유형 | 최대 재시도 | 재시도 지연 |
|-----------|-------------|-----------|
| **타임아웃** | 3 | 즉시 |
| **번역기 오류** | 3 | 즉시 |
| **긴 텍스트 (5000+ 토큰)** | 건너뜀 | - |

### 9.2 상태 관리

**파이프라인 상태:**
- `pending` - 대기 중
- `working` - 처리 중
- `done` - 완료
- `failed` - 실패

**아이템 상태 (`items` 테이블):**
- `new` - 새 항목
- `translated` - 번역됨
- `reviewed` - 검토됨

---

## 10. 배포 및 패치 적용

### 10.1 패치 빌드

**입력:**
- PostgreSQL 체크포인트
- `source_esoteric.json`
- `current_esoteric.json`

**출력:**
- `translations.json` (88.2 MB) - 런타임 사이드카
- `translation_contextual.json` (68.1 MB) - 컨텍스트 엔트리
- `textassets/*.txt` (285개 파일) - 정적 UI 오버라이드

**빌드 명령:**
```powershell
powershell -ExecutionPolicy Bypass -File .\projects\esoteric-ebb\patch\tools\build_patch_from_latest_batch.ps1 `
  -DBBackend postgres `
  -DBDsn "postgres://postgres@127.0.0.1:5433/localize_agent?sslmode=disable" `
  -OutDir "projects/esoteric-ebb/patch/output/korean_patch_build_postgres"
```

### 10.2 배포 패키지

**전체 패키지 포함:**
- BepInEx IL2CPP 런타임
- 한국어 번역 로더 플러그인
- 번역 데이터
- 한국어 폰트 (Noto Sans CJK KR)
- 정적 UI 텍스트 오버라이드

**빌드 명령:**
```powershell
powershell -ExecutionPolicy Bypass -File .\projects\esoteric-ebb\patch\tools\build_patch_from_latest_batch.ps1 `
  -DBBackend postgres `
  -DBDsn "postgres://postgres@127.0.0.1:5433/localize_agent?sslmode=disable" `
  -BuildFullPackage `
  -GameRoot "C:\Program Files (x86)\Steam\steamapps\common\Esoteric Ebb" `
  -OutDir "projects/esoteric-ebb/patch/output/korean_patch_build_postgres"
```

---

## 11. 성능 최적화

### 11.1 병렬 처리

**워커 풀:**
- 동시 워커 수 (concurrency)
- 작업 큐 (channel)

**배치 처리:**
- 배치 크기 (batch_size)
- 작업 분할 (chunking)

### 11.2 캐싱

**세션 캐싱:**
- 세션 ID 재사용
- 컨텍스트 준비 상태 추적

**컨텍스트 준비:**
- 웜업 결과 캐싱
- 준비 상태 저장

---

## 12. 문서화 및 참조

### 12.1 주요 문서

| 문서 | 위치 | 설명 |
|------|------|------|
| **AGENTS.md** | `/` | 운영 메모 및 참조 경로 |
| **README.md** | `/` | 리포지토리 개요 |
| **projects/README.md** | `projects/` | 프로젝트 레이아웃 |
| **context.md** | `projects/esoteric-ebb/patch/` | 패치 컨텍스트 |

### 12.2 프로젝트별 컨텍스트

**Esoteric Ebb:**
- `context/esoteric_ebb_modelfile_system.md` - 모델 파일 시스템
- `context/esoteric_ebb_rules.md` - 번역 규칙
- `context/esoteric_ebb_semantic_review_system.md` - 시맨틱 리뷰 시스템

---

## 13. 요약

### 13.1 아키텍처 원칙

1. **분리**: 공유 코드와 프로젝트별 코드 분리
2. **재사용성**: 다중 프로젝트 지원
3. **모듈화**: 각 단계를 독립 모듈로 구현
4. **확장성**: 새 프로젝트 쉽게 추가
5. **검증 가능성**: 각 단계에서 검증 수행

### 13.2 LLM 도구 선택 전략

1. **OpenCode**: 고성능 번역, 복잡한 평가, 점수 매기기
2. **Ollama**: 빠른 초기 번역, 평가/역번역, 비용 절감
3. **혼합 사용**: 각 작업에 가장 적합한 도구 선택

### 13.3 품질 보장

1. **다단 검증**: 토큰, 자리, 플레이스홀더 검증
2. **점수 기반 필터링**: 품질 기준 미달 항목 식별
3. **리뷰 워크플로우**: 수동 검토 지원
4. **재시도 정책**: 일시적 오류 복구

---

## 부록 A: 빠른 참조

### A.1 명령줄 도구

```powershell
# 번역
go run ./workflow/cmd/go-translate --project esoteric-ebb

# 평가
go run ./workflow/cmd/go-evaluate --project esoteric-ebb

# 시맨틱 리뷰
go run ./workflow/cmd/go-semantic-review --project esoteric-ebb

# 리뷰 서버
go run ./workflow/cmd/go-review
```

### A.2 환경 변수

```powershell
# PostgreSQL DSN
$env:POSTGRES_DSN = "postgres://postgres@127.0.0.1:5433/localize_agent?sslmode=disable"

# OpenCode 서버
$env:OPENCODE_SERVER = "http://127.0.0.1:4112"

# Ollama 서버
$env:OLLAMA_SERVER = "http://127.0.0.1:11434"
```

### A.3 포트

| 서비스 | 포트 |
|--------|------|
| PostgreSQL | 5433 |
| OpenCode | 4112 |
| Ollama | 11434 |
| Ollama (빠름) | 11438 |
| Review | 8094 |
