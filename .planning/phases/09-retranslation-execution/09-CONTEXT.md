---
phase: 09
phase_name: Retranslation Execution — 품질 복원 + 전량 재번역 + 게임 검증
status: discussed
discussed_at: "2026-04-12T18:00:00.000Z"
gray_areas_resolved: 4
---

# Phase 09 Context — 결정 사항

## Phase 목표 재확인

Phase 08에서 파이프라인 코드(`worker.go`, `types.go`, `main.go`)의 voice card/RAG 플래그가 누락된 채 3,412건이 번역됨. Phase 09의 목표는:

1. **Phase 07/07.1에서 추가된 모든 컨텍스트 주입 코드를 완전히 복원**
2. **voice card 생성 파이프라인을 개선** (wiki + RAG 기반으로)
3. **35,009건 전량을 개선된 프롬프트로 재번역**

이는 좁은 버그 픽스(Kattegatt 수동 수정)가 아니라 **컨텍스트 생성 파이프라인 전체 개선**이다.

---

## Gray Area 1: 삭제된 코드 처리 방식

### 결정: 전량 복원

**배경:** `56fa1ea`(Phase 08-01)에서 worktree 버그로 Phase 07/07.1이 추가한 모든 코드가 삭제됨. `f00bbac`에서 파일(packages, data)은 복원했으나 통합 코드(`worker.go`, `types.go`, `main.go`)는 복원하지 않음.

**삭제된 항목 (전량 복원 대상):**
- `workflow/internal/v2pipeline/types.go`: `VoiceCardsPath string`, `VoiceCards map[string]string`, `RAGContextPath string`
- `workflow/internal/v2pipeline/worker.go`: voice card 로딩, nextLines(CONT-01), prevKO/nextKO(CONT-02), RAG 주입
- `workflow/cmd/go-v2-pipeline/main.go`: `--voice-cards`, `--rag-context` CLI 플래그

**근거:** Phase 07/07.1 추가는 additive 방향이었으며, 삭제는 버그. 복원이 맞다.

---

## Gray Area 2 & 3: Voice Card 재설계

### 결정: wiki/RAG 기반 생성 + relationships 필드 추가

#### 현재 문제
- `go-generate-voice-cards` CLI: DB 대화 샘플만 사용, wiki/RAG 미활용
- voice card 형식: `speech_style / honorific / personality` 3개 필드만
- Kattegatt(고어체 화자) 미포함 — voice_cards.json에 없음

#### 결정 1: voice card 생성 개선 (모든 15개 카드 + Kattegatt 포함)

`go-generate-voice-cards` CLI를 다음으로 개선:
- **입력 소스 추가**: wiki 캐릭터 페이지 + DB 공동 출현 화자 + 기존 DB 샘플
- **LLM이 relationships까지 생성**: 화자-청자 관계 정보 포함

#### 결정 2: voice card 형식에 relationships 필드 추가 (Option A)

```json
{
  "Visken": {
    "speech_style": "냉정하고 격식체. 어미: ~니다, ~습니다. 감정 절제, 의학/시체 관련 전문 용어 사용",
    "honorific": "상대에게 격식체 유지. 자신은 항상 전문가적 거리 유지",
    "personality": "창백하고 차가운 mortician. 죽음을 일상으로 취급. 내면에 호기심 있음",
    "relationships": {
      "Snell": "부하 다루듯 무시하거나 간결히 대응",
      "The Cleric": "전문가로서 존중, 정보 교환 파트너"
    }
  }
}
```

- **화자 관계 필터링**: `translateBatch` 시 배치 내 실제 등장 speaker로 relationships 필터링 → 토큰 압박 방지
- **어미 예시 패턴**: `speech_style`에 "어미: ~야, ~잖아" 형태로 구체적 어미 예시 추가

#### 대상 캐릭터 (최우선)
- 기존 15명: Snell, Viira, Ettir, Snurre, Visken, Rollo, Lisa, Alfoz, Modissa, Olzis, Meri, Darrow, Alt, Rix, Arn
- **추가 필수**: Kattegatt (고어체 — `그대/~도다/~노라`, `thou/thy` 원문)

---

## Gray Area 4: 샘플 검증 씬 선정

### 결정: VL_Visken 씬 — Visken과 Ôst

**선정 이유:**
- 유저가 직접 플레이한 씬 (게임 초반 접근 가능)
- Visken: "pale man" mortician — 마법사 분위기, voice card 보유 (기존 15명 포함)
- Ôst: kobold 가드 — 소형 파충류형 생물 (Visken의 liar 입구 경비)
- Visken의 격식체/냉정한 말투가 제대로 번역됐는지 확인 가능

**검증 절차 (10건 샘플):**
1. `--voice-cards`/`--rag-context` 플래그 확인 (`go-v2-pipeline --help`)
2. VL_Visken 씬 10건 샘플 번역
3. Visken 대사에서 말투 보존 확인 — 격식체, 냉정한 전문가 어조
4. Kattegatt 있을 경우: `그대/~도다` 고어체 보존 확인

**추가 검증 씬 (선택):**
- EP_Borgo: 사과 창고/좀비 씬 (유저 플레이 씬)
- EbbIntro: 능력치 대화/이름 묻는 씬 (유저 플레이 씬)

---

## 전제조건 체크리스트 (Phase 09 실행 전 필수)

Phase 08 교훈(L-01~L-04) 기반:

```
[ ] go-v2-pipeline --help에 --voice-cards, --rag-context 플래그 존재 확인
[ ] voice_cards.json 존재 확인 (Kattegatt 포함)
[ ] rag_batch_context.json 존재 확인
[ ] VL_Visken 씬 10건 샘플 번역 후 Visken 말투 육안 검토
[ ] Kattegatt 대사 있을 경우 그대/~도다 고어체 확인
```

---

## 프롬프트 컨텍스트 구조 (Phase 07/07.1 완성 후 8개 필드)

```go
type ClusterTask struct {
    Batch            inkparse.Batch
    PrevGateLines    []string  // continuity (CONT-01)
    GlossaryJSON     string    // Phase 07.1
    NextLines        []string  // continuity (CONT-01)
    PrevKO           []string  // continuity (CONT-02)
    NextKO           []string  // continuity (CONT-02)
    VoiceCards       map[string]string  // voice cards (Phase 07)
    ParentChoiceText string    // branch context (Phase 07)
    RAGHints         string    // RAG world context (Phase 07.1)
}
```

D-18 토큰 예산 우선순위:
- continuity > RAG > glossary > branch > voice
- trimContextForBudget: voice card(마지막 제거) → branch → continuity(먼저 보존)

---

## 결정되지 않은 사항 (Phase 09 Plan 별 결정)

- voice card 생성 시 wiki 페이지 매칭 방법 (exact match vs fuzzy) → 09-01-PLAN에서 결정
- relationships 필드를 voice card JSON에 flat으로 넣을지 nested로 넣을지 → 09-01-PLAN에서 결정
- 전량 재번역 실행 순서 (씬 우선순위, 배치 크기) → 09-02-PLAN에서 결정
