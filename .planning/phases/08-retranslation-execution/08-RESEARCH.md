# Phase 08: retranslation-execution — Research

**Researched:** 2026-04-12
**Domain:** v2pipeline 전체 리셋 + BuildV3Sidecar dedup 수정 + export
**Confidence:** HIGH (전량 코드베이스 직접 검증)

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** 전체 재번역 — score 커트라인 없음, 35,009건 전량 재번역
  - 기존 `go-retranslate-select --score-threshold` 방식 대신 전체 배치를 직접 `pending_translate`로 리셋
  - `retranslation_gen`을 0 → 1로 증가시켜 신 번역 구분
  - unscored 464건도 포함 (재번역 대상)
- **D-02:** 항상 최신 `retranslation_gen` 우선 — score 비교 없이 단순히 gen이 높은 항목이 entries[]에 선택됨
  - 현재 "first-seen-wins" 방식에서 "highest-gen-wins" 방식으로 수정
  - 구현: items를 sort by (source_raw, retranslation_gen DESC) 후 first-seen으로 dedup하면 자연스럽게 최신 gen 우선
- **D-03:** 재번역 전량 완료 후 `go-v2-export` 1회 실행 → 기존 `translations.json` 완전 교체 (백업 없음)
  - 중간 export 없음 (진행 중 덮어쓰기 방지)
  - 롤백 필요 시 git으로 복구
- **D-04:** 이전에 테스트한 초반 진입 씬(고정 리스트)으로 회귀 검증
  - 매 릴리즈마다 동일한 씬을 플레이하여 태그 깨짐 및 대사 흐름 확인
- **D-05:** before/after diff 먼저 생성 → 변경된 번역 중 이상해 보이는 항목만 게임에서 확인
  - diff 도구: 기존 translations.json vs 신 translations.json, source 기준 매칭 후 target diff
  - diff에서 이상 징후 없으면 게임 검증 최소화

### Claude's Discretion
- 전체 배치 리셋 방식: `ResetForRetranslation` 배치 단위 루프 vs SQL 전체 UPDATE — 구현 효율 고려하여 결정
- diff 도구: 별도 Go CLI 작성 vs Python 스크립트 vs 기존 도구 활용 — 빠른 쪽으로 결정
- 전량 35,009건 파이프라인 실행 중 watchdog/lease 설정값 조정 — 기존 v2pipeline 안정화 경험 반영

### Deferred Ideas (OUT OF SCOPE)
- score가 현저히 낮아진 경우(gen=1 < gen=0 - 2.0) 구 버전 fallback — 복잡도 대비 효과 불명확
- 미번역 154건(DB 누락 대화/주문 설명) 해소 — Out of scope for Phase 08
</user_constraints>

---

## Summary

Phase 08은 코드 개발(BuildV3Sidecar 수정 + 전체 리셋 CLI)과 대규모 파이프라인 실행(35,009건 재번역)을 결합한 실행 중심 Phase다.

현재 DB 상태: `done=35,009`, `failed=19`, `working_score=8`(stale — lease 만료 예정). 전체 35,009건을 `pending_translate` + `retranslation_gen=1`로 리셋해야 한다. 배치 수는 1,651개이므로 `ResetForRetranslation` 배치 루프보다 **단일 전체 UPDATE SQL**이 훨씬 효율적이다(1회 트랜잭션 vs 1,651회 쿼리). 단, `retranslation_snapshots`에 현재 gen=0 번역을 스냅샷으로 저장하는 로직도 함께 포함해야 한다.

BuildV3Sidecar의 dedup 수정은 `QueryDone()` 결과 `ORDER BY sort_index`만 보장되어 있으므로, `BuildV3Sidecar` 내부에서 `retranslation_gen DESC`로 정렬 후 first-seen 처리하는 방식이 가장 안전하다. `QueryDone()`은 수정할 필요 없다.

**Primary recommendation:** Wave 0 = BuildV3Sidecar 수정 + 전체 리셋 CLI 신규 작성. Wave 1 = 35,009건 파이프라인 실행. Wave 2 = export + diff + 게임 검증.

---

## 현재 DB 실측 상태 (2026-04-12 직접 확인)

[VERIFIED: PostgreSQL 직접 쿼리 — postgres@localhost:5433/localize_agent]

