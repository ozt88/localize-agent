# Project Research Summary

**Project:** Esoteric Ebb Korean Localization Pipeline (v2)
**Domain:** LLM-based game localization — Ink narrative engine, BepInEx runtime patching
**Researched:** 2026-03-22
**Confidence:** HIGH

## Executive Summary

This project is a ground-up rebuild of a v1 translation pipeline for a Unity/Ink narrative game. The v1 pipeline failed for a single fundamental reason: it extracted each `"^text"` entry from the ink JSON tree as an independent translation unit, but the game engine concatenates consecutive entries into dialogue blocks before sending them to the plugin. This caused 65,233 translations.json keys that the runtime plugin could never match. The v2 approach fixes this at the root: a new ink tree walker merges consecutive `"^"` entries into the same block the game engine renders, producing source strings that match exactly what the plugin receives at runtime. Every subsequent component in the pipeline — clustering, translation, formatting, patch output — builds on this corrected extraction.

The recommended architecture is a 5-layer pipeline: source extraction (ink tree parsing), 2-stage LLM translation (gpt-5.4 for Korean translation, codex-mini for tag restoration), pipeline state management (PostgreSQL lease-based state machine), patch output generation (translations.json + textassets), and a simplified BepInEx plugin matching chain. The 2-stage translation design is the other key architectural decision: v1 proved that asking a single LLM to translate and preserve rich-text markup simultaneously causes tag corruption in 99.7% of retry failures. Separating concerns — translate first with tags stripped, then restore tags with a cheap formatter model — eliminates this entirely. Both approaches are validated by v2 experiments before any pipeline code was written.

The primary risk is in Phase 1: if the ink tree parser misidentifies dialogue block boundaries, every downstream component is wrong and a full retranslation is required. This is a HIGH recovery cost. The mitigation is explicit: build a test harness against known scenes (Snell_Companion used in v2 experiments), compare parser output against plugin capture logs, and require 95%+ exact match before proceeding to translation. Secondary risks (LLM line count drift, tag corruption, plugin fallback collisions) all have MEDIUM or LOW recovery cost and clear automated detection strategies.

## Key Findings

### Recommended Stack

The v2 stack adds no new external dependencies. The entire pipeline is built on Go 1.24 (stdlib `encoding/json` for ink JSON tree parsing), PostgreSQL with pgx/v5 5.7.6 (pipeline state), Python 3.x (ingestion scripts), OpenCode server with gpt-5.4 and codex-mini (LLM translation stages), and BepInEx 6 IL2CPP with HarmonyX (runtime plugin). The critical decision to avoid `encoding/json/v2` (experimental, memory bugs reported, not covered by Go 1 compat promise) and any third-party ink tools (Ink-Localiser works on raw `.ink` source, not compiled JSON) keeps the stack stable and proven. No NuGet additions to the C# plugin are needed.

See `.planning/research/STACK.md` for version table, alternatives analysis, and sources.

**Core technologies:**
- `Go encoding/json` v1 (stdlib): Ink JSON tree walking, LLM response parsing — battle-tested, used in 47 existing files, no third-party ink runtime exists for Go
- `gpt-5.4` via OpenCode: Stage 1 cluster translation — validated in experiments (8/8 line mapping, 16/16 branched dialogue)
- `codex-mini` via OpenCode: Stage 2 tag formatter — validated (4/4 perfect tag count match), fast and cheap for mechanical tag restoration
- `pgx/v5 5.7.6`: PostgreSQL pipeline state machine — proven from v1, lease-based worker pools
- `System.Text.Json` (.NET 6.0): Plugin.cs translation map loading — already in use, no new DLL dependency

### Expected Features

The ink tree parser is the critical path dependency. Nothing downstream is possible until block boundaries are correct. Content-type routing (5 types: dialogue, spells, UI, items, system) determines batch format and prompt strategy; dialogue is the dominant type (71,787 of 77,816 items). Tag handling is the second major quality concern: the 2-stage approach is non-negotiable given v1's failure data.

