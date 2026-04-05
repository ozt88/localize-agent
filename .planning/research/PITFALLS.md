# Pitfalls Research: Context-Aware Retranslation (v1.1)

**Domain:** Adding speaker extraction, context-aware prompting, and selective retranslation to existing 40K-entry Korean game translation pipeline
**Researched:** 2026-04-06
**Confidence:** HIGH (primary sources: codebase analysis, v1.0 post-mortem experience, ink format knowledge, LLM translation patterns)

## Critical Pitfalls

### Pitfall 1: Selective Retranslation Destabilizing Working Translations

**What goes wrong:**
Retranslating a subset of blocks within a batch breaks tone/register consistency with surrounding blocks that were NOT retranslated. The v2 pipeline translates in scene-unit clusters (10-30 lines). If you retranslate lines 5, 12, 18 from a 25-line cluster independently, the new translations may use different register, different pronoun choices, or different vocabulary than lines 1-4, 6-11, etc. The result reads worse than before -- individual lines may be better, but the conversation feels fractured.

**Why it happens:**
The LLM produces translations in context of the other lines in the batch. When you retranslate only some lines, the new LLM call lacks the surrounding Korean translations as context. The new lines are produced in isolation (or with English-only context), creating a mismatch with existing Korean neighbors.

**How to avoid:**
- **Retranslate entire clusters, not individual lines.** When any line in a batch scores below threshold, retranslate the whole batch_id group. This preserves internal consistency.
- Use the existing `batch_id` column in `pipeline_items_v2` to identify cluster boundaries. The batch is already the correct retranslation unit.
- For borderline cases (1-2 bad lines in a 25-line batch), inject the existing Korean translations of "good" lines as `[CONTEXT]` reference so the LLM can match tone.
- Track retranslation provenance: add a `retranslate_round` column so you can diff before/after and rollback if quality regresses.

**Warning signs:**
- Average score across a batch drops after retranslating individual lines within it.
- Pronoun inconsistency within a scene (mixing 너/당신, or 반말/존댓말 mid-conversation).
- Players reporting dialogue that "sounds like two different translators."

**Phase to address:**
Phase 1 (Selective Retranslation Design). Must establish the retranslation unit before any retranslation executes.

---

### Pitfall 2: Speaker Extraction False Positives from isSpeakerTag Heuristic

**What goes wrong:**
The existing `isSpeakerTag()` in `inkparse/parser.go` uses a heuristic: reject known non-speaker patterns, accept proper-case single words. The code comments explicitly state it "prefers false-positive speakers over missing character names." For v1.0 translation this was acceptable -- a wrong speaker label in the prompt just adds noise. For v1.1's tone consistency feature, false speaker attribution causes the LLM to apply the WRONG character's voice to a line, actively degrading translation quality.

**Why it happens:**
Ink uses `#` tags for multiple purposes: speaker names, game commands (PlaySFX, CamFocus), ability checks (DC, FC), item references (OBJ), and game-specific flags. The heuristic in `isGameCommandTag()` rejects CamelCase compounds, known prefixes, and dice notation, but some game-specific tags (single proper-case words like "Minor", "Medium", "Crowns") leak through. When tone consistency rules are applied per-speaker, these misattributions become translation-breaking.

**How to avoid:**
- Build a validated speaker roster from the ink data: extract all unique `speaker` values from `pipeline_items_v2`, manually audit the list (should be manageable -- likely <50 unique values), and create an allow-list.
- Replace the open heuristic with a closed allow-list for tone-consistency purposes. Keep the heuristic for basic prompt labeling but only apply character voice rules to validated speakers.
- Store speaker validation status in a lookup table: `speaker_roster(name TEXT PRIMARY KEY, validated BOOLEAN, voice_profile TEXT)`.

**Warning signs:**
- `SELECT DISTINCT speaker FROM pipeline_items_v2 WHERE speaker != '' ORDER BY speaker` returning unexpected values (game tags posing as speakers).
- Tone rules being applied to non-character entries (e.g., "Attacking" treated as a character name).
- Translation output using a specific character voice for narration lines.

**Phase to address:**
Phase 1 (Speaker Extraction/Validation). Must be done BEFORE tone consistency rules are built on top of it.

