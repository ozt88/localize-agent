# 지식 컴파일러 설계 문서

> Phase 05 포스트모템에서 도출된 GSD 워크플로 확장 설계.
> 2026-04-05 세션에서 논의 완료, 구현 대기.

---

## 1. 풀려는 문제

GSD로 Esoteric Ebb 한국어 로컬라이제이션 작업 중 반복 발생한 문제:

### 1.1 문제의 세 겹

1. **프레임 불일치** — GSD는 "뭘 할지 아는 상태"(Plan→Execute)를 전제하지만, 인게임 검증은 "뭐가 틀렸는지 모르는 상태"에서 시작하는 탐색
2. **지식 유실** — 탐색 중 발생한 학습(시도→실패→결정)이 세션 종료와 함께 증발
3. **조회 불가** — 기록이 있어도 GSD sub-agent(planner, researcher)가 접근 불가 (memory는 메인 에이전트만 읽음)

### 1.2 실제 사례 (Phase 05)

| 루프 | 시도 | 결과 | 교훈 |
|------|------|------|------|
| 1 | sidecar format v2 사용 | translations_loaded=0 | v3 format 필요 |
| 2 | Python build script (v1 DB) | v1 번역 나옴 | Go go-v2-export 필요 |
| 3 | ReplacePlainText (문자 단위 태그 매핑) | 색깔 번짐, 태그 노출 | 한/영 길이 차이로 원천 불가 |
| 4 | TryTranslateCore 먼저 실행 | 이미 번역된 텍스트 재처리 | ContainsKorean 우선 체크 필요 |

4번의 피드백 루프가 모두 기록 없이 ad-hoc으로 진행됨. ReplacePlainText를 만들었다가 삭제하는 비효율 발생.

---

## 2. 해법 탐색 경로

```
MCP 벡터DB (mcp-memory-service, knowledgegraph-mcp)
    ↓ 검토: 시맨틱 검색은 좋으나 추가 인프라 부담
Karpathy "LLM Knowledge Bases" 인사이트
    ↓ 전환: "개인 규모에서는 마크다운 + 인덱스면 충분"
파일시스템 기반 지식 컴파일러 (최종 방향)
```

### 왜 MCP 서버가 아닌가
- GSD 서브에이전트는 이미 Read/Grep/Glob 사용 → 추가 MCP 도구 불필요
- 벡터DB 설치/유지보수 부담
- 마크다운 파일이 사람도 읽을 수 있음
- `.planning/` 내 배치로 GSD 아티팩트 흐름에 자연 통합

### 왜 Claude 내장 memory가 아닌가
- memory는 메인 에이전트만 접근 (서브에이전트 불가)
- MEMORY.md 200줄 제한
- 키워드 매칭만 지원 (파일명/description 기반)

---

## 3. 지식 컴파일러 설계

### 3.1 핵심 원칙

1. **수집과 판단을 분리** — 저장 시점에 "이게 중요한가?" 묻지 않음
2. **관측과 정책을 분리** — "뭘 봤는가"(observation)와 "뭘 결정했는가"(decision)는 다른 계층
3. **컴파일러는 GSD 외부 도구** — 작업 실행과 지식 빌드를 분리
4. **사람은 직접 안 쓰되 승격/검토는 함** — LLM이 작성, 사람이 승인
5. **planner는 원본이 아니라 brief를 받음** — 전체 knowledge가 아니라 Phase 맞춤 요약

### 3.2 파이프라인

```
[1.수집] → [2.컴파일] → [3.린트] → [4.브리핑] → [5.조회]
 raw/       knowledge/    자가검증    planning     planner가
 턴 단위     주기적 변환    무결성확인  brief 생성   brief를 제약
 자동 저장                                        조건으로 사용
```

### 3.3 각 단계 상세

#### Stage 1: 수집 (raw/)

**단위:** 턴 (유저 메시지 → Claude 작업 → 응답 완료 = 1턴)
- PostToolUse: 너무 세밀 (파일 수정마다 발동 → 노이즈)
- Phase 종료: 너무 거칠 (탐색 루프 디테일 유실)
- 턴: 적정 (하루 30턴 → 60-90줄)

**메커니즘:** Claude Code `Stop` Hook (2026-04-05 테스트 완료)

**테스트 결과:**

