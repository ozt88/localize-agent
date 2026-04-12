# Phase 09: Retranslation Execution — Research

**Researched:** 2026-04-12
**Domain:** Go v2 pipeline 코드 복원 + voice card 재설계 + 전량 재번역 실행
**Confidence:** HIGH (코드베이스 직접 검사, git 히스토리 분석)

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Gray Area 1: 삭제된 코드 처리 방식 → 전량 복원**
- `workflow/internal/v2pipeline/types.go`: `VoiceCardsPath`, `VoiceCards`, `RAGContextPath` 필드 추가
- `workflow/internal/v2pipeline/worker.go`: voice card 로딩, `GetNextLines`(CONT-01), `GetAdjacentKO`(CONT-02), RAG 주입
- `workflow/cmd/go-v2-pipeline/main.go`: `--voice-cards`, `--rag-context` CLI 플래그

**Gray Area 2 & 3: Voice Card 재설계 → wiki/RAG 기반 생성 + relationships 필드 추가**
- 입력 소스: wiki 캐릭터 페이지 + DB 공동 출현 화자 + 기존 DB 샘플
- `relationships` 필드 추가 (화자-청자 관계)
- 배치 내 등장 speaker로 relationships 필터링 (토큰 압박 방지)
- `speech_style`에 구체적 어미 예시 ("어미: ~야, ~잖아")
- 대상: 기존 15명 + Kattegatt (고어체 — `그대/~도다/~노라`)

**Gray Area 4: 샘플 검증 씬 → VL_Visken (Visken + Ôst)**
- 10건 샘플 번역 후 Visken 격식체/냉정한 말투 확인
- Kattegatt 있을 경우 `그대/~도다` 고어체 확인

**D-18 토큰 예산 우선순위:**
continuity > RAG > glossary > branch > voice

### Claude's Discretion

- voice card 생성 시 wiki 페이지 매칭 방법 (exact match vs fuzzy) → 09-01-PLAN에서 결정
- relationships 필드를 voice card JSON에 flat으로 넣을지 nested로 넣을지 → 09-01-PLAN에서 결정
- 전량 재번역 실행 순서 (씬 우선순위, 배치 크기) → 09-02-PLAN에서 결정

### Deferred Ideas (OUT OF SCOPE)

- Kattegatt 대사만 수동 수정하는 좁은 버그 픽스 (rejected — 전량 복원이 맞다)
- Phase 07.1-04 A/B 테스트 (미완료 상태로 skip, 실번역 우선)
</user_constraints>

---

## Summary

Phase 09의 핵심 작업은 세 가지다: (1) worktree 버그로 삭제된 Phase 07/07.1 컨텍스트 주입 코드 복원, (2) voice_cards.json 재생성 (Kattegatt 포함, wiki 기반), (3) 35,009건 전량 재번역 실행 및 게임 검증.

**복원 범위가 예상보다 넓다.** `types.go`, `main.go`의 필드/플래그뿐만 아니라, `contracts/v2pipeline.go`의 `GetNextLines`/`GetAdjacentKO` 인터페이스 메서드, `V2PipelineItem.ParentChoiceText` 필드, `store.go`의 구현체, `clustertranslate/types.go`의 `ClusterTask` 확장 필드들, `clustertranslate/prompt.go`의 `trimContextForBudget` 로직 — 이 모든 것이 현재 코드베이스에서 사라진 상태다.

**현재 voice_cards.json이 존재하지 않는다.** `go-generate-voice-cards`는 존재하지만 DB 샘플만 사용하는 구버전이고, Kattegatt가 미포함이다. 재설계된 CLI로 재생성 필요.

**Primary recommendation:** 복원 작업을 계층 순서로 진행한다 — contracts(인터페이스) → store(구현체) → clustertranslate(타입/프롬프트) → v2pipeline(Config/worker/run) → main.go(CLI 플래그). 각 계층을 완료 후 `go build ./...`로 검증.

---

## 삭제된 코드 완전 목록 (Runtime State Inventory)

> Phase 07(49fe6c)~Phase 07.1(ce1679a) 커밋에서 추가됐으나 현재 코드베이스에 없는 것들.
> `git show [commit] -- [file]`로 직접 확인. [VERIFIED: git history]

### 계층 1: contracts/v2pipeline.go

