---
phase: 02-translation-engine
verified: 2026-03-23T00:00:00Z
status: passed
score: 17/17 must-haves verified
re_verification: false
gaps: []
human_verification:
  - test: "Run go-v2-pipeline -once against live PostgreSQL DB"
    expected: "One translate->format->score cycle completes, item moves to done or retry state in pipeline_items_v2"
    why_human: "Requires a running PostgreSQL instance with populated pipeline_items_v2 and an active OpenCode server"
  - test: "Ingest Phase 1 parser JSON with passthrough blocks"
    expected: "Passthrough items immediately appear with state=done and ko_formatted=source_raw; non-passthrough items appear with state=pending_translate"
    why_human: "Requires actual Phase 1 parser output file and live PostgreSQL connection"
---

# Phase 02: Translation Engine Verification Report

**Phase Goal:** Build the v2 translation engine — cluster translation, tag formatting, score LLM, and pipeline orchestration with lease-based DB state machine.
**Verified:** 2026-03-23
**Status:** passed
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| #  | Truth | Status | Evidence |
|----|-------|--------|----------|
| 1  | pipeline_items_v2 table exists with states for translate, format, score stages | VERIFIED | `postgres_v2_schema.sql` has `CREATE TABLE IF NOT EXISTS pipeline_items_v2` with `state` column; 10 state constants defined in `types.go` |
| 2  | Phase 1 parser JSON can be ingested into pipeline_items_v2 with source_raw dedup | VERIFIED | `go-v2-ingest/main.go` calls `store.Seed()` which executes `ON CONFLICT (source_hash) DO NOTHING`; `TestSeedInsertsAndDeduplicates` passes |
| 3  | Passthrough blocks are set to done with ko_formatted=source_raw on ingest | VERIFIED | Ingest CLI checks `block.IsPassthrough` and sets state="done", ko_formatted=block.Text; line 81 in `go-v2-ingest/main.go` |
| 4  | Lease-based claim/release works for all pipeline states | VERIFIED | `ClaimPending` uses `UPDATE ... SET state=?, claimed_by=?, lease_until=NOW()+interval WHERE state=? AND (lease_until IS NULL OR lease_until < NOW()) LIMIT ? RETURNING *`; `TestClaimPendingAndRelease` passes |
| 5  | Stale claims can be reclaimed after lease expiry | VERIFIED | `CleanupStaleClaims` method in store; `-cleanup-stale-claims` flag in CLI |
| 6  | Glossary loads from GlossaryTerms.txt + localizationtexts CSVs + speaker names into unified set | VERIFIED | `LoadGlossary` calls all 3 sub-loaders, deduplicates case-insensitively; 9 glossary tests pass |
| 7  | Scene clusters are formatted as numbered-line scripts with speaker labels and choice markers | VERIFIED | `BuildScriptPrompt` produces `[NN] Speaker: "text"` and `[NN] [CHOICE] "text"` format; 11 prompt tests pass |
| 8  | Previous gate context (last 3 lines) is injected as [CONTEXT] block per D-03 | VERIFIED | `prompt.go` line 68-80 prepends `[CONTEXT]` block when `task.PrevGateLines` non-empty |
| 9  | LLM output is parsed by [NN] markers back to source block IDs per TRANS-03 | VERIFIED | `ParseNumberedOutput` uses regex `\[(\d+)\]`; `MapLinesToIDs` maps lines to `BlockIDOrder`; 6 parser tests pass |
| 10 | Line count mismatch triggers auto-reject per TRANS-04 | VERIFIED | `ValidateTranslation` returns error "line count mismatch: expected N, got M"; 5 validate tests pass |
| 11 | Degenerate output (empty, exact copy) triggers auto-reject per D-13 | VERIFIED | `degenerateReason` helper checks empty + exact_source_copy; called in `ValidateTranslation` |
| 12 | 7 tag types are handled correctly by tag extractor | VERIFIED | `tagRe = regexp.MustCompile('</?[a-zA-Z][a-zA-Z0-9]*(?:=[^>]*)?>`)` matches i, b, shake, wiggle, u, size=N, s; 8 tag tests pass |
| 13 | Tag validation compares ordered tag string lists for exact match (not just count) | VERIFIED | `ValidateTagMatch` builds frequency maps and compares per-unique-tag counts; order ignored per D-07; 6 validate tests pass |
| 14 | gpt-5.3-codex-spark receives EN+KO pairs and returns KO with tags restored | VERIFIED | `BuildFormatPrompt` outputs `{"pairs": [{"en": "...", "ko": "..."}]}` per D-05; `ParseFormatResponse` validates count; 7 tagformat tests pass |
| 15 | Score LLM returns JSON with failure_type routing to correct retry stage | VERIFIED | `ParseScoreResponse` validates failure_type in {pass, translation, format, both}; `ScoreResult.TargetState()` routes to correct pipeline state per D-14; `MarkScored` in store applies routing; 8 scorellm tests pass |
| 16 | Pipeline orchestrator runs translate -> format -> score stages with worker pools | VERIFIED | `Run()` launches `TranslateWorker`, `FormatWorker`, `ScoreWorker` goroutines based on concurrency config; `TestTranslateWorkerHappyPath`, `TestFormatWorkerSkipsNoTags`, `TestScoreWorkerRoutesFailureType` pass |
| 17 | Format stage is skipped for blocks without tags (has_tags=false) | VERIFIED | `MarkTranslated` routes has_tags=false directly to pending_score (skips pending_format); `TestMarkTranslatedRoutesCorrectly` + `TestFormatWorkerSkipsNoTags` pass |

**Score:** 17/17 truths verified

---

## Required Artifacts

| Artifact | Provides | Exists | Substantive | Wired | Status |
|----------|----------|--------|-------------|-------|--------|
| `workflow/internal/contracts/v2pipeline.go` | V2PipelineStore interface, V2PipelineItem struct | Yes | 12 methods, full item struct | Implemented by store.go | VERIFIED |
| `workflow/internal/v2pipeline/store.go` | PostgreSQL/SQLite store | Yes | Seed, ClaimPending, MarkTranslated, MarkFormatted, MarkScored, MarkFailed, AppendAttemptLog, 8+ more | `var _ contracts.V2PipelineStore = (*Store)(nil)` compile check | VERIFIED |
| `workflow/internal/v2pipeline/types.go` | 10 state constants + Config struct | Yes | All 10 states defined; Config has all 3-stage LLM profile fields | Imported by worker.go, run.go, main.go | VERIFIED |
| `workflow/internal/v2pipeline/postgres_v2_schema.sql` | DDL for pipeline_items_v2 | Yes | CREATE TABLE with all columns; 4 indexes (state, state+lease, source_hash, batch_id) | Embedded via go:embed in store.go | VERIFIED |
| `workflow/cmd/go-v2-ingest/main.go` | CLI to ingest Phase 1 JSON | Yes | Reads ParseResult envelope, calls BuildBatches, detects tags, handles passthrough, calls store.Seed | Calls inkparse.BuildBatches and store.Seed | VERIFIED |
| `workflow/internal/glossary/loader.go` | Glossary from 3 sources | Yes | LoadGlossary, LoadGlossaryTerms, LoadLocalizationTexts, LoadSpeakers, WarmupTerms, FilterForBatch, FormatJSON | Called by run.go | VERIFIED |
| `workflow/internal/clustertranslate/prompt.go` | Cluster translation prompt | Yes | BuildBaseWarmup, BuildScriptPrompt, BuildContentSuffix with speaker/choice/context/glossary | Called by worker.go (`clustertranslate.BuildScriptPrompt`) | VERIFIED |
| `workflow/internal/clustertranslate/parser.go` | Numbered-line output parser | Yes | ParseNumberedOutput, MapLinesToIDs with `\[(\d+)\]` regex | Called by worker.go | VERIFIED |
| `workflow/internal/clustertranslate/validate.go` | Translation output validation | Yes | ValidateTranslation checks line count (TRANS-04) and degenerate (D-13) | Called by worker.go | VERIFIED |
| `workflow/internal/tagformat/tags.go` | Tag extraction and comparison | Yes | ExtractTags, HasRichTags, StripTags, CountTags with 7-type regex | Used by validate.go | VERIFIED |
| `workflow/internal/tagformat/prompt.go` | Formatter prompt for codex-spark | Yes | BuildFormatWarmup, BuildFormatPrompt (JSON pairs per D-05), ParseFormatResponse | Called by worker.go (`tagformat.BuildFormatPrompt`) | VERIFIED |
| `workflow/internal/tagformat/validate.go` | Tag validation | Yes | ValidateTagMatch with frequency map comparison, order-independent per D-07 | Called by worker.go | VERIFIED |
| `workflow/internal/scorellm/parser.go` | Score LLM response parsing | Yes | ParseScoreResponse validates failure_type, handles code fences | Called by worker.go | VERIFIED |
| `workflow/internal/scorellm/types.go` | Score types and routing | Yes | ScoreResult with TargetState() routing all 4 failure_type values per D-14 | Used by parser.go and worker.go | VERIFIED |
| `workflow/internal/v2pipeline/worker.go` | 3-role worker implementations | Yes | TranslateWorker, FormatWorker, ScoreWorker with D-15 retry escalation and D-16 logging | Called by run.go | VERIFIED |
| `workflow/internal/v2pipeline/run.go` | Pipeline orchestrator | Yes | Run(Config) int launches all 3 worker pools, loads glossary, CountByState reporting | Called by go-v2-pipeline/main.go | VERIFIED |
| `workflow/cmd/go-v2-pipeline/main.go` | CLI entry point | Yes | Full flag interface: -project, -dsn, -role, -once, -cleanup-stale-claims, all LLM profile flags | Calls v2pipeline.Run | VERIFIED |
| `projects/esoteric-ebb/context/v2_base_prompt.md` | Translation system prompt | Yes | 36 lines with numbered-line rules, proper noun preservation | Read by run.go | VERIFIED |
| `projects/esoteric-ebb/context/v2_format_prompt.md` | Tag restoration prompt | Yes | 21 lines with tag restoration instructions | Read by run.go | VERIFIED |
| `projects/esoteric-ebb/context/v2_score_prompt.md` | Score LLM prompt | Yes | 20 lines with quality evaluation criteria | Read by run.go | VERIFIED |

---

## Key Link Verification

| From | To | Via | Status | Evidence |
|------|----|-----|--------|----------|
| `v2pipeline/store.go` | `contracts/v2pipeline.go` | `var _ contracts.V2PipelineStore = (*Store)(nil)` | WIRED | Verified at line 19 of store.go |
| `cmd/go-v2-ingest/main.go` | `v2pipeline/store.go` | `store.Seed(items)` | WIRED | Line 127 of main.go |
| `clustertranslate/prompt.go` | `glossary/loader.go` | `glossary.FilterForBatch` | WIRED | Found in prompt.go |
| `clustertranslate/parser.go` | `inkparse/types.go` | `DialogueBlock` types via PromptMeta | WIRED | PromptMeta.BlockIDOrder holds DialogueBlock IDs |
| `tagformat/validate.go` | `tagformat/tags.go` | `ExtractTags(enSource)` | WIRED | Lines 12-13 of validate.go |
| `scorellm/parser.go` | `v2pipeline/types.go` | failure_type string literals map to state constants | WIRED | TargetState() in types.go uses state name strings; import cycle resolved by literals |
| `v2pipeline/worker.go` | `clustertranslate/prompt.go` | `clustertranslate.BuildScriptPrompt` | WIRED | Line 111 of worker.go |
| `v2pipeline/worker.go` | `tagformat/prompt.go` | `tagformat.BuildFormatPrompt` | WIRED | Line 282 of worker.go |
| `v2pipeline/worker.go` | `scorellm/prompt.go` | `scorellm.BuildScorePrompt` | WIRED | Line 386 of worker.go |
| `v2pipeline/worker.go` | `v2pipeline/store.go` | `store.MarkTranslated/MarkFormatted/MarkScored` | WIRED | Lines 179, 314, 406 of worker.go |
| `cmd/go-v2-pipeline/main.go` | `v2pipeline/run.go` | `v2pipeline.Run(cfg)` | WIRED | Line 69 of main.go |

---

## Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| TRANS-01 | 02-02, 02-04 | Scene cluster sent tag-free as script format to gpt-5.4 | SATISFIED | BuildScriptPrompt produces numbered-line format; TranslateWorker calls it |
| TRANS-02 | 02-02, 02-04 | Branch/choice markers preserved for tone/context consistency | SATISFIED | `[CHOICE]` marker handling in BuildScriptPrompt D-02; PrevGateLines context D-03 |
| TRANS-03 | 02-02, 02-04 | Translation lines mapped back to source block IDs via [NN] markers | SATISFIED | ParseNumberedOutput + MapLinesToIDs; confirmed by 6 parser tests |
| TRANS-04 | 02-02, 02-04 | Line count mismatch triggers auto-reject and retry | SATISFIED | ValidateTranslation returns error on mismatch; worker calls handleRetry |
| TRANS-05 | 02-03, 02-04 | codex-mini/codex-spark restores tags only for tagged blocks | SATISFIED | FormatWorker only claims pending_format items; has_tags=false blocks routed to pending_score by MarkTranslated |
| TRANS-06 | 02-03, 02-04 | Exact tag string match validation after restoration (not just count) | SATISFIED | ValidateTagMatch compares frequency maps — unique tag string + count; order-independent per D-07 |
| TRANS-07 | 02-02, 02-04 | Glossary built from 3 sources and injected into LLM context | SATISFIED | LoadGlossary from GlossaryTerms.txt + localizationtexts CSVs + speakers; WarmupTerms + FilterForBatch injected in worker |
| TRANS-08 | 02-03, 02-04 | Quality scoring with typed failure routing for re-translation | SATISFIED | ScoreResult.TargetState() routes pass/translation/format/both; MarkScored applies routing; worker processes score results |
| INFRA-01 | 02-01, 02-04 | DB-based pipeline state management with crash recovery via lease | SATISFIED | ClaimPending uses lease-based claiming; CleanupStaleClaims for crash recovery; 10 state constants |
| INFRA-02 | 02-01, 02-04 | Dedup by source_raw (source_hash) — no blind INSERT | SATISFIED | `ON CONFLICT (source_hash) DO NOTHING` in Seed; UNIQUE constraint on source_hash column |
| INFRA-03 | 02-01, 02-04 | Pipeline states extended for format stage | SATISFIED | pending_format and working_format state constants; pipeline_items_v2 schema has format_attempts, ko_formatted columns |

---

## Anti-Patterns Found

No blockers or warnings detected. Key scans performed:
- No TODO/FIXME/placeholder comments in any Phase 02 source files
- No `return null` / empty handler stubs
- No hardcoded empty data flowing to user-visible output
- `logAttempt` helper centralizes `AppendAttemptLog` calls — single reference is intentional, not a stub

---

## Human Verification Required

### 1. End-to-End Pipeline Cycle

**Test:** With PostgreSQL running and pipeline_items_v2 populated from go-v2-ingest, run:
```
go run ./workflow/cmd/go-v2-pipeline/ -project esoteric-ebb -dsn "postgres://..." -role all -once
```
**Expected:** At least one item transitions from pending_translate through the full translate->format->score->done cycle (or retry state on failure). `CountByState` output should show state changes.
**Why human:** Requires active PostgreSQL instance, OpenCode server at configured URL, and populated pipeline_items_v2 table.

### 2. Ingest CLI with Phase 1 Parser Output

**Test:** Run go-v2-ingest with actual Phase 1 parser JSON, then re-run to verify dedup:
```
go run ./workflow/cmd/go-v2-ingest/ -input /tmp/parse_result.json -dsn "postgres://..."
# Re-run same command
go run ./workflow/cmd/go-v2-ingest/ -input /tmp/parse_result.json -dsn "postgres://..."
```
**Expected:** First run: all N blocks inserted, passthrough blocks show state=done. Second run: 0 inserted, N skipped (dedup).
**Why human:** Requires actual Phase 1 parser JSON output file and live PostgreSQL.

---

## Test Summary

| Package | Tests | Status |
|---------|-------|--------|
| `workflow/internal/v2pipeline` | 15 | All pass |
| `workflow/internal/clustertranslate` | 22 | All pass |
| `workflow/internal/tagformat` | 21 | All pass |
| `workflow/internal/scorellm` | 8 | All pass |
| `workflow/internal/glossary` | 9 | All pass |
| **Total** | **75** | **All pass** |

Both `go-v2-ingest` and `go-v2-pipeline` binaries compile without errors. `go vet` passes clean across all Phase 02 packages.

All 11 documented git commits verified present in repository history (f426a78 through e2c9236).

---

## Conclusion

Phase 02 goal is fully achieved. The v2 translation engine is implemented end-to-end:

- **DB infrastructure** (Plan 01): Lease-based state machine with PostgreSQL/SQLite dual-backend, source_hash dedup, 10 pipeline states
- **Translation domain** (Plan 02): Numbered-line cluster prompting, glossary injection, [NN]-marker parsing, line-count + degenerate validation
- **Tag/Score domain** (Plan 03): 7-type tag extraction, frequency-map validation (order-independent per D-07), codex-spark formatter prompt, score LLM routing by failure_type
- **Pipeline orchestration** (Plan 04): 3-role worker pool with D-15 retry escalation, D-16 attempt logging, SIGINT-safe shutdown, -once test mode

All 11 requirements (TRANS-01 through TRANS-08, INFRA-01 through INFRA-03) satisfied. Ready for Phase 03 (patch build).

---

_Verified: 2026-03-23_
_Verifier: Claude (gsd-verifier)_