See `.planning/research/FEATURES.md` for dependency graph, priority matrix, and competitor comparison.

**Must have (table stakes — pipeline fails without these):**
- Ink JSON tree parser with dialogue block extraction (block-level merging, branch/choice structure preservation)
- Scene-unit cluster translation (gpt-5.4, script format, 10-30 lines per batch)
- Source-to-ID line mapping (numbered-line format with `[NN]` markers, strict count validation)
- Tag restoration via codex-mini (2-stage: translate tag-free, restore tags separately)
- Tag count validation (final quality gate before patch output)
- Patch output generation (translations.json + textassets/ + localizationtexts/)
- Plugin.cs matching update (remove `TryTranslateTagSeparatedSegments`, optimize for block-level exact match)
- DB pipeline state with resumability (77K items cannot run in one session)
- Source deduplication (EXISTS check on source_raw before INSERT — explicit project constraint)

**Should have (quality layer — add after core pipeline produces correct output):**
- Glossary/terminology consistency injection (after identifying inconsistencies in first batch)
- Content-type-specific prompt templates beyond dialogue (after dialogue pipeline proven)
- Register-appropriate choice text enforcement at prompt level
- Translation quality scoring and retranslation loop (after full initial pass)
- Passthrough detection optimization (after measuring LLM call waste)

**Defer (post-v2):**
- Multi-game abstraction (only if second game project starts)
- Game version delta tracking (game pinned at 1.1.3)
- Real-time in-game translation (6-8s LLM latency makes this unusable; static patch is zero-latency)
- Web UI / dashboard (CLI + DB queries are faster for this scale)

### Architecture Approach

The architecture has five sequential layers with clear interfaces: Source Extraction (ink JSON -> dialogue block JSON files on filesystem), Translation (PostgreSQL state machine driving 2-stage LLM workers), Patch Output (PostgreSQL done items -> translations.json + textassets), and Game Runtime (BepInEx plugin loading patch artifacts). The key boundary insight is that the ink parser writes to the filesystem (decoupled, run once per game version) while the translation stages communicate through PostgreSQL state (tightly coupled via lease-based state machine). This allows the parser to be independently validated before translation begins.

The v2 code fits entirely within the existing layered architecture. New packages are: `workflow/internal/inkparse/` (pure data transformation, no LLM/DB), `workflow/internal/clustertranslate/`, `workflow/internal/tagformat/`, and `workflow/cmd/` entries for each new pipeline command. No structural reorganization needed.

See `.planning/research/ARCHITECTURE.md` for component diagram, data flow, state machine spec, and build order.

**Major components:**
1. **Ink Tree Parser** (`inkparse` package) — Walk ink JSON, merge `"^text"` entries into dialogue blocks, preserve knot/gate/choice structure, emit content-type metadata
2. **Content Classifier** — Assign content type (dialogue/spell/UI/item/system) and batch grouping strategy; feeds into cluster translation
3. **Cluster Translator** — Build scene scripts from dialogue blocks, send to gpt-5.4 (tag-free), parse numbered-line output, map to source IDs
4. **Tag Formatter** — Send EN-with-tags + KO-without-tags to codex-mini, restore tags, validate exact tag string match
5. **Pipeline Orchestrator** — Lease-based DB state machine extended with `pending_format`/`working_format` states; items without tags skip format stage
6. **Patch Builder** — Read done items from PostgreSQL, generate translations.json (block-level keys), textassets/ (in-place `"^text"` replacement), localizationtexts/ CSV
7. **BepInEx Plugin** — Simplified 3-stage matching chain: Direct -> Decorated -> Contextual; RuntimeLexicon for display-only stat name substitution

### Critical Pitfalls

1. **Dialogue block boundary misidentification** — The v1 root cause. Concatenate all consecutive `"^"` entries until hitting a `"\n"`, divert, or control command. Validate with a test harness against known scenes. Build before any translation starts. Recovery cost is HIGH if wrong (full retranslation required).

