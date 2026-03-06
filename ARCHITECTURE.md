# Architecture Guide

이 문서는 현재 코드 구조(리팩터링 반영 상태)를 설명합니다.

## 1) 레이어 구조

- `workflow/cmd/*`: CLI 진입점, `flag` 파싱/검증
- `workflow/internal/translation`: 번역 도메인 로직
- `workflow/internal/evaluation`: 평가 도메인 로직
- `workflow/internal/contracts`: 공통 인터페이스/DTO
- `workflow/internal/platform`: DB/파일/LLM 같은 사이드이펙트 구현
- `workflow/internal/shared`: 범용 유틸

## 2) 책임 분리 원칙

1. `cmd`는 입력 경계만 담당
- `flag` 파싱
- 기본값/필수값 검증
- `Run(Config)` 호출

2. 도메인(`translation`, `evaluation`)은 비즈니스 로직만 담당
- 프롬프트 구성
- 배치/평가/판정 흐름
- 인터페이스(`contracts`)를 통한 의존성 사용

3. `platform`은 구현 세부사항 담당
- SQLite 접근
- 파일 읽기/쓰기
- LLM 세션/프롬프트 전송

## 3) 현재 엔트리포인트

- 번역: `go run ./workflow/cmd/go-translate ...`
- 평가: `go run ./workflow/cmd/go-evaluate ...`
- 검증: `go run ./workflow/cmd/go-validate ...`
- 반영: `go run ./workflow/cmd/go-apply ...`

## 4) 호출 다이어그램

### 4.1 번역(`go-translate`)

```text
cmd/go-translate/main.go
  -> translation.DefaultConfig()
  -> flag parse
  -> translation.Run(cfg)
       -> platform.NewOSFileStore()
       -> readStrings/readIDs
       -> platform.NewSQLiteCheckpointStore()
       -> newTranslateSkill(...)
       -> newServerClient(...)
       -> runPipeline(...)
            -> buildBatch(...)                  [batch_builder.go]
            -> collectProposals(...)            [proposal_collector.go]
            -> persistResults(...)              [result_persister.go]
                 -> restoreWithRecovery(...)
                 -> checkpoint.UpsertItem(...)
       -> metrics summary / print
```

### 4.2 평가(`go-evaluate`)

```text
cmd/go-evaluate/main.go
  -> evaluation.DefaultConfig()
  -> flag parse
  -> evaluation.Run(cfg)
       -> platform.NewSQLiteEvalStore()
       -> platform.NewOSFileStore()
       -> handleModes(...)
            -> runStatusMode(...)               [modes.go]
            -> runExportMode(...)               [modes.go]
            -> runResetMode(...)                [modes.go]
       -> runEvaluationPipeline(...)            [pipeline.go]
            -> prepareEvaluationWork(...)       [work_builder.go]
            -> newTranslateSkill/newEvaluateSkill
            -> newEvalClient(...)
            -> runEvaluationWorkers(...)        [worker_runner.go]
                 -> runEvalItem(...)
                 -> persistEvaluationOutcome(...) [result_persister.go]
            -> metrics/status summary
```

### 4.3 반영(`go-apply`, DB 기반)

```text
cmd/go-apply/main.go
  -> parse ready status set (default: pass)
  -> load ready entries from evaluation DB (final_ko)
  -> load current localization JSON
  -> apply to target strings
  -> write output JSON (or in-place)
  -> update eval_items.status to next-status (default: applied)
```

## 5) 파일별 역할 (핵심)

### 5.1 translation

- `run.go`: 번역 오케스트레이션
- `pipeline.go`: 파이프라인 조립
- `batch_builder.go`: 배치 입력 구성
- `proposal_collector.go`: LLM 번역 제안 수집
- `result_persister.go`: 결과 반영/체크포인트 저장
- `client.go`: 번역용 LLM 프로필 래퍼
- `skill.go`, `prompts.go`, `tags.go`, `file_logic.go`

### 5.2 evaluation

- `run.go`: 모드 분기 + 파이프라인 진입
- `modes.go`: status/export/reset 모드
- `pipeline.go`: 평가 파이프라인 조립
- `work_builder.go`: DB 작업 준비(적재/재개/대기열)
- `worker_runner.go`: 병렬 워커 실행
- `result_persister.go`: 결과 DB 반영
- `item_runner.go`: 단일 아이템 평가-재작성 루프
- `client.go`, `skill.go`, `prompts.go`, `logic.go`, `file_logic.go`

### 5.3 contracts / platform

- `contracts/files.go`: 파일 저장소 인터페이스
- `contracts/evaluation.go`: 평가 DTO/저장소 인터페이스
- `contracts/translation.go`: 번역 체크포인트 인터페이스
- `platform/filestore.go`: OS 파일 구현
- `platform/eval_store.go`: 평가 SQLite 저장소
- `platform/checkpoint_store.go`: 번역 체크포인트 SQLite 저장소
- `platform/llm_client.go`: 공용 LLM 세션 클라이언트

## 6) 구조 일치 체크 (현재 코드 기준)

아래 항목을 코드와 대조해 확인함:

1. `flag` 파싱 위치
- `cmd/go-translate/main.go`, `cmd/go-evaluate/main.go`에만 존재

2. `Run` 시그니처
- `translation.Run(c Config)`, `evaluation.Run(c Config)`로 통일

3. translation 2차 분해
- `buildBatch`, `collectProposals`, `persistResults` 분리 완료

4. evaluation 2차 분해
- `prepareEvaluationWork`, `runEvaluationWorkers`, `persistEvaluationOutcome` 분리 완료

5. 공용 LLM 경계
- 번역/평가 클라이언트는 `platform/llm_client.go` 공용 전송 경로 사용

6. DB 중심 반영
- `go-apply`는 evaluation DB에서 준비 항목을 읽고 적용 후 상태 갱신

현재 문서는 위 코드 상태와 일치합니다.
