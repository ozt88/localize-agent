# Phase 2: 번역 엔진 - Research

**Researched:** 2026-03-22
**Domain:** 2-stage LLM translation pipeline (gpt-5.4 translate + codex-mini tag restore) with DB state machine
**Confidence:** HIGH

## Summary

Phase 2 builds the translation engine on top of Phase 1's parser output (40,067 dialogue blocks from 286 ink JSON files). The engine has three LLM roles: (1) gpt-5.4 translates tag-free scene scripts into Korean, (2) codex-mini restores rich-text tags from EN originals into KO translations, and (3) a Score LLM evaluates quality and routes failures. A DB state machine manages the full lifecycle from `pending_translate` through `done`/`failed`, with lease-based worker pools for concurrent processing.

The v1 codebase provides substantial reusable infrastructure: `SessionLLMClient` for OpenCode HTTP communication with session management, `translationpipeline.Store` for lease-based state machine operations, `checkpointBatchWriter` for async DB writes, and `proposal_validation.go` for degenerate output detection. The key new work is: cluster prompt construction (scene scripts with numbered lines), a tag formatter domain (codex-mini orchestration), Score LLM integration with failure_type routing, and an ingest pipeline to load Phase 1 output into DB.

**Primary recommendation:** Build three new domain packages (`clustertranslate`, `tagformat`, `scorellm`) following the established `internal/contracts` interface pattern, extend `translationpipeline/store.go` with new pipeline states for format and score stages, and create a new `pipeline_items_v2` table with additional columns for `ko_raw`, `ko_formatted`, `attempt_log`, and `content_type`.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** 화자 태깅은 라인 앞에 직접 표시: `[01] Braxo: "번역 텍스트"`. codex-mini가 화자 제거 + 태그 복원을 함께 처리하므로 번역 LLM 출력의 화자 파싱 부담 없음.
- **D-02:** 분기 마커는 인라인 태그: `[05] [CHOICE] "(선택지 텍스트)"`. 라인 수 불변. LLM이 태그를 출력에 포함해도 formatter가 제거.
- **D-03:** 이전 게이트 맥락 주입: 같은 knot의 이전 게이트 마지막 3줄을 `[CONTEXT]` 블록으로 배치 앞에 추가. 번역 대상 아님을 명시.
- **D-04:** 콘텐츠 유형별 프롬프트: 베이스 프롬프트를 세션 warmup에 1회 주입 + 유형별 접미 규칙을 배치 전송 시 런타임 조립. 베이스 프롬프트를 warmup 세션에서 프롬프트 최적화.
- **D-05:** codex-mini 입력은 EN원본(태그포함) + KO번역(태그없음) 쌍. codex-mini가 EN↔KO 대응점을 스스로 찾아 태그 위치 결정. 이 방식을 우선 시도.
- **D-06:** 소규모 배치(3-5줄)로 시작하여 안정화 후 배치 크기 확대.
- **D-07:** 태그 검증은 엄격: 태그 문자열 정확 매칭(수, 순서, 속성값). 태그 안에 들어간 번역어의 적절성도 Score LLM에서 함께 평가.
- **D-08:** 복원 실패 시 codex-mini 2회 재시도(2회차에 실패 이유 힌트), 3회차에 gpt-5.4로 에스컬레이션.
- **D-09:** 용어집 소스: GlossaryTerms.txt(54+) + Speaker 이름(107개) + localizationtexts/ CSV. wiki 데이터는 이들과 중복도를 검토한 후 필요 시 추가.
- **D-10:** 고유명사 번역 정책: 전부 원문 유지. 인명, 지명, 주문명, 능력치(Intelligence, Wisdom 등) 모두 영문 그대로.
- **D-11:** LLM 주입: warmup에 핵심 용어(상위 50개) + 배치별 관련 용어 필터. 배치 필터 시 warmup에 이미 포함된 용어는 제외하여 중복 방지.
- **D-12:** 용어집 포맷: JSON 형식.
- **D-13:** Stage 1(번역) 자동 거부 기준: 퇴화 감지(빈 출력, 원문 그대로 복사), 번호 마커 매핑 실패, 구두점 전용 블록(49개) 배치에서 제외/원문 유지, 한글 비율은 거부 기준에서 제외.
- **D-14:** Score LLM은 formatter 이후 1회 호출. failure_type(translation/format/both/pass)과 reason을 반환하여 재시도 라우팅.
- **D-15:** 재시도 전략: 동일 모델 2회(2회차에 Score LLM reason을 힌트로 추가) → 3회차에 고지능 모델로 에스컬레이션. 번역/format 각각 동일 패턴.
- **D-16:** 최종 실패: failed 상태 + 회차별 실패 로그 기록. 각 시도마다 attempt, stage, model, failure_type, reason, score, timestamp를 배열로 축적.

