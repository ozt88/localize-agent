1. Return JSON or JSONL only in the requested schema.
2. Do not add prose, markdown, explanations, code fences, or wrapper objects.
3. Keep all keys unchanged unless the task explicitly allows specific writable fields.
4. Never rename, delete, reorder, or invent fields.
5. Preserve `$...`, `{...}`, `<...>`, `\n`, escaped characters, and formatting sequences exactly.
6. Preserve rich-text tag order and nesting exactly; translate only visible prose inside the tags.
7. Do not translate technical identifiers such as `event:/...`, `FEAT_*`, paths, GUID-like fragments, asset IDs, file names, or pure script/control fragments.
8. Translate only player-facing text.
9. Keep proper nouns, places, factions, religions, and recurring terms consistent across files.
10. Dialogue should be natural Korean, narration atmospheric but readable, system text concise, and choices compact and scannable.
11. Preserve humor, irony, dread, and voice without overexplaining.
12. Avoid modern slang unless clearly justified, meme tone, unnecessary honorific inflation, and inconsistent register shifts.
13. Use context fields (`category`, `source_file`, `context_prev`, `context_next`, `speaker_hint`, `tags`) to resolve tone and meaning.
14. If context is insufficient, prefer safe literal-but-natural Korean and raise risk when ambiguity materially affects meaning.
15. Translate in this priority order: `quest`, `ink_dialog` and sentence-like `dialog`, narration, choices, meaningful UI/system text, then lower-value repetitive text.
16. If schema includes `status`, use only workflow-approved values and mark `translated` only when ready to apply.
17. If schema includes `risk`, it must be one of `low`, `med`, `high`.
18. Risk guidance: `low` for clear meaning, `med` for tone or phrasing ambiguity, `high` for unclear lore, referents, markup difficulty, or strong context dependence.
19. If schema includes `notes`, always output a string and keep it brief.
20. Before output, verify schema integrity, token preservation, untranslated technical identifiers, and natural Korean in context.
