# Troubleshooting

## v2pipeline total=0 (아이템이 보이지 않음)
**Error:** `v2pipeline initial state: total=0` — DB에 아이템이 있는데도 0 반환
**Cause:** `applyProjectDefaults`가 project.json의 `translation.checkpoint_backend="sqlite"` 기본값을 상속 → 빈 SQLite DB 연결
**Fix:** `--backend postgres` CLI 플래그 명시. 또는 `applyProjectDefaults` 내 sqlite 기본값 무시 로직 확인 (commit 1265ac3)

## A/B 테스트 scored_batches < total_batches
**Error:** 10개 배치 리셋 후 재번역했는데 2/10만 스코어됨 (나머지 8개 score_with_rag=0)
**Cause:** `max_passes=15`, `concurrency=2`로 217개 아이템 처리 불충분 (패스당 ~13개 처리, 15패스=195개만 처리)
**Fix:** max_passes 증가 또는 concurrency 증가 필요

## ab_test_rag.py psql score_final NULL 에러
**Error:** `null value in column "score_final" of relation "pipeline_items_v2" violates not-null constraint`
**Fix:** `score_final = NULL` → `score_final = -1`로 변경

## watchdog false-kill → 모든 worker 연결 끊김
**Error:** watchdog가 서버를 재시작한 후 모든 translate worker들이 connection refused 에러 연속 발생
**Cause:** `deepProbe`(LLM 요청)가 서버 바쁠 때 timeout → 정상 서버를 false kill
**Fix:** `deepProbe` → `probeServer`(단순 HTTP GET /health 또는 루트) 교체. LLM 요청은 probe에 부적합.

## OpenCode hang → stale working items
**Error:** working_score=20인 항목이 5시간 이상 lease 초과 (lease_until 지남)
**Cause:** OpenCode 서버 응답 불가 → warmup(EnsureContext) 실패 → UpdateRetryState 미호출 → working 상태 고착
**Fix:** `--cleanup-stale-claims` 플래그로 stale reclaim 후 파이프라인 재시작. 장기적: warmup 에러 시 UpdateRetryState 반드시 호출.

## voice card / RAG 플래그 없이 번역 품질 급락
**Error:** 신버전 번역에서 고어체 캐릭터(Kattegatt 등) 말투가 `너/~다` 현대 반말로 평탄화됨
**Cause:** worktree 버그 후 코드 복원 시 `main.go`의 `--voice-cards`, `--rag-context` 플래그가 미복원된 채 파이프라인 실행
**Fix:** Phase 실행 전 `go-v2-pipeline --help`에서 플래그 존재 확인. 10건 샘플로 특수 말투 캐릭터 육안 검토.

## ab_test_rag.py PROJECT_ROOT 경로 중복
**Error:** `projects/esoteric-ebb/rag/projects/esoteric-ebb/rag/...` — 경로 이중 중첩
**Cause:** `PROJECT_ROOT / "projects" / "esoteric-ebb"` 사용했는데 SCRIPT_DIR이 이미 `projects/esoteric-ebb/context/`
**Fix:** `PROJECT_DIR = SCRIPT_DIR.parent` 사용 (이미 esoteric-ebb 디렉토리)
