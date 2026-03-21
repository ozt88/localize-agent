# Architecture Research

**Domain:** Game localization pipeline -- ink JSON parsing, 2-stage LLM translation, BepInEx patching
**Researched:** 2026-03-22
**Confidence:** HIGH (based on existing v1 codebase analysis, validated v2 experiments, ink runtime specification)

## System Overview

```
                              v2 Pipeline Architecture
==========================================================================

 SOURCE EXTRACTION LAYER
 +-----------------------------------------------------------------+
 |  ink JSON files (286 TextAssets)                                 |
 |       |                                                          |
 |  [Ink Tree Parser] -----> Dialogue Block Source (scene scripts)  |
 |       |                                                          |
 |  [Content Classifier] --> Content-typed batches                  |
 |       |                   (dialogue/spell/UI/item/system)        |
 +-----------------------------------------------------------------+
       |
       | scene scripts + metadata (source_file, knot, speaker, tags)
       v
 TRANSLATION LAYER (2-stage LLM)
 +-----------------------------------------------------------------+
 |  Stage 1: Cluster Translator (gpt-5.4)                          |
 |    Input:  tag-free scene script (10-30 lines)                   |
 |    Output: Korean script (same line count, no tags)              |
 |       |                                                          |
 |  [Line Splitter + ID Mapper] (code)                              |
 |       |                                                          |
 |  Stage 2: Tag Formatter (codex-mini)                             |
 |    Input:  EN source with tags + KO translation (tag-needing)    |
 |    Output: KO with tags restored                                 |
 |       |                                                          |
 |  [Tag Count Validator] (code)                                    |
 +-----------------------------------------------------------------+
       |
       | translated items (id -> source_raw -> ko_with_tags)
       v
 PIPELINE STATE LAYER
 +-----------------------------------------------------------------+
 |  PostgreSQL (pipeline_items table)                               |
 |    States: pending_translate -> working_translate ->             |
 |            pending_format -> working_format ->                   |
 |            pending_validate -> done | failed                     |
 |                                                                  |
 |  [Pipeline Orchestrator] (lease-based worker pools)              |
 +-----------------------------------------------------------------+
       |
       | done items with final_ko
       v
 PATCH OUTPUT LAYER
 +-----------------------------------------------------------------+
 |  [Patch Builder]                                                 |
 |    translations.json  -- source->target sidecar map              |
 |    textassets/         -- ink JSON with KO injected (285 files)  |
 |    localizationtexts/  -- CSV overrides (feats, spells, UI)      |
 |    runtime_lexicon.json -- substring/regex runtime rules         |
 +-----------------------------------------------------------------+
       |
       v
 GAME RUNTIME LAYER
 +-----------------------------------------------------------------+
 |  BepInEx Plugin (Plugin.cs)                                      |
 |    TranslationMap    -- direct source->target lookup              |
 |    NormalizedMap     -- tag-stripped fallback lookup               |
 |    ContextualMap     -- source_file-scoped lookup                 |
 |    TextAsset patches -- ink JSON replacement at load time         |
 |    Localization CSV  -- ID-based override                         |
 |    RuntimeLexicon    -- substring/regex live replacement          |
 |    Font injection    -- Korean font fallback into TMP assets      |
 +-----------------------------------------------------------------+
```

## Component Responsibilities

