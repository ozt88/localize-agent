# Phase 2: 번역 엔진 - Context

**Gathered:** 2026-03-22
**Status:** Ready for planning

<domain>
## Phase Boundary

2단계 LLM 아키텍처(gpt-5.4 번역 + gpt-5.3-codex-spark 태그 복원)로 40,067개 대사 블록의 번역 결과를 생성하고, DB 상태 머신으로 대규모 실행을 관리한다. 용어집을 구축하여 번역 일관성을 유지하고, Score LLM으로 품질을 게이팅한다.

</domain>

<decisions>
## Implementation Decisions

### 클러스터 프롬프트 설계

- **D-01:** 화자 태깅은 라인 앞에 직접 표시: `[01] Braxo: "번역 텍스트"`. gpt-5.3-codex-spark가 화자 제거 + 태그 복원을 함께 처리하므로 번역 LLM 출력의 화자 파싱 부담 없음.
- **D-02:** 분기 마커는 인라인 태그: `[05] [CHOICE] "(선택지 텍스트)"`. 라인 수 불변. LLM이 태그를 출력에 포함해도 formatter가 제거.
- **D-03:** 이전 게이트 맥락 주입: 같은 knot의 이전 게이트 마지막 3줄을 `[CONTEXT]` 블록으로 배치 앞에 추가. 번역 대상 아님을 명시.
- **D-04:** 콘텐츠 유형별 프롬프트: 베이스 프롬프트를 세션 warmup에 1회 주입 + 유형별 접미 규칙을 배치 전송 시 런타임 조립. 베이스 프롬프트를 warmup 세션에서 프롬프트 최적화.

### 태그 복원 전략

- **D-05:** gpt-5.3-codex-spark 입력은 EN원본(태그포함) + KO번역(태그없음) 쌍. gpt-5.3-codex-spark가 EN↔KO 대응점을 스스로 찾아 태그 위치 결정. 이 방식을 우선 시도.
- **D-06:** 소규모 배치(3-5줄)로 시작하여 안정화 후 배치 크기 확대.
- **D-07:** 태그 검증: 태그 수 + 각 태그 문자열 존재 확인 (순서는 무시 — 한국어 어순이 영어와 다르므로 태그 순서가 바뀌는 것은 정상). 태그 안에 들어간 번역어의 의미 적절성은 Score LLM에 위임.
- **D-07a:** formatter 모델: 기본 gpt-5.3-codex-spark, 에스컬레이션 gpt-5.3-codex → gpt-5.4. gpt-5.3-codex-spark는 OpenCode 서버에 없음 (실험으로 확인).
- **D-08:** 복원 실패 시 gpt-5.3-codex-spark 2회 재시도(2회차에 실패 이유 힌트), 3회차에 gpt-5.4로 에스컬레이션.

### 용어집 구축 및 주입

- **D-09:** 용어집 소스: GlossaryTerms.txt(54+) + Speaker 이름(107개) + localizationtexts/ CSV. wiki 데이터는 이들과 중복도를 검토한 후 필요 시 추가.
- **D-10:** 고유명사 번역 정책: 전부 원문 유지. 인명, 지명, 주문명, 능력치(Intelligence, Wisdom 등) 모두 영문 그대로. 게임 UI가 이미 영문이므로 자연스러움. 이후 개선 여지 열어둠.
- **D-11:** LLM 주입: warmup에 핵심 용어(상위 50개) + 배치별 관련 용어 필터. 배치 필터 시 warmup에 이미 포함된 용어는 제외하여 중복 방지.
- **D-12:** 용어집 포맷: JSON 형식. 프롬프트 생성 로직이 조립에 적합한 형태로 처리.

### 품질 게이팅 및 재시도

- **D-13:** Stage 1(번역) 자동 거부 기준 (코드 검증):
  - 퇴화 감지: 빈 출력, 원문 그대로 복사
  - 번호 마커 매핑 실패: formatter의 기본 역할, 실패 시 재시도
  - 구두점 전용 블록(49개): 배치에서 제외, 원문 유지
  - 한글 비율은 거부 기준에서 제외 (고유명사 원문 유지 정책과 충돌)
- **D-14:** Score LLM은 formatter 이후 1회 호출. 번역 품질 + 포맷 적절성을 동시 평가. failure_type(translation/format/both/pass)과 reason을 반환하여 재시도 라우팅:
  - failure_type="translation" → Stage 1(gpt-5.4)부터 재시도
  - failure_type="format" → Stage 2(gpt-5.3-codex-spark)만 재시도
  - failure_type="both" → Stage 1부터 재시도
  - failure_type="pass" → done