| state | count | 비고 |
|-------|-------|------|
| done | 35,009 | 전량 재번역 대상 |
| failed | 19 | 재번역 포함 예정 (score_final=-1이 9건, scored가 10건) |
| working_score | 8 | lease_until=2026-04-12 02:58 — 연구 시점에 미만료, 완료 후 stale 처리 필요 |
| **합계** | **35,036** | |

- `retranslation_gen > 0` 항목: 87건 (Phase 07.1 A/B 테스트 중 리셋된 배치)
- `retranslation_gen = 0` 항목: 34,949건
- `done` 기준 배치 수: **1,651개** (전체 리셋 규모)

---

## Standard Stack

### 핵심 — 이미 존재하는 컴포넌트

[VERIFIED: 코드베이스 직접 읽기]

| 컴포넌트 | 위치 | 현재 상태 | Phase 08 역할 |
|---------|------|-----------|--------------|
| `BuildV3Sidecar` | `workflow/internal/v2pipeline/export.go:43` | first-seen-wins dedup | **수정 필요**: highest-gen-wins로 변경 |
| `ResetForRetranslation` | `workflow/internal/v2pipeline/store.go:986` | batch 단위 리셋 (snapshot 포함) | 참조 패턴 — 전체 리셋 구현 기반 |
| `go-v2-pipeline` | `workflow/cmd/go-v2-pipeline/main.go` | 파이프라인 실행기 | 35,009건 실행 시 사용 |
| `go-v2-export` | `workflow/cmd/go-v2-export/main.go` | 패치 export CLI | 완료 후 1회 실행 |
| `go-retranslate-select` | `workflow/cmd/go-retranslate-select/main.go` | threshold 기반 후보 선택 | Phase 08에선 직접 사용 안 함 |

### 신규 작성 필요

| 컴포넌트 | 위치 (권장) | 목적 |
|---------|------------|------|
| `go-v2-reset-all` (CLI) | `workflow/cmd/go-v2-reset-all/main.go` | 전체 done → pending_translate 일괄 리셋 |
| `ResetAllForRetranslation` (store 메서드) | `workflow/internal/v2pipeline/store.go` | 단일 SQL로 전량 snapshot + reset |
| diff 도구 | Python 스크립트 또는 Go CLI | translations.json before/after 비교 |

---

## Architecture Patterns

### Pattern 1: 전체 리셋 — 단일 SQL 방식 (권장)

**What:** 1,651개 배치를 루프하지 않고 한 트랜잭션으로 전량 처리

**Why:** `ResetForRetranslation`은 배치 단위로 `retranslation_snapshots INSERT + UPDATE` 2쿼리를 날림. 1,651개 배치면 3,302 쿼리. 전량 SQL은 2 쿼리로 동일 결과.

**구현 패턴** (`ResetAllForRetranslation` 신규 메서드):

```go
// Source: store.go ResetForRetranslation 패턴 참조, 전체 범위로 확장
func (s *Store) ResetAllForRetranslation(nextGen int) (int, error) {
    now := s.nowValue()
    // 1. 현재 done 항목 전체 스냅샷 저장
    _, err := s.db.Exec(s.rebind(`
        INSERT INTO retranslation_snapshots (id, gen, ko_raw, ko_formatted, score_final, snapshot_at)
        SELECT id, ?, ko_raw, ko_formatted, score_final, ?
        FROM pipeline_items_v2 WHERE state = 'done'
        ON CONFLICT (id, gen) DO NOTHING`),
        nextGen, now,
    )
    if err != nil {
        return 0, fmt.Errorf("snapshot all: %w", err)
    }
    // 2. 전체 done → pending_translate 리셋 (guardrail: score_final=-1, 빈 문자열 필수)
    result, err := s.db.Exec(s.rebind(`
        UPDATE pipeline_items_v2
        SET state = ?, ko_raw = NULL, ko_formatted = NULL, score_final = -1,
            failure_type = '', last_error = '', claimed_by = '', claimed_at = NULL,
            lease_until = NULL, retranslation_gen = ?, updated_at = ?
        WHERE state = 'done'`),
        StatePendingTranslate, nextGen, now,
    )
    if err != nil {
        return 0, fmt.Errorf("reset all: %w", err)
    }
    affected, _ := result.RowsAffected()
    return int(affected), nil
}
```

