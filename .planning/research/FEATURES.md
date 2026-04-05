# Feature Research: Context-Aware Retranslation

**Domain:** Game localization pipeline quality improvement (Ink-based cRPG, Korean)
**Researched:** 2026-04-06
**Confidence:** HIGH (grounded in existing codebase analysis + domain research)

## Feature Landscape

### Table Stakes (Must Have for v1.1 Quality Improvement)

Features without which v1.1 delivers no meaningful improvement over v1.0.

| Feature | Why Expected | Complexity | Pipeline Dependency | Notes |
|---------|--------------|------------|---------------------|-------|
| **Prompt audit & restructuring** | Current prompts already pass `speaker_hint`, `context_en`, `prev_en/next_en`, `lore_context`, `text_role` but rules are a flat numbered list (24 rules in `defaultStaticRules()`) appended inline. Restructuring into a clearer hierarchy directly improves every translation without new data extraction. | LOW | `skill.go`, `prompts.go`, `normalized_input.go` | Highest ROI, lowest risk. `v2_base_prompt.md` has detailed ability-score voice guides (wis/str/int/cha/dex/con with Korean examples) -- these are only in warmup context, not integrated into per-item prompts when `speaker_hint` matches an ability score. |
| **Selective retranslation** | Without this, every quality improvement requires re-running 40K items. The pipeline already has `StatePendingRetranslate`, `ScoreFinal` per item, and threshold-based routing (`Config.Threshold`). Must expose score-based filtering to target only low-quality items. | LOW | `translationpipeline/types.go` (states exist), `PipelineItem.ScoreFinal`, existing retry package format in `go-esoteric-adapt-in` | Infrastructure 80% exists. Need: CLI to query items below score threshold + re-queue them as `pending_retranslate`. Retry package format already supports `retry_reason`, `existing_target`, `context_en`. |
| **Speaker extraction improvement** | `speaker_hint` field exists throughout the pipeline (`translationTask`, `normalizedPromptInput`, `lineContext`, `checkpointPromptMeta`) but is partially populated. Ink JSON uses `#` tags for speaker names. `formatContextLine()` already prefixes `Speaker: text` in context when hint is present. Without reliable speaker info, tone consistency is impossible. | MEDIUM | `translator_package.json` generation (ink parser), `build_context_clusters.py`, `batch_builder.go` `formatContextLine()` | Esoteric Ebb uses ability score speakers (wis, str, int, cha, dex, con) + NPC names (Braxo, Snell, etc.). Must audit actual coverage gap in existing `translator_package.json` data. |

### Differentiators (Significant Quality Uplift)

Features that meaningfully improve translation quality beyond baseline expectations.

| Feature | Value Proposition | Complexity | Pipeline Dependency | Notes |
|---------|-------------------|------------|---------------------|-------|
| **Branch context injection** | Ink scripts use `c-N` (choice) branches within `g-N` gates. When a player picks option 2, subsequent dialogue is in a branch context. Currently, `ContextEN` is the chunk (10-30 lines from the same segment), and `chunkContext` groups lines, but the branch relationship (which choice leads to this dialogue) is flattened. Injecting "this follows player choosing X" gives the LLM crucial disambiguation context. | HIGH | Ink JSON tree structure (`g-N`/`c-N` gates), `chunkContext` in `types.go`, `chunkPromptLines()` in `batch_builder.go`, `ChoiceBlock` field already in `translationTask` | Requires tracing branch paths in ink tree. `translationTask.ChoiceBlock` and `checkpointPromptMeta.ChoiceBlockID` exist but are empty strings -- infrastructure is there but data is not populated. |
| **Tone consistency profiles** | Same character sounds different across scenes because each batch is translated independently. A per-character "voice card" (speech register, typical sentence endings, personality keywords) injected into prompts ensures consistent characterization. | MEDIUM | `lore.go` pattern (term-bank matching), `skill.go` warmup, `v2_base_prompt.md` ability score voice guides | Ability score voices already have detailed Korean guides in `v2_base_prompt.md` (e.g., wis: "침착하고 달관한 어조, 짧은 사실 진술, 2인칭 관찰"). NPC voices need similar treatment. Can reuse `loreEntry` struct pattern -- a "character voice bank" alongside the lore termbank. |
| **Continuity-aware prompt windows** | Current prev/next is 1 line each (`neighborPromptText()` with delta -1/+1). For quality, the LLM needs a sliding window of 3-5 surrounding lines with their Korean translations to maintain flow. `chunkContext` already groups lines -- expand to include translated neighbors. | MEDIUM | `batch_builder.go` `neighborPromptText()`, `chunkPromptLines()`, `prevKO`/`nextKO` fields in `translationTask` | Infrastructure exists but is underutilized. `prevKO`/`nextKO` are only populated when `UseCheckpointCurrent=true` or on retry. For retranslation, surrounding lines already have Korean in DB -- should always populate. |