| 항목 | 타입 | 복원 커밋 참조 |
|------|------|---------------|
| `V2PipelineItem.ParentChoiceText string` | 필드 | 62f70cb |
| `V2PipelineStore.GetNextLines(knot, currentGate string, limit int)` | 인터페이스 메서드 | 62f70cb |
| `V2PipelineStore.GetAdjacentKO(knot string, minSort, maxSort int, limit int)` | 인터페이스 메서드 | 62f70cb |

### 계층 2: v2pipeline/store.go

| 항목 | 복원 커밋 참조 |
|------|---------------|
| `GetNextLines` 구현체 (PostgreSQL) | 62f70cb |
| `GetAdjacentKO` 구현체 (PostgreSQL) | 62f70cb |
| `pipeline_items_v2.parent_choice_text` 컬럼 조회 로직 | 62f70cb |

### 계층 3: clustertranslate/types.go

현재 `ClusterTask` 구조체:
```go
type ClusterTask struct {
    Batch         inkparse.Batch
    PrevGateLines []string
    GlossaryJSON  string
}
```

복원 후 `ClusterTask` 구조체 (71f1c64):
```go
type ClusterTask struct {
    Batch            inkparse.Batch
    PrevGateLines    []string          // D-03
    GlossaryJSON     string            // D-11
    NextLines        []string          // CONT-01
    PrevKO           []string          // CONT-02
    NextKO           []string          // CONT-02
    VoiceCards       map[string]string // TONE-02
    ParentChoiceText string            // BRANCH-01
    RAGHints         string            // D-17
}
```

### 계층 4: clustertranslate/prompt.go

현재: `BuildScriptPrompt`가 PrevGateLines + GlossaryJSON + ContentSuffix만 처리 (149 lines)

복원 후: [VERIFIED: git d7b8971, 71f1c64]
- `buildScriptPromptCore` 분리
- 5개 컨텍스트 블록 주입: NextLines, PrevKO, NextKO, VoiceCards, RAGHints, ParentChoiceText
- `trimContextForBudget` 함수 (D-18 우선순위: continuity → RAG → glossary → branch → voice)
- `contextBudgetTokens = 4000` 상수

### 계층 5: v2pipeline/types.go

현재 `Config` 구조체: `VoiceCardsPath`, `VoiceCards`, `RAGContextPath` 없음

복원 후 (549fe6c + ce1679a):
```go
// Context enrichment (Phase 07)
VoiceCardsPath string            // path to voice_cards.json (optional)
VoiceCards     map[string]string // speaker -> formatted voice guide text (loaded at startup)
RAGContextPath string            // path to rag_batch_context.json (optional, Phase 07.1)
```

### 계층 6: v2pipeline/worker.go

`TranslateWorker` 시그니처 복원 (ce1679a):
```go
func TranslateWorker(ctx context.Context, cfg Config, store contracts.V2PipelineStore,
    llm *platform.SessionLLMClient, glossarySet *glossary.GlossarySet,
    ragCtx *ragcontext.BatchContext,        // 추가
    translateProfile, highProfile platform.LLMProfile,
    workerID string) error
```

`translateBatch` 시그니처 복원 (ce1679a):
```go
func translateBatch(..., ragCtx *ragcontext.BatchContext, ...) error
```

`translateBatch` 내 복원 로직 (549fe6c):
1. voice card 로딩 (startup 시 1회)
2. `store.GetNextLines` 호출 (nextLines)
3. `RetranslationGen > 0` 시 `store.GetAdjacentKO` 호출 (prevKO, nextKO)
4. `items[0].ParentChoiceText` 추출
5. `ragCtx.HintsForBatch(batchID)` 호출 (ce1679a)
6. `ClusterTask` 필드 7개 채움

`ScoreWorker` / `scoreBatch` 시그니처 복원 (ce1679a):
- `ragCtx *ragcontext.BatchContext` 파라미터 추가
- `scorellm.BuildBatchScorePrompt(tasks, ragHintsForScore)` — ragHints 인자 추가

### 계층 7: v2pipeline/run.go

`TranslateWorker` / `ScoreWorker` 호출 시 `ragCtx` 파라미터 전달 추가 (ce1679a):
- `ragcontext.LoadBatchContext(cfg.RAGContextPath)` 호출 (startup 시 1회)