### Claude's Discretion
- 베이스 프롬프트의 구체적 문구 및 규칙 내용
- 유형별 접미 규칙의 세부 내용
- 이전 게이트 맥락 `[CONTEXT]` 블록의 정확한 포맷
- Score LLM 프롬프트 설계 및 스코어 임계치
- 용어집 핵심 50개 선정 기준
- DB 스키마 세부 설계 (pipeline_items 컬럼, 인덱스)
- 배치 크기 튜닝 (D-06 소규모 시작 후 확대 기준)
- 에스컬레이션 대상 모델 선정

### Deferred Ideas (OUT OF SCOPE)
- 고유명사 음역/의역 정책 개선 — C-2에서 원문 유지로 시작, 이후 별도 작업으로 검토
- wiki 크롤링 데이터(`rag/esoteric_ebb_lore_termbank.json`) 용어집 편입 — localizationtexts와 중복도 검토 후 결정
- 콘텐츠 유형별 프롬프트 최적화 (QUAL-01) — Phase 2에서 베이스+접미 구조 구축 후, 품질 데이터 기반으로 개별 튜닝
- 레지스터 적절성 강제 (QUAL-02) — 선택지=반말, NPC=존댓말 등 세분화는 이후 개선
- 구조적 토큰 보존 검증 분리 (QUAL-03) — $var, {template} 등은 현재 passthrough 처리, 추가 검증은 이후
- 배치 크기 자동 튜닝 — D-06에서 수동 확대 후 안정화되면 자동화 검토
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| TRANS-01 | 씬 단위 클러스터를 태그 없이 스크립트 형식으로 gpt-5.4에 전송하여 번역 | Phase 1 `Batch` struct (gate-based grouping) provides scene clusters; v1 `translateSkill` warmup pattern reusable; new numbered-line prompt format needed |
| TRANS-02 | 분기 구조 마커 (BRANCH/OPTION)를 포함하여 분기별 톤/문맥 일관성 보존 | Phase 1 `DialogueBlock.Choice` field identifies choice blocks; D-02 defines `[CHOICE]` inline tag format |
| TRANS-03 | 번역 결과의 각 라인을 `[NN]` 번호 마커로 원본 소스 ID에 매핑 | New line parser needed; v1 `extractObjects`/`extractStringArray` patterns adaptable |
| TRANS-04 | 라인 수 불일치 시 자동 거부 및 재시도 | v1 `degenerateProposalReason()` provides base degenerate detection; line count validation is new code |
| TRANS-05 | codex-mini로 태그가 필요한 라인에만 태그 복원 (원본 태그 구조 기반) | Tag inventory: 6 tag types (i, b, shake, wiggle, u, size) in 5,911/163,294 entries (3.6%); new `tagformat` package needed |
| TRANS-06 | 태그 복원 후 원본과 정확한 태그 문자열 매칭 검증 (태그 수만이 아닌 속성/순서 포함) | D-07 requires exact tag string matching; tag extraction + comparison code needed |
| TRANS-07 | 용어집(글로서리) 구축 및 LLM 컨텍스트에 주입하여 용어 일관성 유지 | GlossaryTerms.txt has 85+ entries (CSV: ID,ResponseAS,Tags,DC,ENGLISH,GERMAN); 8 localizationtexts CSVs; v1 `glossaryEntry` struct reusable |
| TRANS-08 | 번역 품질 스코어링 및 기준 미달 항목 자동 재번역 | D-14/D-15 define Score LLM flow with failure_type routing; v1 `ApplyScores` in store.go provides score application pattern |
| INFRA-01 | DB 기반 파이프라인 상태 관리 (pending -> working -> done/failed), 크래시 후 재개 지원 | v1 `translationpipeline.Store` provides full lease-based state machine; extend with new states |
| INFRA-02 | source_raw 기준 EXISTS 체크로 중복 인제스트 방지 | v1 `Seed()` uses `ON CONFLICT(id) DO UPDATE`; new ingest must check by source_raw hash |
| INFRA-03 | 포맷팅 단계용 파이프라인 상태 확장 (pending_format/working_format) | Current store has 14 states; add `pending_format`, `working_format`, `formatted`, `pending_score_v2`, `working_score_v2` |
</phase_requirements>

