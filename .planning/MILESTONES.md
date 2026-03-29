# Milestones

## v1.0 한국어 번역 파이프라인 v2 (Shipped: 2026-03-29)

**Phases completed:** 7 phases, 22 plans, 37 tasks

**Key accomplishments:**

- Recursive ink JSON tree walker producing 40,067 dialogue blocks from 286 TextAsset files with SHA-256 hashing, branch structure preservation, and speaker/tag metadata
- Content type classifier (5 types via file prefix + tag signals), passthrough detector (ink control/variables/templates), and gate-boundary-aware batch builder with format-specific size limits
- Validation module comparing parser output against 630 runtime capture entries with 88.9% match rate after multi-layer normalization
- Lease-based pipeline state machine with PostgreSQL/SQLite store, source_hash dedup, and Phase 1 JSON ingest CLI
- Glossary loader from 3 game sources with warmup/filter API, numbered-line scene script prompt builder, regex parser with line-to-ID mapping, and line count + degenerate validation
- Tag extraction with frequency-map validation (7 tag types, order-independent per D-07), codex-spark formatter prompt with EN+KO pairs, and Score LLM parser with failure_type routing to pipeline states
- 3-role concurrent worker pool (translate/format/score) with D-15 retry escalation, D-16 attempt logging, and go-v2-pipeline CLI wiring all domain packages into a runnable pipeline
- QueryDone() store method and V3Sidecar export producing esoteric-ebb-sidecar.v3 translations.json from done pipeline items
- InjectTranslations function mirroring parser tree-walk to replace ^text nodes with Korean translations, with 11 tests covering unit and integration scenarios
- go-v2-export CLI producing translations.json v3, TextAsset .json ink injection, and localizationtexts CSV translation with BOM roundtrip and pipeline quality gates
- V3Sidecar dual output: entries[] deduped by source text (first-seen-wins) + contextual_entries[] with all 35K items for ContextualMap disambiguation
- TryTranslate reduced from 8 to 4 stages, removed Decorated/Embedded/TagSeparatedSegments methods, added .json TextAsset loading
- Plugin.cs v2 complete rewrite (3,083 -> 1,821 lines) with 4-stage TryTranslate chain, 12 Harmony patches, runtime_lexicon.json v2 with 29 externalized hardcoded translations
- Parser-level DC/FC prefix strip with export/Plugin.cs simplification and collision-safe DB migration of 537 rows
- Clean patch deployed: 75,204 translations loaded, DC/FC choices render Korean, hits_dcfc_strip field removed confirming 3-stage pipeline simplification
- StripRenderingWrapper pre-processing in TryTranslate for color/noparse/inline tag wrappers, resolving 136 wrapper mismatches (D-01, D-06) and 341 capture false positives (D-08)
- Expanded runtime_lexicon.json from 42 to 281 rules covering UI labels, game mechanics, passthrough proper nouns/numbers, and template regex patterns

---