- **D-15:** 재시도 전략: 동일 모델 2회(2회차에 Score LLM reason을 힌트로 추가) → 3회차에 고지능 모델로 에스컬레이션. 번역/format 각각 동일 패턴.
- **D-16:** 최종 실패: failed 상태 + 회차별 실패 로그 기록. 각 시도마다 attempt, stage, model, failure_type, reason, score, timestamp를 배열로 축적. 분석 후 프롬프트/로직 개선하고 재시도.

### 파이프라인 상태 흐름 (확정)

```
번역(gpt-5.4)
  → 코드 검증(퇴화/마커) → [실패] → 번역 재시도 (D-15 패턴)
  → [통과]
  → formatter(gpt-5.3-codex-spark)
  → 코드 검증(태그 매칭) → [실패] → format 재시도 (D-15 패턴)
  → [통과]
  → Score LLM(품질+포맷, failure_type 반환)
      → "translation" → 번역부터 재시도
      → "format"      → format만 재시도
      → "both"        → 번역부터 재시도
      → "pass"        → done
```

### Claude's Discretion

- 베이스 프롬프트의 구체적 문구 및 규칙 내용
- 유형별 접미 규칙의 세부 내용
- 이전 게이트 맥락 `[CONTEXT]` 블록의 정확한 포맷
- Score LLM 프롬프트 설계 및 스코어 임계치
- 용어집 핵심 50개 선정 기준
- DB 스키마 세부 설계 (pipeline_items 컬럼, 인덱스)
- 배치 크기 튜닝 (D-06 소규모 시작 후 확대 기준)
- 에스컬레이션 대상 모델 선정

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase 1 출력물 (번역 소스)
- `workflow/internal/inkparse/types.go` — DialogueBlock, ParseResult, Batch 구조체 (Phase 2 입력 형식)
- `workflow/internal/inkparse/batcher.go` — BuildBatches: 게이트 기반 배칭 로직, 콘텐츠 유형별 배치 포맷
- `workflow/internal/inkparse/classifier.go` — Classify: 5개 콘텐츠 유형 분류 로직
- `workflow/internal/inkparse/passthrough.go` — IsPassthrough: 번역 불필요 문자열 감지
- `workflow/cmd/go-ink-parse/main.go` — CLI로 286개 TextAsset → 40,067 블록 JSON 출력

### v2 아키텍처 설계
- `.planning/research/ARCHITECTURE.md` — 전체 v2 파이프라인 흐름, 2단계 LLM 패턴, 상태 머신 설계, 빌드 순서

### v1 번역 파이프라인 (재사용 대상)
- `workflow/internal/translation/types.go` — Config, translationTask, proposal, textProfile 구조체
- `workflow/internal/translation/skill.go` — translateSkill: warmup/규칙 빌더 패턴
- `workflow/internal/translation/prompts.go` — buildBatchPrompt, extractObjects: 프롬프트 구축 + LLM 출력 파싱
- `workflow/internal/translation/proposal_validation.go` — degenerateProposalReason: 퇴화 감지 휴리스틱
- `workflow/internal/translation/batch_builder.go` — buildBatch: 체크포인트 필터링, 프로파일 분류, 컨텍스트 조립
- `workflow/internal/translation/checkpoint_writer.go` — checkpointBatchWriter: 비동기 배치 DB 쓰기

### v1 파이프라인 오케스트레이터 (재사용 대상)
- `workflow/internal/translationpipeline/types.go` — 상태 상수, PipelineItem, Config
- `workflow/internal/translationpipeline/store.go` — DB 추상화, lease 기반 claim, stale cleanup
- `workflow/internal/translationpipeline/run.go` — 워커 풀 오케스트레이션

### LLM 클라이언트
- `workflow/pkg/platform/llm_client.go` — SessionLLMClient (OpenCode HTTP), EnsureContext/SendPrompt 인터페이스
- `workflow/pkg/platform/ollama_client.go` — OllamaLLMClient (Ollama HTTP)

### DB 스토어
- `workflow/pkg/platform/checkpoint_store.go` — SQLite/PostgreSQL checkpoint, UpsertItem 패턴
- `workflow/internal/contracts/translation.go` — TranslationCheckpointStore 인터페이스

### 기존 프롬프트 템플릿
- `projects/esoteric-ebb/context/esoteric_ebb_modelfile_system.md` — v1 시스템 프롬프트 (톤, 레지스터, 출력 형식 규칙)
- `projects/esoteric-ebb/context/esoteric_ebb_rules.md` — 프로젝트별 보충 규칙
- `projects/esoteric-ebb/context/esoteric_ebb_context.md` — 세계관/로어 컨텍스트