**failed/working_score 처리:** 전체 리셋 전 `--cleanup-stale-claims` 실행 후 `failed` 항목도 동일 리셋에 포함 (`WHERE state IN ('done', 'failed')`로 확장하거나 failed만 별도 처리).

### Pattern 2: BuildV3Sidecar highest-gen dedup 수정

**What:** `QueryDone()` 결과에 여러 gen이 섞여 있을 때, 같은 source_raw에서 highest gen 선택

**현재 코드** (`export.go:68`):
```go
if !seen[item.SourceRaw] {
    seen[item.SourceRaw] = true
    sidecar.Entries = append(sidecar.Entries, entry)
}
```

**수정 패턴** — `BuildV3Sidecar` 내부에서 정렬 후 dedup:
```go
// Source: export.go BuildV3Sidecar, D-02 highest-gen-wins 구현
// QueryDone()은 ORDER BY sort_index만 보장 — gen 정렬은 BuildV3Sidecar에서 처리
import "sort"

func BuildV3Sidecar(items []contracts.V2PipelineItem) V3Sidecar {
    // Step 1: source_raw별 highest-gen 항목만 선택 (dedup)
    type genKey struct{ sourceRaw string }
    bestGen := make(map[string]int)            // sourceRaw -> max gen
    bestIdx := make(map[string]int)            // sourceRaw -> item index
    for i, item := range items {
        if gen, ok := bestGen[item.SourceRaw]; !ok || item.RetranslationGen > gen {
            bestGen[item.SourceRaw] = item.RetranslationGen
            bestIdx[item.SourceRaw] = i
        }
    }
    // Step 2: 선택된 항목들을 sort_index 순으로 정렬하여 Entries 구성
    // ... (기존 seen map 방식 유지하되, 위 bestIdx 기준으로 처리)
    seen := make(map[string]bool)
    for _, item := range items {
        // contextual_entries는 여전히 ALL items
        // entries dedup: bestIdx에 해당하는 경우만 추가
        if idx, ok := bestIdx[item.SourceRaw]; ok && items[idx].ID == item.ID && !seen[item.SourceRaw] {
            seen[item.SourceRaw] = true
            sidecar.Entries = append(sidecar.Entries, entry)
        }
    }
```

**더 단순한 대안:** `QueryDone()`에 `ORDER BY source_raw, retranslation_gen DESC, sort_index` 추가 후 기존 first-seen 유지. `QueryDone()`을 수정하면 export 외 다른 호출자에도 영향 — BuildV3Sidecar 내부에서 처리하는 게 안전.

**권장:** `sort.SliceStable(items, func(i, j int) bool { ... })` 방식으로 BuildV3Sidecar 진입 시 정렬.

### Pattern 3: 파이프라인 실행 명령

**현재 project.json 설정** (canonical_full_retranslate_live):
[VERIFIED: project.json 직접 읽기]

```
- translate: gpt-5.4, concurrency=2, batch_size=10, timeout=120s
- score: gpt-5.4, concurrency=4(→2 권장), batch_size=10, timeout=180s
- format: gpt-5.4
```

**실행 명령 패턴:**
```powershell
# v2pipeline 실행 (기존 run_pipeline_orchestrated.ps1 활용)
go run ./workflow/cmd/go-v2-pipeline `
  --project-dir projects/esoteric-ebb/output/batches/canonical_full_retranslate_live `
  --backend postgres `
  --dsn "postgres://postgres@127.0.0.1:5433/localize_agent?sslmode=disable" `
  --cleanup-stale-claims `
  --voice-cards projects/esoteric-ebb/context/voice_cards.json `
  --rag-context projects/esoteric-ebb/rag/rag_batch_context.json `
  --role all
```

**주의:** `--backend postgres` 명시 필수 (guardrail 참조).

### Pattern 4: diff 도구 — Python 스크립트 (권장)

**Why Python (Claude's Discretion):** Go CLI 신규 작성보다 빠름. 기존 Python 스크립트 패턴(ab_test_rag.py) 존재.

```python
# before/after diff 생성
import json

def diff_translations(before_path, after_path, out_path):
    before = {e["source"]: e["target"] for e in json.load(open(before_path))["entries"]}
    after = {e["source"]: e["target"] for e in json.load(open(after_path))["entries"]}
    changes = []
    for src in set(before) | set(after):
        b, a = before.get(src, ""), after.get(src, "")
        if b != a:
            changes.append({"source": src, "before": b, "after": a})
    json.dump(changes, open(out_path, "w", encoding="utf-8"), ensure_ascii=False, indent=2)