### Anti-Features (Commonly Considered, Often Problematic)

Features that seem valuable but would hurt more than help in this context.

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| **Full scene retranslation** | "Just retranslate everything with better prompts" | 40K items at ~6-8s each = ~75 hours of LLM time. Most translations already good (avg score 90.7/100). Wastes compute and risks regressing good translations. | Selective retranslation: target items below score threshold (e.g., <85). Score data already exists in `ScoreFinal`. |
| **Multi-pass consensus translation** | Generate N translations, pick best by scoring | 3x-5x cost multiplication. 2-stage architecture already provides quality control. Adding passes yields diminishing returns. | Invest in better single-pass prompts with richer context. Score LLM already validates output quality. |
| **Character relationship graph** | Build full social graph of who knows whom for context | Over-engineering. Ink scripts don't encode relationships explicitly. Graph construction would require manual annotation of 286 TextAsset files. | Per-character voice cards + `scene_hint` (already populated) provide sufficient relationship context. |
| **Automated tone detection via embedding similarity** | Use `sentence-transformers` to detect Korean tone drift | Tone is subjective and culturally loaded. Korean tone markers (honorifics, sentence endings like -요/-다/-네) don't map cleanly to multilingual embedding space. Existing `paraphrase-multilingual-MiniLM-L12-v2` model is too small for nuanced Korean tone. | Character voice cards with explicit Korean style guides + Score LLM with tone-checking criteria. |
| **Score model retraining/fine-tuning** | Fine-tune score model on human quality judgments | No labeled dataset of human quality judgments exists. Creating one requires playing through the game systematically. Fine-tuning local models adds infrastructure complexity. | Improve score prompt with tone-specific criteria. Defer model changes until after v1.1 ships and in-game review happens. |

## Feature Dependencies

```
[Prompt Audit & Restructuring]
    |
    +--enables--> [Tone Consistency Profiles]
    |                 (voice cards injected via improved prompt structure)
    |
    +--enables--> [Branch Context Injection]
    |                 (branch info needs a clear prompt section to land in)
    |
    +--enables--> [Continuity-Aware Prompt Windows]
                      (expanded prev/next needs structured prompt layout)

[Speaker Extraction Improvement]
    |
    +--enables--> [Tone Consistency Profiles]
    |                 (can't apply character voice if speaker unknown)
    |
    +--enables--> [Branch Context Injection]
                      (speaker identity helps disambiguate branch context)

[Selective Retranslation]
    |
    +--requires--> [Score Data in DB]  (ALREADY EXISTS: PipelineItem.ScoreFinal)
    |
    +--uses-----> [All prompt improvements above]
                      (retranslation uses improved prompts)

[Continuity-Aware Prompt Windows]
    +--requires--> [Existing Korean translations in DB]  (ALREADY EXISTS: 35,030 done items)
```

### Dependency Notes

- **Prompt Audit enables everything else:** All other features inject data into prompts. If the 24-rule flat list in `defaultStaticRules()` is already confusing the LLM, adding more data makes it worse. Must restructure first.
- **Speaker Extraction enables Tone Consistency:** You cannot enforce "Braxo sounds like Braxo" if you don't know which lines are Braxo's. Coverage gap in `speaker_hint` must be assessed and filled first.
- **Selective Retranslation is the executor:** It consumes all other improvements. Without it, improved prompts require a full 75-hour re-run. With it, you target only the ~5-15% of items below the quality threshold.
- **Continuity Windows require existing translations:** For retranslation, surrounding lines already have Korean translations. This makes retranslation inherently more context-rich than initial translation.
- **Branch Context conflicts with fast iteration:** It requires ink tree traversal code which is complex. Should not block prompt improvements and selective retranslation.

