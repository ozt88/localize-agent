# Feature Research

**Domain:** LLM-based game localization pipeline (Ink narrative engine, Korean target)
**Researched:** 2026-03-22
**Confidence:** HIGH

## Feature Landscape

### Table Stakes (Pipeline Fails Without These)

Features that are non-negotiable -- without them the v2 pipeline cannot produce a working Korean patch.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Ink JSON tree parser (dialogue block extraction) | v1's fundamental failure was splitting `"^text"` entries individually instead of merging them into the blocks the game engine actually renders. Parser must walk `root[-1]` knots, merge consecutive `"^"` entries, and preserve `g-N`/`c-N` branch structure. Without this, nothing downstream works. | HIGH | 286 TextAsset files, 71,787 contextual entries. Must handle gates, choices, glue markers, tags (`#speaker`, `#DC_check`). This is the hardest single component. |
| Scene-unit cluster translation (LLM stage 1) | Context-preserving translation requires sending 10-30 lines of dialogue together as a "script" rather than individual strings. Experiments confirmed 8/8 perfect mapping with cluster approach vs. constant context loss with per-item translation. | MEDIUM | Uses gpt-5.4 via OpenCode. Tags stripped before sending. Output must be line-separable back to source IDs. Branch markers (BRANCH/OPTION) must survive round-trip. |
| Tag restoration (LLM stage 2, formatter) | LLMs reliably mangle rich text tags (`<b>`, `<color=#hex>`, `<link="N">`) during translation. v1 had 99.7% of persist-skip failures caused by tag validation, not translation quality. Separating formatting from translation is the proven fix. | MEDIUM | Uses codex-mini. Only processes lines that had tags in source. Experiment showed 4/4 perfect tag count match. Must handle nested tags. |
| Tag count validation (post-formatter check) | Final gate ensuring tag counts in translated output match source. Without this, bold/color leaks into subsequent dialogue lines in-game. v1's tag leakage was the most user-visible defect. | LOW | Already partially exists in v1 (`postprocess_validation.go`). v2 needs simpler version: count open/close tags, match to source counts. |
| Content-type-aware batching | Different content types need different prompt formats and batch strategies. Dialogue needs script format (10-30 lines), UI labels need dictionary format (50-100), spells need structured cards (5-10). One-size-fits-all produces poor translations for non-dialogue content. | MEDIUM | 5 content types: dialogue/narration/reaction/choice, spells/tooltips, UI labels, item/quest descriptions, system/tutorial. Each needs its own prompt template and grouping logic. |
| Source-to-ID mapping (line separation) | After cluster translation, each translated line must map back to its original source ID for patch generation. Misalignment here means wrong translations appear in wrong places. | MEDIUM | Cluster output is free-form script. Need deterministic line separation that handles Korean line-break differences from English. Branch structure markers help anchoring. |
| Patch output generation (translations.json + textassets) | Must produce BepInEx TranslationLoader-compatible output: `translations.json` (source-to-target sidecar), `textassets/` (ink JSON with Korean inserted), `localizationtexts/` (CSV format). Without correct output format, the plugin cannot load translations. | MEDIUM | v1 format exists and works. v2 must generate same output structure but from dialogue-block-unit sources instead of fragment-unit sources. |
| Plugin.cs matching logic update | v2 changes source unit from fragments to dialogue blocks. Plugin's fallback chain must be updated: direct match becomes primary, `TryTranslateTagSeparatedSegments` (v1's problem child) gets removed or demoted. Without this, correctly generated patches still won't match at runtime. | MEDIUM | C# code in BepInEx plugin. Matching chain has 7 levels; v2 should make level 1 (direct map) cover 95%+ of cases, simplifying the chain. |
| DB-driven pipeline state (resumability) | 77,816 items cannot be translated in one session. Pipeline must track per-item state (`pending` -> `working` -> `done`/`failed`), support resume, handle crashes gracefully. | LOW | Already exists in v1 (`pipeline_items` table, lease-based claims). Reuse with v2-appropriate states. |
| Source deduplication (source_raw check) | Explicit project constraint: never blindly INSERT items. Must check `source_raw` for existing entries before ingestion. Prevents duplicate translations and wasted LLM calls. | LOW | Existing pattern from v1. Simple EXISTS check before INSERT. |