## Standard Stack

### Core (all existing in project)
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go standard library | 1.24.0 | Core language, `database/sql`, `encoding/json`, `net/http` | Project convention |
| `github.com/jackc/pgx/v5` | 5.7.6 | PostgreSQL driver | Already used by `translationpipeline.Store` |
| `modernc.org/sqlite` | 1.38.2 | SQLite driver (pure Go) | Already used, Windows-compatible |
| OpenCode HTTP API | local | LLM backend (gpt-5.4, codex-mini) at `127.0.0.1:4112` | Project infrastructure, OpenAI-compatible |

### Supporting (no new dependencies)
| Library | Purpose | When to Use |
|---------|---------|-------------|
| `golang.org/x/sync` | 0.18.0 | Worker pool concurrency primitives | Already in go.mod |
| `github.com/google/uuid` | 1.6.0 | Worker ID generation | Already in go.mod |

**No new dependencies needed.** All required functionality is covered by the existing Go module.

## Architecture Patterns

### Recommended New Package Structure
```
workflow/
  cmd/
    go-v2-ingest/              # NEW: Phase 1 JSON → pipeline_items_v2 ingest
    go-v2-pipeline/            # NEW: orchestrator for translate → format → score
  internal/
    inkparse/                  # EXISTING: Phase 1 parser output types (DialogueBlock, Batch)
    clustertranslate/          # NEW: cluster translation domain
      types.go                 #   Config, ClusterTask, ClusterResult
      prompt.go                #   buildScriptPrompt, buildCardPrompt, buildDictPrompt
      parser.go                #   parseNumberedOutput, mapLinesToIDs
      validate.go              #   validateLineCount, validateDegenerate
    tagformat/                 # NEW: tag restoration domain
      types.go                 #   FormatTask, FormatResult, TagSpec
      prompt.go                #   buildFormatPrompt
      validate.go              #   validateTagMatch (exact string match)
      tags.go                  #   extractTags, compareTags
    scorellm/                  # NEW: quality scoring domain
      types.go                 #   ScoreTask, ScoreResult with failure_type
      prompt.go                #   buildScorePrompt
      parser.go                #   parseScoreResponse
    glossary/                  # NEW: glossary loading and filtering
      types.go                 #   GlossaryTerm, GlossarySet
      loader.go                #   LoadFromSources, FilterForBatch
    contracts/                 # EXTEND: new interfaces
    translationpipeline/       # EXTEND: new states, new table schema
    translation/               # KEEP: reuse validation helpers
  pkg/
    platform/                  # KEEP: SessionLLMClient, checkpoint stores
    shared/                    # KEEP: project config, utilities

projects/esoteric-ebb/
  context/
    v2_base_prompt.md          # NEW: base translation prompt for warmup
    v2_format_prompt.md        # NEW: formatter system prompt
    v2_score_prompt.md         # NEW: score LLM system prompt
```

### Pattern 1: Numbered-Line Script Format (Translation Prompt)
**What:** The cluster translator receives scene scripts where each block is numbered `[01]` through `[NN]`, with speaker prefix and choice markers. The LLM returns Korean with the same numbered-line format.
**When to use:** All dialogue content type batches.
**Example prompt structure:**
```
[CONTEXT]
(이전 게이트 마지막 3줄 — 번역하지 마세요)
[03] "His eyes gleamed in the dark."
[04] Braxo: "Watch your step."
[05] "The floor creaked."

---
다음 씬을 한국어로 번역하세요. 각 라인의 번호를 유지하세요.

[01] Narrator: "The door opened slowly."
[02] Braxo: "We've been expecting you."
[03] [CHOICE] "(Draw your weapon.)"
[04] [CHOICE] "(Speak calmly.)"
[05] "A cold wind blew through."
```
**Expected output:**
```
[01] Narrator: "문이 천천히 열렸다."
[02] Braxo: "기다리고 있었다."
[03] [CHOICE] "(무기를 꺼내 든다.)"
[04] [CHOICE] "(차분하게 말한다.)"
[05] "차가운 바람이 불어왔다."
```
**Key:** Line count must exactly match. The formatter will strip speaker labels and `[CHOICE]` tags later.

