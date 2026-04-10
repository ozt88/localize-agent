---
phase: 07-context-enrichment
verified: 2026-04-07T18:00:00Z
status: human_needed
score: 3/4 must-haves verified
human_verification:
  - test: "A/B 테스트 점수 결과 사용자 검토"
    expected: "9개 배치 모두 컨텍스트 주입 후 점수 하락 없음 (ab_test_baseline.txt 기준)"
    why_human: "ab_test_baseline.txt에 결과가 기록되어 있으나, 재번역이 실제로 실행되었는지 (파이프라인 실제 구동 여부), 점수 개선이 voice card / branch context 주입에 의한 것인지 코드만으로는 검증 불가. 사용자가 DB 쿼리로 실제 score_final 값을 확인해야 한다."
---

# Phase 07: Context Enrichment — 톤 프로필 + 분기 맥락 + 연속성 윈도우 Verification Report

**Phase Goal:** 번역 프롬프트에 캐릭터 voice card, 분기 맥락(parent choice text), 연속성 윈도우(next lines, prev/next KO)를 주입하여 번역 품질을 개선한다.
**Verified:** 2026-04-07T18:00:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (ROADMAP Success Criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | 캐릭터별 voice card JSON이 존재하고, speaker_hint 매칭 시 해당 캐릭터의 말투/존댓말 레벨/성격이 번역 프롬프트에 주입된다 | VERIFIED | voice_cards.json 15명 데이터 확인, prompt.go Named Character Voice Guide 섹션 확인, TestBuildScriptPrompt_NamedVoiceCards 통과 |
| 2 | 분기 대화에서 부모 선택지 텍스트가 "Player chose: X" 형태로 프롬프트에 포함되며, 브랜치 깊이 1단계 + 토큰 예산 내로 제한된다 | VERIFIED | prompt.go `Player chose: %q` 패턴 확인, trimContextForBudget 구현 확인, inkparse ParentChoiceText 테스트 4개 통과 |
| 3 | neighborPromptText가 prev/next 3줄 슬라이딩 윈도우로 확장되고, 재번역 시 기존 한국어 번역이 prevKO/nextKO에 채워진다 | VERIFIED | GetNextLines/GetAdjacentKO 구현 + 테스트 통과, worker.go에서 호출 확인, prompt.go [N1]/[K1]/[NK1] 패턴 확인 |
| 4 | 소규모 A/B 테스트에서 컨텍스트 주입 후 번역 점수가 주입 전 대비 하락하지 않는다 (프롬프트 크기 회귀 없음) | HUMAN_NEEDED | ab_test_baseline.txt에 결과 파일 존재 (9개 배치 전부 개선 기록), 그러나 실제 DB 재번역 실행 여부는 코드로 검증 불가 |

**Score:** 3/4 truths verified (4번은 human 검증 필요)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `workflow/internal/clustertranslate/voice_card.go` | VoiceCard 타입 + LoadVoiceCards + BuildNamedVoiceSection | VERIFIED | 3필드 구조체, LoadVoiceCards(os.ReadFile+json.Unmarshal), BuildNamedVoiceSection 모두 구현됨 |
| `workflow/internal/clustertranslate/voice_card_test.go` | 7개 테스트 케이스 | VERIFIED | 7개 테스트 함수 확인, 전체 통과 |
| `workflow/cmd/go-generate-voice-cards/main.go` | voice card 자동 생성 CLI | VERIFIED | 파일 존재, ability-score 제외 로직, LLM 프롬프트 구현 확인 |
| `projects/esoteric-ebb/context/voice_cards.json` | 15명 캐릭터 데이터 (Snell 포함) | VERIFIED | Snell~Arn 15명 전원 speech_style/honorific/personality 3필드 확인 |
| `workflow/internal/inkparse/types.go` | DialogueBlock.ParentChoiceText 필드 | VERIFIED | `ParentChoiceText string json:"parent_choice_text,omitempty"` 확인 |
| `workflow/internal/contracts/v2pipeline.go` | V2PipelineItem.ParentChoiceText + GetNextLines/GetAdjacentKO 인터페이스 | VERIFIED | 모든 필드와 메서드 시그니처 확인 |
| `workflow/internal/v2pipeline/store.go` | GetNextLines, GetAdjacentKO 구현 + parent_choice_text 컬럼 + Seed 확장 | VERIFIED | 851/884 라인에 구현, SQLite 스키마에 컬럼 확인, Seed INSERT에 parent_choice_text 포함 |
| `workflow/internal/clustertranslate/types.go` | ClusterTask 5개 신규 필드 | VERIFIED | NextLines/PrevKO/NextKO/VoiceCards/ParentChoiceText 모두 확인 |
| `workflow/internal/clustertranslate/prompt.go` | BuildScriptPrompt 5종 컨텍스트 주입 + trimContextForBudget | VERIFIED | Player chose / [N1] / [K1] / [NK1] / Named Character Voice Guide / trimContextForBudget / contextBudgetTokens=4000 모두 확인 |
| `workflow/internal/v2pipeline/types.go` | Config.VoiceCardsPath + VoiceCards 필드 | VERIFIED | Phase 07 코멘트와 함께 두 필드 확인 |
| `workflow/internal/v2pipeline/worker.go` | translateBatch에서 5종 컨텍스트 조합 + voice card 로드 | VERIFIED | GetNextLines/GetAdjacentKO/ParentChoiceText/cfg.VoiceCards 모두 translateBatch에 있음 |
| `workflow/cmd/go-v2-pipeline/main.go` | -voice-cards CLI 플래그 | VERIFIED | `fs.StringVar(&cfg.VoiceCardsPath, "voice-cards", ...)` 확인 |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `worker.go` | `prompt.go` | `clustertranslate.BuildScriptPrompt(task)` 호출 | WIRED | worker.go 152라인에서 `prompt, meta := clustertranslate.BuildScriptPrompt(task)` 확인 |
| `worker.go` | `store.go` | `store.GetNextLines`, `store.GetAdjacentKO` 호출 | WIRED | 124/132라인에서 호출 확인 |
| `prompt.go` | `voice_card.go` | `BuildNamedVoiceSection` 사용 (또는 inline) | WIRED | prompt.go에서 VoiceCards map을 직접 순회하여 "Named Character Voice Guide" 섹션 생성 (BuildNamedVoiceSection은 prompt.go 내 inline 패턴으로 대체) |
| `worker.go` | `voice_card.go` | `clustertranslate.LoadVoiceCards(cfg.VoiceCardsPath)` 호출 | WIRED | worker.go 28라인에서 TranslateWorker 시작 시 1회 로드 확인 |
| `go-v2-pipeline/main.go` | `v2pipeline/types.go` | `-voice-cards` 플래그 → `Config.VoiceCardsPath` 설정 | WIRED | main.go에서 `cfg.VoiceCardsPath`에 플래그 바인딩 확인 |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|-------------------|--------|
| `prompt.go BuildScriptPrompt` | task.VoiceCards | worker.go: LoadVoiceCards → map[string]string 변환 → cfg.VoiceCards | voice_cards.json (15명 데이터) | FLOWING |
| `prompt.go BuildScriptPrompt` | task.ParentChoiceText | worker.go: items[0].ParentChoiceText ← DB parent_choice_text ← Seed ← inkparse | inkparse.walker.currentChoiceText | FLOWING |
| `prompt.go BuildScriptPrompt` | task.NextLines | worker.go: store.GetNextLines → DB sort_index 기반 쿼리 | pipeline_items_v2.source_raw | FLOWING |
| `prompt.go BuildScriptPrompt` | task.PrevKO/NextKO | worker.go: store.GetAdjacentKO (RetranslationGen>0 조건) | pipeline_items_v2.ko_formatted (state=done) | FLOWING — 단, 최초 번역 시는 RetranslationGen=0이므로 PrevKO/NextKO는 빈 슬라이스 (의도된 동작) |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| voice card 함수 테스트 통과 (7개) | `go test ./workflow/internal/clustertranslate/ -run TestLoadVoiceCards\|TestBuildNamedVoiceSection -v` | 7개 모두 PASS | PASS |
| inkparse ParentChoiceText 테스트 통과 (4개) | `go test ./workflow/internal/inkparse/ -run TestParentChoiceText -v` | 4개 모두 PASS | PASS |
| store GetNextLines/GetAdjacentKO/Seed 테스트 통과 (8개) | `go test ./workflow/internal/v2pipeline/ -run TestGetNextLines\|TestGetAdjacentKO\|TestSeed -v` | 8개 모두 PASS | PASS |
| BuildScriptPrompt 컨텍스트 주입 테스트 통과 (13개 신규 포함) | `go test ./workflow/internal/clustertranslate/ -run TestBuildScriptPrompt\|TestTrimContext -v` | 14개 모두 PASS | PASS |
| 전체 빌드 성공 | `go build ./workflow/...` | 출력 없음 (성공) | PASS |
| A/B 테스트 결과 파일 존재 | `ls projects/esoteric-ebb/context/ab_test_baseline.txt` | 파일 존재 | PASS |

### Requirements Coverage

REQUIREMENTS.md 파일이 존재하지 않음. 요구사항 정의는 ROADMAP.md의 requirements 필드와 각 PLAN.md의 requirements 필드에서 파생.

| Requirement ID | Source Plan | 관련 Success Criteria | Status | Evidence |
|---------------|-------------|----------------------|--------|----------|
| TONE-01 | 07-01-PLAN.md | SC-1: voice card JSON 존재 + 프롬프트 주입 | SATISFIED | voice_cards.json 15명 확인, LoadVoiceCards/BuildNamedVoiceSection 구현 + 7개 테스트 통과 |
| TONE-02 | 07-03-PLAN.md | SC-1: speaker_hint 매칭 시 말투/존댓말/성격 주입 | SATISFIED | prompt.go Named Character Voice Guide 섹션, worker.go VoiceCards 조합 확인 |
| BRANCH-01 | 07-02-PLAN.md | SC-2: 부모 선택지 텍스트 추출 | SATISFIED | inkparse ParentChoiceText 필드 + extractChoiceDisplayText + 4개 테스트 통과 |
| BRANCH-02 | 07-03-PLAN.md | SC-2: "Player chose: X" 형태로 프롬프트 포함 | SATISFIED | prompt.go `[CONTEXT] Player chose: %q` 패턴 확인 |
| BRANCH-03 | 07-03-PLAN.md | SC-2: 브랜치 깊이 1단계 + 토큰 예산 내 제한 | SATISFIED | inkparse currentChoiceText 복원(D-05), trimContextForBudget(D-08) 구현 확인 |
| CONT-01 | 07-02-PLAN.md, 07-03-PLAN.md | SC-3: next 3줄 슬라이딩 윈도우 | SATISFIED | GetNextLines 구현 + [N1]/[N2] 프롬프트 주입 확인 |
| CONT-02 | 07-02-PLAN.md, 07-03-PLAN.md | SC-3: 재번역 시 prevKO/nextKO 채움 | SATISFIED | GetAdjacentKO 구현 + RetranslationGen>0 조건 worker.go에서 호출 확인 |

**모든 7개 요구사항 SATISFIED.** REQUIREMENTS.md 파일 자체가 없으나 ROADMAP.md 매핑과 완전 일치.

### Anti-Patterns Found

| File | Pattern | Severity | Impact |
|------|---------|----------|--------|
| `workflow/internal/v2pipeline/store.go` | `ScoreHistogram`, `SelectRetranslationBatches`, `ResetForRetranslation` 스텁 구현 | INFO | SUMMARY에서 pre-existing 이슈로 문서화됨. 해당 메서드들은 Phase 07 범위 밖 (Phase 08 재번역 실행에서 사용 예정). TestSelectRetranslationBatches 1개만 실패. |

pre-existing 스텁들은 Phase 08에서 실제 구현 예정이므로 Phase 07 목표 달성에 영향 없음.

### Human Verification Required

#### 1. A/B 테스트 실제 실행 결과 확인

**Test:** `projects/esoteric-ebb/context/ab_test_baseline.txt` 파일의 POST-CONTEXT 수치가 실제 DB에서 조회한 값인지 확인한다.
```sql
SELECT batch_id, AVG(score_final) as avg_score, COUNT(*) as item_count
FROM pipeline_items_v2
WHERE batch_id IN (
  'VL_Meeting/batch-1451', 'UP_Paintings/batch-1423',
  'TS_Potion/batch-1269', 'SO_Snurre/batch-1022',
  'VL_AncientTome/batch-1432', 'SO_Snurre/batch-1029',
  'SH_Modissa/batch-995', 'RM_Rollo/batch-955', 'TS_Arn_Kiosk/batch-1130'
)
GROUP BY batch_id
ORDER BY batch_id;
```
**Expected:** 9개 배치의 score_final이 ab_test_baseline.txt의 POST-CONTEXT 수치와 일치하며, 모두 BASELINE 대비 하락 없음.
**Why human:** ab_test_baseline.txt 파일은 이미 존재하고 결과를 기록하고 있으나, 해당 파일이 실제 파이프라인 재번역 실행 후 DB에서 조회한 값인지, 아니면 수동 기록인지 코드만으로는 확인 불가. 또한 PromptMeta.EstimatedTokens가 contextBudgetTokens(4000) 이내인지도 실행 로그 없이 검증 불가.

### Gaps Summary

자동화 검증으로 확인된 갭 없음. Phase 07의 코드 구현 전체가 완성되고 빌드/테스트를 통과함.

유일한 미결 항목은 A/B 테스트 결과의 사용자 승인이며, 이는 Task 3에서 `checkpoint:human-verify` 태스크로 명시된 것과 일치한다. ab_test_baseline.txt 파일이 생성되어 있어 사용자가 DB 값으로 교차 검증만 하면 된다.

---

_Verified: 2026-04-07T18:00:00Z_
_Verifier: Claude (gsd-verifier)_
