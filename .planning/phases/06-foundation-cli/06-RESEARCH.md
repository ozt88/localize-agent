# Phase 06: Foundation -- 프롬프트 재구조화 + 화자 검증 + 재번역 CLI - Research

**Researched:** 2026-04-06
**Domain:** Go pipeline 프롬프트 엔지니어링, DB 스키마 확장, CLI 도구
**Confidence:** HIGH

## Summary

Phase 06은 기존 v2 파이프라인 코드베이스 위에서 세 가지 작업을 수행한다: (1) `v2StaticRules` 9개 규칙을 4개 섹션(컨텍스트/보이스/태스크/제약)으로 계층화, (2) `pipeline_items_v2`의 `speaker` 컬럼 데이터를 감사하여 allow-list JSON 생성, (3) `score_final` 기반 재번역 후보 선택 CLI 구축. 모든 작업은 Go 표준 라이브러리만으로 구현 가능하며 외부 의존성 추가가 없다.

핵심 코드 변경 지점은 `clustertranslate/prompt.go` (프롬프트 구조), `contracts/v2pipeline.go` + `v2pipeline/store.go` (재번역 쿼리/상태 메서드), 그리고 신규 CLI `cmd/go-retranslate-select/main.go`이다. v1 파이프라인(`translationpipeline/`)에 이미 `StatePendingRetranslate` 상태가 존재하나 v2 파이프라인(`v2pipeline/`)에는 없으므로, v2에 재번역 상태를 추가해야 한다.

**Primary recommendation:** `v2StaticRules` 계층화를 첫 번째로, 화자 allow-list를 두 번째로, 재번역 CLI를 마지막으로 구현하라. 각 단계가 다음 단계의 전제 조건이다.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** 재번역은 클러스터 번역 경로(`BuildScriptPrompt` + `v2StaticRules`)로 단일화. 개별 번역 프롬프트(`defaultStaticRules`)는 UI/overlay 용도로만 유지, 재구조화 대상 아님.
- **D-02:** `v2StaticRules`(9개)를 컨텍스트/보이스/태스크/제약 4개 섹션으로 계층화. Phase 07에서 voice card, branch context, continuity window가 각 섹션에 삽입될 수 있도록 구조 설계.
- **D-03:** ability-score voice guide는 워밍업에 전체 가이드 포함(`BuildBaseWarmup`) + per-batch에 해당 캐릭터 voice guide만 강조 주입(`BuildScriptPrompt`). 두 레이어 모두 적용.
- **D-04:** 2단계 화자 검증: (1) DB에서 `DISTINCT speaker` + 빈도 분포 추출 -> (2) 빈도 낮은(1-2회) 의심 항목은 ink JSON 소스와 교차 검증하여 오인식 필터링.
- **D-05:** 검증된 화자 allow-list를 JSON 파일로 관리. `isSpeakerTag` 오인식을 필터링하는 데 사용.
- **D-06:** 화자 커버리지 목표 90%+ (대화 라인 대비). 미달 시 ink 파서의 `#` 태그 파싱 로직 강화.
- **D-07:** ScoreFinal threshold는 score_final 히스토그램 분포 분석 후 자연스러운 cutoff 지점에서 결정. 고정값이 아닌 데이터 기반.
- **D-08:** 재번역 단위는 반드시 batch_id 전체 클러스터. 개별 라인 재번역 금지 (P1 pitfall: 톤 불일치 방지).
- **D-09:** `retranslation_gen` 컬럼을 pipeline_items_v2에 추가. 각 재번역 세대마다 gen+1. 이전 세대 데이터 유지로 롤백 가능.
- **D-10:** 재번역 CLI는 기존 파이프라인 상태 머신과 통합. `StatePendingRetranslate` 상태를 활용하여 기존 worker가 재번역도 처리.

### Claude's Discretion
- 토큰 예산: Phase 06에서는 프로파일링만 수행. 예산 상한 및 우선순위 전략은 Phase 07 discuss에서 결정.
- 프롬프트 계층 구조의 구체적 포맷 (마크다운 헤딩, 구분자 스타일 등)
- DB migration 구체 구현 (retranslation_gen 컬럼 타입, 기본값)
- score_final 히스토그램 시각화 방식