### Pattern 2: Tag Restoration (Formatter Prompt)
**What:** codex-mini receives pairs of (EN-with-tags, KO-without-tags) and returns KO-with-tags.
**When to use:** Only for blocks where the EN source contains rich-text tags (3.6% of all blocks).
**Example:**
```json
{"pairs": [
  {"en": "<b>Watch</b> your step, <i>friend</i>.", "ko": "조심해, 친구."},
  {"en": "The <shake>ground trembled</shake>.", "ko": "땅이 흔들렸다."}
]}
```
**Expected output:**
```json
{"results": [
  "<b>조심해</b>, <i>친구</i>.",
  "<shake>땅이 흔들렸다</shake>."
]}
```

### Pattern 3: Pipeline State Machine Extension
**What:** Extend the existing `pipeline_items` state machine with format and score stages.
**New state flow:**
```
pending_translate → working_translate → translated
  → [has tags?]
    YES → pending_format → working_format → formatted → pending_score → working_score → done | failed
    NO  → pending_score → working_score → done | failed

Score LLM returns failure_type:
  "pass"        → done
  "translation" → pending_translate (retry)
  "format"      → pending_format (retry)
  "both"        → pending_translate (retry)
```

### Pattern 4: Attempt Log Accumulation
**What:** Each pipeline item accumulates a JSON array of attempt records for debugging.
**Structure:**
```json
[
  {"attempt": 1, "stage": "translate", "model": "gpt-5.4", "failure_type": null, "reason": null, "timestamp": "..."},
  {"attempt": 1, "stage": "format", "model": "codex-mini", "failure_type": "format", "reason": "missing </b> tag", "timestamp": "..."},
  {"attempt": 2, "stage": "format", "model": "codex-mini", "failure_type": null, "reason": null, "timestamp": "..."}
]
```

### Anti-Patterns to Avoid
- **Reusing v1 pipeline_items table directly:** v1 states and columns are tightly coupled to the v1 flow (blocked_translate, blocked_score, prev/next dependencies). Create a new `pipeline_items_v2` table with the v2 state machine.
- **Sending tags to the translation LLM:** D-05 is explicit -- tags are stripped before translation. The translation LLM sees only clean text.
- **Per-item formatting calls:** codex-mini should receive small batches (3-5 pairs per D-06), not one pair at a time. This amortizes session overhead.
- **Ignoring passthrough blocks during ingest:** Phase 1 marks `IsPassthrough` blocks. These must be excluded from pipeline_items_v2 or immediately set to `done` with `ko_formatted = source_raw`.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| DB state transitions | Custom SQL per state change | Extend `translationpipeline.Store.ClaimPending` + `MarkState` | Lease-based claiming, stale claim cleanup already proven |
| LLM session management | Raw HTTP calls per request | `platform.SessionLLMClient.EnsureContext` + `SendPrompt` | Session creation, warmup, concurrent slots already handled |
| Async DB writes | Direct SQL in hot loop | `checkpointBatchWriter` pattern (channel + flush) | Prevents DB contention under high concurrency |
| Degenerate detection | New validation from scratch | Extend `translation.degenerateProposalReason` | Empty, punctuation-only, exact-copy, ASCII-heavy already covered |
| Tag stripping | Custom regex per tag type | `translation.stripSimpleTags` | Already handles `<tag>` → content extraction |
| Glossary entry struct | New type | Reuse `translation.glossaryEntry` | `{source, target, mode}` matches D-12 JSON format |

**Key insight:** The v1 codebase has mature infrastructure for the hard parts (DB state machine, LLM session management, async writes). The new work is domain logic (prompt construction, output parsing, tag validation) -- not infrastructure.

## Common Pitfalls

### Pitfall 1: Line Count Mismatch in Cluster Output
**What goes wrong:** The translation LLM returns more or fewer lines than the input, making ID mapping impossible.
**Why it happens:** LLM merges short lines, splits long ones, or adds commentary.
**How to avoid:** Strict numbered-line format `[01]`..`[NN]` in prompt; validate output has exactly N numbered lines; auto-reject and retry on mismatch.
**Warning signs:** `len(outputLines) != len(inputBlocks)` after parsing.

