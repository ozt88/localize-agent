# Pitfalls Research

**Domain:** Ink JSON game localization pipeline (tree parsing, LLM translation, tag preservation, BepInEx runtime replacement)
**Researched:** 2026-03-22
**Confidence:** HIGH (primary source: v1 post-mortem + ink runtime format spec + codebase analysis)

## Critical Pitfalls

### Pitfall 1: Dialogue Block Boundary Misidentification in Ink JSON Tree

**What goes wrong:**
The ink JSON runtime format stores text as individual `"^text"` entries inside container arrays, but the game engine concatenates consecutive text entries into a single rendered dialogue block. A naive parser that treats each `"^"` entry as an independent translation unit produces source strings that never appear at runtime. This is exactly what broke v1 -- 65,233 translations.json entries that the plugin could not match because the game sends the concatenated form.

**Why it happens:**
The ink JSON runtime format spec does not document how text entries combine into blocks. The `"^"` entries look like independent items. Without studying how the ink runtime actually evaluates containers (sequential concatenation until a newline `"\n"` or divert), a parser author naturally treats each entry as a unit. The ink spec also does not document "glue" mechanics in the JSON format, making it invisible to spec-readers.

**How to avoid:**
- Concatenate all consecutive `"^"` entries within a container until hitting a `"\n"`, a divert (`{"->":"path"}`), or a control command (`"ev"`, `"/ev"`, choice points).
- Validate every generated source string against what the game actually sends by running the plugin in capture mode and comparing.
- Build a small test harness: parse 5-10 known scenes, compare output against v1's `untranslated_capture.json` to verify blocks match runtime expectations.

**Warning signs:**
- Source strings that are fragments (e.g., `"The "`, `"truth"`, `", and only..."` instead of `"The truth, and only..."`).
- Plugin capture log showing many untranslated entries whose text is a concatenation of your source strings.
- NormalizedMap hit rate below 80% on dialogue content.

**Phase to address:**
Phase 1 (Ink Tree Parser). This is the foundation -- every downstream component depends on correct block boundaries.

---

### Pitfall 2: LLM Output Line Count Drift in Cluster Translation

**What goes wrong:**
When sending N lines of dialogue as a script for cluster translation, the LLM returns N-1, N+1, or N+K lines. Lines merge, split, or get commentary added. The post-processing code that maps translated lines back to source IDs by position silently produces wrong mappings -- line 5's translation gets assigned to line 6's ID.

**Why it happens:**
LLMs are not line-counting machines. A translator LLM may merge two short lines into one natural sentence, split a long line, add stage directions, or drop a line it considers redundant. The problem is worst with:
- Very short lines (the LLM combines them for flow).
- Lines that are continuations of each other (the LLM sees them as one thought).
- Choice/branch markers that the LLM treats as formatting rather than content.

**How to avoid:**
- Number every line in the prompt: `[01] The door opens.` / `[02] You hear a sound.` and require the LLM to preserve numbering in output.
- Parse output by extracting `[NN]` markers, not by splitting on newlines.
- Add a hard validation: if output line count != input line count, reject and retry (max 2 retries before flagging for manual review).
- Keep cluster size manageable: 10-20 lines. Beyond 30, drift probability rises sharply.
- For branching dialogue, use explicit structural markers (`BRANCH:`, `OPTION:`) that the LLM must preserve, as validated in v2 experiments.

**Warning signs:**
- Retry rate above 15% for line count mismatch.
- Translated items where the Korean text clearly belongs to an adjacent dialogue line.
- Cluster sizes averaging above 25 lines.

**Phase to address:**
Phase 2 (Cluster Translation). Must be solved before bulk translation begins.

---

### Pitfall 3: Tag Corruption in the Formatter LLM Stage

**What goes wrong:**
The formatter LLM (codex-mini) tasked with restoring tags like `<b><color=#5782FD>text</color></b>` introduces subtle mutations: removing quotes from attributes (`<link="1">` becomes `<link=1>`), reordering attributes, adding spaces inside tags, or hallucinating tag names. These corrupted tags pass simple "tag count" validation but break the Unity rich text parser at runtime, producing visible rendering errors or crashes.

**Why it happens:**
LLMs treat markup as natural language rather than formal syntax. They "understand" that `<b>` means bold, so they may normalize it to their internal representation and regenerate it slightly differently. The v1 pipeline saw 369 persist-skip events, 99.7% of which were correct translations with corrupted tag formatting.

