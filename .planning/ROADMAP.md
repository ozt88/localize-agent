# Roadmap: Esoteric Ebb v2 번역 파이프라인

## Milestones

- ✅ **v1.0 한국어 번역 파이프라인 v2** — Phases 1-5 (shipped 2026-03-29)
- 🚧 **v1.1 번역 품질 개선 — 맥락 기반 재번역** — Phases 6-8 (in progress)

## Phases

<details>
<summary>✅ v1.0 한국어 번역 파이프라인 v2 (Phases 1-5) — SHIPPED 2026-03-29</summary>

- [x] Phase 1: 소스 준비 & 파서 (3/3 plans)
- [x] Phase 2: 번역 엔진 (4/4 plans) — completed 2026-03-22
- [x] Phase 3: 패치 출력 & 전량 실행 (3/3 plans)
- [x] Phase 4: 플러그인 최적화 & 게임 검증 (2/5 plans — 04.1/04.2로 대체)
- [x] Phase 4.1: Plugin.cs v2 재작성 (2/2 plans) — completed 2026-03-29
- [x] Phase 04.2: 소스 정리 & 재export (2/2 plans)
- [x] Phase 5: 미번역 커버리지 개선 (3/3 plans) — completed 2026-03-29

Full details: [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md)

</details>

### 🚧 v1.1 번역 품질 개선 — 맥락 기반 재번역 (In Progress)

**Milestone Goal:** 앞뒤 대사와 어울리지 않는 번역을 개선 — LLM 프롬프트에 화자, 톤, 분기 구조, 연속성 맥락을 주입하고 저품질 항목만 선별 재번역

- [x] **Phase 06: Foundation — 프롬프트 재구조화 + 화자 검증 + 재번역 CLI** (3/3 plans) — completed 2026-04-06
- [x] **Phase 07: Context Enrichment — 톤 프로필 + 분기 맥락 + 연속성 윈도우** - 씬 단위 일관성 향상: 캐릭터 말투, 선택지 맥락, 주변 대사 윈도우 확장 (completed 2026-04-08)
- [ ] **Phase 07.1: RAG 세계관 맥락 주입** - enriched termbank + 배치별 사전 매칭 + Go 파이프라인 통합 (4 plans)
- [ ] **Phase 08: Retranslation Execution — 재번역 실행 + 사이드카 수정 + 검증** - 개선된 프롬프트로 저품질 항목 재번역하고 게임 패치 적용 (2 plans)

## Phase Details

### Phase 06: Foundation — 프롬프트 재구조화 + 화자 검증 + 재번역 CLI
**Goal**: 다른 모든 품질 개선의 전제 조건 확립 — 프롬프트 구조가 컨텍스트 주입을 수용할 준비가 되고, 화자 데이터가 검증되고, 선별 재번역 도구가 동작
**Depends on**: Phase 05 (v1.0 milestone complete)
**Requirements**: PROMPT-01, PROMPT-02, PROMPT-03, SPEAKER-01, SPEAKER-02, SPEAKER-03, RETRANS-01, RETRANS-02, RETRANS-03
**Success Criteria** (what must be TRUE):
  1. 번역 프롬프트가 컨텍스트/보이스/태스크/제약 4개 섹션으로 계층화되어 있고, ability-score voice guide가 speaker_hint 매칭 시 per-item 프롬프트에 주입된다
  2. speaker_hint 커버리지가 대화 라인 대비 90% 이상이며, 검증된 화자 allow-list로 isSpeakerTag 오인식이 필터링된다
  3. ScoreFinal < threshold 기준으로 재번역 후보를 batch_id 단위로 선택하는 CLI가 동작하고, 선택된 항목의 원본 ko_formatted 스냅샷이 보존된다
  4. 프롬프트 토큰 예산이 프로파일링되어 Phase 07 컨텍스트 주입의 여유분이 확인된다
