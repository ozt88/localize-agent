# Phase 07: Context Enrichment -- 톤 프로필 + 분기 맥락 + 연속성 윈도우 - Research

**Researched:** 2026-04-07
**Domain:** Go 파이프라인 프롬프트 확장, ink JSON 파서 확장, DB 연속성 쿼리
**Confidence:** HIGH

## Summary

Phase 07은 Phase 06에서 확립한 4-tier 프롬프트 구조(Context/Voice/Task/Constraints)에 세 가지 컨텍스트를 주입하는 단계다: (1) named 캐릭터 voice card를 LLM으로 자동 생성하여 speaker 매칭 시 프롬프트에 주입, (2) ink 파서에서 choice container의 부모 선택지 텍스트를 추출하여 분기 대화에 "Player chose: X" 맥락 제공, (3) prev/next 슬라이딩 윈도우를 3줄로 확장하고 재번역 시 기존 한국어 번역을 prevKO/nextKO로 채움.

모든 작업은 기존 Go 코드 패턴 위에서 수행된다. voice card는 `lore.go`의 JSON 로드-매칭-주입 패턴을 재사용하고, 분기 맥락은 `inkparse/parser.go`의 `walkContainer`에서 choice path 추적으로 구현하며, 연속성 윈도우는 `store.go`의 `GetPrevGateLines` 패턴을 확장한다. 외부 의존성 추가 없음.

**Primary recommendation:** voice card 생성(일회성 데이터 준비) -> 프롬프트 주입 코드(voice + branch + continuity 통합) -> A/B 테스트 순서로 진행. voice card 생성은 별도 Go CLI 또는 Python 스크립트로, 프롬프트 주입은 `BuildScriptPrompt` 확장으로 구현하라.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** named 캐릭터용 voice card를 LLM 자동 생성. 게임 대사 DB에서 캐릭터별 샘플을 추출하고, LLM이 말투/존댓말/성격을 분석하여 JSON voice card 자동 생성.
- **D-02:** voice card 필드는 기본 3필드: 말투(화법 스타일), 존댓말 레벨(반말/평어/존대), 성격 키워드. ability-score voice guide와 동일한 구조.
- **D-03:** 상위 15명(100회+ 등장) 캐릭터에 대해서만 voice card 생성. Snell(2663)~Thal(100) 범위. 나머지는 voice card 없이 기존 범용 규칙 적용.
- **D-04:** ink JSON 파서(inkparse) 확장으로 choice container의 부모 선택지 텍스트를 추출. DialogueBlock에 ParentChoiceText 필드 추가. 소스 준비 단계에서 해결.
- **D-05:** 브랜치 깊이 1단계 제한 (로드맵 Success Criteria 준수). 토큰 예산 내 유지.
- **D-06:** prev/next 3줄 슬라이딩 윈도우로 확장 (로드맵 기준). 현재 PrevGateLines 3줄에서 양방향으로 확장.
- **D-07:** 재번역 시 prevKO/nextKO를 DB ko_formatted 조회로 채움. prev/next line_id로 해당 아이템의 ko_formatted를 조회. 이미 store.go에 의존성 조회 로직 존재.
- **D-08:** 토큰 예산 초과 시 우선순위: voice card(가장 중요, 마지막 삭제) > branch context > continuity window(가장 먼저 삭제). 낮은 우선순위부터 제거.
- **D-09:** A/B 테스트는 저품질 배치 10개를 컨텍스트 주입 전/후로 번역하여 score 비교. 프롬프트 크기 회귀 없음 확인.

### Claude's Discretion
- voice card JSON 파일 저장 위치 및 로드 방식 (lore.go 패턴 재사용 가능)
- 분기 맥락의 프롬프트 주입 위치 ([CONTEXT] 블록 vs Voice 섹션)
- 토큰 예산 상한값 (프로파일링 결과 기반 결정)
- A/B 테스트 배치 선택 기준 (score_final 분포 기반)
- voice card 생성용 LLM 프롬프트 설계
- continuity window의 next 라인 처리 (최초 번역 시 미번역 상태)

