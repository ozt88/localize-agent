# Architecture Research: Context-Aware Retranslation Integration

**Domain:** LLM translation pipeline enhancement (context injection + selective retranslation)
**Researched:** 2026-04-06
**Confidence:** HIGH (based on direct code analysis of all relevant packages)

## Existing Architecture Summary

```
ink JSON files
  |
  v
[inkparse]  Parse() -> DialogueBlock[] (speaker, gate, choice, tags extracted)
  |
  v
[inkparse]  BuildBatches() -> Batch[] (gate-boundary clustering, content-type grouping)
  |
  v
[v2pipeline/store]  Seed() -> pipeline_items_v2 (PostgreSQL, state machine)
  |
  v
[v2pipeline/worker]  TranslateWorker -> FormatWorker -> ScoreWorker
  |                       |                |               |
  |  [clustertranslate]   |  [tagformat]   |  [scorellm]   |
  |  BuildScriptPrompt()  |  BuildFormat   |  BuildScore   |
  |  ParseNumberedOutput() |  Prompt()     |  Prompt()     |
  v                       v                v               v
pipeline_items_v2: pending_translate -> translated -> formatted -> done/retry
```

### Current Data Already Available in Pipeline

| Data | Where Extracted | Where Stored | Where Used in Prompt |
|------|----------------|--------------|---------------------|
| Speaker | `inkparse.parser.go` `isSpeakerTag()` | `V2PipelineItem.Speaker` + DB `speaker` col | `clustertranslate.prompt.go` line 92: `Speaker: "text"` format |
| Gate context | `V2PipelineStore.GetPrevGateLines()` | Queried at translate time | `clustertranslate.prompt.go` line 69: `[CONTEXT]` block |
| Content type | `inkparse.classifier.go` `Classify()` | `V2PipelineItem.ContentType` | `clustertranslate.prompt.go` `BuildContentSuffix()` |
| Choice marker | `inkparse.parser.go` | `V2PipelineItem.Choice` | `clustertranslate.prompt.go` line 89: `[CHOICE]` marker |
| Glossary | `glossary.LoadGlossary()` | In-memory `GlossarySet` | Warmup + per-batch `## Batch Glossary` |
| Ability voices | Static in `v2_base_prompt.md` | LLM warmup session | Rules for wis/str/int/con/dex/cha tone |

**Key observation:** Speaker extraction already exists and flows through the entire pipeline. The gap is not extraction -- it is (1) richer context injection and (2) the ability to selectively retranslate based on quality scores.

## Integration Analysis: Feature by Feature

### 1. Speaker Extraction Enhancement

**Current state:** `inkparse.parser.go` `isSpeakerTag()` extracts speaker from ink `#` tags. Already stored in `V2PipelineItem.Speaker` and DB `speaker` column. Already formatted in prompt as `[NN] Speaker: "text"`.

