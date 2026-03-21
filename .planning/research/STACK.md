# Stack Research

**Domain:** Ink JSON tree parsing, 2-stage LLM translation pipeline, BepInEx IL2CPP plugin optimization
**Researched:** 2026-03-22
**Confidence:** HIGH (building on proven v1 stack; new components use same languages/tools)

## Existing Stack (Not Re-Researched)

The v1 stack is validated and stays: Go 1.24, PostgreSQL (pgx/v5 5.7.6), Python 3.x, OpenCode server (gpt-5.4 / codex-mini), BepInEx 6 IL2CPP with Harmony. This document covers only the NEW components needed for v2.

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why Recommended | Confidence |
|------------|---------|---------|-----------------|------------|
| Go `encoding/json` (v1, stdlib) | Go 1.24 | Ink JSON tree parsing into `map[string]any` | Ink JSON is deeply nested with heterogeneous types (arrays, strings, objects, numbers). No Go struct can model this -- you need `json.Unmarshal` into `map[string]any` and recursive tree walking. stdlib is battle-tested and already used in 47 files across the codebase. Do NOT use encoding/json/v2 (experimental, still under working group review, memory allocation bugs reported). | HIGH |
| Go `encoding/json` (v1, stdlib) | Go 1.24 | LLM response parsing (cluster translation output, formatter output) | Consistent with existing `platform/llm_client.go` patterns. Structured JSON responses from gpt-5.4 and codex-mini are simple flat objects, no need for streaming or high-performance parsers. | HIGH |
| C# `System.Text.Json` | .NET 6.0 | Plugin.cs translation map loading, matching logic | Already used in Plugin.cs (line 4: `using System.Text.Json`). Stays. No Newtonsoft.Json needed -- BepInEx IL2CPP net6.0 ships with System.Text.Json in the runtime. | HIGH |

### Ink JSON Tree Parser (New Go Package)

| Component | Approach | Why | Confidence |
|-----------|----------|-----|------------|
| Tree walker | Recursive `walkContainer(path string, container []any)` | Ink JSON containers are arrays where the last element is either null or a metadata dict with named sub-containers. This maps perfectly to recursive descent over `[]any`. Each knot in `root[-1]` is a named container = one scene. | HIGH |
| Text block assembly | Concatenate consecutive `"^text"` strings until hitting a control command, newline, or container boundary | This is the v2 fix: v1 split on every `"^"` entry, but the game engine concatenates them into dialogue blocks. The ink runtime format spec confirms text entries are sequential within containers and get concatenated at runtime. | HIGH |
| Branch structure preservation | Track `g-N` (gather) and `c-N` (choice) container names in path | Ink spec: choices are `ChoicePoint` objects with `"*"` key and `"flg"` flags. Gathers (`g-N`) and choices (`c-N`) create the branching tree. Path tracking (e.g., `TS_Snell_Meeting/g-2/c-4`) preserves scene structure for cluster translation. | HIGH |
| Tag extraction | Collect `"#speaker:X"`, `"#DC:X"` strings following text blocks | Tags appear as `"#"` prefixed strings in containers, after text content. These provide metadata (speaker, difficulty checks) needed for translation context. | HIGH |
| Output format | `[]DialogueBlock{Path, SourceRaw, Tags, BranchDepth}` | Each block = one game rendering unit. `SourceRaw` is the concatenated text the game will send to the plugin for matching. This is the fundamental unit for both translation source and plugin matching. | HIGH |

### 2-Stage LLM Translation Pipeline (New Go Commands)

