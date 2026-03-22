# Requirements: Esoteric Ebb 한국어 번역 파이프라인 v2

**Defined:** 2026-03-22
**Core Value:** 게임이 실제로 렌더링하는 대사 블록 단위로 소스를 생성하여, 태그 깨짐 없이 한국어 패치가 동작해야 한다.

**설계 원칙:**
- 코드 = 결정론적 작업 (파싱, 구조 분석, ID 매핑, 검증)
- LLM (gpt-5.4) = 창의적 작업 (번역, 태그/형식 없이 순수 텍스트)
- LLM (codex-mini) = 형식적 작업 (태그 복원, 포맷 맞춤)

## 필수 요구사항

### 준비 (PREP)

- [x] **PREP-01**: 소스 해시를 `len(text)`에서 SHA-256으로 수정하여 v2 항목 삽입 전 중복 충돌 방지

### 파서 & 소스 생성 (PARSE)

- [x] **PARSE-01**: ink JSON 트리를 재귀적으로 워킹하여 연속된 `"^text"` 엔트리를 게임 렌더링 단위(대사 블록)로 병합
- [x] **PARSE-02**: `g-N` (게이트), `c-N` (선택지) 분기 구조를 보존하며 씬(knot) 단위로 소스 생성
- [x] **PARSE-03**: `#speaker`, `#DC_check` 등 태그 메타데이터를 대사 블록에 연결하여 번역 컨텍스트로 제공
- [x] **PARSE-04**: 286개 TextAsset 파일에서 콘텐츠 유형 분류 (대사/주문/UI/아이템/시스템)
- [x] **PARSE-05**: 각 콘텐츠 유형에 최적화된 배칭 형식 적용 (대사=스크립트 10~30줄, UI=사전 50~100개, 주문=카드 5~10개)
- [x] **PARSE-06**: 번역 불필요 문자열 감지 (코드 식별자, 변수 참조, 게임 메커닉 수식) 및 패스스루 처리
- [x] **PARSE-07**: 파서 출력을 게임 런타임 캡처 데이터와 대조하여 블록 경계 정확성 검증

### 번역 엔진 (TRANS)

- [ ] **TRANS-01**: 씬 단위 클러스터를 태그 없이 스크립트 형식으로 gpt-5.4에 전송하여 번역
- [ ] **TRANS-02**: 분기 구조 마커 (BRANCH/OPTION)를 포함하여 분기별 톤/문맥 일관성 보존
- [ ] **TRANS-03**: 번역 결과의 각 라인을 `[NN]` 번호 마커로 원본 소스 ID에 매핑
- [ ] **TRANS-04**: 라인 수 불일치 시 자동 거부 및 재시도
- [ ] **TRANS-05**: codex-mini로 태그가 필요한 라인에만 태그 복원 (원본 태그 구조 기반)
- [ ] **TRANS-06**: 태그 복원 후 원본과 정확한 태그 문자열 매칭 검증 (태그 수만이 아닌 속성/순서 포함)
- [ ] **TRANS-07**: 용어집(글로서리) 구축 및 LLM 컨텍스트에 주입하여 용어 일관성 유지
- [ ] **TRANS-08**: 번역 품질 스코어링 및 기준 미달 항목 자동 재번역

### 파이프라인 인프라 (INFRA)

- [ ] **INFRA-01**: DB 기반 파이프라인 상태 관리 (pending → working → done/failed), 크래시 후 재개 지원
- [ ] **INFRA-02**: source_raw 기준 EXISTS 체크로 중복 인제스트 방지
- [ ] **INFRA-03**: 포맷팅 단계용 파이프라인 상태 확장 (pending_format/working_format)

### 패치 출력 (PATCH)

- [ ] **PATCH-01**: v2 대사 블록 단위 소스로 translations.json 생성 (BepInEx TranslationLoader 호환)
- [ ] **PATCH-02**: 285개 textassets 파일에 한국어 삽입된 ink JSON 생성
- [ ] **PATCH-03**: localizationtexts CSV 및 runtime_lexicon.json 생성

### 플러그인 (PLUGIN)

- [ ] **PLUGIN-01**: Plugin.cs 매칭 로직을 대사 블록 단위 직접 매칭 우선으로 변경
- [ ] **PLUGIN-02**: TryTranslateTagSeparatedSegments 제거 또는 최하위 폴백으로 강등
- [ ] **PLUGIN-03**: 직접 매칭 커버리지 95%+ 달성 확인

### 통합 검증 (VERIFY)

- [ ] **VERIFY-01**: v2 파이프라인으로 전량(77,816건+) 재번역 실행 완료
- [ ] **VERIFY-02**: 패치 적용 후 게임 내에서 태그 깨짐(bold 누출, color 누출) 없이 한국어 표시

## 나중에 추가 (필수 완성 후)

### 품질 향상

- **QUAL-01**: 콘텐츠 유형별 LLM 프롬프트 최적화 (대사/UI/주문/아이템 각각)
- **QUAL-02**: 레지스터 적절성 강제 (선택지=반말, NPC 나레이션=적절한 존댓말)
- **QUAL-03**: 구조적 토큰 보존 검증 분리 ($var, {template} 등)

## 범위 밖

| 기능 | 이유 |
|------|------|
| 패치 빌드 스크립트 수정 (BepInEx/doorstop 보존) | v2 이후 별도 처리 |
| 실시간 인게임 번역 | 6-8초 LLM 지연, 품질 미리뷰 불가 |
| 웹 UI / 대시보드 | 솔로 개발, CLI가 더 빠름 |
| 다국어 지원 | 한국어 전용, 언어별 프롬프트 엔지니어링 필요 |
| 게임 버전 자동 추적 | 1.1.3 고정 |
| 휴먼 리뷰 워크플로우 | 게임 플레이로 검증 |
| MT 혼합 (Google/DeepL) | 톤 일관성 파괴, 디버깅 어려움 |

## 추적 (Traceability)

| 요구사항 | 페이즈 | 상태 |
|----------|--------|------|
| PREP-01 | Phase 1 | Complete |
| PARSE-01 | Phase 1 | Complete |
| PARSE-02 | Phase 1 | Complete |
| PARSE-03 | Phase 1 | Complete |
| PARSE-04 | Phase 1 | Complete |
| PARSE-05 | Phase 1 | Complete |
| PARSE-06 | Phase 1 | Complete |
| PARSE-07 | Phase 1 | Complete |
| TRANS-01 | Phase 2 | Pending |
| TRANS-02 | Phase 2 | Pending |
| TRANS-03 | Phase 2 | Pending |
| TRANS-04 | Phase 2 | Pending |
| TRANS-05 | Phase 2 | Pending |
| TRANS-06 | Phase 2 | Pending |
| TRANS-07 | Phase 2 | Pending |
| TRANS-08 | Phase 2 | Pending |
| INFRA-01 | Phase 2 | Pending |
| INFRA-02 | Phase 2 | Pending |
| INFRA-03 | Phase 2 | Pending |
| PATCH-01 | Phase 3 | Pending |
| PATCH-02 | Phase 3 | Pending |
| PATCH-03 | Phase 3 | Pending |
| PLUGIN-01 | Phase 4 | Pending |
| PLUGIN-02 | Phase 4 | Pending |
| PLUGIN-03 | Phase 4 | Pending |
| VERIFY-01 | Phase 3 | Pending |
| VERIFY-02 | Phase 4 | Pending |

**Coverage:**
- 필수 요구사항: 27 total
- Mapped to phases: 27
- Unmapped: 0

---
*Requirements defined: 2026-03-22*
*Last updated: 2026-03-22 after roadmap creation*