### Differentiators (Quality Improvement Over v1)

Features that elevate translation quality beyond "it works" to "it reads naturally."

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Branch-aware translation context | Ink games have branching dialogue where tone, register, and vocabulary must stay consistent across branches. Sending branch structure (BRANCH/OPTION markers) to the translator LLM preserves narrative coherence that per-line translation destroys. | LOW | Already validated in experiment (Snell_Companion 16-line branch test). Minimal code cost -- just include branch markers in cluster script. |
| Glossary/terminology consistency | Game-specific terms (character names, spell names, faction names, stat names like INTELLIGENCE/CHARISMA) must be translated consistently across 77K+ entries. Inconsistent terminology breaks immersion and confuses players. | MEDIUM | Inject glossary as LLM context/system prompt. Requires building term list from game data first. Korean gaming conventions exist for stat names (e.g., INTELLIGENCE -> "지능"). |
| Register-appropriate choice text | v1 validation already catches polite register in choices (`하세요/합니다` endings). v2 should enforce this at the prompt level, not just validation. Player dialogue choices should use casual/direct register while NPC narration uses appropriate formality. | LOW | Prompt engineering in cluster translation templates. Content-type metadata already distinguishes choices from narration. |
| Passthrough detection for non-translatable content | Some strings are code identifiers, variable references, or formulaic game mechanics (`.StatName>=5-`, `SPELL FireBolt-`) that should pass through unchanged. Detecting these saves LLM calls and prevents mangling. | LOW | v1 has `isLiteralPassthroughSource` and `passthroughControlRe`. Carry forward and extend for v2 source unit format. |
| Structural token preservation ($var, {template}) | Game strings contain structural tokens (`$playerName`, `{statValue}`) that must survive translation intact. v1 has `tokenCompatible()` validation. v2 should separate this from tag handling for cleaner validation. | LOW | Existing v1 code handles this. Minimal changes needed for v2. |
| Per-content-type LLM prompt optimization | Different content types benefit from different prompt strategies: dialogue benefits from "you are a script translator" framing, UI labels from "be terse, match character limits", items from "preserve game-mechanical accuracy." | MEDIUM | Requires 5 prompt templates. Can iterate after initial pipeline works. Not blocking for first pass. |
| Translation quality scoring (post-pipeline) | v1 had evaluation pipeline with score threshold and retranslation loop. v2 should retain this for quality assurance but can defer to after initial full translation pass. | MEDIUM | Existing `go-evaluate` and `go-semantic-review` commands. Reuse infrastructure. |
| Progress tracking and metrics | Visibility into 77K-item pipeline progress: items completed, failed, average latency, token usage. Essential for managing multi-day translation runs. | LOW | v1 has `MetricCollector` and JSONL trace. Reuse. |

### Anti-Features (Deliberately NOT Building)

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Real-time in-game translation (XUnity.AutoTranslator style) | Seems like the "modern" approach -- translate on-the-fly as player encounters text | Adds 6-8 second latency per dialogue line (LLM round-trip). Requires always-on LLM server during gameplay. Network dependency during play. Quality cannot be reviewed before player sees it. | Pre-translate all 77K entries offline, ship as static patch. Zero runtime latency. |
| Web UI / dashboard for translation management | Visual interface for reviewing translations, managing pipeline state | Solo developer project. CLI + DB queries are faster for power users. Building UI is weeks of work that doesn't improve translation quality. | PostgreSQL queries + CLI status commands + JSONL trace logs. |
| Multi-language support | "Why not support Japanese/Chinese/etc. too?" | Fundamentally different prompt engineering per target language. Korean-specific validation (register checking, Hangul detection). Scope explosion without user demand. | Korean-only. Architecture allows future languages but don't build the abstraction now. |
| Automatic game version tracking | Auto-detect game updates and re-extract sources | Game version is pinned at 1.1.3. Over-engineering for a problem that doesn't exist yet. | Manual re-extraction if game updates (out of scope per PROJECT.md). |
| Human-in-the-loop review workflow | Built-in review/approval interface for each translation | Solo project. Review happens by playing the game, not through a review tool. Adds workflow complexity without proportional quality gain for this project scale. | Spot-check by playing game sections. Use `go-semantic-review` for automated oddness detection. |
| Incremental/delta translation | Only translate changed strings when source updates | Source is stable (game version 1.1.3). Full retranslation is the explicit v2 strategy. Delta translation adds complexity for source diffing that isn't needed. | Full retranslation of all 77,816 items. v1 results explicitly discarded. |
| MT (machine translation) fallback (Google/DeepL) | Use cheaper MT for "easy" strings, LLM for hard ones | Consistency nightmare -- two different translation engines produce different tonal quality. Glossary/context handling differs. Debugging quality issues requires knowing which engine produced each line. | Single LLM (gpt-5.4) for all translation. codex-mini only for tag formatting, not translation. |