---

### Pitfall 3: Branch Context Explosion in Ink's Combinatorial Structure

**What goes wrong:**
Ink scripts have deeply nested branching: choices lead to gates, gates contain sub-choices, and branches can rejoin via diverts. Naively injecting "full branch context" into translation prompts means including every possible path that led to the current gate. For a 3-choice node with 2 sub-choices each, that is 6 paths. For 3 levels deep, 216 paths. The prompt becomes enormous, the LLM loses focus, and translation quality drops because the signal-to-noise ratio collapses.

**Why it happens:**
The ink tree structure is a DAG, not a simple linear sequence. The current `GetPrevGateLines()` in `V2PipelineStore` returns the last N lines from the previous gate -- a simple linear lookback. Upgrading to "branch-aware context" without bounding the context window leads to exponential growth. Developers see that "more context = better" and try to include everything.

**How to avoid:**
- **Bound context to the immediate parent branch only.** Do not recursively include grandparent branches. One level of choice context ("player chose X, leading to this scene") is sufficient.
- Use a fixed context budget: max 5 lines of previous context + 1 line of branch descriptor. The existing `PrevGateLines` pattern (3 lines) is close to optimal.
- Format branch context as a single summary line: `[CONTEXT] (Player chose: "Agree to help Braxo")` rather than injecting the full choice text and all alternatives.
- Profile prompt token counts before/after: if context injection increases prompt size by >30%, you are over-injecting.

**Warning signs:**
- Prompt token counts growing beyond 2x baseline for branching scenes.
- LLM output quality degrading on heavily-branched knots (tunnels, hubs) while improving on linear scenes.
- Translation latency increasing for specific knots due to context size.
- Score LLM giving lower scores to retranslated branching content than to v1.0 translations of the same content.

**Phase to address:**
Phase 2 (Branch Context Design). Must be designed with explicit token budget before implementation.

---

### Pitfall 4: Retranslation Breaking the Sidecar Dedup Contract

**What goes wrong:**
The V3Sidecar export uses `entries[]` (deduped by `source_raw`, first-seen-wins) for `TranslationMap` and `contextual_entries[]` (all items) for `ContextualMap`. If you retranslate some occurrences of a duplicated source text but not others, the "first-seen" entry in `entries[]` may be the OLD translation while `contextual_entries[]` has the NEW one -- or vice versa. Plugin.cs uses `TranslationMap` for simple lookups and `ContextualMap` for contextual matches. The two maps become inconsistent, causing the same English text to show different Korean translations depending on which lookup path the plugin takes.

**Why it happens:**
`BuildV3Sidecar()` iterates items in `sort_index` order and takes the first occurrence for `entries[]`. Retranslation changes `ko_formatted` for specific items but the dedup logic does not know which translation is "better." If sort_index 42 (old translation) comes before sort_index 987 (new translation) for the same `source_raw`, the old one wins in `entries[]`.

**How to avoid:**
- When retranslating, update ALL occurrences of the same `source_raw` if context-independent (UI, system text, items). Only allow context-dependent divergence for dialogue.
- Modify `BuildV3Sidecar()` to prefer the highest-scored translation for `entries[]` dedup, not first-seen. Add `score_final` to the sort key.
- Add a validation step in the export pipeline: for each `source_raw` with multiple items, verify that context-independent content types produce identical `ko_formatted`.
- Track which items were retranslated: add `retranslated_at TIMESTAMPTZ` column to detect divergence.

**Warning signs:**
- Same English text showing different Korean translations at different points in the game.
- `entries[]` count changing unexpectedly after retranslation export.
- Plugin.cs fallback from ContextualMap to TranslationMap producing a different result than direct ContextualMap hit.

**Phase to address:**
Phase 3 (Retranslation Execution). Export logic must be updated BEFORE running retranslation, not after.

---

### Pitfall 5: Tone Consistency Rules Creating Monotonous Character Voice

**What goes wrong:**
You define per-character voice profiles (e.g., "Braxo speaks in gruff, short sentences") and inject them into every prompt containing that character. The LLM over-indexes on the voice profile and produces repetitive, formulaic translations. Every Braxo line sounds identical in cadence and vocabulary. The character loses emotional range -- anger, fear, humor all come out in the same "gruff" voice. The translations become technically consistent but dramatically flat.