### 계층 8: cmd/go-v2-pipeline/main.go

복원 플래그 (549fe6c + ce1679a):
```go
fs.StringVar(&cfg.VoiceCardsPath, "voice-cards", cfg.VoiceCardsPath, "path to voice_cards.json for named character voice cards")
fs.StringVar(&cfg.RAGContextPath, "rag-context", cfg.RAGContextPath, "path to rag_batch_context.json for world-building context")
```

### 계층 9: scorellm/prompt.go

`BuildBatchScorePrompt` 시그니처 변경 (ce1679a):
- 현재: `BuildBatchScorePrompt(tasks []ScoreTask) (string, []string)`
- 복원 후: `BuildBatchScorePrompt(tasks []ScoreTask, ragContext string) (string, []string)`

---

## 환경 현황

### 데이터 파일 존재 여부 [VERIFIED: bash ls]

| 파일 | 경로 | 존재 여부 |
|------|------|----------|
| rag_batch_context.json | `projects/esoteric-ebb/rag/rag_batch_context.json` | ✓ 존재 |
| enriched_termbank.json | `projects/esoteric-ebb/rag/enriched_termbank.json` | ✓ 존재 |
| voice_cards.json | `projects/esoteric-ebb/context/voice_cards.json` | ✗ 없음 |
| speaker_allow_list.json | `projects/esoteric-ebb/context/speaker_allow_list.json` | ✓ 존재 |
| wiki_markdown/ | `projects/esoteric-ebb/rag/wiki_markdown/` | ✓ 존재 |

### 코드 패키지 현황 [VERIFIED: bash ls]

| 패키지 | 경로 | 상태 |
|--------|------|------|
| `ragcontext` | `workflow/internal/ragcontext/` | ✓ 존재 (loader.go, matcher.go, types.go) |
| `clustertranslate` | `workflow/internal/clustertranslate/` | ✓ 존재 (voice_card.go 포함) |
| `go-generate-voice-cards` | `workflow/cmd/go-generate-voice-cards/main.go` | ✓ 존재 (구버전, 3필드) |

### DB 상태

Phase 08에서 3,412건이 번역됨 (voice card/RAG 없이). `ResetAllForRetranslation`으로 전체 리셋 필요.

---

## Architecture Patterns

### 복원 순서 (의존성 순)

```
1. contracts/v2pipeline.go    → 인터페이스 정의 (다른 모든 것의 기반)
2. v2pipeline/store.go        → 인터페이스 구현체
3. clustertranslate/types.go  → ClusterTask 확장
4. clustertranslate/prompt.go → BuildScriptPrompt 확장 + trimContextForBudget
5. scorellm/prompt.go         → BuildBatchScorePrompt 시그니처 변경
6. v2pipeline/types.go        → Config 필드 추가
7. v2pipeline/worker.go       → 실제 컨텍스트 수집 + 주입 로직
8. v2pipeline/run.go          → ragCtx 로딩 + 워커 호출 수정
9. cmd/go-v2-pipeline/main.go → CLI 플래그 추가
```

각 계층 완료 후 `go build ./...` 검증. 인터페이스 변경(1단계) 후 구현체(2단계) 없으면 컴파일 에러 발생.

### Voice Card 재설계 패턴

현재 `VoiceCard` 구조체 (3필드):
```go
type VoiceCard struct {
    SpeechStyle string `json:"speech_style"`
    Honorific   string `json:"personality"`
    Personality string `json:"honorific"`
}
```

CONTEXT.md 결정: `relationships` 필드 추가.
`LoadVoiceCards` 함수는 `clustertranslate/voice_card.go`에 있음. 구조체 확장 + JSON 마샬링 자동 처리.

관계 정보 포함 예시:
```json
{
  "Kattegatt": {
    "speech_style": "고어체. 어미: ~도다, ~노라, ~옵니다. thou/thy → 그대/당신. 장중하고 느린 리듬",
    "honorific": "존대 (모든 대상에게 고어 격식체)",
    "personality": "위엄 있는 고대 존재, 신탁적, 신비로운",
    "relationships": {
      "The Cleric": "시험하는 자로 대함 — 고어체 유지",
      "default": "초월적 거리감 — 모두에게 고어체"
    }
  }
}
```