### Deferred Ideas (OUT OF SCOPE)
None -- 논의가 phase scope 내에서 진행됨
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| TONE-01 | 상위 15 named 캐릭터의 voice card JSON 생성 (말투/존댓말/성격) | D-01/D-02/D-03에 따라 LLM 자동 생성. `speaker_allow_list.json`에서 상위 15명 목록 확인됨. DB에서 대사 샘플 추출 가능 (store.go 쿼리 확장). |
| TONE-02 | speaker_hint 매칭 시 voice card를 per-batch 프롬프트에 주입 | `buildVoiceSection()` 패턴 확인. named 캐릭터용 voice card를 ability-score voice와 동일 방식으로 주입. `ClusterTask`에 `VoiceCards` 필드 추가. |
| BRANCH-01 | ink 파서에서 choice container의 부모 선택지 텍스트 추출 | `parser.go`의 `walkContainer` 분석 완료. choice path 추적으로 부모 텍스트 전달 가능. `DialogueBlock`에 `ParentChoiceText` 필드 추가. |
| BRANCH-02 | 분기 맥락 "Player chose: X"를 프롬프트에 포함 | `BuildScriptPrompt`의 `[CONTEXT]` 블록에 삽입하는 것이 자연스러움. 기존 `PrevGateLines` 패턴과 동일 위치. |
| BRANCH-03 | 브랜치 깊이 1단계 + 토큰 예산 내 제한 | D-05에 따라 직계 부모 선택지만 전달. `estimateTokens()` 함수 이미 존재. |
| CONT-01 | prev/next 3줄 슬라이딩 윈도우 확장 | `GetPrevGateLines`가 이미 prev 3줄 구현. next 방향 쿼리 추가 + `ClusterTask` 필드 확장 필요. |
| CONT-02 | 재번역 시 prevKO/nextKO를 DB ko_formatted로 채움 | v2 pipeline에 `prev_line_id`/`next_line_id` 부재. `sort_index` 기반 prev/next 아이템 조회 쿼리 신규 작성 필요. |
</phase_requirements>

## Standard Stack

### Core

기존 스택 유지. 신규 의존성 없음. [VERIFIED: go.mod 직접 확인, 2026-04-07]

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib `encoding/json` | 1.24 | voice card JSON 로드/파싱 | 기존 lore.go, speaker_allowlist.go 패턴 동일 |
| Go stdlib `database/sql` | 1.24 | 연속성 윈도우 DB 쿼리 확장 | 기존 store.go 패턴 동일 |
| Go stdlib `flag` | 1.24 | voice card 생성 CLI | 기존 cmd/ 패턴 동일 |
| `github.com/jackc/pgx/v5` | 5.7.6 | PostgreSQL 드라이버 | 이미 사용 중 |
| `modernc.org/sqlite` | 1.38.2 | SQLite 드라이버 | 이미 사용 중 |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Go CLI로 voice card 생성 | Python 스크립트 | Go가 LLM 클라이언트 + DB 접근 모두 갖추고 있어 Go 권장. Python은 ad-hoc 스크립트용으로만 |
| sort_index 기반 prev/next 조회 | prev_line_id/next_line_id 컬럼 추가 | v1에는 존재하나 v2에서 제거됨. sort_index 쿼리가 스키마 변경 없이 충분. [VERIFIED: v2pipeline/store.go 직접 확인] |

## Architecture Patterns

### 변경 대상 파일 구조

```
workflow/
  internal/
    clustertranslate/
      prompt.go          # BuildScriptPrompt 확장: voice card + branch context + next lines 주입 (수정)
      types.go           # ClusterTask 필드 추가: VoiceCards, ParentChoiceText, NextLines, PrevKO, NextKO (수정)
    inkparse/
      parser.go          # walkContainer에 parentChoiceText 전달 (수정)
      types.go           # DialogueBlock.ParentChoiceText 필드 추가 (수정)
    v2pipeline/
      store.go           # GetNextLines, GetAdjacentKO 쿼리 메서드 추가 + parent_choice_text DB 컬럼 (수정)
      worker.go          # translateBatch에서 voice card/branch/continuity 데이터 조합 (수정)
      types.go           # Config.VoiceCardsPath 필드 추가 (수정)
    contracts/
      v2pipeline.go      # V2PipelineItem.ParentChoiceText 필드 + V2PipelineStore 인터페이스 확장 (수정)
  cmd/
    go-generate-voice-cards/
      main.go            # 일회성 voice card 생성 CLI (신규)
projects/esoteric-ebb/
  context/
    voice_cards.json     # LLM 생성 voice card 데이터 (신규)
```