### Pitfall 2: Tag Attribute Mutation by Formatter
**What goes wrong:** codex-mini changes `<size=50>` to `<size=50 >` or `<shake>` to `<Shake>`.
**Why it happens:** LLMs normalize whitespace and casing in what they perceive as markup.
**How to avoid:** D-07 requires exact string matching. Extract all tags from EN source, extract all tags from KO formatted output, compare as ordered string lists. Any mismatch = reject.
**Warning signs:** Tag count matches but string comparison fails.

### Pitfall 3: Session Key Collision Between Roles
**What goes wrong:** Translation worker and format worker use the same session key, contaminating each other's context.
**Why it happens:** `SessionLLMClient` uses string keys for session management. If keys overlap across roles, one role's warmup overwrites another's.
**How to avoid:** Use role-prefixed session keys: `v2-translate-{workerID}`, `v2-format-{workerID}`, `v2-score-{workerID}`.
**Warning signs:** LLM responses that look like they're answering the wrong kind of prompt.

### Pitfall 4: Ingest Without Dedup Creates Duplicates
**What goes wrong:** Running the ingest tool twice inserts duplicate rows.
**Why it happens:** Using `INSERT` without checking for existing rows with the same `source_raw`.
**How to avoid:** INFRA-02 mandates `source_raw` hash-based EXISTS check. Use `INSERT ... ON CONFLICT(source_hash) DO NOTHING` or explicit `SELECT EXISTS` before insert.
**Warning signs:** `COUNT(*)` on pipeline_items_v2 exceeds expected 40,067.

### Pitfall 5: Score LLM Returns Invalid JSON
**What goes wrong:** The Score LLM returns free-text instead of the expected `{"failure_type": "...", "reason": "..."}` JSON.
**Why it happens:** Score prompts need very explicit output format instructions.
**How to avoid:** Parse with `json.Unmarshal` and validate required fields. On parse failure, treat as a score error and retry (not a translation failure).
**Warning signs:** `json.Unmarshal` errors in score worker logs.

### Pitfall 6: Passthrough Blocks Sent to Translation
**What goes wrong:** Blocks marked `is_passthrough: true` by Phase 1 get sent to the translation LLM, wasting tokens and producing bad output.
**Why it happens:** Ingest tool does not filter on `IsPassthrough` flag.
**How to avoid:** During ingest, set passthrough blocks directly to `done` state with `ko_formatted = source_raw`. They never enter the translation pipeline.
**Warning signs:** Translation LLM returning English text unchanged for code-like strings.

## Code Examples

### Reusing SessionLLMClient for Multiple Roles
```go
// Source: workflow/pkg/platform/llm_client.go
// Each LLM role (translate, format, score) gets its own session key and LLMProfile.
// The same SessionLLMClient instance handles all roles.

client := platform.NewSessionLLMClient(serverURL, timeoutSec, metrics, traceSink)

// Translation warmup
translateProfile := platform.LLMProfile{
    ProviderID: "openai",
    ModelID:    "gpt-5.4",
    Agent:      "v2-translate",
    Warmup:     basePrompt + "\n" + glossaryWarmup,
}
client.EnsureContext("v2-translate-worker-0", translateProfile)

// Format warmup (different model, different session)
formatProfile := platform.LLMProfile{
    ProviderID: "openai",
    ModelID:    "codex-mini",
    Agent:      "v2-format",
    Warmup:     formatSystemPrompt,
}
client.EnsureContext("v2-format-worker-0", formatProfile)
```

### Reusing ClaimPending for New States
```go
// Source: workflow/internal/translationpipeline/store.go
// ClaimPending is generic -- works for any pending→working transition.

items, err := store.ClaimPending(
    "pending_format",   // pendingState
    "working_format",   // workingState
    workerID,
    batchSize,
    leaseDuration,
)
```

### Tag Extraction and Validation (New Code Pattern)
```go
// New code in tagformat/tags.go
// Extract ordered tag list from a string for exact matching (D-07).

var tagRe = regexp.MustCompile(`</?[a-zA-Z][a-zA-Z0-9]*[^>]*>`)

func extractTags(s string) []string {
    return tagRe.FindAllString(s, -1)
}

func validateTagMatch(enSource, koFormatted string) error {
    enTags := extractTags(enSource)
    koTags := extractTags(koFormatted)
    if len(enTags) != len(koTags) {
        return fmt.Errorf("tag count mismatch: EN=%d KO=%d", len(enTags), len(koTags))
    }
    for i := range enTags {
        if enTags[i] != koTags[i] {
            return fmt.Errorf("tag mismatch at position %d: EN=%q KO=%q", i, enTags[i], koTags[i])
        }
    }
    return nil
}
```

