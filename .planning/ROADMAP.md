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

- [ ] **Phase 06: Foundation — 프롬프트 재구조화 + 화자 검증 + 재번역 CLI** - 번역 품질 개선의 전제 조건: 프롬프트 계층화, 화자 데이터 정제, 선별 재번역 도구
- [ ] **Phase 07: Context Enrichment — 톤 프로필 + 분기 맥락 + 연속성 윈도우** - 씬 단위 일관성 향상: 캐릭터 말투, 선택지 맥락, 주변 대사 윈도우 확장
- [ ] **Phase 08: Retranslation Execution — 재번역 실행 + 사이드카 수정 + 검증** - 개선된 프롬프트로 저품질 항목 재번역하고 게임 패치 적용

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
**Plans**: TBD

### Phase 07: Context Enrichment — 톤 프로필 + 분기 맥락 + 연속성 윈도우
**Goal**: 씬 단위 번역 일관성 향상 — 캐릭터별 말투 유지, 선택지 분기 맥락 전달, 주변 대사 윈도우 확장으로 LLM이 문맥을 충분히 인지
**Depends on**: Phase 06
**Requirements**: TONE-01, TONE-02, BRANCH-01, BRANCH-02, BRANCH-03, CONT-01, CONT-02
**Success Criteria** (what must be TRUE):
  1. 캐릭터별 voice card JSON이 존재하고, speaker_hint 매칭 시 해당 캐릭터의 말투/존댓말 레벨/성격이 번역 프롬프트에 주입된다
  2. 분기 대화에서 부모 선택지 텍스트가 "Player chose: X" 형태로 프롬프트에 포함되며, 브랜치 깊이 1단계 + 토큰 예산 내로 제한된다
  3. neighborPromptText가 prev/next 3줄 슬라이딩 윈도우로 확장되고, 재번역 시 기존 한국어 번역이 prevKO/nextKO에 채워진다
  4. 소규모 A/B 테스트에서 컨텍스트 주입 후 번역 점수가 주입 전 대비 하락하지 않는다 (프롬프트 크기 회귀 없음)
**Plans**: TBD

### Phase 08: Retranslation Execution — 재번역 실행 + 사이드카 수정 + 검증
**Goal**: 개선된 프롬프트와 컨텍스트로 저품질 항목을 실제 재번역하고, 게임에 적용하여 품질 향상을 확인
**Depends on**: Phase 07
**Requirements**: RETRANS-04, RETRANS-05
**Success Criteria** (what must be TRUE):
  1. BuildV3Sidecar dedup 로직이 score-aware로 수정되어, 같은 source에 대해 최고 점수 번역이 entries[]에 선택된다
  2. 재번역된 항목이 batch_id 단위로 실행되어 새 translations.json이 생성되고, 게임에서 태그 깨짐 없이 렌더링된다
  3. 재번역 전후 인게임 비교에서 대사 흐름이 자연스러워진 것이 확인된다
**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 6 -> 7 -> 8

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. 소스 준비 & 파서 | v1.0 | 3/3 | Complete | 2026-03 |
| 2. 번역 엔진 | v1.0 | 4/4 | Complete | 2026-03-22 |
| 3. 패치 출력 & 전량 실행 | v1.0 | 3/3 | Complete | 2026-03 |
| 4. 플러그인 최적화 | v1.0 | 2/5 | Complete (04.1/04.2) | 2026-03 |
| 4.1. Plugin.cs v2 | v1.0 | 2/2 | Complete | 2026-03-29 |
| 04.2. 소스 정리 | v1.0 | 2/2 | Complete | 2026-03-29 |
| 5. 미번역 커버리지 | v1.0 | 3/3 | Complete | 2026-03-29 |
| 6. Foundation | v1.1 | 0/? | Not started | - |
| 7. Context Enrichment | v1.1 | 0/? | Not started | - |
| 8. Retranslation Execution | v1.1 | 0/? | Not started | - |