### 전량 재번역 실행 패턴

1. `go-v2-reset-all` — 전체 `pending_translate`로 리셋 (기존 CLI 존재)
2. `go-v2-pipeline --voice-cards ... --rag-context ... --backend postgres` 실행
3. 완료 후 `go-v2-export` — translations.json 생성

**배치 크기 주의:** Phase 08에서 `TranslateConcurrency=8`이 기본값. 35,009건 기준 예상 시간 계산 필요.

---

## Common Pitfalls

### Pitfall 1: `--backend postgres` 미명시
**현상:** `total=0` 출력 후 파이프라인이 즉시 종료
**원인:** `project.json`의 `translation.checkpoint_backend`가 기본값 "sqlite"로 상속됨
**방지:** `go-v2-pipeline` 실행 시 반드시 `--backend postgres` 명시 [VERIFIED: knowledge/guardrails.md]

### Pitfall 2: 컴파일 성공 ≠ 컨텍스트 주입 확인
**현상:** `go build ./...` 통과 후 `go-v2-pipeline --help`에 `--voice-cards` 플래그 없음
**원인:** 계층 8(main.go 플래그)을 빠뜨린 경우
**방지:** `go-v2-pipeline --help` 출력에서 `--voice-cards`, `--rag-context` 플래그 명시적 확인 [VERIFIED: knowledge/guardrails.md]

### Pitfall 3: store 메서드 미구현 시 컴파일 에러
**현상:** `contracts.V2PipelineStore`에 `GetNextLines`/`GetAdjacentKO` 추가 후 `store.go` 미수정 시 컴파일 에러
**방지:** 계층 1(contracts) → 계층 2(store) 순서 준수. 각 단계 `go build ./...` 체크

### Pitfall 4: scorellm.BuildBatchScorePrompt 호출자 업데이트 누락
**현상:** `scoreBatch`에서 `BuildBatchScorePrompt` 호출 시 컴파일 에러 (인자 수 불일치)
**원인:** `BuildBatchScorePrompt` 시그니처 변경 후 `worker.go`의 호출부 미수정
**방지:** `grep -rn BuildBatchScorePrompt` 로 모든 호출자 확인

### Pitfall 5: voice card 로딩 위치 오류
**현상:** 워커 루프마다 voice card 재로드 → 성능 저하
**원인:** Phase 07 구현에서 `TranslateWorker` 시작 시 1회 로딩으로 결정 (cfg.VoiceCards nil 체크로 보호)
**방지:** `if cfg.VoiceCardsPath != "" && cfg.VoiceCards == nil` 가드 패턴 유지

### Pitfall 6: score_final NOT NULL 제약 위반
**현상:** 리셋 SQL에서 `score_final = NULL` → PostgreSQL NOT NULL 제약 위반
**방지:** 리셋 시 `score_final = -1`, `failure_type = ''`, `last_error = ''`, `claimed_by = ''` [VERIFIED: knowledge/guardrails.md]

---

## Code Examples

### 복원 대상: ClusterTask (clustertranslate/types.go)

```go
// Source: git 71f1c64 (Phase 07.1-03)
type ClusterTask struct {
    Batch            inkparse.Batch
    PrevGateLines    []string          // last 3 lines of previous gate (D-03)
    GlossaryJSON     string            // per-batch glossary terms (D-11)
    NextLines        []string          // next 3 lines for continuity (CONT-01)
    PrevKO           []string          // prev 3 KO translations (CONT-02)
    NextKO           []string          // next 3 KO translations (CONT-02)
    VoiceCards       map[string]string // speaker -> voice guide text (TONE-02)
    ParentChoiceText string            // parent choice text (BRANCH-01)
    RAGHints         string            // world-building RAG context (D-17)
}
```

### 복원 대상: contracts 인터페이스 메서드 (contracts/v2pipeline.go)

```go
// Source: git 62f70cb (Phase 07-02)
// GetNextLines returns the first N source_raw texts after the current gate.
GetNextLines(knot, currentGate string, limit int) ([]string, error)

// GetAdjacentKO returns ko_formatted texts adjacent to given sort_index range.
// prevKO: items before minSort (closest first), nextKO: items after maxSort.
GetAdjacentKO(knot string, minSort, maxSort int, limit int) (prevKO []string, nextKO []string, err error)
```