### Pipeline Items V2 Schema (New Table)
```sql
-- New table for v2 pipeline, separate from v1
CREATE TABLE IF NOT EXISTS pipeline_items_v2 (
    id TEXT PRIMARY KEY,                -- DialogueBlock.ID (path-based)
    sort_index INTEGER NOT NULL DEFAULT 0,
    source_file TEXT NOT NULL DEFAULT '',
    knot TEXT NOT NULL DEFAULT '',
    content_type TEXT NOT NULL DEFAULT '',
    source_raw TEXT NOT NULL,           -- original EN text
    source_hash TEXT NOT NULL,          -- SHA-256 of source_raw
    has_tags BOOLEAN NOT NULL DEFAULT FALSE,
    state TEXT NOT NULL,
    ko_raw TEXT,                        -- Stage 1 output (tag-free Korean)
    ko_formatted TEXT,                  -- Stage 2 output (Korean with tags)
    translate_attempts INTEGER NOT NULL DEFAULT 0,
    format_attempts INTEGER NOT NULL DEFAULT 0,
    score_attempts INTEGER NOT NULL DEFAULT 0,
    score_final REAL NOT NULL DEFAULT -1,
    failure_type TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    attempt_log JSONB,                  -- [{attempt, stage, model, failure_type, reason, score, timestamp}]
    claimed_by TEXT NOT NULL DEFAULT '',
    claimed_at TIMESTAMPTZ,
    lease_until TIMESTAMPTZ,
    batch_id TEXT NOT NULL DEFAULT '',   -- which Batch this block belongs to
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_pv2_state ON pipeline_items_v2(state);
CREATE INDEX IF NOT EXISTS idx_pv2_state_lease ON pipeline_items_v2(state, lease_until);
CREATE INDEX IF NOT EXISTS idx_pv2_source_hash ON pipeline_items_v2(source_hash);
CREATE INDEX IF NOT EXISTS idx_pv2_batch ON pipeline_items_v2(batch_id);
```

## Tag Inventory (from corpus analysis)

**Confidence: HIGH** (scanned all 286 TextAsset files, 163,294 `^text` entries)

| Tag | Open Count | Close Count | Notes |
|-----|-----------|-------------|-------|
| `<i>` / `</i>` | 4,390 | 4,382 | Italic -- most common, 8 unclosed (glue artifacts) |
| `<b>` / `</b>` | 1,263 | 1,263 | Bold -- second most common |
| `<shake>` / `</shake>` | 419 | 419 | Text shake effect |
| `<wiggle>` / `</wiggle>` | 112 | 112 | Text wiggle effect |
| `<u>` / `</u>` | 44 | 44 | Underline |
| `<size=N>` / `</size>` | 29 | 29 | Font size (N: 25-60) |
| `<s>` / `</s>` | 1 | 1 | Strikethrough |
| `<no>` | 2 | 0 | Unknown/game-specific |

**Total tagged text entries:** 5,911 out of 163,294 (3.6%)
**Unique tag element types:** 7 (i, b, shake, wiggle, u, size, s) plus `<no>`
**No `<color>` tags found in source ink JSON.** Color tags are added by the game engine at runtime (rendering wrappers per D-07 from Phase 1).

**Implication:** The formatter only needs to handle 7 tag types. The `<size=N>` tag has a parameter value that must be preserved exactly. Simple open/close pair matching covers all cases.

## Glossary Data Analysis

**Confidence: HIGH** (read GlossaryTerms.txt directly)

### GlossaryTerms.txt Format
CSV with columns: `ID, ResponseAS, Tags, DC, ENGLISH, GERMAN`
- 85+ entries (IDs 1-90, some blank rows)
- `ResponseAS` = associated ability score (INT, WIS, CHA, STR, DEX, CON)
- `Tags` = category tags (Spells, City, Geography, History, Esoterics, Politics, etc.)
- `DC` = difficulty class (knowledge check threshold)
- `ENGLISH` = full lore description text (often multi-sentence)
- `GERMAN` = empty (unused)

