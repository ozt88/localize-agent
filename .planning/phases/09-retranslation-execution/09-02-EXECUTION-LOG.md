# 09-02 Execution Log

## Task 1: 전체 DB 리셋 + 파이프라인 실행

### 1-1. 사전 체크리스트

- [x] `go-v2-pipeline --voice-cards`, `--rag-context` 플래그 확인 — 정상
- [x] `voice_cards.json` 존재 — 30,623 bytes (27개 캐릭터)
- [x] `rag_batch_context.json` 존재 — 389,394 bytes

### 1-2. 리셋 실행

**명령:**
```
go run ./workflow/cmd/go-v2-reset-all \
  --project esoteric-ebb \
  --backend postgres \
  --dsn "postgres://postgres:postgres@localhost:5433/localize_agent" \
  --dry-run=false
```

**결과:**
```
Before reset: map[done:8069 failed:26951 pending_score:10 pending_translate:6]
After reset: map[pending_score:10 pending_translate:35026]
Reset 35020 items to pending_translate (retranslation_gen=1)
```

total=35,036 (pending_translate=35,026 + pending_score=10) — 정상.

**참고:** project.json에 checkpoint_dsn 없음 — --dsn 플래그 직접 지정 필요.

### 1-3. 파이프라인 실행

**명령:**
```
go run ./workflow/cmd/go-v2-pipeline \
  --project esoteric-ebb \
  --backend postgres \
  --dsn "postgres://postgres:postgres@localhost:5433/localize_agent" \
  --voice-cards projects/esoteric-ebb/context/voice_cards.json \
  --rag-context projects/esoteric-ebb/rag/rag_batch_context.json \
  --translate-concurrency 8 \
  --cleanup-stale-claims
```

**초기 로그:**
```
v2pipeline: opencode server already running at http://127.0.0.1:4115
v2pipeline stale cleanup: reclaimed=0
v2pipeline initial state: pending_score=10 pending_translate=35026 total=35036
v2pipeline: started workers (translate=8, format=2, score=4, role=)
```

**실행 시작 시각:** 2026-04-12T17:xx KST (백그라운드 PID 26373)
**실행 방식:** 백그라운드 (`pipeline_run.log`에 출력)

### 1-4. 진행 상황 (실행 시작 직후)

```
state             | count
pending_translate | 34961
working_translate |    44
failed            |    22
done              |     9
```

파이프라인 정상 진행 확인.

---

## Task 2: 번역 완료 통계 + 품질 스팟체크

**상태:** 파이프라인 실행 완료 후 진행 예정

완료 기준:
- done 건수 >= 34,500 (99% 이상)
- failed < 350 (1% 미만)
- Kattegatt 씬 고어체 보존
- Visken 씬 격식체 유지