2. **LLM output line count drift in cluster translation** — LLMs merge or split lines silently. Use explicit `[NN]` numbered markers in prompts and parse by extracting markers (not by splitting on newlines). Hard reject batches where output count differs from input; retry max 2x. Keep clusters to 10-20 lines; beyond 30, drift probability rises sharply.

3. **Tag corruption in the formatter LLM stage** — Even codex-mini mutates attribute quotes and tag structure subtly. Validate with exact string match on each tag (not just count). For common patterns (fewer than 50 unique tag patterns expected), consider deterministic code restoration instead of LLM. Design the tag extraction/injection protocol before implementing the formatter prompt.

4. **Source hash deduplication failure** — v1 used `len(text)` as a hash, causing silent collision at 77K+ scale. Replace with SHA-256 before any v2 pipeline run. Use a fresh checkpoint DB (archive v1 DB separately) to avoid cross-contamination with fragment-level v1 data.

5. **TextAsset structural corruption** — In-place ink JSON text injection must replace `"^text"` string values only, never add/remove array elements. Validate by walking both original and modified JSON trees and confirming identical container counts and control flow structure.

## Implications for Roadmap

Based on combined research, the phase structure is dictated by hard dependencies: the ink tree parser must be correct before any translation work begins, and the tag formatter stage cannot run until the cluster translator has produced `ko_raw` values. There is no beneficial reordering; the phases below reflect the build order documented in ARCHITECTURE.md.

### Phase 0: Pipeline Preparation

**Rationale:** The v1 checkpoint database used `len(text)` as a source hash, which causes silent deduplication collisions at 77K+ scale. This must be fixed before a single v2 translation item is inserted, otherwise the pipeline will silently report items as done when they have not been translated.
**Delivers:** Fresh PostgreSQL pipeline DB with SHA-256 source hashing, archived v1 DB, pre-flight collision check script.
**Addresses:** Source deduplication constraint (project rule), Pitfall 6 (source hash dedup failure).
**Avoids:** Silent pipeline corruption that would require identifying and re-running thousands of affected items mid-translation.

### Phase 1: Ink Tree Parser

**Rationale:** The entire pipeline depends on correct dialogue block boundaries. This is the only component with HIGH recovery cost if wrong. It is also pure Go with no LLM or DB dependencies, making it fully testable in isolation.
**Delivers:** `inkparse` package producing dialogue block JSON files from 286 TextAsset ink JSON files; content-type metadata per block; test harness comparing output against plugin capture logs.
**Addresses:** Ink JSON tree parser (P1 table stakes), content-type classification, source deduplication (block-level source_raw).
**Avoids:** Pitfall 1 (block boundary misidentification), Pitfall 4 (branch/choice structure loss), Pitfall 7 (content type conflation at classification stage).
**Note:** Needs research-phase for ChoicePoint flag bitfield semantics before implementation.

### Phase 2: Cluster Translation Domain

**Rationale:** With correct source blocks available, the cluster translation stage can be built and validated independently. Line count mapping is the highest-risk part of this stage and must be proven with realistic test cases before bulk translation runs.
**Delivers:** `clustertranslate` package; `go-cluster-translate` command; scene-script prompt templates; numbered-line output parser; `ko_raw` values stored in pipeline_items.
**Addresses:** Scene-unit cluster translation (P1 table stakes), source-to-ID mapping (P1), branch-aware context (P2), content-type-aware batching.
**Uses:** gpt-5.4 via OpenCode, pgx/v5, existing `platform/llm_client.go`.
**Avoids:** Pitfall 2 (LLM line count drift — mitigated by `[NN]` markers and count validation), Pitfall 3 (tag corruption — by stripping tags before sending).

### Phase 3: Tag Formatter Domain

