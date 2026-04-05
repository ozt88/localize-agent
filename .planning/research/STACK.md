# Stack Research: Context-Aware Retranslation (v1.1)

**Domain:** Game translation pipeline quality improvement -- speaker extraction, branch context, tone consistency, prompt restructuring, selective retranslation
**Researched:** 2026-04-06
**Confidence:** HIGH

## Executive Assessment

**No new external dependencies needed.** The v1.1 milestone is a data-enrichment and prompt-engineering project, not a technology-stack project. All five target features (speaker extraction, branch context, tone consistency, prompt improvement, selective retranslation) are implementable with the existing Go standard library, PostgreSQL, and OpenCode LLM backend.

The existing codebase already has scaffolding for most of what v1.1 needs:
- `speaker_hint` field exists in `itemMeta`, `translationTask`, `normalizedPromptInput`, and `translatorPackageLine` -- but is sparsely populated (~40% coverage)
- `context_en` with scene context already flows through prompts via `normalizedBatchPromptPayload.Contexts`
- `semanticreview` package already has direct scoring infrastructure (0-100 scale, `directScoreResult`)
- `lore.go` and `glossary.go` demonstrate the exact pattern for context enrichment: load file -> match entries -> inject into prompt
- Retry package format and `go-esoteric-adapt-in` already support selective retranslation input

The work is: (1) better data extraction from ink JSON, (2) richer data structures in the pipeline, (3) better prompt construction, (4) a selection/scoring pass before retranslation.

## Recommended Stack (Additions Only)

### Core Technologies -- NO CHANGES

| Technology | Version | Status | Notes |
|------------|---------|--------|-------|
| Go | 1.24.0 | Keep | All new logic is pure Go |
| PostgreSQL | 17 | Keep | Add columns to pipeline state, no migration tool needed |
| OpenCode (gpt-5.4) | Current | Keep | Translation + scoring LLM |
| OpenCode (codex-mini) | Current | Keep | Tag restoration, unchanged |
| Python 3.x | Current | Keep | Only for ink JSON extraction script enhancements |

### New Go Packages -- NONE

No new `go.mod` dependencies required. Rationale per feature:

| Feature | Implementation With | Why No External Dep |
|---------|--------------------|--------------------|
| Speaker extraction | `regexp`, `strings`, `encoding/json` (stdlib) | Ink tag parsing is pattern matching on known `#tag` tokens; `infer_speaker_hint()` in Python extractor already does this, Go port follows same logic |
| Branch context tracking | `encoding/json` tree traversal (stdlib) | Ink JSON is a tree with `c-N` choice branches; traversal is recursive `map[string]any` walking, already proven in the v2 ink parser |
| Tone consistency | Existing `lore.go` load-match-inject pattern | Tone profiles are a static JSON file matched by speaker name; identical pattern to glossary/lore enrichment |
| Prompt restructuring | `strings`, `fmt` (stdlib) | Modifying `buildBatchPrompt()` and `normalizePromptInput()` -- pure string construction |
| Selective retranslation | Existing `semanticreview.DirectScorer` | `directScoreResult` with `CurrentScore`/`FreshScore` and `ReasonTags` already does 0-100 quality scoring |

### Supporting Data Files (NEW)

| File | Format | Purpose | Created By |
|------|--------|---------|-----------|
| `speaker_profiles.json` | `{"SpeakerName": {"tone": "...", "speech_level": "...", "style_notes": "..."}}` | Per-character translation style guide for tone consistency | Manual curation + LLM-assisted generation from dialogue samples |
| `branch_context_index.json` | `{"segment_id": {"parent_choice_en": "...", "branch_depth": N}}` | Pre-computed choice-branch ancestry for each segment | Enhancement to `extract_assetripper_textasset.py` |
| `retranslation_candidates.json` | `[{"id": "...", "score": N, "reason_tags": [...]}]` | Output of scoring pass, input to selective retranslation | `go-semantic-review --mode direct-score` |

## Integration Points with Existing Code