### 복원 대상: TranslateWorker 시그니처 (worker.go)

```go
// Source: git ce1679a (Phase 07.1-03)
func TranslateWorker(ctx context.Context, cfg Config, store contracts.V2PipelineStore,
    llm *platform.SessionLLMClient, glossarySet *glossary.GlossarySet,
    ragCtx *ragcontext.BatchContext,
    translateProfile, highProfile platform.LLMProfile,
    workerID string) error
```

### 복원 대상: translateBatch 내 컨텍스트 수집 (worker.go)

```go
// Source: git 549fe6c + ce1679a
// Voice card 로딩 (1회)
if cfg.VoiceCardsPath != "" && cfg.VoiceCards == nil {
    cards, err := clustertranslate.LoadVoiceCards(cfg.VoiceCardsPath)
    // ...
    cfg.VoiceCards = make(map[string]string)
    for name, card := range cards {
        cfg.VoiceCards[name] = fmt.Sprintf("%s, %s, %s", card.SpeechStyle, card.Honorific, card.Personality)
    }
}

// Next lines (CONT-01)
var nextLines []string
if items[0].Gate != "" {
    nextLines, _ = store.GetNextLines(items[0].Knot, items[0].Gate, 3)
}

// PrevKO/NextKO (CONT-02, 재번역 gen>0 시만)
var prevKO, nextKO []string
if items[0].RetranslationGen > 0 && len(items) > 0 {
    minSort := items[0].SortIndex
    maxSort := items[len(items)-1].SortIndex
    prevKO, nextKO, _ = store.GetAdjacentKO(items[0].Knot, minSort, maxSort, 3)
}

// ParentChoiceText (BRANCH-01)
parentChoiceText := ""
if len(items) > 0 && items[0].Choice != "" {
    parentChoiceText = items[0].ParentChoiceText
}

// RAG hints (D-17)
var ragHints string
if ragCtx != nil {
    hints := ragCtx.HintsForBatch(batchID)
    ragHints = ragcontext.FormatHints(hints)
}
```

### voice card에 relationships 필드 추가 (clustertranslate/voice_card.go)

```go
// CONTEXT.md 결정: relationships 필드 추가
type VoiceCard struct {
    SpeechStyle   string            `json:"speech_style"`
    Honorific     string            `json:"honorific"`
    Personality   string            `json:"personality"`
    Relationships map[string]string `json:"relationships,omitempty"` // 화자→청자 관계
}
```

---

## Don't Hand-Roll

| 문제 | 직접 만들지 말 것 | 사용할 것 | 이유 |
|------|-----------------|-----------|------|
| voice card 생성 | 새 CLI 처음부터 | 기존 `go-generate-voice-cards` 개선 | 기본 프레임워크(LLM 호출, DB 연결, 출력)는 구현됨 |
| RAG 힌트 로딩 | ragcontext 재구현 | 기존 `ragcontext` 패키지 | `rag_batch_context.json`이 이미 생성됨 |
| DB 리셋 | 수동 SQL | 기존 `go-v2-reset-all` CLI | Phase 08에서 구현됨 |
| translations.json 생성 | 직접 파일 쓰기 | 기존 `go-v2-export` CLI | `BuildV3Sidecar` highest-gen dedup 포함 |
| 토큰 예산 관리 | 직접 계산 | `trimContextForBudget` 복원 | D-18 우선순위 로직이 이미 설계됨 |

---

## 전량 재번역 실행 준비 체크리스트

Phase 08 교훈(L-01~L-04) [VERIFIED: knowledge/guardrails.md + STATE.md]:

1. **코드 복원 완료 확인:** `go build ./...` 에러 없음
2. **CLI 플래그 확인:** `go-v2-pipeline --help` → `--voice-cards`, `--rag-context` 존재
3. **데이터 파일 확인:** `voice_cards.json` 존재 + Kattegatt 포함, `rag_batch_context.json` 존재
4. **샘플 검증:** VL_Visken 씬 10건 번역 → Visken 격식체/냉정 어조 확인
5. **리셋:** `go-v2-reset-all --backend postgres` 실행 후 `total=35009` 확인
6. **파이프라인 실행:** `--backend postgres` 명시 필수

---

## Environment Availability