### Pattern 1: Voice Card 로드-매칭-주입 (lore.go 패턴 재사용)

**What:** JSON 파일에서 캐릭터별 voice card를 로드하고, 배치 내 speaker가 매칭되면 프롬프트에 주입
**When to use:** 매 배치 번역 시 speaker 필드가 named 캐릭터와 일치할 때
**Example:**
```go
// Source: workflow/internal/translation/lore.go 패턴 적용 [VERIFIED: 코드 직접 확인]

// voiceCard represents a named character's translation voice profile.
type voiceCard struct {
    Name         string `json:"name"`
    SpeechStyle  string `json:"speech_style"`  // 말투 (화법 스타일)
    Honorific    string `json:"honorific"`     // 존댓말 레벨 (반말/평어/존대)
    Personality  string `json:"personality"`   // 성격 키워드
}

// loadVoiceCards loads voice card JSON (map[name]voiceCard).
func loadVoiceCards(path string) (map[string]voiceCard, error) {
    if path == "" { return nil, nil }
    raw, err := os.ReadFile(path)
    if err != nil { return nil, err }
    var cards map[string]voiceCard
    return cards, json.Unmarshal(raw, &cards)
}

// buildNamedVoiceSection creates per-batch voice guide for named characters.
func buildNamedVoiceSection(speakers []string, cards map[string]voiceCard) string {
    seen := make(map[string]bool)
    var sb strings.Builder
    for _, s := range speakers {
        card, ok := cards[s]
        if ok && !seen[s] {
            seen[s] = true
            fmt.Fprintf(&sb, "- **%s**: %s, %s, %s\n",
                s, card.SpeechStyle, card.Honorific, card.Personality)
        }
    }
    if sb.Len() == 0 { return "" }
    return "\n## Named Character Voice Guide\n" + sb.String()
}
```

### Pattern 2: 부모 선택지 텍스트 추출 (inkparse 확장)

**What:** `walkContainer`의 choice 분기 진입 시 해당 choice의 display text를 자식 블록에 전달
**When to use:** choice container(c-N) 안에서 대사 블록 생성 시
**Example:**
```go
// Source: inkparse/parser.go walkContainer 패턴 [VERIFIED: 코드 직접 확인]

// walkContainer 재귀 호출 시 choiceText 파라미터 추가 또는
// walker 구조체에 currentChoiceText 필드 추가

// DialogueBlock에 추가:
// ParentChoiceText string `json:"parent_choice_text,omitempty"`

// walkContainer에서 c-N 서브컨테이너 진입 시:
// 1. c-N 컨테이너의 "s" 배열에서 extractTextFromArray로 선택지 텍스트 추출
// 2. 자식 walkContainer 호출 시 이 텍스트를 parentChoiceText로 전달
// 3. flushBlock에서 DialogueBlock.ParentChoiceText에 저장
```

### Pattern 3: 연속성 윈도우 확장 (sort_index 기반)

**What:** 현재 배치의 앞뒤 아이템 source_raw/ko_formatted를 DB에서 조회
**When to use:** 매 배치 번역 시 컨텍스트 윈도우 구성
**Example:**
```go
// Source: v2pipeline/store.go GetPrevGateLines 패턴 확장 [VERIFIED: 코드 직접 확인]

// GetNextLines returns the first N source_raw texts after the current gate.
func (s *Store) GetNextLines(knot, currentGate string, limit int) ([]string, error) {
    rows, err := s.db.Query(s.rebind(`
        SELECT source_raw FROM pipeline_items_v2
        WHERE knot = ? AND gate != ? AND gate != ''
          AND sort_index > (SELECT MAX(sort_index) FROM pipeline_items_v2 WHERE knot = ? AND gate = ?)
        ORDER BY sort_index ASC
        LIMIT ?`),
        knot, currentGate, knot, currentGate, limit,
    )
    // ... scan rows ...
}

// GetAdjacentKO returns ko_formatted for items adjacent to the batch (by sort_index).
// Used during retranslation to provide prevKO/nextKO context.
func (s *Store) GetAdjacentKO(batchItems []V2PipelineItem, limit int) (prevKO, nextKO []string, err error) {
    minSort := batchItems[0].SortIndex
    maxSort := batchItems[len(batchItems)-1].SortIndex
    // Query items with sort_index < minSort (prev) and > maxSort (next)
    // WHERE state = 'done' AND ko_formatted IS NOT NULL
}
```