### 1. Speaker Extraction (Python extractor enhancement)

**Where:** `projects/esoteric-ebb/patch/tools/extract_assetripper_textasset.py`

**Current state:** `infer_speaker_hint(tag_tokens)` (line 96) already extracts speaker from ink `#tag` tokens by matching `r"^[A-Z][A-Za-z0-9_'-]{2,}$"` against non-ignored tokens. Returns `None` for narration, system text, and lines without speaker tags.

**What to enhance:**
- **Speaker propagation:** Carry speaker across consecutive lines in same segment (ink convention: speaker tag appears once, applies until next speaker tag or segment boundary)
- **Reply speaker extraction:** Parse `reply` tag to identify reactor vs. initiator
- **Speaker census:** New output: list of all unique speakers with line counts, for manual `speaker_profiles.json` curation

**Technology:** Python `re` + `collections.Counter`. Zero new packages.

### 2. Branch Context (Python extractor + Go pipeline enhancement)

**Where:** `extract_assetripper_textasset.py` (extraction) + `workflow/internal/translation/normalized_input.go` (prompt injection)

**Current state:** `choice_block_id` and `block_kind` exist in segment metadata. `flush_segment()` (line 167) tracks `choice_block` kind. But the **parent choice text** (what player choice led to this branch) is not captured.

**What to add:**
- During ink JSON traversal, when entering a `c-N` container, record the choice text from the parent `ChoicePoint`
- Store as `parent_choice_text_en` in segment metadata
- New field in `translationTask`:

```go
// Addition to existing translationTask struct in types.go
BranchContextEN string // "Player chose: [choice text]" or empty for main flow
BranchDepth     int    // 0 = main flow, 1+ = nested in choice branch
```

- In `normalizedBatchPromptItem`, add optional `branch_context` field
- In prompt rules, add: "If `branch_context` is present, the dialogue follows the specified player choice"

### 3. Tone Consistency (New data file + prompt enhancement)

**Where:** `workflow/internal/translation/skill.go` (warmup rules) + new `speaker_profile.go` (follows `lore.go` pattern)

**Current state:** `SpeakerHint` passes through to prompt as a bare name string. Rule 8 mentions using `current_ko`/`prev_ko`/`next_ko` for tone consistency, but the LLM has no character-specific style guidance.

**Implementation (follows existing `lore.go` pattern exactly):**

```go
// speaker_profile.go -- mirrors lore.go structure
type speakerProfile struct {
    Tone        string `json:"tone"`         // e.g., "formal elderly scholar"
    SpeechLevel string `json:"speech_level"` // e.g., "하십시오체", "해요체", "해체"
    StyleNotes  string `json:"style_notes"`  // e.g., "archaic vocabulary, measured pacing"
}

// Load from JSON file, match by SpeakerHint, inject as prompt field
// matchedSpeakerProfile(profiles, speakerHint) -> *speakerProfile
```

**Prompt injection:** New field `speaker_tone` in `normalizedPromptInput` and `normalizedBatchPromptItem`. New static rule: "If `speaker_tone` is present, use the specified speech level and match the described personality."

### 4. Prompt Restructuring (Modify existing prompt builders)

**Where:** `workflow/internal/translation/prompts.go`, `skill.go`, `normalized_input.go`

**Current state:** `defaultStaticRules()` has 24 rules in a flat numbered list. `buildBatchPrompt()` concatenates everything into a single instruction block. Rules mix structural guidance (JSON format) with translation guidance (fragment handling) with context usage rules.

**What to change:**
- **Warmup restructure:** Group 24 rules into sections (OUTPUT FORMAT, CONTEXT USAGE, TRANSLATION STYLE, SPECIAL CASES) for better LLM comprehension
- **Per-item enrichment:** Add `speaker_tone`, `branch_context` to normalized prompt items
- **Reduce prompt noise:** Move verbose structural rules (fragment_pattern, structure_pattern) from every prompt into warmup-only context; keep only a brief reference in per-batch prompts
- **Add continuity window:** Expand from prev/next 1 line to prev/next 2-3 lines when available (data already exists in segment context)