**How to avoid:**
- Extract exact tags from the source text before sending to the formatter. Pass them as a numbered reference list: `TAG1=<b><color=#5782FD>`, `TAG2=</color></b>`. The formatter only needs to place `TAG1`...`TAG2` around the correct Korean word.
- Validate output with a strict tag-matching check: exact string match on each tag (not just count), correct nesting order, and attribute preservation.
- For simple cases (single bold/color wrap), do tag restoration in code rather than LLM -- only use the formatter LLM for complex multi-tag interleaving.
- Keep a tag registry: enumerate every unique tag pattern in the corpus. There are likely fewer than 50 unique patterns. Build deterministic restoration for common ones.

**Warning signs:**
- Tag validation failures above 5% per batch.
- Tags passing count validation but failing exact-string validation.
- Runtime bold/color "leaking" across dialogue boundaries in-game.

**Phase to address:**
Phase 3 (Formatter LLM + Tag Restoration). Design the tag extraction/injection protocol before implementing the formatter prompt.

---

### Pitfall 4: Ink Branch/Choice Structure Loss During Tree Flattening

**What goes wrong:**
Ink's choice structure uses a two-phase evaluation: the choice text is reconstructed from stack evaluation (start content + choice-only content), not stored directly. A parser that flattens the tree into a linear script loses the distinction between:
- Text that appears before the choice is selected (always visible).
- Text that only appears in the choice button (choice-only, inside `[]`).
- Text that appears after selection (the chosen path).

This causes: (a) choice-only text getting translated as regular dialogue, (b) conditional text (gated by `"flg"` on ChoicePoints) being included unconditionally, (c) once-only choices (`*`) being treated identically to sticky choices (`+`), leading to re-translation of text that will never appear again.

**Why it happens:**
The ink JSON ChoicePoint structure (`{"*": "path", "flg": 18}`) uses a bitfield for flags that is not intuitive. The `c-N` containers hold choice content but are nested in ways that make linear traversal confusing. Without understanding the flag semantics (0x1=condition, 0x2=start content, 0x4=choice-only, 0x10=once-only), parsers misidentify which text belongs to which presentation context.

**How to avoid:**
- Parse ChoicePoint flags explicitly. Build a flag decoder that maps the bitfield to human-readable properties.
- Tag each text entry with its presentation context: `DIALOGUE`, `CHOICE_BUTTON`, `CHOICE_RESULT`, `CONDITIONAL`.
- Generate separate translation batches for choice text vs. dialogue text -- they have different translation constraints (choice text must be short, dialogue can be long).
- Write a test suite against 3-4 well-understood scenes with known branching (e.g., Snell_Companion from v2 experiments) and verify the parser produces the expected structure.

**Warning signs:**
- Choice text appearing as regular dialogue in translations.
- Translations that are too long for choice buttons in the UI.
- Missing translations for choice-gated text that only appears on specific paths.

**Phase to address:**
Phase 1 (Ink Tree Parser). Must handle branches correctly from the start -- retrofitting branch awareness is a rewrite.

---

### Pitfall 5: Plugin Matching Chain Fallback Cascade Causing Wrong Translations

**What goes wrong:**
The v1 plugin has a 7-stage fallback chain. When an exact match fails, it tries normalized matching (tags stripped, whitespace collapsed), then contextual, then runtime lexicon, then tag-separated segments. Each fallback stage is progressively looser, increasing the chance of matching the wrong source text. Two different source texts that normalize to the same string produce a collision -- whichever was loaded first wins, silently serving the wrong translation.

**Why it happens:**
Fallback chains are designed for resilience, but each level trades precision for recall. The NormalizedMap uses `StringComparer.Ordinal` but strips all tags and collapses whitespace, so `"<b>Attack</b> the door"` and `"Attack the door"` map to the same key. With 65,000+ entries, collision probability is significant. The v1 code even has `!NormalizedMap.ContainsKey(normalized)` -- first-write-wins semantics that make collisions silent.

**How to avoid:**
- With v2's block-level source units, exact matching should cover 95%+ of cases. Design the plugin to require exact match for dialogue content and only use normalized fallback for UI strings.
- Remove `TryTranslateTagSeparatedSegments` entirely (already planned for v2).
- Add collision detection: when building NormalizedMap, log if a normalized key already exists with a different target. This surfaces silent data quality issues.
- Keep the fallback chain to 3 stages max: exact -> decorated (tag-stripped core) -> contextual. Each stage should log when it activates so you can measure reliance on fallbacks.

