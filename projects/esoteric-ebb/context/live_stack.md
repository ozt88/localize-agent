# Esoteric Ebb Live Stack Reference

## Operational Truth
- Official live batch entrypoint:
  - `projects/esoteric-ebb/output/batches/canonical_full_retranslate_live`
- Underlying live batch data dir:
  - `projects/esoteric-ebb/output/batches/canonical_full_retranslate_dual_score_20260311_1`
- Official live database:
  - PostgreSQL
  - DSN: `postgres://postgres@127.0.0.1:5433/localize_agent?sslmode=disable`

## Live Stack

| Service | Address |
|---------|---------|
| PostgreSQL | `127.0.0.1:5433` |
| OpenCode | `127.0.0.1:4112` |
| Review | `http://127.0.0.1:8094` |

- PostgreSQL data dir: `workflow/output/postgres17_data`
- PostgreSQL log: `workflow/output/postgres17.log`
- Pipeline entrypoint: `projects/esoteric-ebb/cmd/run_pipeline_orchestrated.ps1`

## Recovery Commands

```powershell
# Full stack
powershell -ExecutionPolicy Bypass -File scripts\recover-live-stack.ps1 -Action status -Profile custom
powershell -ExecutionPolicy Bypass -File scripts\recover-live-stack.ps1 -Action recover -Profile custom

# PostgreSQL
powershell -ExecutionPolicy Bypass -File scripts\manage-postgres5433.ps1 -Action status
powershell -ExecutionPolicy Bypass -File scripts\manage-postgres5433.ps1 -Action restart

# OpenCode
powershell -ExecutionPolicy Bypass -File scripts\manage-opencode-serve.ps1 -Action status
powershell -ExecutionPolicy Bypass -File scripts\manage-opencode-serve.ps1 -Action restart

# Review
powershell -ExecutionPolicy Bypass -File scripts\manage-review.ps1 -Action status
powershell -ExecutionPolicy Bypass -File scripts\manage-review.ps1 -Action restart

# Pipeline
powershell -ExecutionPolicy Bypass -File projects\esoteric-ebb\cmd\run_pipeline_orchestrated.ps1 -Action status -Profile custom
powershell -ExecutionPolicy Bypass -File projects\esoteric-ebb\cmd\run_pipeline_orchestrated.ps1 -Action restart -Profile custom
```

## Important Warnings
- Windows PostgreSQL default service may come back on port `5432`; this is NOT the operational instance.
- The operational PostgreSQL is the workspace-local cluster on port `5433`, not `C:\Program Files\PostgreSQL\17\data`.
- If review shows wrong counts, check whether it is pointed at SQLite fallback instead of PostgreSQL 5433.
- If `opencode` is down, translation/overlay lanes will fail with transport errors.

## Review
- Review should be started against PostgreSQL 5433 / `localize_agent`.
- SQLite-backed review is acceptable only as emergency fallback.

## Overlay Lane
- Overlay/UI rows use lanes: `pending_overlay_translate` / `working_overlay_translate`
- Current overlay ID prefix: `ovl-mainmenu-`
- Route command:
  ```powershell
  powershell -ExecutionPolicy Bypass -File projects\esoteric-ebb\cmd\run_pipeline_orchestrated.ps1 -Action route-overlay-ui -Profile custom
  ```

## Score Transport Note
- `empty response body` on score is a real transport failure, not display noise.
- The pipeline requeues score rows back to `pending_score` on score batch errors.

## Reference Paths
- Live batch alias: `projects/esoteric-ebb/output/batches/canonical_full_retranslate_live`
- Live batch config: `projects/esoteric-ebb/output/batches/canonical_full_retranslate_live/project.json`
- Live source/current/ids:
  - `canonical_full_retranslate_live/source_esoteric.json`
  - `canonical_full_retranslate_live/current_esoteric.json`
  - `canonical_full_retranslate_live/ids_esoteric.txt`
- Live translator package chunks: `canonical_full_retranslate_live/canonical_translation_chunks.json`
- Review logs: `workflow/output/go-review.stdout.log`, `workflow/output/go-review.stderr.log`
- OpenCode logs: `workflow/output/opencode/`
- Key binaries: `workflow/bin/go-translation-pipeline.exe`, `workflow/bin/go-translate.exe`, `workflow/bin/go-review.exe`
- Overlay inputs:
  - `projects/esoteric-ebb/patch/input/source_overlay_20260315_wave1_merge.json`
  - `projects/esoteric-ebb/patch/input/source_overlay_20260315_enriched_refined_gameplay_only.json`
- Fragment artifacts:
  - `workflow/output/fragment_production_candidates.json`
  - `workflow/output/fragment_broken_speech_batch_report_top15.json`
  - `workflow/output/fragment_broken_speech_backup_top15.json`