```

### Anti-Patterns to Avoid

- **배치 루프 리셋:** 1,651번 `ResetForRetranslation` 호출 — 필요 없음, 단일 SQL로 처리
- **중간 export:** 파이프라인 진행 중 export 실행 — 부분 done + 부분 pending 상태가 섞임, D-03에서 금지
- **QueryDone 정렬 수정:** `ORDER BY source_raw, gen DESC` 추가 시 다른 호출자(진행률 카운트 등)에 영향 가능 — BuildV3Sidecar 내부에서 처리
- **--backend 생략:** project.json 기본값이 sqlite인 경우 `total=0` 오류 발생 (guardrail)

---

## Don't Hand-Roll

| 문제 | 빌드하면 안 되는 것 | 기존 도구 | 이유 |
|------|-------------------|----------|------|
| 배치 리셋 스냅샷 | 커스텀 스냅샷 테이블 | `retranslation_snapshots` + `ResetForRetranslation` 패턴 | 이미 스키마와 로직 존재 |
| 파이프라인 실행 | 새 파이프라인 구현 | `go-v2-pipeline` + `run_pipeline_orchestrated.ps1` | 모든 watchdog/lease/retry 로직 포함 |
| 패치 export | 커스텀 JSON 빌더 | `go-v2-export` | CleanTarget, 멀티라인 explode 등 복잡 로직 내장 |
| stale claim 정리 | 수동 SQL UPDATE | `--cleanup-stale-claims` 플래그 | 원자적 처리 보장 |

---

## Common Pitfalls

### Pitfall 1: score_final NOT NULL 제약 위반
**What goes wrong:** 리셋 쿼리에서 `score_final = NULL` 설정 시 PostgreSQL `not-null constraint` 에러
**Why it happens:** `pipeline_items_v2.score_final REAL NOT NULL DEFAULT -1`
**How to avoid:** 리셋 시 반드시 `score_final = -1`
**Warning signs:** `null value in column "score_final" violates not-null constraint`

### Pitfall 2: --backend postgres 미명시
**What goes wrong:** `v2pipeline initial state: total=0` — DB에 35,009건 있는데 0 반환
**Why it happens:** `applyProjectDefaults`가 project.json `translation.checkpoint_backend="sqlite"` 상속
**How to avoid:** 항상 `--backend postgres` CLI 플래그 명시
**Warning signs:** `total=0`이면서 `--db` 플래그 미사용 상태

### Pitfall 3: working_score=8 stale 미처리
**What goes wrong:** 리셋 대상에서 working_score 항목이 누락되어 8건이 고아 상태로 남음
**Why it happens:** 현재 working_score=8 (lease=2026-04-12 02:58, 연구 시점 미만료)
**How to avoid:** 리셋 실행 전 `--cleanup-stale-claims` 또는 lease 만료 대기 후 `failed`로 전환 처리
**Warning signs:** 리셋 후 `working_score > 0`이 남아 있으면 확인 필요

### Pitfall 4: retranslation_gen=1 이미 존재하는 87건
**What goes wrong:** Phase 07.1 A/B 테스트 중 9개 배치가 이미 gen=1로 리셋됨 — 이 항목들은 done 상태로 gen=1 존재
**Why it happens:** 87건은 A/B 테스트 배치로 gen=1 번역 완료
**How to avoid:** `ResetAllForRetranslation(nextGen=1)` 시 `ON CONFLICT (id, gen) DO NOTHING`으로 스냅샷 중복 방지. 이 87건은 이미 gen=1이므로 nextGen=2가 필요하지 않음 — 전체 일괄로 gen=1로 리셋하면 이 항목들은 gen=1 스냅샷만 덮어쓰려는 시도가 됨 (conflict skip). 신규 번역도 gen=1로 저장됨 — 동일 gen에서 highest-gen dedup은 여전히 동작 (같은 gen이면 first-seen이 선택, 정상).
**Warning signs:** `UNIQUE constraint (id, gen)` 에러 — `ON CONFLICT DO NOTHING` 사용 시 발생 안 함

### Pitfall 5: concurrency=2 translate 속도
**What goes wrong:** gpt-5.4 concurrency=2 (안정성 확보를 위한 Phase 07.1 수정값)로 35,009건 처리 시 매우 오래 걸림
**Why it happens:** OpenCode 불안정으로 3에서 2로 낮춘 상태
**How to avoid:** translate concurrency=2 유지하되 score concurrency를 조정하거나, 장시간 실행 수용. 예상 시간: batch_size=10 기준 3,501배치 × 평균 처리 시간
**Warning signs:** idle이 지속되면 watchdog 트리거 확인

### Pitfall 6: BuildV3Sidecar 정렬 시 contextual_entries 영향
**What goes wrong:** items 정렬 시 contextual_entries도 같은 순서로 나와야 함
**Why it happens:** contextual_entries는 ALL items를 포함하므로 정렬 전/후 순서 변경 영향 없음 (순서 불변 요건 없음)
**How to avoid:** 정렬은 entries dedup에만 영향. contextual_entries는 수정 없이 전량 포함

---

## Code Examples

### 전체 리셋 store 메서드 (신규)
```go
// Source: workflow/internal/v2pipeline/store.go ResetForRetranslation 패턴 기반
// guardrail: score_final=-1 (NOT NULL), failure_type/last_error/claimed_by=''
func (s *Store) ResetAllForRetranslation(nextGen int) (int, error) {
    now := s.nowValue()
    if s.backend == "postgres" {
        _, err := s.db.Exec(`
            INSERT INTO retranslation_snapshots (id, gen, ko_raw, ko_formatted, score_final, snapshot_at)
            SELECT id, $1, ko_raw, ko_formatted, score_final, $2
            FROM pipeline_items_v2 WHERE state IN ('done', 'failed')
            ON CONFLICT (id, gen) DO NOTHING`,
            nextGen, now,
        )
        if err != nil {
            return 0, fmt.Errorf("snapshot all: %w", err)
        }
        result, err := s.db.Exec(`
            UPDATE pipeline_items_v2
            SET state = $1, ko_raw = NULL, ko_formatted = NULL, score_final = -1,
                failure_type = '', last_error = '', claimed_by = '', claimed_at = NULL,
                lease_until = NULL, retranslation_gen = $2, updated_at = $3
            WHERE state IN ('done', 'failed')`,
            StatePendingTranslate, nextGen, now,
        )
        if err != nil {
            return 0, fmt.Errorf("reset all: %w", err)
        }
        affected, _ := result.RowsAffected()
        return int(affected), nil
    }
    // SQLite 경로: rebind 사용
    // ...
}
```

### BuildV3Sidecar highest-gen dedup (수정)
```go
// Source: workflow/internal/v2pipeline/export.go BuildV3Sidecar
// D-02: items를 source_raw별 highest retranslation_gen 우선으로 정렬 후 first-seen
import "sort"