### Deferred Ideas (OUT OF SCOPE)
- Score LLM 프롬프트 개선 (맥락 인식 점수) -- v2 요구사항 SCORE-01, SCORE-02
- 고유명사 정책 통일 -- v2 요구사항 NAMING-01, NAMING-02
- 미번역 154건 해소 -- Out of Scope (별도 작업)
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PROMPT-01 | 현재 24개 flat rule을 계층 구조(컨텍스트, 보이스, 태스크, 제약)로 재구조화 | D-01에 따라 `v2StaticRules` 9개만 대상. `BuildBaseWarmup` + `BuildScriptPrompt` 구조 분석 완료. |
| PROMPT-02 | ability-score voice guide를 warmup에서 per-item 프롬프트로 통합 (speaker_hint 매칭 시) | `v2_base_prompt.md`에 6개 ability score 정의 확인. `BuildScriptPrompt`에 speaker 필드 이미 존재. |
| PROMPT-03 | 프롬프트 토큰 예산 프로파일링 및 최적화 | 현재 warmup 구조와 per-batch 구조 코드 분석 완료. 측정 로직만 추가하면 됨. |
| SPEAKER-01 | translator_package.json의 speaker_hint 커버리지 감사 (대화 라인 대비 비율 측정) | `pipeline_items_v2.speaker` 컬럼 + `content_type='dialogue'` WHERE 절로 쿼리 가능. |
| SPEAKER-02 | ink JSON # 태그 파싱 강화로 speaker_hint 커버리지 90%+ 달성 | `isSpeakerTag()` 로직 전수 분석. `isGameCommandTag()` 하드코딩 목록 확인. |
| SPEAKER-03 | 검증된 화자 allow-list 생성 (isSpeakerTag 오인식 필터링) | 2단계 검증 패턴(DB 빈도 + ink 교차) 설계 완료. JSON 파일 포맷 결정. |
| RETRANS-01 | ScoreFinal < threshold 기준 재번역 후보 쿼리 CLI 구현 | v2 store에 score 쿼리 메서드 부재 -- 신규 추가 필요. DB 인덱스 추가 필요. |
| RETRANS-02 | batch_id 단위 재번역 (개별 라인이 아닌 클러스터 전체) | `batch_id` 컬럼 + `idx_pv2_batch` 인덱스 이미 존재. 배치 단위 쿼리 패턴 확인. |
| RETRANS-03 | 재번역 전 원본 ko_formatted 스냅샷 보존 (롤백용) | `retranslation_gen` + 스냅샷 테이블/컬럼 설계 필요. |
</phase_requirements>

## Standard Stack

### Core

기존 스택 유지. 신규 의존성 없음. [VERIFIED: go.mod 직접 확인]

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib `encoding/json` | 1.24 | voice allow-list JSON 로드 | 기존 lore.go 패턴 동일 |
| Go stdlib `database/sql` | 1.24 | DB 쿼리 확장 | 기존 store.go 패턴 동일 |
| Go stdlib `flag` | 1.24 | CLI flag 파싱 | 기존 cmd/ 패턴 동일 |
| `github.com/jackc/pgx/v5` | 5.7.6 | PostgreSQL 드라이버 | 이미 사용 중 |
| `modernc.org/sqlite` | 1.38.2 | SQLite 드라이버 | 이미 사용 중 |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| 별도 마이그레이션 도구 | `ALTER TABLE` 직접 실행 | 2-3개 컬럼 추가뿐이므로 마이그레이션 도구 오버킬 |
| tiktoken Go 바인딩 | 문자열 길이 근사 | 토큰 프로파일링에 정확한 카운트 필요하나, Go tiktoken 라이브러리가 불안정. 문자 수 / 4 근사로 충분 |

## Architecture Patterns

### 변경 대상 파일 구조

```
workflow/
  internal/
    clustertranslate/
      prompt.go          # v2StaticRules 계층화 + per-batch voice 주입 (수정)
      types.go           # ClusterTask에 SpeakerVoice 필드 추가 (수정)
    contracts/
      v2pipeline.go      # 재번역 상태 상수 + 신규 메서드 시그니처 (수정)
    v2pipeline/
      store.go           # 신규 쿼리 메서드 구현 (수정)
      types.go           # StatePendingRetranslate 상수 (수정)
      postgres_v2_schema.sql  # retranslation_gen 컬럼 (수정)
  cmd/
    go-retranslate-select/
      main.go            # 신규 CLI (신규)
projects/esoteric-ebb/
  context/
    speaker_allow_list.json  # 검증된 화자 목록 (신규 데이터)
```

