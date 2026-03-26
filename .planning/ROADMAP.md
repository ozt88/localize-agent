# Roadmap: Esoteric Ebb v2 번역 파이프라인

## Overview

v1의 근본 문제(소스 단위 != 렌더링 단위)를 해결하기 위해 ink JSON 트리 파서를 새로 구축하고, 2단계 LLM 번역(gpt-5.4 번역 + codex-mini 태그 복원) 파이프라인으로 40,067건을 전량 재번역하여, 태그 깨짐 없는 한국어 패치를 생성한다. 파서 정확성이 전체 파이프라인의 전제조건이므로 Phase 1에서 검증 완료 후 진행한다.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3, 4): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 1: 소스 준비 & 파서** - 해시 수정, ink JSON 파서 구축, 대사 블록 단위 소스 생성 및 검증
- [x] **Phase 2: 번역 엔진** - 클러스터 번역(gpt-5.4) + 태그 복원(codex-mini) + 파이프라인 상태 관리 (completed 2026-03-22)
- [ ] **Phase 3: 패치 출력 & 전량 실행** - 77K건 파이프라인 통과, translations.json/textassets/localizationtexts 생성
- [ ] **Phase 4: 플러그인 최적화 & 게임 검증** - Plugin.cs 매칭 정리, 게임 내 태그 깨짐 없이 동작 확인

## Phase Details

### Phase 1: 소스 준비 & 파서
**Goal**: ink JSON 트리를 게임 렌더링 단위(대사 블록)로 정확히 파싱하여, 번역 파이프라인의 올바른 소스를 생성한다
**Depends on**: Nothing (first phase)
**Requirements**: PREP-01, PARSE-01, PARSE-02, PARSE-03, PARSE-04, PARSE-05, PARSE-06, PARSE-07
**Success Criteria** (what must be TRUE):
  1. 소스 해시가 SHA-256 기반이며, 77K+ 규모에서 충돌 없이 중복 체크가 동작한다
  2. 286개 TextAsset 파일에서 파싱된 대사 블록이 게임 런타임 캡처 데이터와 95%+ 일치한다
  3. 각 대사 블록에 speaker/DC_check 메타데이터와 콘텐츠 유형(대사/주문/UI/아이템/시스템)이 분류되어 있다
  4. 콘텐츠 유형별 배칭 형식(스크립트/사전/카드)이 적용되어 번역 입력이 준비되어 있다
  5. 번역 불필요 문자열(코드 식별자, 변수 참조)이 패스스루 처리되어 있다
**Plans**: 3 plans

Plans:
- [x] 01-01-PLAN.md -- Core ink JSON parser with TDD (types, tree walker, block merger, SHA-256 hash, glue, CLI)
- [x] 01-02-PLAN.md -- Content type classification, passthrough detection, batch builder
- [x] 01-03-PLAN.md -- Parser validation against game runtime capture data (4,550 entries)

### Phase 2: 번역 엔진
**Goal**: 2단계 LLM 아키텍처(gpt-5.4 번역 + codex-mini 태그 복원)로 번역 결과를 생성하고, DB 상태 머신으로 대규모 실행을 관리한다
**Depends on**: Phase 1
**Requirements**: TRANS-01, TRANS-02, TRANS-03, TRANS-04, TRANS-05, TRANS-06, TRANS-07, TRANS-08, INFRA-01, INFRA-02, INFRA-03
**Success Criteria** (what must be TRUE):
  1. 씬 단위 클러스터가 태그 없이 gpt-5.4로 번역되고, 라인 수가 원본과 정확히 일치한다
  2. codex-mini가 태그 필요 라인에 원본과 동일한 태그 구조(속성/순서 포함)를 복원한다
  3. 파이프라인 상태(pending -> working -> done/failed + pending_format/working_format)가 DB에서 관리되며, 크래시 후 재개가 동작한다
  4. 용어집이 LLM 컨텍스트에 주입되어 반복 등장 용어의 번역이 일관된다
  5. 품질 기준 미달 항목이 자동으로 재번역된다
**Plans**: 4 plans

Plans:
- [x] 02-01-PLAN.md -- V2 pipeline contracts, DB schema (pipeline_items_v2), PostgreSQL store, ingest CLI
- [x] 02-02-PLAN.md -- Glossary loader (3 sources) + cluster translation domain (prompts, parser, validator)
- [x] 02-03-PLAN.md -- Tag format domain (codex-mini prompts, tag validation) + Score LLM domain (response parser, failure routing)
- [x] 02-04-PLAN.md -- Pipeline orchestrator (3-role workers, retry logic) + CLI entry point + prompt templates

### Phase 3: 패치 출력 & 전량 실행
**Goal**: v2 파이프라인으로 40,067건+ 전량을 처리하고, BepInEx 호환 패치 아티팩트를 생성한다
**Depends on**: Phase 2
**Requirements**: PATCH-01, PATCH-02, PATCH-03, VERIFY-01
**Success Criteria** (what must be TRUE):
  1. 40,067건+ 항목이 v2 파이프라인을 통과하여 done 상태에 도달한다
  2. translations.json이 대사 블록 단위 키로 생성되어 BepInEx TranslationLoader에서 로드된다
  3. 285개 textassets 파일에 한국어가 삽입된 ink JSON이 생성되며, 원본과 컨테이너 구조가 동일하다
  4. localizationtexts CSV 및 runtime_lexicon.json이 생성된다
**Plans**: 3 plans

Plans:
- [x] 03-01-PLAN.md -- Store.QueryDone() + translations.json v3 export domain logic
- [x] 03-02-PLAN.md -- ink JSON injection (InjectTranslations) with TDD
- [x] 03-03-PLAN.md -- Export CLI wiring + localizationtexts CSV + verification checkpoint

### Phase 4: 플러그인 최적화 & 게임 검증
**Goal**: Plugin.cs 매칭 로직을 대사 블록 단위에 최적화하고, 게임 내에서 태그 깨짐 없이 한국어가 표시됨을 확인한다
**Depends on**: Phase 3
**Requirements**: PLUGIN-01, PLUGIN-02, PLUGIN-03, VERIFY-02
**Success Criteria** (what must be TRUE):
  1. Plugin.cs가 대사 블록 직접 매칭 우선으로 동작하며, TryTranslateTagSeparatedSegments가 제거/강등되어 있다
  2. 직접 매칭 커버리지가 95%+ 달성된다
  3. 패치 적용 후 게임 내에서 bold 누출, color 누출 없이 한국어가 표시된다
**Plans**: 3 plans

Plans:
- [x] 04-01-PLAN.md -- V3 sidecar contextual_entries export (TDD)
- [ ] 04-02-PLAN.md -- Plugin.cs chain reduction (8->4 stages) + TextAsset .json loading
- [ ] 04-03-PLAN.md -- Patch deployment, game verification, hit rate analysis

## Progress

**Execution Order:**
Phases execute in numeric order: 1 -> 2 -> 3 -> 4

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. 소스 준비 & 파서 | 3/3 | Complete |  |
| 2. 번역 엔진 | 4/4 | Complete   | 2026-03-22 |
| 3. 패치 출력 & 전량 실행 | 3/3 | Complete |  |
| 4. 플러그인 최적화 & 게임 검증 | 0/3 | Not started | - |