| Hook 타입 | 발동 | 파일 쓰기 | 턴 컨텍스트 접근 | 요약 생성 |
|-----------|------|----------|----------------|----------|
| `command` | ✅ | ✅ (bash로 직접) | ❌ (환경변수/stdin 없음) | ❌ |
| `prompt`  | ✅ | ❌ (도구 없음) | ✅ (transcript 참조 확인) | ❌ (검증/게이트 모드) |

**핵심 발견:**
1. `type: "command"` — 매 응답 완료 시 bash 명령 실행. 파일 쓰기 가능하나 대화 컨텍스트에 접근 불가
2. `type: "prompt"` — 턴 컨텍스트(transcript)에 접근 가능하나, **생성이 아닌 검증 모드로 동작**. 주어진 프롬프트를 "이 응답이 조건에 부합하는가?" 로 해석하며, 요약을 생성하거나 파일에 쓰지 않음
3. prompt 훅 출력은 대화 피드백("Stop hook feedback:")으로만 전달됨

**결론:** Stop Hook만으로는 "턴 요약 자동 수집"이 불가능.

**채택된 대안: B — CLAUDE.md 행동 지시**
- CLAUDE.md에 "매 응답 마지막에 `.knowledge/raw/{YYYY-MM-DD}.md`에 턴 요약 append" 지시 추가
- 장점: 컨텍스트 접근 O, 파일 쓰기 O, 추가 인프라 불필요
- 단점: 에이전트가 가끔 빼먹을 수 있음 (MVP 허용 범위)
- 구현: CLAUDE.md "Knowledge Compiler — 턴 수집" 섹션으로 추가 완료

#### Stage 2: 컴파일 (raw/ → knowledge/)

LLM이 raw를 읽고 구조화된 지식 파일로 변환.

**knowledge/ 파일 구조:**

```
knowledge/{project}/
  index.md                    # 전체 요약 + 키워드 인덱스
  architecture.md             # 현재 시스템 구조와 제약 조건
  decisions.md                # 시도 → 결과 → 결정 (상태 태그 포함)
  anti-patterns.md            # "이것은 하지 마라" 목록과 이유
  troubleshooting.md          # 에러 메시지 ↔ 해결책 매핑
```

**Decision 상태 태그:**
- `[active]` — 현재 유효한 결정
- `[rejected]` — 시도했으나 기각된 접근
- `[superseded]` — 새 결정으로 대체됨
- `[uncertain]` — 검증 필요한 가설

**decisions.md 예시:**

```markdown
## [rejected] ReplacePlainText 직접 교체

- **맥락**: TMP 태그가 포함된 텍스트의 inner text만 번역으로 교체
- **시도**: 문자 인덱스 기반으로 태그 위치 보존하며 번역 삽입
- **결과**: 한/영 길이 차이로 태그 위치 밀림 → 색깔 번짐, </color> 누출
- **결정**: 삭제. 평문만 반환, 게임 엔진이 렌더링 담당
- **원칙**: 소스/타겟 길이가 다른 번역에서 태그 위치 보존은 원천 불가
- **출처**: raw/2026-04-05-1430.md

## [active] ContainsKorean 우선 체크 원칙

- **맥락**: 위 ReplacePlainText 기각에서 도출
- **결정**: stripped 텍스트에 한국어가 있으면 TryTranslateCore 호출 없이 즉시 passthrough
- **근거**: 게임이 이미 번역된 텍스트에 렌더링 태그를 씌워서 전달.
           TryTranslateCore를 먼저 호출하면 lexicon substring이 이미 번역된 텍스트 일부와 매칭 → 태그 손상
```

**컴파일 주기:**
- Phase 시작 직전: incremental (마지막 컴파일 이후 새 raw만)
- `/compile` 명령: 수동 즉시 실행
- Phase 종료 시: full reconcile (전체 raw + knowledge 재검증)

#### Stage 3: 린트 (knowledge/ 자가검증)

컴파일 후 LLM이 knowledge를 스캔:
- 충돌하는 active decision?
- 근거 없는 anti-pattern?
- superseded인데 active로 남은 항목?
- 오래 업데이트 안 된 uncertain?

#### Stage 4: 브리핑 (knowledge/ → planning brief)

Phase 시작 시 해당 Phase와 관련된 지식만 뽑아서 brief 생성.