| Component | Responsibility | Existing Code | v2 Changes |
|-----------|----------------|---------------|------------|
| **Ink Tree Parser** | Walk ink JSON tree, merge `"^text"` entries into dialogue blocks, preserve knot/gate/choice structure | `go-esoteric-adapt-in` (v1: line-level) | NEW: block-level merging, branch structure preservation |
| **Content Classifier** | Assign content types (dialogue/spell/UI/item/system), determine batch grouping strategy | `extract_scene_texts.py` classify_text | EXTEND: per-type batching format |
| **Cluster Translator** | Translate scene scripts as coherent units via gpt-5.4, tag-free input | `translation/skill.go` (v1: per-item) | NEW: scene-script prompt format, 10-30 line batches |
| **Tag Formatter** | Restore markup tags from EN source into KO translation via codex-mini | None (v1 tried inline) | NEW: dedicated stage |
| **Line Splitter / ID Mapper** | Parse LLM cluster output back to individual lines, map to source IDs | None | NEW |
| **Tag Count Validator** | Verify tag counts match between EN source and formatted KO | `postprocess_validation.go` | ADAPT: tag-count-only mode |
| **Pipeline Orchestrator** | DB state machine with lease-based worker pools | `translationpipeline/` | EXTEND: new states for format stage |
| **Patch Builder** | Generate translations.json, textassets/, localizationtexts/ from done items | `go-esoteric-apply-out` | ADAPT: block-level output format |
| **BepInEx Plugin** | Runtime text interception and replacement in Unity game | `Plugin.cs` | SIMPLIFY: remove TryTranslateTagSeparatedSegments, optimize for block matching |

## Recommended v2 Project Structure

New code fits within the existing layered architecture. No structural reorganization needed.

```
workflow/
  cmd/
    go-ink-parse/              # NEW: ink JSON tree -> dialogue block source
    go-cluster-translate/      # NEW: scene-cluster translation runner
    go-tag-format/             # NEW: tag restoration runner
    go-translation-pipeline/   # EXTEND: add format stage to state machine
  internal/
    inkparse/                  # NEW: ink JSON tree walker + block merger
    clustertranslate/          # NEW: cluster translation domain (prompt, parse, validate)
    tagformat/                 # NEW: tag formatter domain (prompt, validate)
    contracts/                 # EXTEND: new interfaces for format stage
    translation/               # KEEP: shared prompt/validation utilities
    translationpipeline/       # EXTEND: new pipeline states
  pkg/
    platform/                  # KEEP: LLM client, DB stores (possibly extend for format checkpoint)
    shared/                    # KEEP: project config, utilities

projects/esoteric-ebb/
  cmd/
    go-esoteric-adapt-in/      # REPLACE: v2 version using inkparse package
  context/
    cluster_translate_*.md     # NEW: system prompts for cluster translation
    tag_format_*.md            # NEW: system prompts for tag formatting
  source/
    dialogue_blocks/           # NEW: parsed dialogue block source files
```

### Structure Rationale

- **`inkparse/` as separate domain package:** Ink tree walking is pure data transformation with no LLM or DB dependencies. Isolating it makes it testable with fixture JSON files and reusable across games that use ink.
- **`clustertranslate/` and `tagformat/` as separate domains:** The 2-stage LLM pattern has distinct prompt formats, output parsers, and validation rules. Separating them prevents entanglement and allows independent iteration.
- **Existing `translationpipeline/` extended rather than replaced:** The lease-based state machine is proven infrastructure. Adding `pending_format`/`working_format` states is simpler than building a new orchestrator.

## Architectural Patterns

### Pattern 1: Dialogue Block Extraction (Ink JSON Tree Walk)

**What:** Recursive walk of ink JSON tree to produce dialogue blocks that match game rendering units. Each block is the concatenation of consecutive `"^text"` entries within a single gate/choice container.

**When to use:** Source extraction phase. Run once per game version to produce the canonical source set.

**Trade-offs:** Block-level source means fewer, larger translation units (better context) but requires careful handling of branch points where the tree forks.

**Key implementation detail:**
```
ink JSON node types:
  "^text"     -> text content (accumulate into current block)
  "#tag"      -> metadata tag (speaker, DC check -- attach to block)
  "ev" / "out" -> evaluation frame (skip, not translatable)
  "->knot"    -> divert (block boundary)
  ["c-N",...]  -> choice container (each choice = new block with OPTION marker)
  ["g-N",...]  -> gate container (sequential blocks within)
```

The parser must track the **path** (e.g., `TS_Snell_Meeting/g-2/c-4`) because this path is the canonical ID prefix for all lines in that block. This path also becomes the `segment_id` used by the plugin's contextual matching.

### Pattern 2: 2-Stage LLM Translation (Translate then Format)