| Component | Approach | Why | Confidence |
|-----------|----------|-----|------------|
| Stage 1: Cluster translator | Send 10-30 line scene scripts to gpt-5.4, tags stripped, script format | Experiment validated: 8/8 perfect mapping on scene script, 16/16 on branched dialogue. Script format preserves context and branch structure. Tags removed to prevent LLM corruption (v1's core failure mode). | HIGH |
| Stage 1: Response parsing | Numbered line format (`1. Korean text`), parsed back to source block IDs | Simpler than JSON output for creative translation. Line-by-line mapping verified in experiments. Go string splitting is trivial. | HIGH |
| Stage 2: Tag formatter | Send only tag-bearing lines to codex-mini with original tags + Korean translation | Experiment validated: 4/4 perfect tag count match. codex-mini is fast/cheap, perfect for mechanical tag insertion. Only ~15-20% of lines need tags, so this stage processes a fraction of total volume. | HIGH |
| Stage 2: Validation | Count tag pairs in output vs. source; reject on mismatch | Tag count validation is the only check needed (experiment proved tag structure is preserved when count matches). No complex XML/HTML parsing required. | HIGH |
| Batch orchestration | Extend existing `go-translation-pipeline` pattern | v1 already has `translationpipeline/run.go` with checkpoint management, retry logic, PostgreSQL state tracking. v2 adds a new pipeline command reusing the same `platform` and `contracts` packages. | HIGH |
| Content-type routing | Switch input format by content type (script/card/dictionary/document) | 5 content types identified: dialogue (script), spells (structured card), UI (dictionary), items (card+context), system (document). Each gets its own prompt template but uses the same 2-stage pipeline. | MEDIUM |

### BepInEx Plugin Optimization (C# Changes)

| Component | Approach | Why | Confidence |
|-----------|----------|-----|------------|
| Target framework | .NET 6.0 (`net6.0` in csproj) | Already configured. BepInEx 6 bleeding edge (be.753+) uses .NET 6 for IL2CPP. No change needed. | HIGH |
| Harmony patches | HarmonyX (bundled with BepInEx 6) | Already in use via `0Harmony.dll` reference. Harmony patches intercept Unity text rendering methods. No version change needed. | HIGH |
| Matching chain simplification | Remove `TryTranslateTagSeparatedSegments`, strengthen direct matching | v2 produces dialogue-block-level sources that match what the game sends. The segment fallback that caused tag leaks becomes unnecessary. Keep: TranslationMap direct, Decorated, Normalized, Contextual, RuntimeLexicon. Remove: TagSeparatedSegments. | HIGH |
| Dictionary structure | `Dictionary<string, string>` with `StringComparer.Ordinal` | Already used. Ordinal comparison is correct for exact source matching. v2 blocks are complete dialogue strings, so exact matching hit rate should be much higher than v1. | HIGH |

### Supporting Libraries

| Library | Version | Purpose | When to Use | Confidence |
|---------|---------|---------|-------------|------------|
| `github.com/jackc/pgx/v5` | 5.7.6 | PostgreSQL driver for pipeline state | Already in go.mod. Used for translation checkpoints, source item storage, batch tracking. No change. | HIGH |
| `golang.org/x/sync` | 0.18.0 | `errgroup` for concurrent batch processing | Already indirect dep. Use for parallel LLM calls within rate limits (existing autoscaler manages RPM). | HIGH |
| `modernc.org/sqlite` | 1.38.2 | Local checkpoint fallback | Already in go.mod. Keep as fallback for development/testing without PostgreSQL. | HIGH |

## Installation

No new dependencies needed. v2 builds entirely on the existing Go module and C# project:

```bash
# Go: no new packages to install
# The v2 commands will be new files under workflow/cmd/ using existing packages

# C#: no new NuGet packages
# Plugin.cs modifications use only System.Text.Json and BepInEx APIs already referenced
```

## Alternatives Considered

| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| Ink JSON parsing | Go stdlib `encoding/json` into `map[string]any` | Third-party ink runtime (inkjs, godot-ink) | No Go ink runtime exists. JS/C# runtimes are full story players -- we only need to extract text blocks from the JSON tree, not execute ink logic. A custom tree walker is simpler and more precise for our extraction needs. |
| Ink JSON parsing | Go stdlib `encoding/json` v1 | Go `encoding/json/v2` (experimental) | v2 is behind `GOEXPERIMENT=jsonv2` flag, still under active working group review (weekly meetings since Nov 2025), and has reported memory allocation bugs (issue #75026). Not production-ready. |
| Ink JSON parsing | Custom Go tree walker | Ink-Localiser (Node.js tool) | Ink-Localiser works on raw `.ink` source files, not compiled JSON. We only have compiled ink JSON from the game. Also, it assigns IDs per text line, not per dialogue block -- same granularity problem as v1. |
| LLM response format | Numbered lines for translation | JSON structured output | Creative translation benefits from free-form output. JSON schema constraints can make translations stilted. Numbered lines are trivial to parse and validated in experiments. |
| LLM response format | Separate formatter stage | Single-stage with tags | v1 proved LLM corrupts tags (99.7% of persist-skip failures were format, not translation). 2-stage separation is the validated fix. |
| High-perf JSON | Go stdlib | `goccy/go-json`, `jsoniter` | We parse ~286 TextAsset files once at pipeline start. Not a hot path. Stdlib correctness matters more than microsecond gains. |
| Plugin matching | Simplified chain (5 strategies) | Keep all 7 strategies from v1 | `TagSeparatedSegments` is the root cause of tag leaks. `TryTranslateEmbedded` may still be useful but should be evaluated -- it was a workaround for v1's granularity problem. |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `encoding/json/v2` (Go experimental) | Still under working group review, memory bugs reported, not covered by Go 1 compatibility promise | `encoding/json` v1 -- stable, well-understood, used throughout codebase |
| Ink-Localiser / Dink / any `.ink`-source tool | Works on raw ink source files, not compiled JSON; wrong granularity (per-line, not per-block) | Custom Go tree walker on compiled ink JSON |
| `goccy/go-json` or `jsoniter` | Adds dependency for marginal perf gain on non-hot-path; risks subtle behavioral differences | `encoding/json` stdlib |
| Newtonsoft.Json in Plugin.cs | Would add a DLL dependency to the BepInEx plugin; `System.Text.Json` already available in net6.0 runtime | `System.Text.Json` (already in use) |
| LangChain / Python LLM orchestration | Adds Python dependency to the translation pipeline; Go already has working LLM client with retry/checkpoint | Existing Go `platform/llm_client.go` with OpenCode backend |
| Single-stage LLM translation with tags | 99.7% of v1 persist-skip failures were tag corruption. LLMs reliably corrupt XML-like tags in creative translation context | 2-stage: strip tags, translate, then restore tags with codex-mini |

## Stack Patterns by Variant

**For dialogue content (71,787 entries):**
- Parse ink JSON tree into dialogue blocks (Go tree walker)
- Cluster by scene/knot (10-30 lines per batch)
- Stage 1: gpt-5.4 script-format translation
- Stage 2: codex-mini tag restoration (tag-bearing lines only)
- Output: PostgreSQL items with block-level source_raw

**For non-dialogue content (UI, items, spells, system):**
- Same 2-stage pipeline but different input formats
- UI labels: dictionary format, 50-100 per batch
- Spells/items: structured card format, 5-10 per batch
- System text: document format, full sections
- These use existing v1 ingestion paths (already in PostgreSQL)

**For plugin matching (C#):**
- v2 sources are block-level = direct match should hit ~90%+ (vs. v1's fallback-heavy matching)
- Simplified chain: Direct -> Decorated -> Normalized -> Contextual -> RuntimeLexicon
- Remove TagSeparatedSegments entirely (the v1 problem source)

## Version Compatibility

| Component | Compatible With | Notes |
|-----------|-----------------|-------|
| Go 1.24 + `encoding/json` v1 | All existing workflow packages | No compatibility concerns; same version as v1 |
| BepInEx 6 (be.753+) | .NET 6.0, HarmonyX, IL2CPP | Already validated with Esoteric Ebb 1.1.3 |
| `System.Text.Json` | .NET 6.0 (bundled) | No separate package needed; included in net6.0 runtime |
| pgx/v5 5.7.6 | PostgreSQL 17 | Already in use and validated |
| gpt-5.4 (OpenCode) | OpenAI-compatible API at 127.0.0.1:4112 | Cluster translation validated in experiments |
| codex-mini (OpenCode) | OpenAI-compatible API at 127.0.0.1:4112 | Tag formatter validated in experiments |

## Sources

- [Ink JSON Runtime Format Specification](https://github.com/inkle/ink/blob/master/Documentation/ink_JSON_runtime_format.md) -- Container structure, text entry format, choice/branch mechanics (HIGH confidence)
- [Ink Architecture Overview](https://github.com/inkle/ink/blob/master/Documentation/ArchitectureAndDevOverview.md) -- Runtime design, container model (HIGH confidence)
- [Ink-Localiser](https://github.com/wildwinter/Ink-Localiser) -- Existing tool for ink localization, confirmed it operates on raw .ink not compiled JSON (HIGH confidence)
- [Dink Pipeline](https://wildwinter.medium.com/dink-a-dialogue-pipeline-for-ink-5020894752ee) -- Alternative ink dialogue pipeline, uses script-like format (MEDIUM confidence)
- [Go encoding/json v2 status](https://go.dev/blog/jsonv2-exp) -- Experimental, working group review ongoing, not production-ready (HIGH confidence)
- [Go encoding/json v2 memory bug](https://github.com/golang/go/issues/75026) -- Skyrocketing memory allocation reported (HIGH confidence)
- [BepInEx Releases](https://github.com/bepinex/bepinex/releases) -- BepInEx 6 bleeding edge builds, .NET 6 target (HIGH confidence)
- [BepInEx Bleeding Edge Builds](https://builds.bepinex.dev/projects/bepinex_be) -- Build be.753+ with Il2CppInterop 1.5.0 (HIGH confidence)
- [LLM Structured Output for Translation](https://flounder.dev/posts/structured-output-for-translation/) -- Patterns for batch translation with structured output (MEDIUM confidence)
- [Ink Localization Discussion #529](https://github.com/inkle/ink/issues/529) -- Shipped game with ink localization in 7 languages, real-world patterns (MEDIUM confidence)
- Existing codebase analysis: `go.mod`, `Plugin.cs`, `workflow/internal/`, `workflow/cmd/` (HIGH confidence -- direct inspection)
- v2 experiment results from project memory (HIGH confidence -- validated by user)

---
*Stack research for: Ink JSON tree parsing + 2-stage LLM translation pipeline + BepInEx plugin optimization*
*Researched: 2026-03-22*