**Plans:** 3 plans
Plans:
- [x] 06-01-PLAN.md — v2StaticRules 4-tier 계층화 + per-batch voice guide 주입 + 토큰 프로파일링
- [x] 06-02-PLAN.md — 화자 커버리지 감사 + allow-list JSON 생성 + Go 필터 함수
- [x] 06-03-PLAN.md — 재번역 CLI + DB 스키마 확장 (retranslation_gen, snapshots, score 인덱스)

### Phase 07: Context Enrichment — 톤 프로필 + 분기 맥락 + 연속성 윈도우
**Goal**: 씬 단위 번역 일관성 향상 — 캐릭터별 말투 유지, 선택지 분기 맥락 전달, 주변 대사 윈도우 확장으로 LLM이 문맥을 충분히 인지
**Depends on**: Phase 06
**Requirements**: TONE-01, TONE-02, BRANCH-01, BRANCH-02, BRANCH-03, CONT-01, CONT-02
**Success Criteria** (what must be TRUE):
  1. 캐릭터별 voice card JSON이 존재하고, speaker_hint 매칭 시 해당 캐릭터의 말투/존댓말 레벨/성격이 번역 프롬프트에 주입된다
  2. 분기 대화에서 부모 선택지 텍스트가 "Player chose: X" 형태로 프롬프트에 포함되며, 브랜치 깊이 1단계 + 토큰 예산 내로 제한된다
  3. neighborPromptText가 prev/next 3줄 슬라이딩 윈도우로 확장되고, 재번역 시 기존 한국어 번역이 prevKO/nextKO에 채워진다
  4. 소규모 A/B 테스트에서 컨텍스트 주입 후 번역 점수가 주입 전 대비 하락하지 않는다 (프롬프트 크기 회귀 없음)
**Plans:** 3/3 plans complete
Plans:
- [x] 07-01-PLAN.md — Voice card 인프라 + LLM 자동 생성 CLI + voice_cards.json 생성
- [x] 07-02-PLAN.md — inkparse ParentChoiceText + store GetNextLines/GetAdjacentKO + ClusterTask 확장
- [x] 07-03-PLAN.md — 프롬프트 통합 주입 + 토큰 예산 관리 + worker 조합 + A/B 테스트

### Phase 07.1: RAG 세계관 맥락 주입 — enriched termbank + 배치별 사전 매칭 + Go 파이프라인 통합 (INSERTED)

**Goal:** 위키 + glossary 데이터를 enriched termbank으로 통합하고, 배치별 세계관 힌트를 사전 매칭하여, 번역/평가 프롬프트의 [CONTEXT] 섹션에 동적 주입 -- 기존 lore.go 정적 매칭을 RAG 기반으로 대체
**Requirements**: D-01, D-02, D-03, D-04, D-05, D-06, D-07, D-08, D-09, D-10, D-11, D-12, D-13, D-14, D-15, D-17, D-18, D-19, D-20
**Depends on:** Phase 07
**Success Criteria** (what must be TRUE):
  1. enriched_termbank.json이 wiki lore(254건) + glossary 설명부(~800건)를 통합하여 500건 이상 존재한다
  2. rag_batch_context.json이 전체 배치에 대해 top-3 세계관 hint를 사전 매칭하여 존재한다
  3. Go ragcontext 패키지가 JSON을 로드하고, translateBatch/scoreBatch 양쪽에서 [CONTEXT] 섹션에 RAG 힌트를 주입한다
  4. trimContextForBudget이 D-18 우선순위(continuity > RAG > glossary > branch > voice)를 따른다
  5. 10배치 A/B 테스트에서 RAG 주입 후 번역 스코어가 하락하지 않는다
**Plans:** 3/4 plans executed
Plans:
- [x] 07.1-01-PLAN.md — 위키 Markdown 변환 + enriched termbank 빌드 (wiki + glossary 통합)
- [x] 07.1-02-PLAN.md — 배치별 RAG 사전 매칭 빌더 (enriched termbank 기반 word-boundary matching)
- [x] 07.1-03-PLAN.md — Go ragcontext 패키지 + ClusterTask 확장 + 프롬프트 주입 + worker 통합
- [ ] 07.1-04-PLAN.md — RAG A/B 테스트 실행 + 사용자 검증