## Feature Dependencies

```
[Ink JSON Tree Parser]
    |
    +--requires--> [Content-Type Classification]
    |                  |
    |                  +--feeds--> [Content-Type-Aware Batching]
    |                                  |
    |                                  +--feeds--> [Scene-Unit Cluster Translation]
    |                                                  |
    |                                                  +--requires--> [Source-to-ID Mapping]
    |                                                  |                  |
    |                                                  |                  +--feeds--> [Tag Restoration]
    |                                                  |                                  |
    |                                                  |                                  +--feeds--> [Tag Count Validation]
    |                                                  |                                                  |
    |                                                  +--feeds--> [Patch Output Generation]  <-----------+
    |                                                                  |
    |                                                                  +--feeds--> [Plugin.cs Matching Update]
    |
    +--parallel--> [Glossary Building] --enhances--> [Cluster Translation]
    +--parallel--> [Passthrough Detection] --filters--> [Cluster Translation]

[DB Pipeline State] --enables--> [All translation stages] (resumability)
[Source Deduplication] --gates--> [DB Pipeline State] (ingestion)
```

### Dependency Notes

- **Ink JSON Tree Parser is the critical path.** Everything downstream depends on correctly parsed dialogue blocks. This must be built and validated first.
- **Content-Type Classification feeds Batching.** Parser must emit content type metadata (dialogue vs. UI vs. spell etc.) so batching logic can group correctly.
- **Tag Restoration depends on Source-to-ID Mapping.** Must know which translated lines had tags in their source to know which lines to send to the formatter LLM.
- **Plugin.cs update depends on Patch Output Generation.** Can only test plugin matching after patches exist in the new format.
- **Glossary and Passthrough Detection are parallel.** Both enhance cluster translation quality but neither blocks the core pipeline. Can be added incrementally.
- **DB Pipeline State is infrastructure.** Largely reused from v1, enables all stages but doesn't block initial development/testing of individual stages.

## MVP Definition

### Launch With (Core Pipeline)

Minimum set to produce a working Korean patch from v2 architecture:

- [ ] Ink JSON tree parser with dialogue block extraction -- the foundation everything depends on
- [ ] Scene-unit cluster translation (gpt-5.4, script format) -- core translation capability
- [ ] Source-to-ID line mapping -- connects cluster output back to individual entries
- [ ] Tag restoration via codex-mini -- solves v1's critical tag leakage problem
- [ ] Tag count validation -- final quality gate
- [ ] Patch output generation (translations.json + textassets) -- produces installable patch
- [ ] Plugin.cs matching update (remove TryTranslateTagSeparatedSegments) -- runtime compatibility
- [ ] DB pipeline state with resumability -- operational necessity for 77K items
- [ ] Source deduplication -- project constraint

### Add After Core Works (Quality Layer)

Features to add once the pipeline produces correct output end-to-end:

- [ ] Glossary/terminology injection -- after seeing first batch results and identifying inconsistencies
- [ ] Content-type-specific prompt templates (beyond dialogue) -- after dialogue pipeline is proven
- [ ] Register enforcement in prompts -- after reviewing choice text quality
- [ ] Translation quality scoring/retranslation loop -- after full initial pass completes
- [ ] Passthrough detection optimization -- after seeing which items waste LLM calls

### Future Consideration (Post-v2)

- [ ] Multi-game support abstraction -- only if a second game project starts
- [ ] Game version delta tracking -- only if Esoteric Ebb releases an update
- [ ] Automated in-game screenshot comparison -- valuable but high complexity

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Ink JSON tree parser | HIGH | HIGH | P1 |
| Scene-unit cluster translation | HIGH | MEDIUM | P1 |
| Source-to-ID mapping | HIGH | MEDIUM | P1 |
| Tag restoration (formatter LLM) | HIGH | MEDIUM | P1 |
| Tag count validation | HIGH | LOW | P1 |
| Patch output generation | HIGH | MEDIUM | P1 |
| Plugin.cs matching update | HIGH | MEDIUM | P1 |
| DB pipeline state | HIGH | LOW | P1 |
| Source deduplication | MEDIUM | LOW | P1 |
| Content-type-aware batching | MEDIUM | MEDIUM | P1 |
| Glossary/terminology consistency | MEDIUM | MEDIUM | P2 |
| Branch-aware context | MEDIUM | LOW | P2 |
| Register-appropriate choices | MEDIUM | LOW | P2 |
| Per-content-type prompts | MEDIUM | MEDIUM | P2 |
| Passthrough detection | LOW | LOW | P2 |
| Quality scoring/retranslation | MEDIUM | MEDIUM | P2 |
| Progress metrics | LOW | LOW | P2 |
| Structural token preservation | MEDIUM | LOW | P2 |

**Priority key:**
- P1: Must have for working pipeline (core v2 architecture)
- P2: Should have, add after core pipeline produces correct output
- P3: Not applicable for v2 scope (deferred items are in "Future Consideration")

## Competitor/Reference Analysis

| Feature | XUnity.AutoTranslator | Commercial TMS (Crowdin/Lokalise) | This Pipeline |
|---------|----------------------|-----------------------------------|---------------|
| Translation method | Real-time API calls during gameplay | Human + MT/LLM hybrid | Offline LLM batch (gpt-5.4) |
| Context awareness | None (single string) | TM leverage, glossary | Scene-unit clusters with branch structure |
| Tag handling | Passthrough (no translation of tagged content) | Platform-managed placeholders | 2-stage: strip for translation, restore with formatter LLM |
| Quality assurance | None | Human review workflow | Automated tag validation + semantic scoring |
| Ink support | Generic Unity string hooks | No Ink awareness | Native Ink JSON tree parsing |
| Patch delivery | Runtime hook (latency) | Export files | Static patch (translations.json sidecar) |
| Batch size | 1 string at a time | Varies | 10-30 lines per cluster (dialogue), 50-100 (UI) |

## Sources

- [Ink localization discussion (inkle/ink #98)](https://github.com/inkle/ink/issues/98) -- Ink is not designed for localization; text fragment stitching is a known problem
- [Ink localization tips (inkle/ink #89)](https://github.com/inkle/ink/issues/89) -- Community approaches to Ink translation
- [Ink Localisation Tool by wildwinter](https://github.com/wildwinter/Ink-Localiser) -- String extraction and ID assignment utility
- [Gridly: AI translation in game localization](https://www.gridly.com/blog/ai-translation-game-localization/) -- Modern AI pipeline integration patterns
- [XUnity.AutoTranslator](https://github.com/bbepis/XUnity.AutoTranslator) -- Reference for BepInEx-based translation approach
- [Localization QA guide (LocalizeDirect)](https://www.localizedirect.com/posts/lqa-what-is-game-localization-testing) -- QA testing patterns for game localization
- v1 codebase: `postprocess_validation.go`, `proposal_validation.go` -- Existing validation patterns carried forward

---
*Feature research for: LLM-based Ink game localization pipeline (Esoteric Ebb Korean)*
*Researched: 2026-03-22*