**Why it happens:**
LLMs are instruction-followers. A strong voice directive ("always use short, gruff sentences") overrides the tone signals in the source text. The existing `v2_base_prompt.md` already has detailed ability score voices (wis, str, int, cha, dex, con) with examples -- these work because they describe personality, not rigid style rules. Adding NPC voice profiles with overly prescriptive rules eliminates the natural variation that makes dialogue feel alive.

**How to avoid:**
- Define character voices as **personality descriptions, not style rules.** "Braxo is a weary soldier who has seen too much" NOT "Braxo always speaks in short, clipped sentences."
- Include 2-3 examples showing the character's RANGE (angry, thoughtful, joking) rather than a single tone.
- Test voice profiles on 10 diverse lines per character before applying to full retranslation. Compare against existing translations -- if the new versions all sound the same while the originals had more variety, the profile is too restrictive.
- Use the existing ability score voice approach from `v2_base_prompt.md` as the template: personality + examples + "translate accordingly."

**Warning signs:**
- Running the same voice profile on 10+ lines and all translations having similar sentence structure.
- Score LLM consistently rating retranslated character dialogue at 6-7 (adequate but not good) instead of a mix of 5s and 9s.
- Korean text losing exclamation marks, questions, ellipses that exist in the English source.

**Phase to address:**
Phase 2 (Tone Consistency Design). Voice profiles must be tested on sample sets before bulk application.

---

### Pitfall 6: Score LLM Threshold Gaming -- Retranslating What Scores Low vs What Reads Bad

**What goes wrong:**
You use `score_final` from the existing Score LLM to select items for retranslation. But the Score LLM evaluates translations in isolation (EN + KO pair) without surrounding context. A translation that scores 8/10 in isolation may read terribly in context (wrong register for the scene, contradicts the previous line). Conversely, a translation scoring 5/10 may actually work fine in its scene context. You end up retranslating the wrong items.

**Why it happens:**
The current `BuildScorePrompt` sends just `EN: {source}\nKO: {formatted}\nHas tags: {bool}`. There is no scene context, no surrounding lines, no speaker information. The score measures "is this Korean sentence a reasonable translation of this English sentence?" not "does this Korean sentence fit in this conversation?"

**How to avoid:**
- Build a **context-aware scoring** prompt that includes 2-3 surrounding lines (both EN and KO) plus speaker info. This should be a new scoring mode, not a replacement of the existing isolate score.
- Use the context-aware score specifically for retranslation candidate selection. Keep the existing isolated score for pipeline pass/fail routing.
- Consider a two-pass selection: (1) Score LLM flags low-scoring items, (2) human spot-check of a sample (50-100 items) from both low-score and medium-score buckets to calibrate the threshold.
- The selection query should weight batch-level metrics: if 3+ items in a batch score below threshold, retranslate the whole batch.

**Warning signs:**
- After retranslation, the overall average score improves but player feedback does not.
- Retranslation candidates concentrated in specific content types (e.g., all UI text) rather than distributed across narrative.
- Items with `score_final` between 6-7 being ignored while items scoring 4-5 are retranslated, but the 6-7 items are the ones players actually notice.

**Phase to address:**
Phase 1 (Retranslation Candidate Selection). Scoring methodology must be validated before selecting which items to retranslate.

---

### Pitfall 7: Prompt Size Regression Breaking Existing Translation Quality

**What goes wrong:**
Adding speaker info, branch context, tone rules, and glossary to the translation prompt pushes it beyond the LLM's effective attention window. The warmup (system prompt + context + rules + glossary) was already substantial. Adding per-character voice profiles, branch descriptors, and enhanced context causes the LLM to lose track of earlier instructions. Translation quality on SIMPLE lines degrades because the model is processing too many instructions.

**Why it happens:**
Each context feature seems small in isolation: +50 tokens for speaker profile, +100 tokens for branch context, +200 tokens for character voice examples. But they compound. The existing warmup in `BuildBaseWarmup()` already concatenates systemPrompt + contextText + rules + glossary. The v2_base_prompt.md is ~1500 tokens. Adding per-batch context on top of per-session context creates a layering effect where the model cannot prioritize.