**What:** Separate translation quality from markup fidelity. Stage 1 (gpt-5.4) receives tag-free text and focuses on natural Korean. Stage 2 (codex-mini) receives the EN-with-tags plus the KO-without-tags and mechanically restores tags.

**When to use:** Any content with inline markup (bold, color, links). Content without tags skips Stage 2 entirely.

**Trade-offs:**
- Pro: Eliminates the tag-corruption problem that caused 99.7% of v1 retry failures
- Pro: Stage 2 uses a cheaper/faster model (codex-mini)
- Con: Two LLM calls per tagged item increases total latency
- Con: Line-mapping between cluster output and source IDs must be exact (off-by-one = wrong tag on wrong line)

**Validated by experiment:** 4/4 tag restoration accuracy, 8/8 line mapping accuracy.

### Pattern 3: Content-Type-Aware Batching

**What:** Different content types get different prompt formats and batch sizes optimized for their structure.

**When to use:** Always. This is the core batching strategy for v2.

| Content Type | Batch Format | Size | LLM Prompt Style |
|-------------|-------------|------|-------------------|
| Dialogue/narration/choice | Scene script (numbered lines) | 10-30 lines | "Translate this scene script" |
| Spells/tooltips | Structured cards (name + desc) | 5-10 items | "Translate these spell cards" |
| UI labels | Dictionary (key: value pairs) | 50-100 items | "Translate this UI dictionary" |
| Item/quest descriptions | Cards with context | 5-10 items | "Translate these item descriptions" |
| System/tutorial text | Full document | Entire section | "Translate this document" |

**Trade-off:** More prompt templates to maintain, but dramatically better translation quality per type. The v1 one-size-fits-all JSON prompt was a core quality limitation.

### Pattern 4: Lease-Based Pipeline State Machine

**What:** DB-driven state machine where worker pools claim items by setting `claimed_by + lease_until` atomically. Stale leases are reclaimed automatically.

**When to use:** Pipeline orchestration. Already proven in v1.

**v2 extension:** Add states for the format stage:
```
pending_translate -> working_translate -> translated
  -> pending_format -> working_format -> formatted
  -> pending_validate -> done | failed
```

Items without tags skip `pending_format` and go directly from `translated` to `pending_validate`.

## Data Flow

### Complete v2 Pipeline Flow

```
[1] Game Assets (ink JSON files on disk)
         |
         | go-esoteric-adapt-in (or go-ink-parse)
         v
[2] Dialogue Block Source (JSON: id, source_raw, source_file, knot_path, speaker, tags[], text_role)
         |
         | pipeline_ingest.py (dedup by source_raw, INSERT into pipeline_items)
         v
[3] PostgreSQL pipeline_items (state: pending_translate)
         |
         | go-cluster-translate (worker pool, claims batches by scene/knot)
         |   Build scene script -> gpt-5.4 -> parse output -> map lines to IDs
         v
[4] pipeline_items (state: translated, ko_raw stored)
         |
         | Items WITH tags: state -> pending_format
         | Items WITHOUT tags: state -> pending_validate (skip format)
         |
         | go-tag-format (worker pool, claims tagged items)
         |   EN-with-tags + KO-without-tags -> codex-mini -> KO-with-tags
         v
[5] pipeline_items (state: formatted, ko_formatted stored)
         |
         | go-validate (tag count check: EN tags == KO tags)
         v
[6] pipeline_items (state: done | failed)
         |
         | go-esoteric-apply-out (read done items, build patch artifacts)
         v
[7] Patch Artifacts
    translations.json    -- 65K+ source->target entries (block-level keys)
    textassets/           -- 285 ink JSON files with KO injected
    localizationtexts/    -- 8 CSV files with KO overrides
    runtime_lexicon.json  -- substring/regex replacement rules
         |
         | Patch build script (copy to game dir with BepInEx/fonts)
         v
[8] Game Runtime
    Plugin.cs loads translations.json, textassets/, etc.
    Harmony patches intercept text display calls
    TranslationMap lookup -> NormalizedMap fallback -> ContextualMap -> RuntimeLexicon
```

### Key Data Transformation Points

