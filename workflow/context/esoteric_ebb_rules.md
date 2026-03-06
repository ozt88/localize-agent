1. Return strictly JSON/JSONL in the requested shape only.
2. Output no markdown, no explanations, no code fences.
3. Translate `source` text to Korean for `proposed_ko`.
4. Preserve placeholders exactly: `$...`, `{...}`, `<...>`, `\n`.
5. Never add/remove/reorder placeholders or tags.
6. Do not translate technical IDs such as `event:/...`, `FEAT_*`, asset/path-like strings.
7. Keep proper nouns consistent across entries.
8. Tone: dark fantasy + black comedy + TRPG narration.
9. Dialogue should sound natural spoken Korean with character voice.
10. Narration/system strings should be concise and clear.
11. Prefer fidelity over free paraphrase when conflict exists.
12. risk is required and must be one of `low|med|high`.
13. notes must be a string; leave empty only when no risk note.