### 용어집 소스
- `projects/esoteric-ebb/extract/1.1.3/ExportedProject/Assets/Resources/glossaryterms/GlossaryTerms.txt` — 게임 내장 용어 54+ (CSV 형식)
- `projects/esoteric-ebb/extract/1.1.3/ExportedProject/Assets/StreamingAssets/TranslationPatch/localizationtexts/` — Feats, Spells, QuestPoints CSV

### 프로젝트 설정
- `projects/esoteric-ebb/project.json` — LLM 프로파일(low_llm, high_llm, score_llm), 서버 URL, 배치 크기

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `translation/skill.go` warmup 패턴: 시스템 프롬프트 + 규칙 조립 → 세션 초기화. v2 베이스 프롬프트 warmup에 동일 패턴 적용
- `translation/prompts.go` 출력 파싱: extractObjects, extractStringArray 등 6+ 포맷 대응. v2 번호 마커 파싱에 확장 가능
- `translation/proposal_validation.go` 퇴화 감지: 빈 출력, 구두점만, 원문 복사, ASCII 과다 — v2 D-13에 직접 재사용
- `translationpipeline/store.go` lease 기반 claim: atomic UPDATE + lease_until — v2 worker pool에 그대로 사용
- `platform/llm_client.go` 세션 관리: EnsureContext + SendPrompt — v2 번역/format/score 3개 역할에 각각 세션 키 부여
- `platform/checkpoint_store.go` UpsertItem 패턴: 상태+결과 원자적 저장 — v2 회차별 실패 로그 저장에 확장

### Established Patterns
- CLI: `cmd/go-*/main.go` → flag → LoadProjectConfig → domain.Run(Config)
- 계약: `internal/contracts/` 인터페이스 → `pkg/platform/` 구현
- 워커: role별 concurrency 설정 (low_llm, high_llm, score_llm)
- 체크포인트: checkpointBatchWriter로 비동기 배치 쓰기, flush interval 제어

### Integration Points
- Phase 1 → Phase 2: `go-ink-parse` 출력 JSON → pipeline_items 인제스트 (INFRA-02: source_raw 기준 중복 체크)
- 새 DB 상태: pending_translate → working_translate → translated → pending_format → working_format → formatted → pending_score → working_score → done | failed
- 새 패키지: `workflow/internal/clustertranslate/` (번역 프롬프트, 파서), `workflow/internal/tagformat/` (태그 복원)
- 새 CLI: `workflow/cmd/go-cluster-translate/`, `workflow/cmd/go-tag-format/`
- project.json 확장: v2용 프롬프트 경로, 용어집 경로, 배치 크기 설정

</code_context>

<specifics>
## Specific Ideas

- formatter(gpt-5.3-codex-spark)의 역할이 v2에서 확장됨: 화자 제거 + 인라인 태그([CHOICE]) 제거 + 리치텍스트 태그 복원 + 번호 마커 → ID 매핑. 단순한 "태그 삽입기"가 아니라 "번역 출력 정규화기"에 가까움.
- Score LLM의 failure_type 반환은 JSON 형식: `{"translation_score": N, "format_score": N, "failure_type": "...", "reason": "..."}`. 파이프라인이 이를 파싱하여 재시도 라우팅.
- 구두점 전용 블록 49개(말줄임표, `<wiggle>...</wiggle>`)는 배치 조립 전에 필터하여 원문 유지. 이들은 `IsPassthrough`에 추가하거나 별도 필터로 처리.
- `<i>단어</i>` 패턴 151개는 번역 필요 — 짧지만 의미 있는 텍스트. 태그 복원 대상.
- 회차별 실패 로그 형식: `[{attempt, stage, model, failure_type, reason, score, timestamp}, ...]` — DB에 JSON 배열로 저장.

</specifics>

<deferred>
## Deferred Ideas

- 고유명사 음역/의역 정책 개선 — C-2에서 원문 유지로 시작, 이후 별도 작업으로 검토
- wiki 크롤링 데이터(`rag/esoteric_ebb_lore_termbank.json`) 용어집 편입 — localizationtexts와 중복도 검토 후 결정
- 콘텐츠 유형별 프롬프트 최적화 (QUAL-01) — Phase 2에서 베이스+접미 구조 구축 후, 품질 데이터 기반으로 개별 튜닝
- 레지스터 적절성 강제 (QUAL-02) — 선택지=반말, NPC=존댓말 등 세분화는 이후 개선
- 구조적 토큰 보존 검증 분리 (QUAL-03) — $var, {template} 등은 현재 passthrough 처리, 추가 검증은 이후
- 배치 크기 자동 튜닝 — D-06에서 수동 확대 후 안정화되면 자동화 검토

</deferred>

---

*Phase: 02-translation-engine*
*Context gathered: 2026-03-22*