| Dependency | Required By | Available | Note |
|------------|------------|-----------|------|
| PostgreSQL | v2pipeline DB | ✓ | `projects/esoteric-ebb/output/` 내 |
| OpenCode server | LLM 번역 | ✓ (runtime) | `manage-opencode-serve.ps1`로 자동 시작 |
| rag_batch_context.json | RAG 주입 | ✓ | `projects/esoteric-ebb/rag/` 존재 |
| voice_cards.json | voice card 주입 | ✗ | 재생성 필요 (Kattegatt 포함) |
| wiki_markdown/ | voice card 생성 | ✓ | `projects/esoteric-ebb/rag/wiki_markdown/` |
| go-v2-reset-all | 전체 리셋 | ✓ | Phase 08에서 구현됨 |
| go-v2-export | 번역 export | ✓ | highest-gen dedup 포함 |
| BepInEx + 게임 | 인게임 검증 | ✓ | E: 드라이브, Plugin.cs v2 |

**Missing with fallback:** voice_cards.json — 09-01 Plan에서 재생성

---

## Open Questions

1. **store.go GetNextLines/GetAdjacentKO 구현체 SQL**
   - 알고 있는 것: PostgreSQL + `pipeline_items_v2` 테이블, `sort_index`, `knot`, `gate` 컬럼 존재
   - 불명확: `gate` 컬럼 타입이 문자열이라 ORDER BY gate 순서가 lexicographic → sort_index 기반으로 구현해야 함
   - 권고: `GetNextLines`는 `sort_index > (현재 gate의 max sort_index)` 조건으로 구현

2. **relationships 필터링 구현 위치**
   - `translateBatch`에서 배치 내 등장 speaker 추출 후 relationships 필터링
   - `ClusterTask.VoiceCards`가 이미 `map[string]string`이므로 필터링 전 포맷팅 단계에서 relationships 제거 가능
   - 09-01-PLAN에서 구체화

3. **wiki 페이지 매칭 방식**
   - wiki_markdown 디렉토리에 파일명이 캐릭터 이름과 정확히 일치하지 않을 수 있음
   - 09-01-PLAN에서 `ls rag/wiki_markdown/` 확인 후 결정

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `pipeline_items_v2.parent_choice_text` 컬럼이 DB 스키마에 존재 | 계층 2 복원 | store.go GetAdjacentKO 구현 시 컴파일/런타임 오류 |
| A2 | VL_Visken 씬이 PostgreSQL DB에 존재 (ingest됨) | 샘플 검증 | 검증 씬 교체 필요 |
| A3 | `rag_batch_context.json`의 batchID 포맷이 현재 DB batchID와 일치 | RAG 주입 | HintsForBatch 항상 nil 반환 → RAG 주입 없음 |

---

## Sources

### Primary (HIGH confidence)
- [VERIFIED: git show 549fe6c] — Phase 07-03 v2pipeline voice card + context 통합 커밋
- [VERIFIED: git show ce1679a] — Phase 07.1-03 RAG 통합 커밋
- [VERIFIED: git show 62f70cb] — Phase 07-02 GetNextLines/GetAdjacentKO 추가 커밋
- [VERIFIED: git show d7b8971 + 71f1c64] — clustertranslate prompt.go 확장 커밋
- [VERIFIED: codebase read] — 현재 코드베이스 상태 (삭제 범위 직접 확인)
- [VERIFIED: knowledge/guardrails.md] — --backend postgres, 샘플 검증, 플래그 확인 guardrails
- [VERIFIED: knowledge/decisions.md] — voice card 재설계 결정, watchdog 수정

### Secondary (MEDIUM confidence)
- [CITED: .planning/phases/09-retranslation-execution/09-CONTEXT.md] — 모든 locked decisions

---

## Metadata

**Confidence breakdown:**
- 삭제 범위 파악: HIGH — git 히스토리 직접 확인
- voice card 재설계 방향: HIGH — CONTEXT.md locked decision
- store.go SQL 구현 상세: MEDIUM — 패턴은 알지만 정확한 쿼리는 구현 시 결정
- 전량 재번역 시간 예측: LOW — Phase 08에서 3,412건/? 시간 데이터 없음

**Research date:** 2026-04-12
**Valid until:** 프로젝트 완료 시 (코드베이스 기반 — 변경 없으면 유효)