**Gap:** Speaker data is per-block, not per-scene. The LLM sees one speaker label per line but has no character profile or tone guidance beyond the ability-score voices in `v2_base_prompt.md`. Named NPCs (Braxo, She'lia, Captain Morgan, etc.) have no character voice descriptions.

**Integration approach -- MODIFY existing, no new package:**
- **Modify:** `projects/esoteric-ebb/context/v2_base_prompt.md` -- add NPC character voice profiles (same pattern as ability-score voices section)
- **Modify:** `clustertranslate/prompt.go` `BuildScriptPrompt()` -- inject per-batch speaker summary when batch contains named speakers
- **New file (data only):** `projects/esoteric-ebb/context/character_voices.json` -- structured speaker-to-personality mapping

**No new Go package needed.** Speaker extraction code is already solid. The improvement is in prompt content, not parser logic.

### 2. Branch Context Tracking

**Current state:** `GetPrevGateLines(knot, currentGate, limit=3)` fetches last 3 source_raw texts from the previous gate in the same knot. Injected as `[CONTEXT]` block in prompt. Choice blocks are marked with `[CHOICE]` in prompt.

**Gap:** When dialogue branches after a choice (e.g., player selects option A, NPC responds differently), the LLM only sees the previous gate's raw text -- it does not know which choice led to this branch. The gate/choice tree structure (knot -> gate -> choice) is in the DB but not used to build branch-aware context.

**Integration approach -- MODIFY `clustertranslate` + `v2pipeline/worker`:**

```
Current flow:
  GetPrevGateLines(knot, gate, 3) -> [CONTEXT] block (up to 3 EN lines)

Enhanced flow:
  GetBranchContext(knot, gate, choice) -> BranchContext {
    PrevGateLines []string   // existing: last 3 lines of prev gate
    ParentChoice  string     // NEW: the choice text that led to this gate
    SiblingBranch []string   // NEW: 1-2 lines from sibling branches for contrast
  }
```

**Changes:**
- **Modify:** `contracts/v2pipeline.go` -- add `GetBranchContext()` to `V2PipelineStore` interface (or extend `GetPrevGateLines`)
- **Modify:** `v2pipeline/store.go` -- implement `GetBranchContext()` query (SQL: join same-knot items on choice hierarchy)
- **Modify:** `clustertranslate/types.go` -- extend `ClusterTask` with `BranchContext` field
- **Modify:** `clustertranslate/prompt.go` -- extend `BuildScriptPrompt()` to render branch context in prompt
- **Modify:** `v2pipeline/worker.go` `translateBatch()` -- call `GetBranchContext()` instead of `GetPrevGateLines()`

**No new Go package needed.** This is a data-flow enrichment within existing boundaries.

### 3. Prompt Restructuring

**Current state:** Prompt is built in two stages:
1. **Warmup** (`clustertranslate.BuildBaseWarmup`): system prompt + context + rules + glossary -> session warmup (sent once per session)
2. **Per-batch** (`clustertranslate.BuildScriptPrompt`): `[CONTEXT]` block + numbered lines + batch glossary + content-type suffix

**Gap:** All context is front-loaded in warmup. Per-batch prompt only has gate context, no scene summary, no tone hints, no character dynamics for the specific batch.

**Integration approach -- MODIFY `clustertranslate/prompt.go`:**

The prompt restructuring is a modification of `BuildScriptPrompt()`, not a new component. The enrichment adds:
1. **Scene header:** Brief scene description derived from knot name + gate context
2. **Active speakers:** List speakers in this batch with their role/tone hints
3. **Branch position:** Where this dialogue sits in the choice tree
4. **Tone directive:** Per-content-type + per-speaker tone calibration

```
Enhanced prompt structure:
  ## Scene: [knot name / human-readable]
  Speakers: Braxo (gruff merchant), Player (ability: wis)
  Branch: after choosing "Ask about the shipment"

  [CONTEXT]
  (previous gate lines)

  ---

  [01] Braxo: "text"
  [02] "narration"
  [03] wis: "text"

  ## Batch Glossary
  ...

  ## Content Rules
  ...
```

**Changes:**
- **Modify:** `clustertranslate/prompt.go` -- restructure `BuildScriptPrompt()` to include scene header, speaker list, branch position
- **Modify:** `clustertranslate/types.go` -- extend `ClusterTask` with scene metadata fields
- **New file (data only):** `projects/esoteric-ebb/context/character_voices.json` -- lookup table for speaker descriptions

### 4. Selective Retranslation

**Current state:** The pipeline state machine already supports retranslation: `MarkScored()` routes items back to `pending_translate` when `failure_type` is "translation" or "both". Score threshold is hardcoded at 7 in `v2_score_prompt.md`. All 40,067 items are currently in `done` state.

**Gap:** There is no mechanism to:
1. Query done items by score range and select them for retranslation
2. Reset selected done items back to `pending_translate` with enhanced context
3. Track retranslation generations (v1.0 translation vs v1.1 retranslation)

**Integration approach -- NEW CLI entry point + MODIFY store:**

```
New entry point: workflow/cmd/go-retranslate-select/main.go
  |
  v
Uses existing V2PipelineStore + new methods:
  - QueryByScoreRange(minScore, maxScore float64) -> []V2PipelineItem
  - ResetForRetranslation(ids []string, reason string) -> int
  |
  v
Resets selected items to pending_translate
  |
  v
Existing v2pipeline workers pick them up with enhanced prompts
```

**Changes:**
- **New CLI:** `workflow/cmd/go-retranslate-select/main.go` -- select items by score range, speaker, knot, content type; reset to pending_translate
- **Modify:** `contracts/v2pipeline.go` -- add `QueryByScoreRange()` and `ResetForRetranslation()` to interface
- **Modify:** `v2pipeline/store.go` -- implement new query/reset methods
- **Modify:** DB schema -- add `retranslation_gen` column (INTEGER DEFAULT 0, incremented on each retranslation cycle) to track generations

**The existing pipeline workers handle the rest.** Once items are reset to `pending_translate`, TranslateWorker picks them up with whatever prompt improvements are in place.

## Component Boundary Map

```
EXISTING (modify)                          NEW (create)
=================                          ============

inkparse/                                  cmd/go-retranslate-select/
  parser.go         (no change)              main.go  (new CLI entry point)
  batcher.go        (no change)
  classifier.go     (no change)            projects/esoteric-ebb/context/
  types.go          (no change)              character_voices.json (data file)

contracts/
  v2pipeline.go     (add 2-3 methods)

v2pipeline/
  store.go          (implement new methods)
  worker.go         (enrich translateBatch context)
  types.go          (no change)
  postgres_v2_schema.sql (add retranslation_gen col)

clustertranslate/
  types.go          (extend ClusterTask)
  prompt.go         (restructure BuildScriptPrompt)
  parser.go         (no change)
  validate.go       (no change)

scorellm/           (no change)
tagformat/          (no change)
glossary/           (no change)
fragmentcluster/    (no change)
```

## Data Flow: Enhanced Translation

### Before (v1.0 pipeline)

```
Seed -> pending_translate -> [TranslateWorker: basic prompt] -> translated -> ...
                                    |
                          GetPrevGateLines(3)
                          Speaker label in [NN] line
                          Static warmup rules
```

### After (v1.1 pipeline)

```
                              character_voices.json
                                    |
Seed/Reset -> pending_translate -> [TranslateWorker: enriched prompt] -> translated -> ...
                                    |
                          GetBranchContext(knot, gate, choice) -> {
                            PrevGateLines, ParentChoice, SiblingBranch
                          }
                          Speaker label + speaker description
                          Scene header with knot/gate context
                          Branch position description
                          Enhanced content-type rules
```

### Selective Retranslation Flow

```
[go-retranslate-select CLI]
  --min-score 3.5 --max-score 6.9
  --speaker "Braxo"
  --content-type "dialogue"
  |
  v
QueryByScoreRange() -> filter by criteria -> candidate IDs
  |
  v
ResetForRetranslation(ids, "v1.1-context-improvement")
  -> SET state='pending_translate', retranslation_gen=gen+1
  |
  v
Existing TranslateWorker picks up with enriched prompts
  -> format -> score -> done (with improved context)
```

## Patterns to Follow

### Pattern 1: Extend Contracts Interface, Not Bypass It

**What:** All new store methods go through `contracts.V2PipelineStore` interface.
**Why:** Existing code uses compile-time interface checks (`var _ contracts.V2PipelineStore = (*Store)(nil)`). Bypassing the interface breaks the architecture's clean layering.
**Example:**
```go
// In contracts/v2pipeline.go -- ADD to V2PipelineStore interface:
QueryByScoreRange(minScore, maxScore float64, filters map[string]string) ([]V2PipelineItem, error)
ResetForRetranslation(ids []string, reason string) (int, error)
GetBranchContext(knot, gate, choice string) (*BranchContext, error)
```

### Pattern 2: Enrich ClusterTask, Not Worker

**What:** All new context data flows through `ClusterTask` struct into `BuildScriptPrompt()`. The worker only assembles data; prompt construction stays in `clustertranslate`.
**Why:** Keeps prompt logic testable without LLM. Worker tests stay focused on orchestration.
**Example:**
```go
// In clustertranslate/types.go -- extend ClusterTask:
type ClusterTask struct {
    Batch           inkparse.Batch
    PrevGateLines   []string       // existing
    GlossaryJSON    string         // existing
    BranchContext   *BranchContext  // NEW: choice tree context
    ActiveSpeakers  []SpeakerInfo  // NEW: speaker descriptions for this batch
    SceneHeader     string         // NEW: human-readable scene description
}
```

### Pattern 3: DB Migration via Schema Extension

**What:** Add new columns with DEFAULT values so existing data stays valid.
**Why:** 40,067 items already in `done` state. Schema changes must not break existing data.
**Example:**
```sql
ALTER TABLE pipeline_items_v2 ADD COLUMN retranslation_gen INTEGER NOT NULL DEFAULT 0;
ALTER TABLE pipeline_items_v2 ADD COLUMN retranslation_reason TEXT NOT NULL DEFAULT '';
```

## Anti-Patterns to Avoid

### Anti-Pattern 1: New Pipeline Stage

**What people do:** Add a "context enrichment" stage between seed and translate (e.g., `pending_enrich -> working_enrich -> pending_translate`).
**Why it is wrong:** Adds state machine complexity, new worker role, new failure modes. Context enrichment is a query-time operation, not a persistence stage.
**Do this instead:** Enrich context at translate time in `translateBatch()`. Query branch context and speaker data when building the prompt, not as a separate pipeline phase.

### Anti-Pattern 2: Separate Retranslation Pipeline

**What people do:** Create a parallel `v2pipeline_retranslation` table or a `go-retranslate` worker with its own state machine.
**Why it is wrong:** Duplicates the translate->format->score flow. Two codepaths for the same logic means divergence bugs.
**Do this instead:** Reset items in the existing `pipeline_items_v2` table to `pending_translate`. The existing workers handle them identically, but with enriched prompts.

### Anti-Pattern 3: Speaker Profile in LLM Warmup

**What people do:** Put all 50+ character profiles into the session warmup text.
**Why it is wrong:** Warmup is sent once per session. Most characters appear in few batches. Bloating warmup wastes LLM context window and dilutes focus.
**Do this instead:** Include only active speakers (those present in the current batch) in the per-batch prompt.

## Build Order (Dependency-Driven)

The features have clear dependencies that dictate build order:

```
Phase 1: Speaker Profiles + Prompt Restructure
  - character_voices.json (data authoring, no code dependency)
  - clustertranslate/types.go (extend ClusterTask)
  - clustertranslate/prompt.go (restructure BuildScriptPrompt)
  - v2pipeline/worker.go (pass speaker info to ClusterTask)
  Tests: prompt output tests, no LLM needed

Phase 2: Branch Context
  - contracts/v2pipeline.go (add GetBranchContext)
  - v2pipeline/store.go (implement SQL query)
  - clustertranslate/prompt.go (render branch context)
  - v2pipeline/worker.go (call GetBranchContext)
  Tests: store query tests, prompt rendering tests
  Depends on: Phase 1 (prompt structure must be ready)

Phase 3: Selective Retranslation
  - contracts/v2pipeline.go (add QueryByScoreRange, ResetForRetranslation)
  - v2pipeline/store.go (implement methods)
  - DB migration (retranslation_gen column)
  - cmd/go-retranslate-select/main.go (new CLI)
  Tests: store tests, CLI integration test
  Depends on: Phase 1+2 (prompts must be improved before retranslating)

Phase 4: Execute Retranslation + Validate
  - Run go-retranslate-select to reset low-score items
  - Run v2pipeline workers (translate->format->score)
  - Compare score distributions before/after
  - In-game verification
  Depends on: Phase 1+2+3
```

**Rationale:** Prompt improvements (Phase 1-2) must land before retranslation (Phase 3-4) because retranslating with the same prompts would produce the same quality. The CLI selector (Phase 3) is a simple tool that depends on the enriched prompts being ready.

## Schema Changes

### New Columns

```sql
-- Track retranslation generations
ALTER TABLE pipeline_items_v2
  ADD COLUMN retranslation_gen INTEGER NOT NULL DEFAULT 0;

-- Record why an item was selected for retranslation
ALTER TABLE pipeline_items_v2
  ADD COLUMN retranslation_reason TEXT NOT NULL DEFAULT '';

-- Index for score-based queries
CREATE INDEX IF NOT EXISTS idx_pv2_score_final
  ON pipeline_items_v2(score_final)
  WHERE state = 'done';

-- Index for speaker-based queries
CREATE INDEX IF NOT EXISTS idx_pv2_speaker
  ON pipeline_items_v2(speaker)
  WHERE speaker != '';
```

### No Changes Needed

- `id`, `source_hash`, `state` columns: unchanged
- State machine transitions: unchanged (reset goes through existing states)
- `batch_id`: unchanged (retranslated items keep original batch_id for context locality)

## Sources

- Direct code analysis of all packages in `workflow/internal/` and `workflow/pkg/`
- `contracts/v2pipeline.go`: V2PipelineStore interface (lines 1-104)
- `v2pipeline/worker.go`: TranslateWorker, translateBatch (lines 1-204)
- `clustertranslate/prompt.go`: BuildScriptPrompt, BuildBaseWarmup (lines 1-149)
- `inkparse/parser.go`: isSpeakerTag, walkFlatContent (lines 1-503)
- `v2pipeline/postgres_v2_schema.sql`: current schema
- `projects/esoteric-ebb/context/v2_base_prompt.md`: current prompt structure
- Memory: `project_translation_quality.md` -- context gap diagnosis

---
*Architecture research for: context-aware retranslation integration*
*Researched: 2026-04-06*