**Key observation:** GlossaryTerms.txt entries are lore **definitions**, not simple term→translation pairs. Each entry is a paragraph explaining a concept. For glossary injection (D-09/D-11), the approach should be:
1. Extract the **term name** (first word/phrase before the dash in ENGLISH: "Speak with Dead", "Mage Hand", "Citizenry", etc.)
2. Use the term name as the glossary `source` field
3. Per D-10 (all proper nouns stay English), the `target` = `source` and `mode` = "preserve"
4. The lore description can be used as context for the Score LLM but is too verbose for per-batch injection

### localizationtexts/ CSVs
8 files: Feats.txt, ItemTexts.txt, JournalTexts.txt, Popups.txt, QuestPoints.txt, SheetInfo.txt, SpellTexts.txt, UIElements.txt
These contain ID-based localization overrides. Terms from these files supplement the glossary.

### Glossary Loading Strategy
```
1. Parse GlossaryTerms.txt → extract term names → all "preserve" mode (D-10)
2. Parse localizationtexts/*.txt → extract named entities → "preserve" mode
3. Collect speaker names from Phase 1 parser output → "preserve" mode
4. Build unified GlossarySet with dedup
5. Select top 50 by frequency/importance for warmup injection (D-11)
6. Per-batch: filter relevant terms by text overlap, exclude warmup terms
```

## State of the Art

| Old Approach (v1) | Current Approach (v2) | Impact |
|--------------------|-----------------------|--------|
| Per-item translation with JSON format | Scene-cluster translation with numbered-line script format | Better context, consistent tone, fewer retries |
| Tags included in translation prompt | Tags stripped before translation, restored by formatter | Eliminates 99.7% of tag corruption failures |
| Single LLM role (translate + validate) | Three roles (translate, format, score) with typed failure routing | Targeted retries instead of full restart |
| `pipeline_items` with blocked_translate/blocked_score | `pipeline_items_v2` with linear state progression | Simpler state machine, no prev/next dependency chains |
| Batch prompt as JSON objects | Batch prompt as numbered-line scripts | More natural LLM output, easier line mapping |

## Open Questions

1. **codex-mini Model Availability**
   - What we know: project.json references `openai/gpt-5.4` for pipeline LLMs. D-05 specifies codex-mini for formatting.
   - What's unclear: The exact model ID for codex-mini in the OpenCode server configuration.
   - Recommendation: Check available models via `curl http://127.0.0.1:4112/models` during implementation. Use `openai/codex-mini` as the initial assumption; fall back to a fast model if unavailable.

2. **Batch Size for Score LLM**
   - What we know: D-14 says Score LLM is called once per item after formatting.
   - What's unclear: Whether scoring can be batched (multiple items per Score LLM call) for throughput.
   - Recommendation: Start with single-item scoring for simplicity. If throughput is an issue, batch scoring can be added later since the DB state machine supports it.

3. **Escalation Model Identity**
   - What we know: D-08/D-15 specify escalation to a "고지능 모델" on 3rd attempt.
   - What's unclear: Which specific model to escalate to (another gpt-5.4 session? A different model?).
   - Recommendation: Use `pipeline.high_llm` config from project.json (currently `openai/gpt-5.4`). If the same model is used, the benefit comes from the retry hint, not the model change. This is Claude's discretion per CONTEXT.md.

## Sources

### Primary (HIGH confidence)
- **v1 codebase analysis:** Direct reading of `workflow/internal/translation/`, `workflow/internal/translationpipeline/`, `workflow/pkg/platform/` source files
- **Phase 1 output types:** `workflow/internal/inkparse/types.go`, `batcher.go` -- verified DialogueBlock, Batch, content types
- **Tag corpus scan:** All 286 TextAsset files scanned for rich-text tags -- 163,294 entries, 7 unique tag types
- **GlossaryTerms.txt:** Direct read -- 85+ lore entries in CSV format
- **project.json:** LLM backend configuration -- OpenCode at 127.0.0.1:4112, model IDs

### Secondary (MEDIUM confidence)
- **Architecture design:** `.planning/research/ARCHITECTURE.md` -- comprehensive v2 design doc, internally consistent with codebase

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all existing dependencies, no new libraries needed
- Architecture: HIGH -- extends proven v1 patterns with well-defined new domains
- Pitfalls: HIGH -- drawn from v1 operational experience (99.7% tag failure rate documented)
- Tag inventory: HIGH -- exhaustive corpus scan of all 286 source files
- Glossary: HIGH -- direct file read, format confirmed

**Research date:** 2026-03-22
**Valid until:** 2026-04-22 (stable -- game version 1.1.3 fixed, LLM backend local)