1. **Ink tree -> Dialogue blocks:** Many-to-one merge. Multiple `"^text"` entries become one block. The merge boundary is defined by diverts, choices, and gate transitions. This is the critical transformation that v1 got wrong.

2. **Scene script -> Line-mapped translations:** One-to-many split. The cluster LLM output (N lines) must map back to N source IDs. The line splitter uses the numbered-line format from the prompt to enforce exact count matching.

3. **KO-raw + EN-tags -> KO-formatted:** One-to-one per item. The formatter receives a single EN line with tags and a single KO line without tags, returns the KO line with tags inserted at corresponding positions.

4. **Done items -> translations.json:** The key in translations.json is the full merged block text (what the game engine actually sends to the plugin). This is why block-level source extraction is essential -- the key must match what the game produces at runtime.

### State Management

| Store | What It Tracks | Technology |
|-------|---------------|------------|
| `pipeline_items` table | Item state, ko_raw, ko_formatted, claimed_by, lease | PostgreSQL |
| Source JSON files | Canonical EN source per dialogue block | Filesystem (generated, versioned) |
| Patch artifacts | Final output for game consumption | Filesystem (generated) |
| Plugin runtime maps | In-memory lookup tables loaded at game start | C# Dictionary (transient) |

## Build Order (Dependencies)

The components have clear sequential dependencies. This directly informs phase structure.

### Phase 1: Ink Tree Parser (no dependencies)

Build the `inkparse` package that walks ink JSON and produces dialogue block source. This is pure code (no LLM, no DB), fully testable with fixture files.

**Outputs:** Dialogue block source JSON files with correct block boundaries.
**Validates:** Block boundaries match game rendering units (can verify by comparing against v1 translations.json keys).
**Must be correct before:** Any translation work begins, because wrong block boundaries propagate through the entire pipeline.

### Phase 2: Cluster Translation Domain (depends on Phase 1)

Build `clustertranslate` with scene-script prompt construction, gpt-5.4 integration, output parser, and line-to-ID mapping.

**Outputs:** `ko_raw` values stored in pipeline_items.
**Validates:** Line count matches, no hallucinated lines, consistent tone within scene.
**Must be correct before:** Tag formatting, because the formatter needs clean KO text as input.

### Phase 3: Tag Formatter Domain (depends on Phase 2)

Build `tagformat` with codex-mini integration, tag restoration prompt, and tag count validation.

**Outputs:** `ko_formatted` values stored in pipeline_items.
**Validates:** Tag count EN == Tag count KO, tag structure preserved.
**Can run in parallel with:** Nothing -- needs translated output.

### Phase 4: Pipeline Integration (depends on Phases 2-3)

Extend `translationpipeline` with new states (`pending_format`, `working_format`). Wire up the full flow from pending_translate through done.

**Outputs:** Complete pipeline run producing 77K+ done items.
**Validates:** End-to-end throughput, error rates, stale lease recovery.

### Phase 5: Patch Output + Plugin Update (depends on Phase 4)

Adapt `go-esoteric-apply-out` for block-level output. Update Plugin.cs to remove `TryTranslateTagSeparatedSegments` and optimize for block-level matching.

**Outputs:** Working Korean patch with no tag leaks.
**Validates:** In-game testing -- text displays correctly, no bold leaks, no missing translations.

## Anti-Patterns

### Anti-Pattern 1: Line-Level Source Extraction from Ink JSON

**What people do:** Extract each `"^text"` entry as a separate translation item.
**Why it's wrong:** The game engine concatenates multiple `"^"` entries into a single display string. The plugin receives the concatenated form and cannot match individual fragments. This was the v1 root cause.
**Do this instead:** Walk the ink tree and merge all consecutive `"^text"` entries within a gate/container into a single dialogue block.

### Anti-Pattern 2: Asking the Translation LLM to Preserve Tags

**What people do:** Include markup tags in the translation prompt and instruct the LLM to preserve them.
**Why it's wrong:** LLMs reliably corrupt tags -- changing attribute quotes, reordering attributes, dropping closing tags. In v1, 99.7% of retry failures were tag corruption on otherwise correct translations.
**Do this instead:** Strip tags before translation, translate tag-free text, then use a separate formatter LLM call (or code) to restore tags mechanically.