**Rationale:** The 2-stage separation is the validated fix for v1's 99.7% tag-related failure rate. This stage processes only the ~40% of items that carry rich-text tags; items without tags skip directly to validation.
**Delivers:** `tagformat` package; `go-tag-format` command; tag extraction/injection protocol; codex-mini prompt; exact tag string match validation; `ko_formatted` values in pipeline_items.
**Addresses:** Tag restoration (P1 table stakes), tag count validation (P1).
**Uses:** codex-mini via OpenCode, existing `postprocess_validation.go` patterns.
**Avoids:** Pitfall 3 (tag corruption — design extraction/injection protocol first, use exact-string validation not just count).

### Phase 4: Pipeline Integration and Patch Output

**Rationale:** With all three domain packages proven in isolation, wire them into the full end-to-end flow: extend the state machine with format stage states, run a complete 77K-item pass, and generate the patch artifacts from done items.
**Delivers:** Extended `translationpipeline` with `pending_format`/`working_format` states; full pipeline run producing 65K+ done items; `go-esoteric-apply-out` adapted for block-level output (translations.json, textassets/, localizationtexts/).
**Addresses:** Patch output generation (P1 table stakes), DB pipeline state with resumability (P1), pipeline integration.
**Avoids:** Pitfall 8 (TextAsset structural corruption — in-place replace only, structural diff validation after injection).

### Phase 5: Plugin Optimization and Quality Layer

**Rationale:** With a complete patch available, the plugin matching chain can be simplified and tested against real game data. Quality layer features (glossary, register enforcement, quality scoring) require seeing the first full translation output to identify where they are actually needed.
**Delivers:** Simplified Plugin.cs matching chain (Direct -> Decorated -> Contextual, remove `TryTranslateTagSeparatedSegments`); glossary injection; register prompt enforcement for choice text; translation quality scoring; in-game testing.
**Addresses:** Plugin.cs matching update (P1 table stakes), glossary/terminology consistency (P2), register-appropriate choices (P2), quality scoring (P2).
**Avoids:** Pitfall 5 (plugin fallback cascade — block-level matching targets 95%+ exact hit rate, NormalizedMap collision logging added).

### Phase Ordering Rationale

- **Phase 0 before everything:** The source hash bug is a silent data corruption issue. Running any translation before fixing it risks polluting the checkpoint DB with incorrect done-state records.
- **Phase 1 before Phase 2:** Block boundaries must be validated before translation begins. Wrong boundaries propagate silently and require full retranslation to fix.
- **Phase 2 before Phase 3:** The formatter LLM needs `ko_raw` (translated, tag-free Korean) as input. It cannot run without Phase 2 output.
- **Phase 3 before Phase 4 integration:** Pipeline integration requires all states to be defined and their workers to exist. Cannot wire the state machine until both translation and formatter commands exist.
- **Phase 5 last:** Quality layer improvements require seeing actual translation output to know where they are needed. Plugin simplification requires a patch in the new block-level format to test against.

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 1:** ChoicePoint flag bitfield semantics (`"flg"` field on `"*"` nodes) are not fully documented in the ink spec. Requires reading ink C# runtime source to decode all flag combinations correctly. Also: glue mechanics (`<>` in ink source compiles to what JSON structure?) needs spec verification.
- **Phase 3:** Optimal tag extraction/injection protocol needs design before implementation. Should determine whether a tag registry (enumerate all ~50 unique patterns, restore deterministically) is better than LLM-based restoration for common patterns.

Phases with standard patterns (skip research-phase):
- **Phase 0:** Straightforward SQL schema change and hash function swap. No unknowns.
- **Phase 2:** Cluster translation prompt design is validated by v2 experiments. Standard Go LLM client patterns apply. No additional research needed.
- **Phase 4:** Pipeline state machine extension follows established v1 patterns. Patch output format is unchanged from v1 (translations.json format is known and works).
- **Phase 5:** Plugin chain simplification is a known change. Quality scoring reuses existing `go-evaluate` and `go-semantic-review` infrastructure.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Building on validated v1 stack. No new external dependencies. All new components use Go stdlib + existing packages. |
| Features | HIGH | Table stakes features directly validated by v1 failure analysis (99.7% tag failure rate, block boundary root cause). Quality layer features are well-understood but not validated by experiment yet. |
| Architecture | HIGH | Component boundaries, data flow, and state machine design all derived from existing codebase analysis. 5-phase build order matches hard dependency graph. |
| Pitfalls | HIGH | Pitfalls 1, 2, 3, 5, 6 are from direct v1 post-mortem. Pitfalls 4, 7, 8 are from ink spec analysis. No speculative pitfalls. |