### Phase 08: Retranslation Infrastructure — dedup + reset 인프라 구축 ✅ CLOSED
**Goal (재정의)**: highest-gen dedup + 전체 리셋 인프라 확립. 실번역은 Phase 09로 이월.
**Depends on**: Phase 07.1
**Closed reason**: 파이프라인 실행 시도했으나 voice cards/RAG 플래그 누락(worktree 버그 후 미복원)으로 품질 목표 미달. 인프라만 완료로 인정.
**Plans:**
- [x] 08-01-PLAN.md — BuildV3Sidecar highest-gen dedup + ResetAllForRetranslation + go-v2-reset-all CLI + 전체 리셋
- [x] 08-02-PLAN.md — 파이프라인 실행 시도 + 품질 gap 발견 + watchdog 수정 (CLOSED_INCOMPLETE → Phase 09 이월)

### Phase 09: Retranslation Execution — 품질 복원 + 전량 재번역 + 게임 검증
**Goal**: voice cards + RAG를 go-v2-pipeline에 재통합하고, 35,009건 전량을 개선된 프롬프트로 재번역하여 게임에 적용
**Depends on**: Phase 08
**Requirements**: RETRANS-04, RETRANS-05
**Lessons from Phase 08** (반드시 준수):
  - 실행 전 `--voice-cards`, `--rag-context` 플래그 존재 확인
  - 10건 샘플 번역 후 특수 말투 캐릭터(Kattegatt 등) 육안 검토
  - voice_cards.json, rag_batch_context.json 존재 확인
**Success Criteria** (what must be TRUE):
  1. go-v2-pipeline worker가 voice card(캐릭터 말투)와 RAG context(세계관 힌트)를 프롬프트에 주입한다
  2. Kattegatt 등 고어체 캐릭터의 말투가 `그대/~도다` 체로 번역된다
  3. 35,009건 전량 재번역이 완료되어 translations.json이 생성된다
  4. 게임에서 태그 깨짐 없이 한국어가 렌더링된다
**Plans:**
- [x] 09-01-PLAN.md — voice cards + RAG go-v2-pipeline 재통합 + voice_cards.json 생성 + 샘플 검증
- [ ] 09-02-PLAN.md — 전량 재번역 실행 (35,009건)
- [ ] 09-03-PLAN.md — export + before/after diff + 인게임 검증

## Progress

**Execution Order:**
Phases execute in numeric order: 6 -> 7 -> 7.1 -> 8

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. 소스 준비 & 파서 | v1.0 | 3/3 | Complete | 2026-03 |
| 2. 번역 엔진 | v1.0 | 4/4 | Complete | 2026-03-22 |
| 3. 패치 출력 & 전량 실행 | v1.0 | 3/3 | Complete | 2026-03 |
| 4. 플러그인 최적화 | v1.0 | 2/5 | Complete (04.1/04.2) | 2026-03 |
| 4.1. Plugin.cs v2 | v1.0 | 2/2 | Complete | 2026-03-29 |
| 04.2. 소스 정리 | v1.0 | 2/2 | Complete | 2026-03-29 |
| 5. 미번역 커버리지 | v1.0 | 3/3 | Complete | 2026-03-29 |
| 6. Foundation | v1.1 | 3/3 | Complete | 2026-04-06 |
| 7. Context Enrichment | v1.1 | 3/3 | Complete | 2026-04-08 |
| 7.1 RAG 세계관 맥락 | v1.1 | 4/4 | Complete | 2026-04-12 |
| 8. Retranslation Infrastructure | v1.1 | 2/2 | Closed (인프라만 완료) | 2026-04-12 |
| 9. Retranslation Execution | v1.1 | 1/3 | In Progress|  |