```markdown
# Phase Brief: ink 태그 번역 적용 개선

## 이번 Phase에서 지켜야 할 제약
- [MUST] 번역 적용은 반드시 TryTranslate 체인을 통할 것
- [MUST NOT] PlainText 직접 교체 시도 금지 (ink 태그 파싱 순서 충돌)
- [CHECK] 새 TMP 태그 발견 시 AllTmpTagRegex에 즉시 추가

## 관련 과거 결정
- [rejected] ReplacePlainText 직접 교체 → decisions.md 참조
- [active] ContainsKorean 우선 체크 원칙
```

**핵심:** brief는 "참고 문서"가 아니라 **planning guardrail** — MUST/MUST NOT이 planner 행동을 제약.

#### Stage 5: 조회

- planner/researcher는 planning_brief.md를 Phase 계획의 입력으로 사용
- 상세 근거 필요 시 knowledge/ 원본 직접 참조
- 모든 접근은 파일시스템 읽기(Read/Grep)만으로 동작

### 3.4 GSD 연동 지점

```
gsd-phase-researcher.md
  tools: ..., Read, Grep, Glob  (이미 있음)
  
  추가할 프롬프트:
  "Phase 계획 전에 .knowledge/ 디렉토리에서 관련 키워드를 검색하라.
   decisions.md의 [rejected] 항목은 같은 접근을 시도하지 말라."
```

---

## 4. Exploration Phase 제안

지식 컴파일러와 별개로, 탐색적 작업을 위한 GSD Phase 타입 추가.

| 측면 | 일반 Phase | Exploration Phase |
|------|-----------|-------------------|
| 전제 | 무엇을 할지 안다 | 무엇이 문제인지 모른다 |
| 목표 | 코드/기능 완성 | 원인 파악, 가설 검증 |
| 산출물 | 코드 변경 | 지식 (관측, 결정, 제약) |
| 완료 조건 | 기능 동작 | 문제 원인 확정 또는 가설 기각 |

**내부 루프:**
```
Observe → Hypothesize → Test → Conclude → Compile
  ↑                                          │
  └──────────── 새 관측 발생 시 ◄─────────────┘
```

---

## 5. CPR과의 관계

| 구분 | CPR | 지식 컴파일러 |
|------|-----|------------|
| 비유 | RAM (작업 기억) | Disk (장기 기억) |
| 범위 | 현재 세션 | 프로젝트 전체, 세션 횡단 |
| 트리거 | /compress, /preserve | Phase 전환 시 자동, /compile 수동 |
| 소비자 | 현재 세션의 에이전트 | 다음 세션의 planner/researcher |

분리 유지. /preserve 호출 시 CPR에도 저장 + raw/에도 고품질 기록 추가.

---

## 6. 구현 순서

### MVP (먼저)
1. **raw/ 수집** — Stop Hook + haiku 자동 요약
2. **`/compile` 커맨드** — raw/ → knowledge/ 변환
3. **knowledge/ → planner 연동** — researcher가 knowledge/ 참조

### 이후 확장
4. planning brief 자동 생성
5. lint 단계
6. Exploration Phase 타입 도입
7. 점진적 컴파일 최적화

MVP에서는 brief 없이 planner가 knowledge/를 직접 읽어도 충분.

---

## 7. 현재 상태

- [x] 설계 합의 완료
- [x] Stop Hook 글로벌 설정에 추가 (테스트용 command 타입)
- [x] `.knowledge/raw/` 디렉토리 생성
- [x] GSD 1.32.0 업데이트 (response_language: "ko" 포함)
- [x] Stop Hook 발동 테스트 — command 타입 정상 발동 확인 (따옴표 이스케이프 수정 필요했음)
- [x] Stop Hook prompt 컨텍스트 접근 확인 — 접근 가능하나 검증 모드로 동작, 생성 불가
- [x] MVP Stage 1 수집 메커니즘 결정 — 대안 B (CLAUDE.md 행동 지시) 채택, CLAUDE.md에 섹션 추가 완료
- [x] MVP Stage 2 컴파일 자동화 — researcher Step 0 (incremental) + verifier Step 10b (full reconcile) 에이전트 프롬프트에 추가
- [x] MVP Stage 3 gsd-phase-researcher 프롬프트에 knowledge/ 참조 지시 추가 (Step 0 + Step 3 연동)
- [x] 실전 테스트 — raw/→knowledge/ 컴파일 수동 실행, 4파일 생성 확인 (decisions, anti-patterns, troubleshooting, index)
