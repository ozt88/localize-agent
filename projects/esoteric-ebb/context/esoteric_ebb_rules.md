1. Return JSON or JSONL only in the requested schema.
2. Do not add prose, markdown, explanations, code fences, or wrapper objects.
3. Keep all keys unchanged unless the task explicitly allows specific writable fields.
4. Never rename, delete, reorder, or invent fields.
5. Preserve `$...`, `{...}`, `<...>`, `\n`, escaped characters, and formatting sequences exactly.
6. Preserve rich-text tag order and nesting exactly; translate only the visible prose.
7. Translate only player-facing text. Leave pure script/control fragments and technical identifiers untranslated.
8. Keep proper nouns and recurring terms consistent across files.
9. Do not default to polite Korean. Avoid `-습니다`, `-세요`, and `-하십시오` unless context clearly requires them.
10. Choice lines must read like short player actions, not explanations to the player.
11. For `ROLL/DC/BUY`-style prefixes, preserve the prefix exactly and translate only the player-facing text after it.
12. Do not preserve English word order when natural Korean requires reordering.
13. Do not leave visible English prose untranslated inside rich-text tags unless it is a true proper noun.
14. Use context to decide the function of the line: choice, dialogue, narration, system text, or noise.
15. If schema includes `status`, use only workflow-approved values.
16. If schema includes `risk`, it must be one of `low`, `med`, `high`.
17. If schema includes `notes`, always output a string and keep it brief.
18. Before output, verify schema integrity, token preservation, and natural Korean in context.