### Pattern 1: v2StaticRules 계층화

**What:** 현재 `v2StaticRules` 9개 flat string 슬라이스를 4개 명명 섹션으로 재구조화
**When to use:** `BuildBaseWarmup()`에서 워밍업 조립 시

현재 구조 (prompt.go:12-22):
```go
var v2StaticRules = []string{
    "1. Translate the following scene into Korean.",
    "2. Maintain the [NN] line numbers exactly as given.",
    // ... 9 rules flat
}
```

권장 구조:
```go
// v2PromptSections defines the 4-tier prompt structure for cluster translation.
// Phase 07 will inject voice cards into Voice, branch context into Context,
// and continuity window into Context.
type v2PromptSections struct {
    Context     []string // scene/branch context rules
    Voice       []string // speaker voice and tone rules
    Task        []string // what to do (translate, preserve markers)
    Constraints []string // output format constraints
}

var v2Sections = v2PromptSections{
    Context: []string{
        "[CONTEXT] lines are for reference only -- do not translate them.",
    },
    Voice: []string{
        "Match the tone and register of the original.",
    },
    Task: []string{
        "Translate the following scene into Korean.",
        "Preserve speaker labels (e.g., 'Braxo:') in your output.",
        "Preserve [CHOICE] markers in your output.",
    },
    Constraints: []string{
        "Maintain the [NN] line numbers exactly as given.",
        "Do not add, remove, or merge lines.",
        "All proper nouns (names, places, spells, abilities) stay in English.",
        "Output only the translated lines, no commentary.",
    },
}
```

[VERIFIED: clustertranslate/prompt.go 직접 분석]

### Pattern 2: Per-batch Voice Guide 주입

**What:** `BuildScriptPrompt`에서 배치 내 speaker가 ability score (wis/str/int/cha/dex/con)인 경우 해당 voice guide를 프롬프트에 주입
**When to use:** 배치에 ability score speaker가 포함된 경우에만

```go
// In BuildScriptPrompt, after building numbered lines:
func buildVoiceSection(speakers []string, voiceGuide map[string]string) string {
    seen := make(map[string]bool)
    var sb strings.Builder
    for _, s := range speakers {
        s = strings.ToLower(s)
        if voiceGuide[s] != "" && !seen[s] {
            seen[s] = true
            sb.WriteString(fmt.Sprintf("- **%s**: %s\n", s, voiceGuide[s]))
        }
    }
    if sb.Len() == 0 {
        return ""
    }
    return "## Voice Guide (이 배치의 화자)\n" + sb.String()
}
```

[VERIFIED: v2_base_prompt.md에 6개 ability score voice 정의 확인]

### Pattern 3: 재번역 CLI (기존 패턴 준수)

**What:** `cmd/go-retranslate-select/main.go` -- score 기반 후보 선택 + 상태 리셋
**When to use:** 재번역 실행 전 후보 선택 단계

기존 CLI 패턴 (go-v2-pipeline/main.go 참조):
```go
// 1. flag.NewFlagSet 생성
// 2. cfg 구조체에 바인딩
// 3. fs.Parse(os.Args[1:])
// 4. domain Run() 호출
```

재번역 CLI 고유 플래그:
```go
fs.Float64Var(&cfg.ScoreThreshold, "score-threshold", 0, "score_final 임계값 (미만 항목 선택)")
fs.BoolVar(&cfg.DryRun, "dry-run", true, "선택 결과만 출력, 상태 변경 없음")
fs.BoolVar(&cfg.Histogram, "histogram", false, "score_final 분포 히스토그램 출력")
fs.StringVar(&cfg.ContentType, "content-type", "", "특정 content_type만 필터")
```

[VERIFIED: go-v2-pipeline/main.go, go-v2-ingest/main.go 패턴 확인]

### Pattern 4: 스냅샷 보존 (retranslation_gen)

**What:** 재번역 전 `ko_formatted` 스냅샷을 별도 테이블 또는 JSONB 컬럼에 보존
**When to use:** 상태를 `pending_retranslate`로 리셋하기 직전

