# GSD Debug Knowledge Base

Resolved debug sessions. Used by `gsd-debugger` to surface known-pattern hypotheses at the start of new investigations.

---

## ab-test-stale-translate-claims — worker --once 플래그가 빈 큐에서 종료하지 않아 stale working 아이템 발생
- **Date:** 2026-04-11
- **Error patterns:** stale, working_translate, pending_translate, --once, cleanup-stale-claims, ab_test, score, timeout, lease, ClaimPending
- **Root cause:** (1) worker.go TranslateWorker: --once 플래그가 ClaimPending 빈 반환 시 idle sleep 후 continue를 반복 — --once 체크(75행)에 도달하지 않아 프로세스가 자연 종료되지 않음. subprocess timeout으로 강제 종료되면 working_translate 아이템이 lease 만료 전까지 고착됨. (2) ab_test_rag.py run_pipeline_stage(): 완료 조건이 pending_{stage}=0만 체크하고 working_{stage}=0은 확인하지 않아 stale 아이템이 있어도 다음 스테이지로 진행, 해당 아이템이 score=0으로 남음.
- **Fix:** ab_test_rag.py run_pipeline_stage() 수정 — 완료 조건을 pending=0 AND working=0으로 변경; pending=0 but working>0 시 --cleanup-stale-claims + --lease-sec 1 실행; pass_timeout=180 / lease_sec=150 명시; max_passes=30으로 상향.
- **Files changed:** projects/esoteric-ebb/context/ab_test_rag.py
---