**No new technology.** This is editing string construction in 3 existing files.

### 5. Selective Retranslation (Existing infrastructure, new workflow)

**Where:** `workflow/internal/semanticreview/` (scoring) + new CLI entry point

**Current state:**
- `go-semantic-review --mode direct-score` scores translations 0-100
- `ReportItem` has `ReasonTags`: `semantic_drift`, `lexical_drift`, `closer_to_prev`, `closer_to_next`, `format_residue`
- `retryPackageItem` format already supports enriched retranslation input
- `go-esoteric-adapt-in` already ingests retry packages

**What to add -- new CLI command `go-retranslation-selector`:**
1. Run scoring pass (reuse `DirectScorer` from `semanticreview`)
2. Filter by threshold (e.g., score < 75 or specific `ReasonTags`)
3. Enrich candidates with speaker profile + branch context from new data files
4. Output retry package compatible with `go-esoteric-adapt-in`
5. Add `retranslation_reason` from `ReasonTags` to retry items for prompt injection

**This is primarily a pipeline wiring exercise**, connecting existing `DirectScorer` output to existing retry package ingestion.

## What NOT to Add

| Avoid | Why | What to Do Instead |
|-------|-----|-------------------|
| External NLP libraries (spaCy, NLTK) | Ink scripts have explicit speaker tags (`#SpeakerName`); NLP-based entity extraction is overkill for structured source data | Parse existing `#tag` tokens in `extract_assetripper_textasset.py` |
| Vector database (Pinecone, ChromaDB) | Fixed 40K item corpus with pre-computed segment context; RAG retrieval adds latency for marginal gain | Pre-computed segment context + branch ancestry from ink tree |
| Separate embedding model for tone analysis | `semanticreview` already has `paraphrase-multilingual-MiniLM-L12-v2`; tone is better handled by explicit style profiles than embedding distance | Curate `speaker_profiles.json` with explicit tone descriptions |
| Schema migration tool (golang-migrate, goose) | 2-3 column additions to one table; `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` is simpler | Hand-written ALTER TABLE statements |
| Caching layer (Redis) | OpenCode server handles caching; pipeline is batch-oriented with checkpoint resumption | Existing checkpoint store |
| Web UI for review | CLI pipeline philosophy is a project constraint | JSON export + `go-semantic-review` report |
| Fine-tuned scoring model | gpt-5.4 already scores effectively in `DirectScorer`; no local fine-tuning infrastructure | Improve scoring prompts in `buildMinimalDirectScorePrompt()` |
| LangChain / orchestration framework | Go pipeline already has retry, checkpoint, concurrency management | Existing `platform/llm_client.go` patterns |

## Alternatives Considered

| Category | Recommended | Alternative | Why Not Alternative |
|----------|-------------|-------------|-------------------|
| Speaker extraction | Enhance existing Python extractor | Rewrite extractor in Go | Python script already works for 286 files; this runs once per game version, not a hot path |
| Tone profiles | Static JSON, manually curated | Fully LLM-generated profiles | Start manual for ~20 named characters; human review essential for Korean speech level (formality hierarchy is nuanced) |
| Branch context | Pre-computed index file | Runtime ink tree traversal | Pre-computation = O(1) lookup at translation time; runtime traversal requires loading full ink JSON per batch |
| Quality scoring | Existing `semanticreview.DirectScorer` | New dedicated scoring pipeline | DirectScorer already does 0-100 with current/fresh comparison; adding speaker/context fields to input is a small change |
| Retranslation selection | Score threshold + reason tag filter | Human review of all 40K items | Scoring identifies ~2-5K candidates; human spot-checks the threshold boundary |
| Prompt format | Sectioned warmup + lean per-item | Single monolithic prompt (current) | Sectioned format reduces cognitive load for LLM; 24 flat rules in current warmup likely causes rule drift |

## DB Schema Additions