권장 접근: `retranslation_snapshots` 테이블 신규 생성

```sql
CREATE TABLE IF NOT EXISTS retranslation_snapshots (
    id TEXT NOT NULL,
    gen INTEGER NOT NULL,
    ko_raw TEXT,
    ko_formatted TEXT,
    score_final REAL,
    snapshot_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, gen)
);
```

이유: `pipeline_items_v2`에 스냅샷 컬럼을 추가하면 row 크기가 과도하게 증가 (40K rows x 2개 추가 TEXT 컬럼). 별도 테이블이 정규화된 설계. [ASSUMED]

### Anti-Patterns to Avoid

- **개별 라인 재번역:** batch_id 전체를 선택하지 않고 개별 item만 재번역하면 씬 내 톤 불일치 발생. 재번역 CLI는 반드시 batch_id 단위로 선택해야 한다. (P1 pitfall)
- **v2StaticRules를 동적 생성:** 프롬프트 섹션은 컴파일 타임 상수여야 한다. 런타임에 조건부로 규칙을 추가/제거하면 디버깅이 극히 어려워진다.
- **allow-list를 DB에 저장:** 화자 allow-list는 코드와 함께 버전 관리되어야 한다. DB에 저장하면 리뷰 불가능. JSON 파일이 적절.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| 토큰 카운팅 | 정확한 tokenizer 구현 | `len(text) / 4` 근사 (영어) 또는 `len([]rune(text)) / 2` (한국어) | Phase 06은 프로파일링만, 정확한 예산 관리는 Phase 07 |
| DB 마이그레이션 | 마이그레이션 프레임워크 | `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` 직접 SQL | 컬럼 2-3개 추가뿐 |
| 히스토그램 시각화 | 차트 라이브러리 | 터미널 ASCII 히스토그램 (20줄 미만 코드) | CLI 출력용, 정밀 시각화 불필요 |
| 화자 검증 자동화 | ML 기반 화자 분류기 | DB 쿼리 + 수동 검증 | 50개 미만 고유 화자, 수동이 정확도 최고 |

## Common Pitfalls

### Pitfall 1: v2 파이프라인에 StatePendingRetranslate 부재

**What goes wrong:** CONTEXT.md D-10은 `StatePendingRetranslate`를 활용하라고 명시하지만, 이 상태는 v1 파이프라인(`translationpipeline/types.go:10`)에만 존재. v2 파이프라인(`contracts/v2pipeline.go`, `v2pipeline/types.go`)에는 정의되어 있지 않다.
**Why it happens:** v2 파이프라인은 v1의 재번역 로직을 포팅하지 않은 상태로 v1.0이 완료됨.
**How to avoid:** `contracts/v2pipeline.go`에 `StatePendingRetranslate = "pending_retranslate"` 추가. `v2pipeline/types.go`에 re-export 추가. `V2PipelineStore` 인터페이스에 `ResetForRetranslation(batchID string, gen int) error` 메서드 추가. Store 구현에서 이 메서드가 ko_raw/ko_formatted 스냅샷을 저장한 후 상태를 리셋. [VERIFIED: contracts/v2pipeline.go 직접 확인 -- 재번역 상태 없음]
**Warning signs:** "pending_retranslate" 상태로 설정한 항목이 기존 TranslateWorker에 의해 claim되지 않음.

### Pitfall 2: TranslateWorker의 ClaimPending이 pending_retranslate를 무시

**What goes wrong:** `TranslateWorker`는 `store.ClaimPending(StatePendingTranslate, ...)` 을 호출. `pending_retranslate` 상태 항목을 claim하려면 별도 호출이 필요하거나, 상태를 `pending_translate`로 리셋해야 한다.
**Why it happens:** `ClaimPending`은 명시적으로 `pendingState` 파라미터를 받아 해당 상태만 claim.
**How to avoid:** 두 가지 선택지: (A) 재번역 CLI가 상태를 `pending_translate`로 리셋 (기존 worker가 그대로 처리), (B) `pending_retranslate` 상태를 유지하고 worker 루프에 추가 claim 호출. D-10에 따르면 "기존 worker가 재번역도 처리"이므로 **(A)가 적합** -- `ResetForRetranslation`이 스냅샷 저장 후 state를 `pending_translate`로 설정. [VERIFIED: worker.go:36 ClaimPending 호출 확인]
**Warning signs:** 재번역 후보 선택 후 worker가 아무 것도 처리하지 않음.