### Anti-Patterns to Avoid
- **prev_line_id/next_line_id 컬럼 추가:** v2 파이프라인은 sort_index 기반 순서 관리. v1의 linked-list 패턴을 도입하면 스키마 복잡도만 증가. sort_index 쿼리로 충분. [VERIFIED: v2 store.go에 prev/next_line_id 부재 확인]
- **voice card를 warmup에 전체 주입:** 15명 전체 voice card를 매 세션 워밍업에 넣으면 토큰 낭비. per-batch에 해당 speaker만 주입하라. ability-score는 이미 warmup에 있으므로 named character와 분리.
- **모든 캐릭터에 voice card 생성:** D-03에 따라 100회+ 등장 15명만. 나머지는 범용 규칙으로 충분하며, 데이터가 부족하면 voice card 품질도 낮아진다.
- **next 라인의 ko_formatted를 최초 번역에서 기대:** 최초 번역 시 next 라인은 아직 미번역 상태. nextKO는 재번역 시에만 유효. 최초 번역에서는 next EN source_raw만 제공.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| voice card 생성 | 규칙 기반 말투 분류기 | LLM 기반 자동 분석 (D-01) | 15명 캐릭터의 말투 뉘앙스는 규칙으로 포착 불가. LLM이 대사 샘플에서 직접 분석하는 것이 정확 |
| 토큰 카운트 | tiktoken Go 바인딩 | `estimateTokens()` 근사 함수 | 이미 구현되어 있고, 정확한 토큰 수보다 예산 비교에 충분 [VERIFIED: prompt.go에 존재] |
| prev/next 아이템 조회 | linked list (prev/next_line_id) | sort_index 기반 SQL 쿼리 | v2 스키마에 이미 sort_index 존재, 인덱스 활용 가능 |

**Key insight:** Phase 07의 모든 작업은 기존 패턴의 확장이다. 신규 아키텍처 도입이 필요한 부분이 없다.

## Common Pitfalls

### Pitfall 1: Voice Card 품질 -- LLM이 대사 샘플에서 일관된 프로필을 추출 못함
**What goes wrong:** LLM에게 캐릭터 대사 10-20개를 주고 말투 분석을 시키면, 같은 캐릭터라도 씬에 따라 다른 말투를 사용하여 일관되지 않은 프로필이 나올 수 있다.
**Why it happens:** 게임 캐릭터는 상황에 따라 말투가 변한다 (전투 vs 일상, 긴장 vs 이완). 짧은 샘플에서 "대표적 말투"를 추출하기 어렵다.
**How to avoid:** (1) 캐릭터당 20개 이상의 대사 샘플을 다양한 씬에서 추출. (2) LLM 프롬프트에 "이 캐릭터의 기본 모드(baseline tone)를 분석하라"고 명시. (3) 생성된 voice card를 사람이 한 번 검토 후 확정. (4) voice card가 너무 구체적이면 오히려 해로움 -- 2-3 문장으로 간결하게.
**Warning signs:** 같은 캐릭터의 다른 대사가 voice card와 완전히 어울리지 않는 번역을 생성할 때.

