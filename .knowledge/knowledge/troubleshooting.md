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

## ab_test_rag.py PROJECT_ROOT 경로 중복
**Error:** `projects/esoteric-ebb/rag/projects/esoteric-ebb/rag/...` — 경로 이중 중첩
**Cause:** `PROJECT_ROOT / "projects" / "esoteric-ebb"` 사용했는데 SCRIPT_DIR이 이미 `projects/esoteric-ebb/context/`
**Fix:** `PROJECT_DIR = SCRIPT_DIR.parent` 사용 (이미 esoteric-ebb 디렉토리)