### Pitfall 3: score_final 인덱스 부재로 쿼리 성능 저하

**What goes wrong:** `SELECT * FROM pipeline_items_v2 WHERE score_final < ? AND state = 'done'` 쿼리가 40K rows 풀 스캔.
**Why it happens:** 현재 인덱스: `idx_pv2_state`, `idx_pv2_state_lease`, `idx_pv2_source_hash`, `idx_pv2_batch`. score_final 인덱스 없음.
**How to avoid:** `CREATE INDEX IF NOT EXISTS idx_pv2_score ON pipeline_items_v2(score_final) WHERE state = 'done'` 추가. [VERIFIED: postgres_v2_schema.sql 직접 확인]

### Pitfall 4: 워밍업과 per-batch 간 voice guide 중복

**What goes wrong:** D-03에 따라 워밍업에 전체 voice guide, per-batch에 해당 화자 voice guide를 주입. 그런데 워밍업에 이미 `v2_base_prompt.md` 전체(ability score voices 포함)가 포함되어 있으므로 per-batch에서 같은 내용을 반복하면 토큰 낭비.
**Why it happens:** `BuildBaseWarmup`은 `systemPrompt` 파라미터로 `v2_base_prompt.md` 전체를 받음. 이미 ability score 정의가 포함.
**How to avoid:** per-batch 주입은 워밍업의 전체 가이드를 반복하지 않고, "이 배치에서 주의할 화자: wis, str" 형태의 짧은 리마인더로 제한. 토큰 절약. [VERIFIED: prompt.go BuildBaseWarmup + v2_base_prompt.md 구조 확인]

### Pitfall 5: retranslation_gen 컬럼의 소급 적용

**What goes wrong:** 기존 40K items 전체의 `retranslation_gen`이 0인 상태에서 재번역 후 gen=1이 됨. 하지만 gen=0 항목의 ko_formatted 스냅샷은 retranslation_snapshots에 없음.
**Why it happens:** ALTER TABLE ADD COLUMN은 기존 row에 기본값만 설정, 스냅샷은 없음.
**How to avoid:** gen=0은 "원본 번역"을 의미하며, 원본 ko_formatted는 pipeline_items_v2에 이미 있으므로 별도 스냅샷 불필요. `ResetForRetranslation()`이 gen 증가 전에 현재 값을 스냅샷 테이블에 복사하면 됨. [ASSUMED]

## Code Examples

### 1. score_final 히스토그램 쿼리

```sql
-- score_final 분포를 0.5 간격 버킷으로 분석
SELECT
    FLOOR(score_final * 2) / 2 AS bucket,
    COUNT(*) AS cnt
FROM pipeline_items_v2
WHERE state = 'done' AND score_final >= 0
GROUP BY bucket
ORDER BY bucket;
```

[VERIFIED: pipeline_items_v2 스키마 확인, score_final REAL 타입]

### 2. 화자 커버리지 감사 쿼리

```sql
-- 대화 라인 대비 speaker 커버리지
SELECT
    COUNT(*) AS total_dialogue,
    COUNT(CASE WHEN speaker != '' THEN 1 END) AS with_speaker,
    ROUND(100.0 * COUNT(CASE WHEN speaker != '' THEN 1 END) / COUNT(*), 1) AS coverage_pct
FROM pipeline_items_v2
WHERE content_type = 'dialogue';

-- 고유 화자 + 빈도 분포
SELECT speaker, COUNT(*) AS cnt
FROM pipeline_items_v2
WHERE speaker != ''
GROUP BY speaker
ORDER BY cnt DESC;
```

[VERIFIED: pipeline_items_v2.speaker, content_type 컬럼 확인]

### 3. batch_id 단위 재번역 후보 선택

```sql
-- score_final < threshold인 항목이 1개라도 있는 batch_id 전체 선택
SELECT DISTINCT p.batch_id
FROM pipeline_items_v2 p
WHERE p.state = 'done'
  AND p.batch_id IN (
    SELECT batch_id FROM pipeline_items_v2
    WHERE state = 'done' AND score_final >= 0 AND score_final < $1
  );
```