### Pitfall 2: 분기 맥락 추출 -- ink JSON에서 choice text와 choice 후 대사의 연결이 자명하지 않음
**What goes wrong:** ink JSON에서 c-N 컨테이너의 display text는 `"s"` 배열 안에 있지만, choice 후 실행되는 대사는 다른 컨테이너에 있을 수 있다 (divert로 연결). ParentChoiceText 추출이 모든 경우를 커버하지 못할 수 있다.
**Why it happens:** ink의 choice 구조는 (1) 선택지 표시 텍스트 (s 배열), (2) 선택 후 즉시 표시되는 텍스트 (같은 c-N 컨테이너), (3) divert로 다른 knot/gate로 이동하는 경우 세 가지가 혼합된다. D-04는 (2)만 처리하면 충분하다.
**How to avoid:** (1) D-05 브랜치 깊이 1단계 제한을 엄격히 준수. (2) 직계 부모 c-N 컨테이너의 display text만 추출. (3) divert 이후의 분기 맥락은 무시 (토큰 예산 + 복잡도 관리). (4) 파서 테스트에 실제 ink JSON의 choice 구조를 포함.
**Warning signs:** ParentChoiceText가 빈 문자열인 choice 블록이 과다하게 많을 때.

### Pitfall 3: 연속성 윈도우 -- next 라인이 미번역 상태일 때 프롬프트 설계
**What goes wrong:** 최초 번역 시 next 라인의 ko_formatted가 NULL. nextKO에 빈 문자열을 넣으면 LLM이 "이전 번역이 없음"을 맥락 부재로 오해할 수 있다.
**Why it happens:** 파이프라인은 sort_index 순서로 처리하므로, 현재 배치보다 뒤의 아이템은 아직 번역되지 않았다.
**How to avoid:** (1) 최초 번역에서는 next EN source_raw만 [CONTEXT]에 포함하고 nextKO는 생략. (2) 재번역 시에만 prevKO/nextKO를 모두 채움. (3) 프롬프트에 명시: "nextKO가 비어있으면 아직 번역되지 않은 라인입니다."
**Warning signs:** next 라인 관련 프롬프트 텍스트가 빈 상태로 LLM에 전달될 때.

### Pitfall 4: 토큰 예산 초과 -- voice card + branch + continuity가 합산되면 예상보다 큼
**What goes wrong:** 세 컨텍스트가 동시에 존재하는 배치에서 프롬프트가 토큰 예산을 초과하여 LLM 응답 품질이 저하되거나 잘림 발생.
**Why it happens:** voice card (100-200 토큰) + branch context (50-100 토큰) + continuity window 6줄 (200-600 토큰) = 최대 900 토큰 추가. 기존 프롬프트 + 배치 본문과 합산 시 예산 초과 가능.
**How to avoid:** (1) D-08 우선순위 적용: continuity window 먼저 축소, branch 다음, voice card 마지막. (2) `estimateTokens()` 결과가 임계값 초과 시 자동 축소 로직 구현. (3) 토큰 예산 상한을 프로파일링으로 결정 (Phase 06에서 기초 데이터 수집 완료).
**Warning signs:** `PromptMeta.EstimatedTokens`가 급격히 증가하는 배치 존재.

### Pitfall 5: A/B 테스트 배치 선택 편향
**What goes wrong:** 테스트용 10개 배치를 무작위로 선택하면, voice card 없는 배치나 분기가 없는 배치만 선택되어 컨텍스트 주입 효과를 제대로 측정하지 못함.
**Why it happens:** 대부분의 배치는 named 캐릭터가 없거나 (ability-score 화자) 분기가 없다 (직선 대화).
**How to avoid:** (1) 테스트 배치를 세 카테고리에서 선택: named 캐릭터 포함 배치, 분기 포함 배치, 일반 배치. (2) score_final이 가장 낮은 배치 중에서 선택 (개선 여지가 큰 배치). (3) 최소 2개는 named 캐릭터, 2개는 분기 포함으로 보장.
**Warning signs:** A/B 테스트 결과가 무의미한 차이를 보일 때 (배치 구성 확인 필요).

## Code Examples

### Voice Card JSON 구조
```json
// Source: D-02 voice card 3필드 설계 + ability-score voice guide 패턴 [VERIFIED: v2_base_prompt.md]
{
  "Snell": {
    "speech_style": "조용하고 신중한 어조, 짧은 문장 선호",
    "honorific": "평어",
    "personality": "내성적, 관찰력 있음, 가끔 날카로운 유머"
  },
  "Viira": {
    "speech_style": "당당하고 직설적인 화법, 명령형 사용",
    "honorific": "반말",
    "personality": "자신감, 리더십, 실용적"
  }
}
```