**Warning signs:**
- Plugin diagnostic log showing high normalized-match rates but low exact-match rates.
- Players reporting translations that seem to belong to a different scene.
- NormalizedMap collision count above 100 at load time.

**Phase to address:**
Phase 5 (Plugin Optimization). But the data model from Phase 1 (block-level sources) is what makes this solvable.

---

### Pitfall 6: Source Hash Deduplication Failure Across Pipeline Runs

**What goes wrong:**
The v1 pipeline uses `len(text)` as a "hash" for checkpoint deduplication (documented in CONCERNS.md). Two source texts of the same byte length are considered identical. When re-running the pipeline for v2's 77,816 items, this means: (a) items with identical lengths skip translation, (b) content changes that preserve length don't invalidate stale translations, (c) the pipeline reports "already done" for items that have never been translated.

**Why it happens:**
The `sourceHash` was likely a quick placeholder (`fmt.Sprintf("%x", len(meta.enText))`) that was never replaced with a real hash. It works in small tests where lengths rarely collide but fails at 77K+ scale.

**How to avoid:**
- Replace with SHA-256 of source text content before starting v2 translation runs. This is a one-line fix: `fmt.Sprintf("%x", sha256.Sum256([]byte(meta.enText)))`.
- Add a pre-flight check: query the checkpoint DB for hash collisions (group by hash, having count > 1) and report them before starting a run.
- For v2, the source texts are different (block-level vs. fragment-level), so start with a fresh checkpoint DB rather than risking cross-contamination with v1 data.

**Warning signs:**
- Pipeline reporting items as "done" that have no translation in the output.
- Output translations.json having fewer entries than expected.
- Items with different source text but the same checkpoint status.

**Phase to address:**
Phase 0 (Pipeline Prep). Fix before any v2 translation work begins.

---

### Pitfall 7: Content-Type Conflation in Batch Assembly

**What goes wrong:**
Mixing content types in a single translation batch -- e.g., sending UI labels alongside narrative dialogue, or spell tooltips mixed with system tutorial text -- degrades translation quality for all items. The LLM's register shifts mid-batch: it translates a dramatic death scene, then encounters "Settings" and translates it in the same dramatic tone, or vice versa.

**Why it happens:**
Simplifying batch assembly by ignoring content type is easier to implement. The v2 design acknowledges 5 content types (dialogue, spells/tooltips, UI labels, item descriptions, system/tutorial) but implementing 5 separate batch pipelines with different prompt templates feels like over-engineering.

**How to avoid:**
- Implement content type classification in the tree parser (Phase 1) and carry it through as metadata.
- Use the content type to select the prompt template and batch grouping strategy as designed in the v2 spec.
- At minimum, separate dialogue from non-dialogue. Dialogue needs script format with context; UI labels need dictionary format.
- Validate: sample 20 items from each content type after translation. If UI labels sound literary or dialogue sounds sterile, the batching is wrong.

**Warning signs:**
- Korean UI text using honorifics or narrative register.
- Spell descriptions that read like dialogue.
- Inconsistent formality levels within the same content type.

**Phase to address:**
Phase 1 (Tree Parser) for classification, Phase 2 (Translation) for batch assembly.

---

### Pitfall 8: TextAsset Direct Injection Breaking Ink Runtime State

**What goes wrong:**
The v2 pipeline plans to output `textassets/` files with Korean text directly inserted into ink JSON. If the Korean text changes the byte positions or structural indices of the JSON, the ink runtime's internal pointer system breaks. Saved games reference content by path (`knot/stitch/g-N`), so altering the tree structure can corrupt save compatibility or cause the runtime to read the wrong text.

**Why it happens:**
The ink JSON runtime uses indexed arrays where position matters. Inserting Korean text that is longer or shorter than the English original does not break the JSON validity but can shift array indices if done carelessly. The `"^text"` entries must remain at exactly the same positions in their container arrays.

**How to avoid:**
- Replace text content in-place: change only the string value after `"^"`, never add or remove array elements.
- Verify JSON structural integrity: after injection, parse both original and modified JSON, walk the tree, and confirm that every container has the same number of elements and the same control flow structure.
- Test save compatibility: start a game with English textassets, save, load with Korean textassets, verify the game resumes correctly.
- Never inject into containers that hold ChoicePoints or diverts -- those are structural, not content.

**Warning signs:**
- Game crashes on loading specific scenes after patching.
- Saved games failing to load or resuming at wrong dialogue points.
- Ink runtime errors in BepInEx log about invalid paths or missing containers.