[VERIFIED: batch_id 컬럼 + idx_pv2_batch 인덱스 존재 확인]

### 4. ResetForRetranslation store 메서드 패턴

```go
// ResetForRetranslation snapshots current translations and resets a batch for retranslation.
func (s *Store) ResetForRetranslation(batchID string, gen int) (int, error) {
    tx, err := s.db.Begin()
    if err != nil {
        return 0, err
    }
    defer tx.Rollback()

    // 1. Snapshot current translations
    _, err = tx.Exec(s.rebind(`
        INSERT INTO retranslation_snapshots (id, gen, ko_raw, ko_formatted, score_final, snapshot_at)
        SELECT id, ?, ko_raw, ko_formatted, score_final, ?
        FROM pipeline_items_v2
        WHERE batch_id = ? AND state = 'done'`),
        gen, s.nowValue(), batchID,
    )
    if err != nil {
        return 0, fmt.Errorf("snapshot: %w", err)
    }

    // 2. Reset items to pending_translate
    now := s.nowValue()
    result, err := tx.Exec(s.rebind(`
        UPDATE pipeline_items_v2
        SET state = 'pending_translate',
            ko_raw = NULL, ko_formatted = NULL,
            score_final = -1, failure_type = '',
            translate_attempts = 0, format_attempts = 0, score_attempts = 0,
            retranslation_gen = retranslation_gen + 1,
            claimed_by = '', claimed_at = NULL, lease_until = NULL,
            updated_at = ?
        WHERE batch_id = ? AND state = 'done'`),
        now, batchID,
    )
    if err != nil {
        return 0, fmt.Errorf("reset: %w", err)
    }

    if err := tx.Commit(); err != nil {
        return 0, err
    }
    n, _ := result.RowsAffected()
    return int(n), nil
}
```

[VERIFIED: store.go 기존 트랜잭션 패턴 + rebind 메서드 확인]

### 5. 토큰 프로파일링 유틸

```go
// estimateTokens returns an approximate token count.
// English: ~4 chars/token, Korean: ~2 chars/token.
func estimateTokens(text string) int {
    runes := []rune(text)
    koreanCount := 0
    for _, r := range runes {
        if r >= 0xAC00 && r <= 0xD7AF { // Hangul syllables
            koreanCount++
        }
    }
    englishChars := len(runes) - koreanCount
    return englishChars/4 + koreanCount/2
}
```

[ASSUMED -- 토큰/문자 비율은 모델별로 다르나, 프로파일링 목적에는 근사치로 충분]

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| v1 `translationpipeline` 재번역 | v2 `v2pipeline` 재번역 (신규) | Phase 06 | v2 상태 머신에 재번역 경로 추가 필요 |
| flat rule list (v2StaticRules) | 4-section hierarchical prompt | Phase 06 | Phase 07 컨텍스트 주입의 기반 |
| isSpeakerTag heuristic-only | heuristic + allow-list filter | Phase 06 | 오인식 방지 |

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | retranslation_snapshots 별도 테이블이 인라인 컬럼보다 적합 | Architecture Pattern 4 | LOW -- 둘 다 동작하며, 컬럼 방식도 가능. 성능 차이 미미 (40K rows) |
| A2 | gen=0 원본 번역은 스냅샷 불필요 (pipeline_items_v2에 이미 존재) | Pitfall 5 | LOW -- ResetForRetranslation이 gen 증가 전 스냅샷 저장하므로 gen=0 복원 가능 |
| A3 | 토큰 근사치 len/4 (영어) + len/2 (한국어) | Code Example 5 | LOW -- 프로파일링 목적이므로 10-20% 오차 허용 |
| A4 | 재번역 상태를 pending_translate로 리셋하는 것이 별도 pending_retranslate 유지보다 적합 | Pitfall 2 | MEDIUM -- D-10이 "StatePendingRetranslate 활용"을 명시하나, 기존 worker 재사용을 위해 pending_translate 리셋이 더 간결. 다만 재번역 추적을 위해 retranslation_gen > 0으로 구분 가능 |

## Open Questions