### ClusterTask 확장
```go
// Source: clustertranslate/types.go 확장 [VERIFIED: 코드 직접 확인]
type ClusterTask struct {
    Batch            inkparse.Batch
    PrevGateLines    []string          // prev 3줄 EN context (기존)
    NextLines        []string          // next 3줄 EN context (신규)
    PrevKO           []string          // prev 3줄 기존 KO 번역 (재번역 시, 신규)
    NextKO           []string          // next 3줄 기존 KO 번역 (재번역 시, 신규)
    GlossaryJSON     string            // per-batch glossary terms (기존)
    VoiceCards       map[string]string  // speaker -> voice guide text (신규)
    ParentChoiceText string            // 부모 선택지 텍스트 (신규)
}
```

### BuildScriptPrompt 확장 -- [CONTEXT] 블록에 분기 맥락 + next 라인 추가
```go
// Source: prompt.go BuildScriptPrompt 패턴 [VERIFIED: 코드 직접 확인]

// [CONTEXT] 블록 확장 (기존 PrevGateLines 뒤에 추가):
// 1. ParentChoiceText가 있으면: [CONTEXT] Player chose: "선택지 텍스트"
// 2. NextLines가 있으면: [CONTEXT] (다음 줄 -- 번역하지 마세요) [C+1] "next line"
// 3. PrevKO/NextKO가 있으면: [CONTEXT] (이전 번역 참고) [K1] "이전 한국어 번역"

// Voice section 확장 (기존 ability-score 뒤에 추가):
// Named character voice cards도 "## Voice Guide" 섹션에 통합
```