## MVP Definition

### Phase 1: Foundation (Prompt Audit + Speaker Coverage + Selective Retranslation)

Minimum viable quality improvement -- the smallest set that produces measurable score gains.

- [ ] **Prompt audit & restructuring** -- Refactor `skill.go` `defaultStaticRules()` and `prompts.go` `buildBatchPrompt()` into hierarchical prompt with clear sections (context, voice, task, constraints). Integrate ability-score voice guides from `v2_base_prompt.md` into runtime prompts when `speaker_hint` matches an ability score, not just warmup.
- [ ] **Speaker extraction coverage audit** -- Analyze `translator_package.json` to measure coverage gap. Count dialogue lines with vs. without `speaker_hint`. Parse ink JSON `#` tags more aggressively if needed. Target: >90% speaker attribution for dialogue-role lines.
- [ ] **Selective retranslation MVP** -- CLI command or pipeline mode to query items by `ScoreFinal < threshold` + re-queue as `pending_retranslate`. Populate `prevKO`/`nextKO` from existing translations for retranslation items.

### Phase 2: Context Enrichment (after Phase 1 validates gains)

- [ ] **Branch context injection** -- Extract parent choice text from ink tree path. Add `branch_context` field to `translationTask`. Inject "Player chose: X" into prompt context section.
- [ ] **Tone consistency profiles** -- Build character voice bank JSON (reusing `loreEntry` pattern). Match by `speaker_hint`. Inject voice card into prompt when character detected.
- [ ] **Continuity-aware prompt windows** -- Expand from 1 prev/next to 3-line window. Always populate Korean references from DB during retranslation runs.

### Future Consideration (v1.2+)

- [ ] **Score LLM prompt improvement** -- Add tone-consistency and context-coherence criteria to scoring prompts. Defer because scoring changes invalidate existing ScoreFinal values.
- [ ] **Cross-scene glossary expansion** -- `glossary.go` works for exact matches. May need fuzzy matching or expanded term list after reviewing retranslation results.
- [ ] **Semantic review pipeline integration** -- `go-semantic-review` currently separate. Could feed its output directly into retranslation targets.

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority | Rationale |
|---------|------------|---------------------|----------|-----------|
| Prompt audit & restructuring | HIGH | LOW | **P1** | Highest ROI. Improves every retranslated item. No new data needed. |
| Selective retranslation MVP | HIGH | LOW | **P1** | Pipeline states already exist. Without this, no way to apply improvements efficiently. |
| Speaker extraction improvement | HIGH | MEDIUM | **P1** | Data quality gate for tone work. Must quantify gap before deciding effort. |
| Tone consistency profiles | HIGH | MEDIUM | **P2** | Big quality win but requires speaker data + restructured prompts. |
| Continuity-aware prompt windows | MEDIUM | MEDIUM | **P2** | Good for flow, infrastructure mostly exists. Less impactful than speaker/tone. |
| Branch context injection | MEDIUM | HIGH | **P2** | Valuable for choice-heavy scenes but complex ink tree traversal. Affects subset of items. |

**Priority key:**
- P1: Foundation for v1.1 -- must ship first, other features build on these
- P2: Quality enrichment -- add after P1 validates measurable gains
- P3: Future (not listed) -- defer until v1.1 in-game review

## Existing Pipeline Capabilities Inventory

What already exists, preventing wheel reinvention.