1. **재번역 상태 전략: pending_translate vs pending_retranslate**
   - What we know: D-10은 `StatePendingRetranslate` 활용을 명시. 그러나 v2 TranslateWorker는 `StatePendingTranslate`만 claim. 별도 상태를 만들면 worker 루프 수정 필요.
   - What's unclear: 사용자가 의도한 것이 (A) 추적용 구분 상태인지 (B) 별도 처리 로직인지
   - Recommendation: `pending_translate`로 리셋하되, `retranslation_gen > 0`으로 구분. Worker 변경 최소화. 만약 사용자가 별도 워밍업/프롬프트를 원하면 Phase 07에서 확장.

2. **score_final 분포 실측**
   - What we know: 평균 90.7이라는 리서치 데이터
   - What's unclear: 실제 히스토그램 형태, 자연 cutoff 지점
   - Recommendation: CLI에 `--histogram` 모드 구현하여 실행 시 데이터 기반 결정

## Project Constraints (from CLAUDE.md)

- **LLM 백엔드**: OpenCode 서버 (gpt-5.4 번역, codex-mini 포맷팅) -- 변경 없음
- **게임 버전**: 1.1.3 고정, ink JSON 구조 변경 없음
- **DB 규칙**: source_raw 기준 중복 체크 필수, 맹목적 INSERT 금지
- **Go 컨벤션**: PascalCase 내보내기, camelCase 비공개, snake_case.go 파일명
- **CLI 패턴**: `cmd/go-*/main.go` -> flag 파싱 -> config 로드 -> domain `Run()`
- **테스트**: `<source_file>_test.go` co-located, `testing` 표준 패키지
- **인터페이스**: contracts/ 패키지에 정의, 컴파일 타임 검증 (`var _ Interface = (*impl)(nil)`)
- **에러 처리**: `fmt.Fprintf(os.Stderr, ...) return 1/2` 패턴
- **GSD 워크플로**: Edit/Write 전 GSD 명령으로 시작

## Sources

### Primary (HIGH confidence)

- `workflow/internal/clustertranslate/prompt.go` -- v2StaticRules 9개 규칙, BuildBaseWarmup, BuildScriptPrompt 전체 분석
- `workflow/internal/clustertranslate/types.go` -- ClusterTask 구조체, PromptMeta
- `workflow/internal/contracts/v2pipeline.go` -- V2PipelineItem (ScoreFinal 필드), V2PipelineStore 인터페이스, 상태 상수
- `workflow/internal/v2pipeline/store.go` -- 전체 메서드 목록, 트랜잭션 패턴, rebind 패턴
- `workflow/internal/v2pipeline/worker.go` -- TranslateWorker ClaimPending 호출, translateBatch 플로우
- `workflow/internal/v2pipeline/types.go` -- Config, 상태 상수 re-export (재번역 상태 부재 확인)
- `workflow/internal/v2pipeline/postgres_v2_schema.sql` -- 현재 스키마 (인덱스 포함)
- `workflow/internal/v2pipeline/export.go` -- BuildV3Sidecar dedup 로직
- `workflow/internal/inkparse/parser.go` -- isSpeakerTag 휴리스틱, isGameCommandTag 하드코딩 목록
- `workflow/internal/translation/skill.go` -- defaultStaticRules 24개 (재구조화 대상 아님 확인)
- `workflow/internal/translation/lore.go` -- JSON 로드 + term 매칭 + 프롬프트 주입 패턴
- `workflow/internal/translationpipeline/types.go` -- v1 StatePendingRetranslate 존재 확인
- `projects/esoteric-ebb/context/v2_base_prompt.md` -- ability score voices 6개 정의
- `workflow/cmd/go-v2-pipeline/main.go` -- CLI 패턴 참조
- `.planning/research/PITFALLS.md` -- 7개 pitfall 전체 분석

### Secondary (MEDIUM confidence)

- `.planning/research/SUMMARY.md` -- 프로젝트 리서치 종합
- `.knowledge/raw/2026-04-06.md` -- 세션 기록

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- 신규 의존성 없음, 기존 코드 직접 확인
- Architecture: HIGH -- 변경 대상 파일 전수 분석, 기존 패턴 재사용
- Pitfalls: HIGH -- v2 코드베이스 직접 분석으로 재번역 상태 부재 발견

**Research date:** 2026-04-06
**Valid until:** 2026-05-06 (안정적 코드베이스, 외부 의존성 없음)