### store.go 신규 쿼리 -- sort_index 기반 next 라인 + ko_formatted 조회
```go
// Source: GetPrevGateLines 패턴 확장 [VERIFIED: 코드 직접 확인]

// GetNextLines: sort_index > MAX(current gate) 기준으로 다음 N줄 조회
// GetAdjacentKO: sort_index 기준 앞뒤 아이템의 ko_formatted 조회
//   - WHERE state = 'done' AND ko_formatted IS NOT NULL
//   - 재번역 시에만 유효 (최초 번역 시 NULL 반환)
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| flat 24-rule list | v2Sections 4-tier (Phase 06) | 2026-04-06 | Phase 07에서 각 tier에 컨텍스트 삽입 가능 |
| PrevGateLines prev 3줄만 | prev/next 3줄 양방향 윈도우 | Phase 07 | 전후 맥락 모두 LLM에 제공 |
| ability-score voice만 | ability-score + named character voice card | Phase 07 | 상위 15명 캐릭터의 톤 일관성 향상 |
| 분기 맥락 없음 | ParentChoiceText "Player chose: X" | Phase 07 | 선택지 분기 후 대화가 맥락을 인지 |

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | voice card 3필드(말투/존댓말/성격)로 LLM이 일관된 톤을 생성할 수 있다 | Voice Card | LOW -- 필드가 부족하면 추가 가능, 기존 ability-score 6개 필드가 동작 검증됨 |
| A2 | sort_index 기반 prev/next 쿼리가 동일 씬 내 순서를 정확히 반영한다 | Continuity Window | LOW -- sort_index는 파싱 순서와 일치하도록 설계됨 [VERIFIED: Seed 함수에서 순차 삽입] |
| A3 | 캐릭터당 대사 샘플 20개면 voice card 생성에 충분하다 | Voice Card Generation | MEDIUM -- 캐릭터에 따라 다양한 톤이 있을 수 있음. 부족하면 샘플 수 증가 필요 |
| A4 | choice container의 "s" 배열에 display text가 항상 존재한다 | Branch Context | LOW -- ink 스펙에서 flg & 0x2일 때만 display text 존재 확인됨 [VERIFIED: parser.go tryExtractChoiceText] |

## Open Questions (RESOLVED)

1. **Voice card 생성 도구의 형태** -- RESOLVED
   - What we know: Go CLI가 DB + LLM 접근 모두 가능, Python도 psql + LLM 접근 가능
   - Resolution: Go CLI로 확정 (Plan 01 Task 2: `workflow/cmd/go-generate-voice-cards/main.go`). 기존 SessionLLMClient + DB 접근 패턴 재사용. Python 대비 빌드 일관성 + LLM 클라이언트 재사용 이점.

2. **토큰 예산 상한값** -- RESOLVED
   - What we know: Phase 06에서 토큰 프로파일링 데이터 수집 완료 (estimateTokens 함수 존재)
   - Resolution: `contextBudgetTokens = 4000` 상수로 확정 (Plan 03 Task 1). 근거: 기존 프롬프트 평균 ~2000 토큰 + 최대 900 토큰 추가(voice card 200 + branch 100 + continuity 600) = ~3000, 여유분 포함 4000. Claude's Discretion 영역으로 프로파일링 기반 결정.

3. **ParentChoiceText가 있는 아이템의 비율** -- RESOLVED
   - What we know: ink 파서에서 Choice 필드가 비어있지 않은 블록이 존재
   - Resolution: 정확한 비율은 실행 시 DB 쿼리로 확인 가능하나, 플랜 수립에는 영향 없음. choice container 내 블록에만 ParentChoiceText가 채워지고, 나머지는 빈 문자열(기본값). 프롬프트 주입 시 빈 문자열이면 스킵하므로 비율에 관계없이 동작 정확.

## Project Constraints (from CLAUDE.md)

- Go 코드: `workflow/` 및 `projects/*/cmd/` 하위 -- 모든 수정 이 범위 내
- 명명 규칙: flat lowercase 패키지명, snake_case.go 파일, PascalCase/camelCase 필드
- 모듈 설계: types.go에 Config/타입, feature.go에 로직, _test.go 코-로케이션
- DB 규칙: source_raw 기준 중복 체크 필수
- LLM 백엔드: OpenCode 서버 (gpt-5.4 번역) -- voice card 생성에도 동일 백엔드 사용
- 에러 처리: `fmt.Fprintf(os.Stderr, ...)` + return 1/2 패턴
- GSD 워크플로 준수: Edit/Write 전 GSD 명령 시작

## Sources

### Primary (HIGH confidence)
- `workflow/internal/clustertranslate/prompt.go` -- v2Sections 4-tier 구조, buildVoiceSection, estimateTokens [직접 확인]
- `workflow/internal/clustertranslate/types.go` -- ClusterTask, PromptMeta 구조 [직접 확인]
- `workflow/internal/inkparse/parser.go` -- walkContainer, tryExtractChoiceText, choice 구조 [직접 확인]
- `workflow/internal/inkparse/types.go` -- DialogueBlock 필드 목록 [직접 확인]
- `workflow/internal/v2pipeline/store.go` -- GetPrevGateLines, 스키마, sort_index 구조 [직접 확인]
- `workflow/internal/v2pipeline/worker.go` -- translateBatch 흐름, ClusterTask 조합 [직접 확인]
- `workflow/internal/contracts/v2pipeline.go` -- V2PipelineItem, V2PipelineStore 인터페이스 [직접 확인]
- `workflow/internal/translation/lore.go` -- JSON 로드-매칭-주입 패턴 [직접 확인]
- `projects/esoteric-ebb/context/speaker_allow_list.json` -- 상위 15명 화자 목록 + 빈도 [직접 확인]
- `projects/esoteric-ebb/context/v2_base_prompt.md` -- ability-score voice guide [직접 확인]

### Secondary (MEDIUM confidence)
- `.planning/phases/06-foundation-cli/06-RESEARCH.md` -- Phase 06 연구 결과, 프롬프트 계층화 결정
- `.planning/phases/07-context-enrichment/07-CONTEXT.md` -- D-01~D-09 사용자 결정
- `.planning/research/PITFALLS.md` -- P1(클러스터 깨짐), P2(speaker 오탐) 위험 요소

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- go.mod 변경 없음, 모든 패턴이 기존 코드에서 검증됨
- Architecture: HIGH -- 모든 확장점(BuildScriptPrompt, ClusterTask, store.go, parser.go)을 직접 확인
- Pitfalls: HIGH -- 실제 코드 구조에서 도출, v1 post-mortem 경험 반영

**Research date:** 2026-04-07
**Valid until:** 2026-05-07 (안정적 스택, 1개월)