```sql
-- Enrich existing pipeline state for query/filter
ALTER TABLE pipeline_items_v2 ADD COLUMN IF NOT EXISTS speaker_hint TEXT DEFAULT '';
ALTER TABLE pipeline_items_v2 ADD COLUMN IF NOT EXISTS branch_depth INTEGER DEFAULT 0;
ALTER TABLE pipeline_items_v2 ADD COLUMN IF NOT EXISTS quality_score REAL DEFAULT -1;
ALTER TABLE pipeline_items_v2 ADD COLUMN IF NOT EXISTS retranslation_reason TEXT DEFAULT '';

-- Index for selective retranslation queries
CREATE INDEX IF NOT EXISTS idx_quality_score
  ON pipeline_items_v2(quality_score)
  WHERE quality_score >= 0;
```

## Installation

```bash
# No new Go dependencies -- go.mod unchanged
# No new Python packages -- stdlib only

# New data files to create:
# 1. Run enhanced extractor to get speaker census
python projects/esoteric-ebb/patch/tools/extract_assetripper_textasset.py
# -> produces speaker list + branch context index

# 2. Manually curate speaker_profiles.json from speaker census
# -> ~20 named characters with tone/speech_level/style_notes

# 3. Score existing translations for retranslation candidates
go run ./workflow/cmd/go-semantic-review --mode direct-score ...
# -> produces retranslation_candidates.json
```

## Version Compatibility

| Component | Current Version | v1.1 Compatible | Notes |
|-----------|----------------|-----------------|-------|
| Go 1.24.0 | 1.24.0 | Yes | No new language features needed |
| pgx/v5 v5.7.6 | v5.7.6 | Yes | ALTER TABLE is standard SQL |
| modernc.org/sqlite v1.38.2 | v1.38.2 | Yes | Checkpoint store unchanged |
| OpenCode gpt-5.4 | Current | Yes | Prompt changes only |
| OpenCode codex-mini | Current | Yes | Tag restoration unchanged |
| Python 3.x | Current | Yes | Extractor script enhancement, stdlib only |

## Sources

- **Codebase analysis** (HIGH confidence -- direct inspection):
  - `workflow/internal/translation/types.go` -- `translationTask`, `itemMeta` with existing `SpeakerHint`, `ContextEN` fields
  - `workflow/internal/translation/prompts.go` -- `buildBatchPrompt()`, `buildSinglePrompt()`, 24-rule static rules
  - `workflow/internal/translation/skill.go` -- `defaultStaticRules()`, warmup construction
  - `workflow/internal/translation/normalized_input.go` -- prompt input normalization, batch payload structure
  - `workflow/internal/translation/lore.go` -- load-match-inject enrichment pattern (model for speaker profiles)
  - `workflow/internal/translation/glossary.go` -- term matching enrichment pattern
  - `workflow/internal/semanticreview/direct_score.go` -- `DirectScorer`, 0-100 scoring, `directScoreResult`
  - `workflow/internal/semanticreview/scoring.go` -- `ReportItem`, `ReasonTags`, alignment penalties
  - `workflow/internal/contracts/evaluation.go` -- `EvalResult` with Fidelity/Fluency/Tone/Consistency scores
  - `projects/esoteric-ebb/patch/tools/extract_assetripper_textasset.py` -- `infer_speaker_hint()`, `flush_segment()`, tag token parsing
  - `projects/esoteric-ebb/cmd/go-esoteric-adapt-in/main.go` -- retry package ingestion, `enrichEntry()` pattern
  - `go.mod` -- current dependency list (pgx/v5 5.7.6, sqlite v1.38.2)
- **Memory files** (HIGH confidence -- user-validated context):
  - `project_translation_quality.md` -- problem statement: translations don't match surrounding context
  - `project_v2_pipeline_context.md` -- ink JSON structure, v2 architecture decisions

---
*Stack research for: Esoteric Ebb v1.1 context-aware retranslation*
*Researched: 2026-04-06*