| Capability | Location | Current State | Gap for v1.1 |
|------------|----------|---------------|-------------|
| Speaker hint field | `translationTask.SpeakerHint`, `normalizedPromptInput.SpeakerHint`, `lineContext.SpeakerHint` | Populated from `translator_package.json`. Used in `formatContextLine()` as `Speaker: text` prefix in context. | Coverage incomplete -- many dialogue lines have empty `speaker_hint`. |
| Context window | `ContextEN`, `ContextLines` (chunk 10-30 lines), `PrevEN/NextEN` (1 line), `PrevKO/NextKO` | Chunk context for dialogue. Korean refs only populated on retry or `UseCheckpointCurrent`. | Need wider Korean window + always-populate for retranslation. |
| Lore injection | `lore.go`, `esoteric_ebb_lore_termbank.json` | Term-bank matched by keyword. Up to `LoreMaxHints` per item. `formatLoreHints()` produces compact string. | Pattern reusable for character voice cards. |
| Ability score voices | `v2_base_prompt.md` lines 29-56 | Detailed Korean voice guides: register, sentence patterns, example translations for each of 6 ability scores. | Only in warmup context. Not dynamically injected when `speaker_hint` is `wis`/`str`/`int`/`cha`/`dex`/`con`. |
| Glossary | `glossary.go`, `matchedGlossaryEntries()` | Mandatory terminology enforcement with `translate`/`preserve` modes. | Working well. No change needed for v1.1. |
| Score threshold | `translationpipeline.Config.Threshold` | `float64` field used during pipeline scoring for pass/fail. `PipelineItem.ScoreFinal` persisted per item. | Need CLI to query items below threshold + re-queue. |
| Retranslation state machine | `StatePendingRetranslate`, `StateWorkingRetranslate` in `translationpipeline/types.go` | Full state transitions defined. Pipeline workers handle retranslation role. | Need to populate retranslation queue from score query. |
| Retry package format | `retryPackageItem` in `go-esoteric-adapt-in` | Supports `retry_reason`, `existing_target`, `context_en`, `speaker_hint`, `top_candidates`. | Can serve as data format for selective retranslation input. |
| Choice block ID | `translationTask.ChoiceBlock`, `checkpointPromptMeta.ChoiceBlockID` | Field exists in types. Present in pack_json. | Currently empty strings -- data not populated from ink tree. |
| Fragment/structure hints | `deriveFragmentHints()`, `deriveStructureHints()` in `normalized_input.go` | Regex-based detection of action-quote, open-quote, definition patterns. | Working well for structural disambiguation. No change needed. |
| Focused context | `buildFocusedContextEN()` in `normalized_input.go` | Wraps target line in `[[BODY_EN]]...[[/BODY_EN]]` markers within context. | Good pattern. Retain for v1.1. |
| Text role classification | `effectiveTextProfile()`, `lineContext.TextRole` | `dialogue`, `narration`, `reaction`, `choice`, `ui_label`, `tooltip`, etc. | Robust classification. No change needed. |
| Overlay/UI translation | `overlayStaticRules()`, `newOverlayTranslateSkill()` | Separate simplified rules for context-free UI text. | Correct separation. UI items should not be retranslation targets. |

## Sources

- Codebase analysis: `workflow/internal/translation/` (skill.go, prompts.go, batch_builder.go, normalized_input.go, types.go, lore.go), `workflow/internal/translationpipeline/types.go`, `projects/esoteric-ebb/cmd/go-esoteric-adapt-in/main.go`, `projects/esoteric-ebb/tools/build_context_clusters.py`, `projects/esoteric-ebb/context/v2_base_prompt.md`
- [Dink: A Dialogue Pipeline for Ink](https://wildwinter.medium.com/dink-a-dialogue-pipeline-for-ink-5020894752ee) -- Ink speaker metadata extraction patterns
- [Ink Localisation Tool](https://wildwinter.medium.com/ink-localisation-tool-7e321f834794) -- Line-level ID tagging for Ink localization
- [Localizing Ink with Unity](https://johnnemann.medium.com/localizing-ink-with-unity-42a4cf3590f3) -- Ink localization challenges
- [Ink WritingWithInk documentation](https://github.com/inkle/ink/blob/master/Documentation/WritingWithInk.md) -- Official Ink tag syntax
- [Ink issue #529: Shipped game with localisation](https://github.com/inkle/ink/issues/529) -- Real-world Ink localization experience
- [Embracing AI in Localization 2025-2028 Roadmap](https://medium.com/@hastur/embracing-ai-in-localization-a-2025-2028-roadmap-a5e9c4cd67b0) -- Industry LLM localization direction
- [Best LLM for Translation 2026](https://www.noviai.ai/models-prompts/best-llm-for-translation/) -- Model comparison for translation quality
- [Game Localization QA Guide](https://www.transphere.com/guide-to-game-localization-quality-assurance/) -- Quality threshold practices in game localization

---
*Feature research for: Context-aware retranslation quality improvement (v1.1)*
*Researched: 2026-04-06*