**Overall confidence:** HIGH

### Gaps to Address

- **ChoicePoint flag bitfield:** The `"flg"` field on ink JSON ChoicePoint nodes uses undocumented bit combinations. Must read ink C# runtime source (`Ink.Parsed.Choice`, `Ink.Runtime.ChoicePoint`) during Phase 1 planning to build a correct flag decoder. Without this, choice text vs. dialogue text vs. conditional text will be misclassified.

- **Glue mechanics in compiled ink JSON:** Ink's glue `<>` operator (which prevents implicit newlines between text runs) compiles to an unknown JSON structure. This may affect block boundary detection in scenes that use glue. Verify against ink runtime spec before finalizing the tree walker.

- **Content-type-specific prompt templates (non-dialogue):** Prompt templates for spells, UI labels, items, and system text are not yet designed. This is deferred to Phase 5 and does not block the core dialogue pipeline, but needs attention before claiming complete v2 coverage.

- **Tag registry composition:** The claim that there are "fewer than 50 unique tag patterns" in the corpus is an estimate. Should enumerate all actual `<b>`, `<color>`, `<link>` patterns from the source corpus during Phase 3 to confirm whether deterministic restoration is viable.

## Sources

### Primary (HIGH confidence)

- Existing v1 codebase analysis (`Plugin.cs`, `CONCERNS.md`, `workflow/internal/`, `go.mod`) — component boundaries, pitfall root causes, stack validation
- v2 experiment results (project memory `project_v2_pipeline_context.md`) — cluster translation line mapping (8/8, 16/16), tag restoration (4/4), validated by user
- [Ink JSON Runtime Format Specification](https://github.com/inkle/ink/blob/master/Documentation/ink_JSON_runtime_format.md) — container structure, text entry format, choice/branch mechanics
- [BepInEx Releases](https://github.com/bepinex/bepinex/releases) / [Bleeding Edge Builds](https://builds.bepinex.dev/projects/bepinex_be) — BepInEx 6 .NET 6 target confirmed
- [Go encoding/json v2 status](https://go.dev/blog/jsonv2-exp) + [memory bug](https://github.com/golang/go/issues/75026) — confirmed not production-ready

### Secondary (MEDIUM confidence)

- [Dink Pipeline](https://wildwinter.medium.com/dink-a-dialogue-pipeline-for-ink-5020894752ee) — script-like format for ink dialogue, informs cluster prompt design
- [LLM Structured Output for Translation](https://flounder.dev/posts/structured-output-for-translation/) — batch translation patterns with structured output
- [Ink Localization Discussion #529](https://github.com/inkle/ink/issues/529) — real-world ink localization in 7 languages (community patterns)
- [BallonsTranslator line count drift issue](https://github.com/dmMaze/BallonsTranslator/issues/861) — confirms LLM batch translation line count drift is a known problem
- [Translator++ batch algorithm](https://dreamsavior.net/translator-ver-7-10-27-better-algorithm-for-local-llm/) — JSON cloaking approach for structured output

### Tertiary (LOW confidence — needs validation during implementation)

- Estimate of "fewer than 50 unique tag patterns" in corpus — verify by enumerating actual source data during Phase 3
- Claim that ~40% of items carry rich-text tags — estimate from v1 data; verify after Phase 1 extraction completes

---
*Research completed: 2026-03-22*
*Ready for roadmap: yes*
