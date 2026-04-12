---
phase: 08-retranslation-execution
plan: 02
subsystem: v2pipeline
tags: [retranslation, voice-cards, rag, quality, pipeline]
status: CLOSED_INCOMPLETE
dependency_graph:
  requires:
    - phase: 08-01
      provides: highest-gen dedup, ResetAllForRetranslation, full DB reset
  provides:
    - "파이프라인 재시작 절차 (stale cleanup + failed reset SQL)"
    - "watchdog deepProbe → probeServer 수정 (false-kill 방지)"
    - "retranslate.go 누락 메서드 구현 (컴파일 에러 해결)"
    - "번역 품질 gap 발견 및 문서화"
  affects: [09-retranslation-quality-restore]
tech_stack:
  added: []
  patterns:
    - "failed items SQL reset: UPDATE WHERE state='failed' (ResetAllForRetranslation 대신)"
    - "watchdog shallow probe: probeServer(HTTP GET) > deepProbe(LLM request)"
key_files:
  created:
    - workflow/internal/v2pipeline/retranslate.go (복원 후 메서드 구현)
  modified:
    - workflow/internal/v2pipeline/run.go (watchdog probe 수정)
    - workflow/internal/v2pipeline/store.go (ScoreHistogram, SelectRetranslationBatches, ResetForRetranslation 추가)
    - workflow/internal/contracts/v2pipeline.go (RetranslationCandidate 추가)
    - workflow/internal/v2pipeline/types.go (ScoreHistogramBucket 추가)
decisions:
  - "Phase 08-02 CLOSED INCOMPLETE: 파이프라인은 실행됐으나 품질 전제조건 미충족"
  - "Phase 08을 '인프라 완료 phase'로 재정의하고 실번역은 Phase 09로 이월"
  - "gen=0 done 항목 6건 → pending_translate 리셋 (새 방식 번역 규정 준수)"
  - "watchdog deepProbe 제거: LLM 요청 probe는 서버 바쁠 때 false-kill 유발"
---

## 실행 결과

### 완료된 작업
- 파이프라인 2회 실행 시도 (done 3,412건 생성)
- stale claims 정리 + failed → pending_translate SQL 리셋 절차 확립
- watchdog false-kill 버그 수정 (deepProbe → probeServer)
- retranslate.go 컴파일 에러 수정 (누락 메서드 3개 구현)
- gen=0 구버전 항목 6건 리셋

### 미완료 / 품질 미달 판정

**판정: 파이프라인 번역물 전량 폐기**

AR_Kattegatt 씬 비교에서 확인된 품질 문제:
- 구버전(gen=0): Kattegatt(마검)가 `그대/~도다/~노라` 고어체 사용 → 캐릭터 정체성 유지
- 신버전(gen=1): `너/~다` 현대 반말로 평탄화 → 캐릭터 고유 말투 소실

**원인:** worktree 버그(Phase 08-01)로 삭제됐다 복원된 voice card/RAG 플래그가
`go-v2-pipeline/main.go`에 재통합되지 않은 채 파이프라인이 실행됨.

Phase 08 목표("voice card + RAG 포함 프롬프트로 재번역")가 실제로는 달성되지 않음.

---

## 교훈 (Phase 09 진입 전 반드시 확인)

### L-01: 파이프라인 실행 전 프롬프트 주입 검증 필수
**무슨 일이:** voice cards, RAG context가 플래그에서 제거된 상태로 파이프라인을 35,000건 돌렸다.
**왜 못 잡았나:** 컴파일은 됐고 파이프라인도 돌았다. 번역 품질 문제는 실행 중에는 보이지 않는다.
**다음엔:** 파이프라인 대량 실행 전, 소수 배치(10건)로 샘플 출력을 사람이 직접 검토한다.

### L-02: worktree 비활성화 결정 후 플래그 누락 여부 재확인하지 않았다
**무슨 일이:** 08-01에서 worktree 버그로 삭제 → f00bbac에서 코드는 복원됐으나 `main.go` 플래그는 복원 안 됨.
**왜 못 잡았나:** 복원 커밋이 코드 파일은 되살렸지만 `main.go` diff를 검토하지 않았다.
**다음엔:** Phase 완료 커밋 전, `git diff HEAD~1 -- **/main.go`로 CLI 플래그 회귀 확인.

### L-03: Phase 목표와 실제 파이프라인 설정이 일치하는지 확인하지 않았다
**무슨 일이:** Phase 08 goal에 "voice card + RAG 포함"이라고 명시됐지만 실행 전 체크리스트가 없었다.
**왜 못 잡았나:** 08-01 완료 후 바로 파이프라인을 돌렸다. 전제조건 검증 단계가 없었다.
**다음엔:** Phase 08.x/09 실행 전 체크리스트:
  - [ ] `go-v2-pipeline --help`에서 `--voice-cards`, `--rag-context` 플래그 확인
  - [ ] `voice_cards.json` 존재 확인
  - [ ] `rag_batch_context.json` 존재 확인
  - [ ] 10건 샘플 번역 후 speaker 있는 항목의 말투 육안 확인

### L-04: 고어체/특수 말투 캐릭터는 voice card 없이 번역 불가
**무슨 일이:** v1(Gemma fine-tuned)은 `thou/thy`를 보고 `그대/~도다`로 자연 추론했으나,
gpt-5.4는 프롬프트 규칙("natural spoken Korean")을 따라 현대 반말로 처리했다.
**다음엔:** voice_cards.json에 Kattegatt 등 특수 말투 캐릭터 반드시 포함.
특히 고어체(thou/thy), 경어(각하 계열), 비인간 존재의 말투를 명시적으로 지정.
