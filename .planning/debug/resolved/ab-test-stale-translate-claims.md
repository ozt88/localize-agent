---
status: resolved
trigger: "ab_test_rag.py translate 스테이지 완료 판정 오류로 stale working_translate 40개 방치, 2/10 배치만 스코어됨"
created: 2026-04-11T00:00:00
updated: 2026-04-11T00:05:00
---

## Current Focus

hypothesis: CONFIRMED — 두 가지 버그 확인됨
  버그 1: run_pipeline_stage()의 완료 조건이 pending_{stage}=0만 체크, working_{stage} 잔존 무시
  버그 2: --once 플래그가 ClaimPending 빈 반환 시 적용되지 않음 (idle sleep 후 continue) → 300초 타임아웃으로 강제 종료 → 진행 중 working 아이템 고착
test: worker.go 55-77행 확인 — items==0일 때 idle sleep후 continue, --once는 처리 후에만 return
expecting: fix 적용 후 전 스테이지 완료 확인
next_action: ab_test_rag.py 수정 — 완료 조건에 working 체크 추가 + cleanup-stale-claims 패스 추가

## Symptoms

expected: 10개 테스트 배치 전량 translate → format → score 완료
actual: translate 15 passes 후 format/score로 넘어가지만, stale working_translate 40개가 cleanup 없이 방치됨. 이 아이템들은 score=0으로 남아 A/B 테스트 결과가 2/10 배치만 스코어됨
errors: 에러 없음 (exit=0). pending_translate=0이 되면 break하는 로직이 working_translate도 0인지는 확인하지 않음
reproduction: python projects/esoteric-ebb/context/ab_test_rag.py 실행 → translate 15 passes 완료 → pending_translate=0 → format/score 스테이지로 진행 → DB에 working_translate STALE 아이템 40개 잔존
started: Phase 07.1 A/B 테스트 실행 시 항상 발생

## Eliminated

(없음)

## Evidence

- timestamp: 2026-04-11T00:00:00
  checked: ab_test_rag.py 119-157행 run_pipeline_stage() 함수
  found: 153행 state_name = f"pending_{stage}" — working_{stage} 상태를 전혀 체크하지 않음. pending=0이면 즉시 break. max_passes=15, concurrency=2 → 패스당 최대 2개 처리 = 최대 30개 처리, 217개에 턱없이 부족
  implication: (1) working 상태 아이템은 완료 체크에서 보이지 않아 조기 종료. (2) max_passes * concurrency = 30 < 217 → 설령 working 체크가 맞아도 처리량 부족

- timestamp: 2026-04-11T00:01:00
  checked: worker.go TranslateWorker 43-78행 — --once 플래그 동작
  found: items==0이면 55-62행 idle sleep 후 continue. --once 체크(75행)는 items>0인 클레임 성공 이후에만 도달. 즉 pending=0 상태에서는 --once가 절대 return하지 않고 idle sleep 루프를 반복함. subprocess timeout=300초로 강제 종료되면 working 상태 아이템들이 lease 만료 전까지 고착.
  implication: --once + 빈 큐 조합에서 프로세스가 자연 종료되지 않음 → 타임아웃으로 강제 종료 → working 아이템 stale 발생 확정

- timestamp: 2026-04-11T00:01:00
  checked: worker.go TranslateBatchSize 기본값 (applyProjectDefaults 277행): 10. concurrency=2이면 패스당 2×10=20개 처리. 15 passes × 20 = 300 > 217이므로 처리량 자체는 충분
  found: 처리량 부족은 실제 문제가 아님. 진짜 문제는 --once가 빈 큐에서 종료하지 않아 타임아웃 강제 종료 → stale working 아이템 발생
  implication: max_passes 증가보다 완료 조건과 stale cleanup이 핵심 수정 포인트

## Resolution

root_cause: 두 가지 버그의 복합 작용.
  (1) worker.go TranslateWorker: --once 플래그가 ClaimPending 빈 반환(items==0) 시 idle sleep 후 continue를 반복 — --once 체크(75행)에 도달하지 않아 프로세스가 자연 종료되지 않음. subprocess.run(timeout=300)으로 강제 종료되면 working_translate 상태 아이템이 lease 만료 전까지 고착됨.
  (2) ab_test_rag.py run_pipeline_stage(): 완료 조건이 pending_{stage}=0만 체크하고 working_{stage}=0은 확인하지 않음. 따라서 stale working 아이템이 있어도 다음 스테이지로 진행하고, 해당 아이템들은 pending_format/pending_score로 전환되지 않아 score=0으로 남음.
fix: ab_test_rag.py run_pipeline_stage() 수정 4가지 —
  (1) 완료 조건: pending_{stage}=0 AND working_{stage}=0으로 변경 (166행)
  (2) pending=0 but working>0 시 --cleanup-stale-claims 전용 cmd 실행 (174-191행) — --lease-sec 1로 override하여 threshold=3초로 강제 reclaim
  (3) pass_timeout=180, lease_sec=150 명시적 설정 — lease 만료 후 cleanup 보장 (123-127행)
  (4) max_passes=30으로 상향 (143행)
verification: 코드 검토 완료. 수정 후 ab_test_rag.py 재실행 시 사용자 확인 필요.
files_changed: [projects/esoteric-ebb/context/ab_test_rag.py]