**How to avoid:**
- Set a hard token budget for the total prompt (warmup + batch content). Measure the current baseline and allow at most 20% growth.
- Make context features mutually exclusive or ranked: for dialogue content, inject speaker + branch context. For UI/system content, inject NOTHING extra (these don't need it).
- Use content_type to gate which context features apply: `ContentDialogue` gets full context, `ContentSpell/Item/UI/System` get zero added context.
- A/B test on a fixed set of 100 diverse items: translate with current prompt vs enhanced prompt. If average score drops on ANY content type, the prompt is too heavy.

**Warning signs:**
- Prompt token count exceeding 3000 tokens for the warmup alone.
- LLM ignoring rules that appear early in the prompt (e.g., "keep proper nouns in English" being violated).
- Translation time per batch increasing by >50%.
- Score regressions on content types that did not benefit from the added context.

**Phase to address:**
Phase 2 (Prompt Engineering). Must profile token budgets before finalizing prompt structure.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Retranslate individual lines instead of full batches | Fewer LLM calls, faster iteration | Tone inconsistency within scenes, breaks cluster coherence | Never for dialogue; acceptable for isolated UI/system strings |
| Skip speaker roster validation, trust isSpeakerTag heuristic | No manual audit needed | Wrong voice profiles applied to non-character lines | Only if tone consistency feature is not being built |
| Inject all branch history as context | More information for LLM | Prompt bloat, attention dilution, quality regression on branching scenes | Never -- always bound context window |
| Use existing isolated score for retranslation selection | No new scoring logic needed | Wrong items selected, effort wasted on already-good translations | Acceptable for first pass; must upgrade to context-aware scoring before second retranslation round |
| Retranslate without snapshotting current ko_formatted | Simpler pipeline, no storage overhead | Cannot rollback if retranslation makes things worse | Never -- always snapshot before retranslation |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| PostgreSQL pipeline_items_v2 | Updating ko_raw/ko_formatted without resetting score_final and failure_type | When retranslating: SET ko_raw=new, ko_formatted=NULL, score_final=-1, failure_type='', state='pending_format' (or pending_score if no tags) |
| Plugin.cs V3Sidecar | Exporting after partial retranslation, creating mixed old/new translations in entries[] | Complete all retranslation for a batch before exporting. Add export-time validation that no items are in working_* states |
| Score LLM | Running existing score prompt on retranslated items and comparing to original scores | Score distribution will shift because retranslated items had enhanced prompts. Compare retranslated items ONLY to each other, not to original scores |
| Batch claiming (ClaimBatch) | Retranslation pipeline claiming batches that are partially done (some items already good) | Add a new state `pending_retranslate` distinct from `pending_translate` to avoid collision with fresh translation pipeline |
| OpenCode session warmup | Using same session key for retranslation (enhanced prompt) and original translation | Use distinct session keys: "v2-translate-{workerID}" vs "v2-retranslate-{workerID}" so warmup context does not bleed across modes |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Full-table scan for retranslation candidates | Query `SELECT * FROM pipeline_items_v2 WHERE score_final < 7` scans all 40K rows | Add index `idx_pv2_score ON pipeline_items_v2(score_final) WHERE state='done'` | Noticeable at 40K rows, painful at 100K+ |
| Re-scoring all retranslated items sequentially | Score stage bottleneck when 5000+ items need re-scoring | Reuse existing batch score prompt (BuildBatchScorePrompt) with batches of 10 | When retranslation volume exceeds 2000 items |
| Loading full speaker roster into every prompt | Adding 500+ token character profiles to warmup for every batch | Load speaker profile only for speakers present in the current batch | When character roster exceeds 20 characters |
| Exporting sidecar after every retranslation batch | Disk I/O and JSON serialization for 75K+ entries per export | Export only after full retranslation round completes | Always -- export is expensive |

## "Looks Done But Isn't" Checklist

- [ ] **Speaker roster:** Extracted all unique speakers -- verify none are game command tags by checking against known character list
- [ ] **Retranslation scope:** Selected candidates by score -- verify by manually reading 20 random samples in context (not isolation)
- [ ] **Branch context:** Injected choice context -- verify prompt token count stays under budget for worst-case branching knots
- [ ] **Tone consistency:** Applied voice profiles -- verify character has emotional RANGE by checking happy/angry/neutral lines for same character
- [ ] **Sidecar export:** Generated new translations.json -- verify entries[] prefers highest-scored translation, not first-seen
- [ ] **Rollback path:** Retranslation complete -- verify original ko_formatted values are snapshotted and recoverable
- [ ] **Plugin.cs compatibility:** New sidecar loaded -- verify no new untranslated lines appear in capture log (retranslation did not change source keys)
- [ ] **Score calibration:** Context-aware scores computed -- verify they correlate with human judgment on a 50-item sample

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Retranslation broke cluster consistency | MEDIUM | Identify affected batch_ids, retranslate entire batches, re-export sidecar |
| Wrong speaker profiles applied | LOW | Fix speaker_roster, retranslate only affected speakers' lines (filtered by speaker column) |
| Branch context bloated prompts | LOW | Revert to previous prompt template, re-run affected batches with bounded context |
| Sidecar dedup inconsistency | LOW | Re-run BuildV3Sidecar with score-aware dedup, re-export |
| Tone consistency made dialogue flat | MEDIUM | Remove voice profile from prompt, retranslate affected character's lines with personality-only description |
| Score threshold selected wrong items | HIGH | Snapshot wasted, need context-aware re-scoring of all 40K items, then re-select candidates |
| Prompt size regression | MEDIUM | A/B comparison identifies regression, revert prompt, retranslate affected batches with simpler prompt |

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| P1: Cluster-breaking retranslation | Phase 1 (Retranslation Design) | Retranslation unit is batch_id, never individual lines for dialogue |
| P2: Speaker false positives | Phase 1 (Speaker Extraction) | Manual audit of DISTINCT speaker values, allow-list created |
| P3: Branch context explosion | Phase 2 (Context Design) | Prompt token count profiled, max 20% growth over baseline |
| P4: Sidecar dedup breakage | Phase 3 (Retranslation Execution) | Export uses score-aware dedup, validation pass confirms consistency |
| P5: Monotonous character voice | Phase 2 (Tone Consistency) | 10-line diversity test per character before bulk application |
| P6: Wrong retranslation candidates | Phase 1 (Candidate Selection) | Context-aware scoring built, human-validated on 50-item sample |
| P7: Prompt size regression | Phase 2 (Prompt Engineering) | Token budget set, A/B test on 100 items shows no regression |

## Sources

- Codebase analysis: `workflow/internal/inkparse/parser.go` (isSpeakerTag heuristic), `workflow/internal/clustertranslate/prompt.go` (BuildScriptPrompt), `workflow/internal/v2pipeline/worker.go` (translateBatch), `workflow/internal/v2pipeline/export.go` (BuildV3Sidecar dedup logic), `workflow/internal/scorellm/prompt.go` (BuildScorePrompt)
- Project context: `projects/esoteric-ebb/context/v2_base_prompt.md` (existing prompt structure, ability score voices)
- DB schema: `workflow/internal/v2pipeline/postgres_v2_schema.sql` (pipeline_items_v2 structure)
- v1.0 post-mortem: source unit mismatch, plugin compatibility lessons
- [Ink documentation - WritingWithInk.md](https://github.com/inkle/ink/blob/master/Documentation/WritingWithInk.md) -- tag system, branching mechanics
- [Dink: A Dialogue Pipeline for Ink](https://wildwinter.medium.com/dink-a-dialogue-pipeline-for-ink-5020894752ee) -- speaker identification patterns in ink
- [Localizing Ink with Unity](https://johnnemann.medium.com/localizing-ink-with-unity-42a4cf3590f3) -- ink localization challenges
- [Multilingual Prompt Engineering Survey](https://arxiv.org/html/2505.11665v1) -- cross-lingual prompt consistency
- [AI Prompt Engineering for Localization](https://custom.mt/ai-prompt-engineering-for-localization-2024-techniques/) -- localization-specific prompt patterns
- Memory: `project_translation_quality.md` -- documented context quality issue motivating v1.1

---
*Pitfalls research for: Context-aware retranslation added to existing 40K-entry Korean game translation pipeline*
*Researched: 2026-04-06*