**Phase to address:**
Phase 4 (Patch Output Generation). Must validate structural integrity before shipping.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Using line position for LLM output mapping | Simpler parsing, no markers needed | Silent misalignment when LLM adds/drops lines | Never -- always use explicit ID markers |
| Skipping tag exact-match validation (count only) | Fewer false rejections, faster throughput | Corrupted tags reaching players, bold/color leaks | Never -- exact match is the only reliable check |
| Single prompt template for all content types | Faster development, one code path | Quality degradation across all types | Only in prototype/smoke test phase |
| Fresh checkpoint DB per pipeline version | Avoids cross-contamination | Loses ability to diff against v1 results | Acceptable for v2 launch -- archive v1 DB separately |
| Hardcoded PostgreSQL DSN in project.json | Quick setup, no env var wiring | Credential exposure risk if password added later | Only while DSN has no password and is localhost |
| `TryTranslateTagSeparatedSegments` fallback | Catches some missed translations | Produces wrong translations via segment-level substitution | Never in v2 -- remove entirely |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| OpenCode LLM server | Not handling session timeout -- session expires mid-batch, subsequent calls fail silently with empty responses | Check response content length, re-create session on empty/error response. Add circuit breaker that pauses workers when server is down. |
| PostgreSQL checkpoint DB | Blind INSERT without source_raw dedup check | Always use `ingest_untranslated.py` pattern: EXISTS check on source_raw before INSERT. Never use raw SQL INSERT for pipeline items. |
| BepInEx TranslationLoader | Loading translations.json as a flat dictionary without handling duplicate source keys | Use first-match semantics explicitly. Log duplicates at load time. Better: ensure the pipeline never produces duplicate source keys. |
| Ink JSON textassets | Modifying JSON with string replacement (regex on raw JSON text) instead of proper parse-modify-serialize | Parse JSON to tree, modify text values, serialize back. String replacement breaks on escaped characters, Unicode, nested quotes. |
| Unity font fallback | Assuming the game's default font supports Korean glyphs | Ship Pretendard-Bold.otf and patch font fallback chain. Test with characters outside BMP (rare but possible in Korean). |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Regex compilation inside per-item hot loops | Pipeline slows 3-5x as corpus grows; CPU profiling shows regex dominance | Hoist all `regexp.MustCompile` to package-level vars (6 instances identified in CONCERNS.md) | Noticeable above 10K items |
| Loading entire translations.json into a single Dictionary | Memory spike at plugin startup; long load time on game boot | Already unavoidable for exact-match semantics, but keep entry count minimal by deduplicating in the pipeline | Above 100K entries (current: 65K, manageable) |
| Glossary term matching as O(items x terms) | Translation runs slow linearly with glossary size | Pre-compile glossary regexes once, store on struct. Use Aho-Corasick for multi-pattern matching if glossary exceeds 500 terms | Above 200 glossary terms |
| Plugin NormalizedMap building at game startup | Game load time increases with corpus size | Pre-compute NormalizedMap in the pipeline and ship as a separate file rather than computing at runtime | Above 80K entries |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Bold/color tags leaking across dialogue boundaries | Players see entire paragraphs in bold or colored text that should be normal | Exact block-level matching eliminates segment-level substitution that causes leaks |
| Untranslated text in choices but translated in narration | Immersion-breaking inconsistency; players think the patch is broken | Ensure tree parser captures choice text with the same priority as dialogue; test all choice paths |
| Korean text overflowing UI elements (buttons, tooltips) | Text gets clipped, overlaps, or breaks layout | Add max-length constraints per content type in the translation prompt; test UI elements in-game |
| Inconsistent honorific register across scenes | Characters alternate between formal/informal Korean randomly | Use glossary entries for character-specific register; cluster by scene to maintain consistency |
| Translated stat names breaking game mechanics | `"Strength"` translated to `"힘"` but game code checks for English string `"Strength"` | Keep stat names in runtime_lexicon.json for display-only replacement; never translate in textassets where game logic reads them |

## "Looks Done But Isn't" Checklist

