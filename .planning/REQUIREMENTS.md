# Requirements: Esoteric Ebb 번역 품질 개선

**Defined:** 2026-04-06
**Core Value:** 게임이 실제로 렌더링하는 대사 블록 단위로 소스를 생성하여, 태그 깨짐 없이 한국어 패치가 동작해야 한다.

## v1.1 Requirements

Requirements for context-aware retranslation milestone. Each maps to roadmap phases.

### 프롬프트 (PROMPT)

- [ ] **PROMPT-01**: 현재 24개 flat rule을 계층 구조(컨텍스트, 보이스, 태스크, 제약)로 재구조화
- [ ] **PROMPT-02**: ability-score voice guide를 warmup에서 per-item 프롬프트로 통합 (speaker_hint 매칭 시)
- [ ] **PROMPT-03**: 프롬프트 토큰 예산 프로파일링 및 최적화

### 화자 (SPEAKER)

- [ ] **SPEAKER-01**: translator_package.json의 speaker_hint 커버리지 감사 (대화 라인 대비 비율 측정)
- [ ] **SPEAKER-02**: ink JSON # 태그 파싱 강화로 speaker_hint 커버리지 90%+ 달성
- [ ] **SPEAKER-03**: 검증된 화자 allow-list 생성 (isSpeakerTag 오인식 필터링)

### 톤 (TONE)

- [ ] **TONE-01**: 캐릭터별 voice card JSON 생성 (말투, 존댓말 레벨, 성격 키워드)
- [ ] **TONE-02**: loreEntry 패턴 재사용하여 speaker_hint 매칭 시 voice card 프롬프트 주입

### 분기 (BRANCH)

- [ ] **BRANCH-01**: ink 트리에서 부모 선택지 텍스트 추출 로직 구현
- [ ] **BRANCH-02**: translationTask에 branch_context 필드 추가 + 프롬프트 주입
- [ ] **BRANCH-03**: 브랜치 깊이 제한 (depth-1 부모만) + 토큰 예산 내 유지

### 연속성 (CONT)

- [ ] **CONT-01**: neighborPromptText 확장 — prev/next 1줄 → 3줄 슬라이딩 윈도우
- [ ] **CONT-02**: 재번역 시 기존 한국어 번역을 prevKO/nextKO에 항상 채움

### 재번역 (RETRANS)

- [ ] **RETRANS-01**: ScoreFinal < threshold 기준 재번역 후보 쿼리 CLI 구현
- [ ] **RETRANS-02**: batch_id 단위 재번역 (개별 라인이 아닌 클러스터 전체)
- [ ] **RETRANS-03**: 재번역 전 원본 ko_formatted 스냅샷 보존 (롤백용)
- [ ] **RETRANS-04**: BuildV3Sidecar dedup 로직을 score-aware로 수정
- [ ] **RETRANS-05**: 재번역 실행 + 인게임 검증

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### 스코어링

- **SCORE-01**: Score LLM 프롬프트에 톤 일관성 + 맥락 정합성 평가 기준 추가
- **SCORE-02**: 맥락 인식 점수 산출 (현재 EN+KO 쌍만 평가 → 주변 라인 포함)

### 고유명사

- **NAMING-01**: 고유명사 정책 통일 (Cleric→성직자 vs 원문 유지)
- **NAMING-02**: 전체 번역에 통일된 고유명사 정책 적용

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| 전량 재번역 | 75시간 LLM 비용, 대부분 이미 양호 (avg score 90.7) — 선별 재번역으로 대체 |
| Multi-pass consensus 번역 | 3-5x 비용, 2단계 아키텍처가 이미 품질 제어 |
| 캐릭터 관계 그래프 | ink에 관계 미인코딩, 수동 주석 286 파일 필요 — voice card로 충분 |
| 임베딩 기반 톤 감지 | MiniLM 한국어 톤 뉘앙스 부족 — voice card + Score LLM으로 대체 |
| Score 모델 fine-tuning | 라벨링 데이터 없음 — v1.1 이후 인게임 리뷰 후 결정 |
| 미번역 154건 해소 | 별도 작업으로 분리 가능 — 품질 개선과 무관 |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| PROMPT-01 | Phase 06 | Pending |
| PROMPT-02 | Phase 06 | Pending |
| PROMPT-03 | Phase 06 | Pending |
| SPEAKER-01 | Phase 06 | Pending |
| SPEAKER-02 | Phase 06 | Pending |
| SPEAKER-03 | Phase 06 | Pending |
| TONE-01 | Phase 07 | Pending |
| TONE-02 | Phase 07 | Pending |
| BRANCH-01 | Phase 07 | Pending |
| BRANCH-02 | Phase 07 | Pending |
| BRANCH-03 | Phase 07 | Pending |
| CONT-01 | Phase 07 | Pending |
| CONT-02 | Phase 07 | Pending |
| RETRANS-01 | Phase 06 | Pending |
| RETRANS-02 | Phase 06 | Pending |
| RETRANS-03 | Phase 06 | Pending |
| RETRANS-04 | Phase 08 | Pending |
| RETRANS-05 | Phase 08 | Pending |

**Coverage:**
- v1.1 requirements: 18 total
- Mapped to phases: 18
- Unmapped: 0

---
*Requirements defined: 2026-04-06*
*Last updated: 2026-04-06 after roadmap creation*
