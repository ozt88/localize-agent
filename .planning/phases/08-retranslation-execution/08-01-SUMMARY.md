---
phase: 08-retranslation-execution
plan: 01
subsystem: v2pipeline
tags: [retranslation, dedup, reset, cli]
dependency_graph:
  requires: []
  provides: [highest-gen-dedup, reset-all-cli, retranslation-gen-column]
  affects: [go-v2-export, go-v2-pipeline]
tech_stack:
  added: []
  patterns: [2-pass-map-dedup, alter-table-migration]
key_files:
  created:
    - workflow/cmd/go-v2-reset-all/main.go
  modified:
    - workflow/internal/contracts/v2pipeline.go
    - workflow/internal/v2pipeline/export.go
    - workflow/internal/v2pipeline/export_test.go
    - workflow/internal/v2pipeline/store.go
decisions:
  - "D-02 implemented: BuildV3Sidecar uses 2-pass highest-gen dedup (map[source_raw]bestRecord)"
  - "retranslation_gen column added via ALTER TABLE IF NOT EXISTS at runtime"
  - "snapshot_at stored as RFC3339 TEXT (not TIMESTAMPTZ) for portability"
metrics:
  duration: 4min
  completed: 2026-04-11T18:57:55Z
  tasks: 2
  files: 5
---

# Phase 08 Plan 01: BuildV3Sidecar Dedup + Reset-All CLI Summary

BuildV3Sidecar에 highest-gen 2-pass dedup 적용, go-v2-reset-all CLI 작성, 35,028건 전체 리셋 완료

## Task Results

| Task | Name | Commit | Key Changes |
|------|------|--------|-------------|
| 1 | BuildV3Sidecar highest-gen dedup + ResetAllForRetranslation | 56fa1ea | 2-pass map dedup in export.go, RetranslationGen field in contracts, ResetAllForRetranslation in store.go |
| 2 | go-v2-reset-all CLI + full reset | 524c20d | CLI with dry-run/cleanup-stale-claims, retranslation_gen column migration, 35,028 items reset |

## Implementation Details

### BuildV3Sidecar Highest-Gen Dedup (D-02)

Changed from single-pass `if !seen[source_raw]` (first-seen-wins) to 2-pass approach:
- Pass 1: Build `best map[string]bestRecord` tracking highest `RetranslationGen` per source_raw
- Pass 2: Only add item to entries[] if `best[item.SourceRaw].id == item.ID`
- contextual_entries[] unchanged (all items)
- Legacy behavior preserved: when all gen=0, first-seen-wins (same ID wins the map)

### ResetAllForRetranslation Store Method

- Creates `retranslation_snapshots` table if not exists
- Adds `retranslation_gen` column to `pipeline_items_v2` via ALTER TABLE IF NOT EXISTS
- Snapshot INSERT with ON CONFLICT DO NOTHING (idempotent)
- UPDATE done+failed -> pending_translate, clear ko_raw/ko_formatted/score/claims

### Reset Execution Result

```
Before: map[done:35009 failed:19 working_score:8]
After:  map[pending_translate:35028 working_score:8]
Reset 35028 items to pending_translate (retranslation_gen=1)
```

8 working_score items remain (stale claims not in done/failed state).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] snapshot_at TEXT encoding error**
- Found during: Task 2 execution
- Issue: `nowValue()` returns `time.Time` for postgres, but `retranslation_snapshots.snapshot_at` is TEXT column
- Fix: Used `time.Now().UTC().Format(time.RFC3339)` for snapshot_at, kept `nowValue()` for pipeline_items_v2.updated_at (TIMESTAMPTZ)
- Files modified: workflow/internal/v2pipeline/store.go

**2. [Rule 3 - Blocking] retranslation_gen column missing from DB schema**
- Found during: Task 2 execution
- Issue: Column referenced in UPDATE but not in existing schema DDL
- Fix: Added `ALTER TABLE pipeline_items_v2 ADD COLUMN IF NOT EXISTS retranslation_gen INTEGER NOT NULL DEFAULT 0` at runtime
- Files modified: workflow/internal/v2pipeline/store.go

**3. [Rule 1 - Bug] ProjectConfig field paths**
- Found during: Task 2 build
- Issue: `projCfg.CheckpointBackend` doesn't exist; fields are nested under `projCfg.Translation.*`
- Fix: Changed to `projCfg.Translation.CheckpointBackend`, `.CheckpointDSN`, `.CheckpointDB`
- Files modified: workflow/cmd/go-v2-reset-all/main.go

## Verification

- `go test ./workflow/internal/v2pipeline/... -run "TestBuildV3Sidecar" -count=1` PASS
- `go test ./workflow/internal/v2pipeline/... -run "TestExportBuildV3Sidecar_MixedItems" -count=1` PASS (no regression)
- `go build ./workflow/cmd/go-v2-reset-all/` SUCCESS
- dry-run output: "Would reset 35028 items (done=35009, failed=19)"
- actual reset: pending_translate=35028, done=0, failed=0

## Self-Check: PASSED