### Anti-Pattern 3: Per-Item Translation Without Scene Context

**What people do:** Translate each string individually with minimal context (previous/next line).
**Why it's wrong:** Dialogue lines in narrative games reference earlier conversation, maintain speaker tone, and use scene-specific vocabulary. Per-item translation produces inconsistent register and tone across a scene.
**Do this instead:** Batch lines by scene/knot and translate as a coherent script. The cluster approach preserves narrative flow and character voice.

### Anti-Pattern 4: Validating Tags by String Comparison

**What people do:** Compare the full tag structure (`<b><color=#5782FD>`) character-by-character between EN and KO.
**Why it's wrong:** This is too strict -- it rejects valid translations where tag order is semantically equivalent. It also does not catch the real failure mode (missing or extra tags).
**Do this instead:** Validate by tag count only. Count each unique tag type in EN and KO and require exact match. Position validation is unnecessary because the formatter places tags correctly by design.

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| OpenCode (gpt-5.4) | HTTP POST to 127.0.0.1:4112, OpenAI-compatible API | Session-based, supports concurrent slots. Used for Stage 1 (translation) |
| OpenCode (codex-mini) | Same endpoint, different model parameter | Used for Stage 2 (tag formatting). Lower latency, lower cost |
| PostgreSQL (5433) | pgx/v5 driver, `pipeline_items` table | Lease-based state machine. Source of truth for pipeline progress |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| inkparse -> clustertranslate | Filesystem (dialogue block JSON files) | Decoupled: parser runs once, translator reads output |
| clustertranslate -> tagformat | PostgreSQL (ko_raw column in pipeline_items) | Coupled through DB state machine |
| tagformat -> patch builder | PostgreSQL (ko_formatted column in pipeline_items) | Coupled through DB state machine |
| patch builder -> Plugin.cs | Filesystem (translations.json, textassets/) | Decoupled: builder writes, plugin reads at game start |
| Plugin.cs -> game engine | Harmony patches (IL2CPP method interception) | Tightly coupled to game version (1.1.3) |

## Scaling Considerations

| Concern | Current Scale (77K items) | If Expanded (multiple games) |
|---------|--------------------------|------------------------------|
| LLM throughput | ~56 RPM peak, 6-8s latency, manageable | Add more OpenCode instances, autoscaler already exists |
| DB state management | Single PostgreSQL, sub-second queries | Fine for 500K+ items, partition by project if needed |
| Ink parser performance | 286 files, seconds to parse | Pure Go, handles thousands of files trivially |
| Patch artifact size | 65K entries in translations.json, ~2MB | Plugin loads into memory at startup, fine up to ~500K entries |

### First Bottleneck: LLM Throughput for Full Retranslation

77K items at 20 lines per cluster = ~3,900 cluster calls for Stage 1. At 56 RPM peak, that is ~70 minutes. Stage 2 (tag formatting) runs only on tagged items (~40% of total), adding ~30 minutes. Total: ~2 hours for a full run. The autoscaler handles burst management.

### Second Bottleneck: Line Mapping Errors

If the cluster LLM returns the wrong number of lines, the entire cluster fails. The mitigation is strict line-count validation with retry (up to 3 attempts). This is a quality bottleneck, not a performance one.

## Sources

- Existing codebase analysis (`.planning/codebase/ARCHITECTURE.md`, `.planning/codebase/STRUCTURE.md`)
- v2 experiment results documented in `project_v2_pipeline_context.md`
- ink runtime specification: ink JSON format uses `"^"` prefix for text content, `"#"` for tags, nested arrays for containers
- BepInEx IL2CPP plugin framework: Harmony patches for method interception in Unity IL2CPP builds
- v1 pipeline operational data: 77,816 items processed, 369 persist-skip failures (99.7% tag-related)

---
*Architecture research for: game localization pipeline (ink JSON + 2-stage LLM + BepInEx patching)*
*Researched: 2026-03-22*