- [ ] **Tree Parser:** Parses all 286 TextAsset files without error -- verify it also handles the edge cases: empty knots, knots with only control flow (no text), deeply nested branches (3+ levels)
- [ ] **Cluster Translation:** 8/8 lines match in test -- verify with 30-line clusters, clusters containing only 1-2 lines, and clusters with Korean/Japanese source text (passthrough detection)
- [ ] **Tag Restoration:** Tags restored correctly in test -- verify with nested tags (`<b><color><link>...</link></color></b>`), self-closing tags, and tags with Unicode attribute values
- [ ] **Plugin Matching:** 95% exact match rate -- verify with text containing newlines, leading/trailing whitespace, and Unicode normalization differences (NFC vs NFD)
- [ ] **Patch Build:** translations.json loads without error -- verify the JSON is valid, has no duplicate keys, and all source keys actually appear in the game at runtime
- [ ] **Save Compatibility:** Game loads with patch -- verify saves made before patching still load, saves made with patch load without patch, and mid-dialogue saves resume correctly
- [ ] **Full Playthrough:** Tested one scene -- verify the main menu, character creation, combat UI, inventory, journal, and dialogue all display Korean correctly

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Wrong block boundaries (Pitfall 1) | HIGH | Rewrite parser, regenerate all source items, retranslate everything. No partial fix possible. |
| LLM line count drift (Pitfall 2) | MEDIUM | Re-run affected batches with stricter prompt. Items can be identified by validation flags. |
| Tag corruption (Pitfall 3) | LOW | Re-run formatter LLM on affected items only (codex-mini is fast). No retranslation needed. |
| Branch structure loss (Pitfall 4) | HIGH | Same as Pitfall 1 -- parser rewrite required. |
| Plugin fallback collision (Pitfall 5) | MEDIUM | Add collision detection, identify affected entries, re-key with more specific source strings. |
| Source hash collision (Pitfall 6) | LOW | Fix hash function, clear checkpoint DB, re-run pipeline. One-line code fix. |
| Content type conflation (Pitfall 7) | MEDIUM | Reclassify items, re-batch, retranslate affected types. Dialogue (the bulk) is likely fine. |
| TextAsset structural corruption (Pitfall 8) | HIGH | Regenerate all textassets from original ink JSON. May require reverifying save compatibility. |

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| P1: Block boundary misidentification | Phase 1: Ink Tree Parser | Compare generated sources against plugin capture log; 95%+ exact match rate |
| P2: LLM line count drift | Phase 2: Cluster Translation | Validation pass rejects batches with count mismatch; retry rate below 10% |
| P3: Tag corruption | Phase 3: Formatter + Tag Restoration | Exact tag string match validation (not just count); 0 corrupted tags in output |
| P4: Branch/choice structure loss | Phase 1: Ink Tree Parser | Test suite against 5 known branching scenes; all choice paths represented |
| P5: Plugin fallback cascade | Phase 5: Plugin Optimization | Exact match rate above 95%; NormalizedMap collision count logged and below 50 |
| P6: Source hash dedup failure | Phase 0: Pipeline Prep | Fix hash, verify no collisions in checkpoint DB before starting translation |
| P7: Content type conflation | Phase 1 (classify) + Phase 2 (batch) | Sample review: 20 items per type, consistent register and format |
| P8: TextAsset structural corruption | Phase 4: Patch Output | Structural diff between original and modified JSON confirms identical tree shape |

## Sources

- v1 pipeline post-mortem (PROJECT.md, CONCERNS.md) -- direct experience with all critical pitfalls
- [Ink JSON Runtime Format Specification](https://github.com/inkle/ink/blob/master/Documentation/ink_JSON_runtime_format.md) -- container/choice/text structure
- [Machine Translation and HTML Tags (Transifex)](https://community.transifex.com/t/machine-translation-html-tags/87) -- tag preservation challenges in MT
- [BallonsTranslator Translation Count Mismatch (GitHub Issue #861)](https://github.com/dmMaze/BallonsTranslator/issues/861) -- LLM batch translation line count drift
- [Translator++ Batch Translation Algorithm](https://dreamsavior.net/translator-ver-7-10-27-better-algorithm-for-local-llm/) -- JSON cloaking for structured LLM output
- [BepInEx Runtime Patching Documentation](https://docs.bepinex.dev/master/articles/dev_guide/runtime_patching.html) -- Harmony patching mechanics
- [BepInEx IL2CPP Utilities](https://github.com/BepInEx/BepInEx.Utility.IL2CPP) -- text replacement patterns
- Codebase analysis: Plugin.cs (3000+ lines), CONCERNS.md (18 documented issues)

---
*Pitfalls research for: Ink JSON game localization pipeline (Esoteric Ebb v2)*
*Researched: 2026-03-22*