func BuildV3Sidecar(items []contracts.V2PipelineItem) V3Sidecar {
    // sort: same source_raw → highest gen first; within same gen → sort_index order
    sorted := make([]contracts.V2PipelineItem, len(items))
    copy(sorted, items)
    sort.SliceStable(sorted, func(i, j int) bool {
        if sorted[i].SourceRaw != sorted[j].SourceRaw {
            return false // 다른 source_raw는 순서 유지
        }
        return sorted[i].RetranslationGen > sorted[j].RetranslationGen
    })
    seen := make(map[string]bool)
    for _, item := range sorted {
        // ... 기존 로직 동일: seen 체크 후 Entries 추가
    }
    // contextual_entries는 원래 items 순서 (sort 전) 사용
```

### go-v2-reset-all CLI 패턴
```go
// Source: workflow/cmd/go-retranslate-select/main.go 패턴 기반
// workflow/cmd/go-v2-reset-all/main.go
func run() int {
    // 1. store 열기 (--backend, --dsn 명시 필요)
    store, err := v2pipeline.OpenStore(cfg.CheckpointBackend, cfg.CheckpointDB, cfg.CheckpointDSN)
    // 2. 현재 max gen 조회
    var currentMaxGen int
    db.QueryRow(`SELECT COALESCE(MAX(retranslation_gen), 0) FROM pipeline_items_v2`).Scan(&currentMaxGen)
    nextGen := currentMaxGen + 1  // 기존 gen=1이 있어도 nextGen=2가 아닌 1 사용 — D-01에서 gen=1 지정
    // D-01: retranslation_gen을 0→1로 — 현재 max가 1이면 nextGen=1 (conflict skip)
    nextGen = 1  // 고정 (D-01 결정)
    // 3. dry-run 지원
    // 4. ResetAllForRetranslation(1) 호출
    count, err := store.ResetAllForRetranslation(1)
}
```

### go-v2-export 실행 명령
```bash
# Source: workflow/cmd/go-v2-export/main.go 플래그 확인
go run ./workflow/cmd/go-v2-export \
  --backend postgres \
  --dsn "postgres://postgres@127.0.0.1:5433/localize_agent?sslmode=disable" \
  --out-dir "E:/SteamLibrary/steamapps/common/Esoteric Ebb/BepInEx/plugins/TranslationLoader" \
  --textasset-dir "projects/esoteric-ebb/extract/1.1.3/ExportedProject/Assets/StreamingAssets" \
  --project-dir "projects/esoteric-ebb/output/batches/canonical_full_retranslate_live"
```

---

## Runtime State Inventory

| Category | Items Found | Action Required |
|----------|-------------|-----------------|
| Stored data | `pipeline_items_v2`: 35,009 done, 19 failed, 8 working_score (총 35,036건) | `ResetAllForRetranslation(1)` — working_score 先 reclaim |
| Stored data | `retranslation_snapshots`: 87건 gen=1 (A/B 테스트 배치) | `ON CONFLICT DO NOTHING` 로 중복 skip |
| Live service config | OpenCode 서버 포트 4112, DSN postgres@5433 | pipeline 실행 전 서버 기동 확인 |
| OS-registered state | 없음 (v2pipeline은 process로 실행, 등록된 서비스 없음) | 없음 |
| Secrets/env vars | DSN이 project.json에 하드코딩 — SOPS 사용 안 함 | 없음 |
| Build artifacts | `workflow/.bin/` 하위 컴파일된 바이너리 | go build 후 갱신 필요 (선택) |

**translations.json 현재 위치:** `E:/SteamLibrary/steamapps/common/Esoteric Ebb/BepInEx/plugins/TranslationLoader/translations.json` — D-03에 따라 export 시 완전 교체

---

## Validation Architecture

Phase 08은 실행 중심 Phase로, 자동 테스트보다 상태 검증이 핵심이다.

### 검증 단계별 체크리스트

| 단계 | 검증 방법 | 기준 |
|------|-----------|------|
| 리셋 후 | `SELECT state, COUNT(*) FROM pipeline_items_v2 GROUP BY state` | `pending_translate=35,028` (35,009+19), `done=0`, `working_*=0` |
| 파이프라인 실행 중 | 상태 카운트 주기적 확인 | `done + failed`가 점진적 증가 |
| 파이프라인 완료 | `done / total > 0.98` | 실패율 2% 미만 (v2-export 자체 fail-rate check: >20% abort) |
| export 후 | `len(sidecar.entries)` 확인 | 기존 entries 수와 비교 |
| diff 생성 후 | changed/unchanged 비율 | 전체 교체이므로 대부분 변경 정상 |
| 게임 검증 | 고정 씬 목록 플레이 | 태그 깨짐 없음, 대사 자연스러움 |

### 기존 테스트

```bash
go test ./workflow/internal/v2pipeline/... -run TestBuildV3Sidecar -v
go test ./workflow/internal/v2pipeline/... -run TestResetForRetranslation -v
```

`export_test.go`와 `retranslate_test.go`에 관련 단위 테스트 존재. BuildV3Sidecar 수정 후 기존 테스트 통과 확인 + 신규 highest-gen 케이스 테스트 추가 필요.

---

## Open Questions (RESOLVED)

1. **working_score=8 처리 방식**
   - What we know: lease_until=2026-04-12 02:58:53 (연구 시점 직후 만료 예정)
   - What's unclear: 만료 전 실행하면 `working_score` 상태로 리셋 대상 제외됨
   - Recommendation: 리셋 전 `--cleanup-stale-claims` 실행하거나 lease 만료 대기 후 진행. 또는 `WHERE state IN ('done', 'failed', 'working_score')` 포함하되 이 8건은 stale이므로 실질적으로 안전함
   - **RESOLVED:** 리셋 전 `--cleanup-stale-claims` 실행으로 처리. Plan 01 Task 2 사전 조건에 반영.

2. **retranslation_gen 값: 1 고정 vs max+1**
   - What we know: 현재 87건 gen=1 존재 (A/B 테스트 배치)
   - What's unclear: 이 87건을 gen=1로 재리셋하면 스냅샷 `ON CONFLICT DO NOTHING`으로 skip — 이전 gen=1 스냅샷 유실 없음. 신규 번역도 gen=1로 저장됨 — BuildV3Sidecar에서 동일 gen일 경우 sort_index 기준으로 처리됨 (정상)
   - Recommendation: **gen=1 고정** (D-01 결정대로). max+1로 하면 gen=2가 되어 별도 처리 불필요하게 복잡해짐
   - **RESOLVED:** gen=1 고정 (D-01). Plan 01 Task 2에 반영.

3. **파이프라인 예상 소요 시간**
   - What we know: concurrency=2, batch_size=10, 35,009건 → ~3,501 배치
   - What's unclear: gpt-5.4 평균 응답 시간 (Phase 07.1에서 translate+format+score 3단계 포함)
   - Recommendation: 파이프라인을 백그라운드 실행으로 설정하고 진행률 모니터링. 중단 후 재개(`--cleanup-stale-claims`)는 언제든 가능
   - **RESOLVED:** 사용자가 직접 파이프라인 실행 — Plan 02 objective에 실행 명령 명시. 소요 시간 불확실성은 수동 단계로 수용.

---

## Environment Availability

[VERIFIED: 직접 쿼리]

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| PostgreSQL | DB 백엔드 | ✓ | 17 (포트 5433) | — |
| localize_agent DB | pipeline 상태 | ✓ | 35,036 items 확인 | — |
| OpenCode 서버 | translate/format/score | 확인 필요 | 포트 4112 | run_pipeline_orchestrated.ps1이 자동 기동 |
| Go 1.24 | 빌드 | 프로젝트 기준 사용 중 | 1.24.0 | — |
| translations.json (현재) | diff 기준 | ✓ | E:/SteamLibrary/... | — |

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | retranslation_gen=1 고정이 적절 (현재 max gen=1인 87건과 충돌 없음) | 전체 리셋 패턴 | ON CONFLICT DO NOTHING으로 안전하게 처리되므로 위험 낮음 |
| A2 | working_score=8이 lease 만료 직후 stale이 됨 | Runtime State Inventory | 만료 전 리셋 시도 시 8건 누락 — cleanup-stale-claims로 해결 가능 |
| A3 | diff 도구 Python 스크립트가 Go CLI보다 빠름 | diff 도구 | 빌드 불필요, 단순 JSON 처리이므로 Python이 더 빠름 |

---

## Sources

### Primary (HIGH confidence)
- `workflow/internal/v2pipeline/export.go` — BuildV3Sidecar 현재 구현 직접 읽기
- `workflow/internal/v2pipeline/store.go` — ResetForRetranslation, QueryDone 직접 읽기
- `workflow/internal/v2pipeline/retranslate.go` — executeReset 패턴 직접 읽기
- `workflow/cmd/go-v2-pipeline/main.go` — 파이프라인 플래그 전체 직접 읽기
- `workflow/cmd/go-v2-export/main.go` — export 플래그 및 BuildV3Sidecar 호출 직접 읽기
- PostgreSQL DB 직접 쿼리 — 현재 상태 실측
- `projects/esoteric-ebb/output/batches/canonical_full_retranslate_live/project.json` — LLM 설정값 확인

### Secondary (MEDIUM confidence)
- `.knowledge/knowledge/guardrails.md` — score_final NOT NULL, --backend postgres 패턴 (이전 세션 검증)
- `.knowledge/knowledge/troubleshooting.md` — 알려진 함정 목록
- `.planning/phases/07.1-rag-mcp-pageindex-mcp/07.1-04-SUMMARY.md` — Phase 07.1 인계 사항

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — 전량 코드 직접 확인
- Architecture: HIGH — 기존 패턴 명확, 신규 작성은 단순 확장
- Pitfalls: HIGH — guardrails.md + troubleshooting.md에 이전 세션 검증 결과 반영
- 파이프라인 소요 시간 예측: LOW — 실측 미확인

**Research date:** 2026-04-12
**Valid until:** Phase 08 완료 시 (DB 상태는 연구 시점 기준, 실행 전 재확인 권장)
