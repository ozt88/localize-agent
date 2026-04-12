---
phase: 09-retranslation-execution
plan: "01"
subsystem: translation-pipeline
tags: [go, postgres, clustertranslate, voice-cards, rag, v2pipeline, multiline-parsing]

requires:
  - phase: 08-infrastructure
    provides: v2pipeline 인프라, retranslation_snapshots 테이블, ResetAllForRetranslation CLI

provides:
  - contracts/v2pipeline.go — GetNextLines, GetAdjacentKO 인터페이스
  - v2pipeline/store.go — 위 인터페이스 PostgreSQL 구현
  - clustertranslate/types.go — ClusterTask 6개 컨텍스트 필드
  - clustertranslate/prompt.go — 5개 컨텍스트 블록 주입 + quoteForPrompt (멀티라인 안전)
  - clustertranslate/parser.go — [NN] 청크 분할 기반 멀티라인 파싱
  - v2pipeline worker/run — voice card lazy load + RAG 컨텍스트 주입
  - go-v2-pipeline — --voice-cards, --rag-context CLI 플래그
  - projects/esoteric-ebb/context/voice_cards.json — 27개 캐릭터 프로필 + relationships
  - VL_Visken 10건 샘플 번역 완료 (멀티라인 블록 포함)

affects: [09-02-full-retranslation, 09-03-patch-build]

tech-stack:
  added: []
  patterns:
    - "quoteForPrompt: %q 대신 실제 개행 보존 + 내부 따옴표만 이스케이프"
    - "[NN] 마커 기준 청크 분할로 멀티라인 대화 블록 단일 항목으로 파싱"
    - "stripQuotes: 균형/불균형 따옴표 모두 처리 + \\n→개행 이스케이프 복원"
    - "voice card lazy load: 첫 번째 배치 처리 시 한 번만 로드"
    - "relationships 필터링: 배치 내 등장 화자에 한해 관계 정보 주입"

key-files:
  created:
    - projects/esoteric-ebb/context/voice_cards.json
    - .planning/phases/09-retranslation-execution/09-01-SUMMARY.md
  modified:
    - workflow/internal/contracts/v2pipeline.go
    - workflow/internal/v2pipeline/store.go
    - workflow/internal/clustertranslate/types.go
    - workflow/internal/clustertranslate/voice_card.go
    - workflow/internal/clustertranslate/prompt.go
    - workflow/internal/clustertranslate/parser.go
    - workflow/internal/clustertranslate/parser_test.go
    - workflow/internal/scorellm/prompt.go
    - workflow/internal/v2pipeline/types.go
    - workflow/internal/v2pipeline/worker.go
    - workflow/internal/v2pipeline/run.go
    - workflow/cmd/go-v2-pipeline/main.go
    - workflow/cmd/go-generate-voice-cards/main.go
    - projects/esoteric-ebb/project.json

key-decisions:
  - "quoteForPrompt 도입: %q는 \\n 이스케이프로 멀티라인 파괴 — 실제 개행 보존이 근본 수정"
  - "stripQuotes 불균형 따옴표 처리: 프롬프트 수정에도 레거시 LLM 응답 방어층 유지"
  - "파서를 라인 단위 → [NN] 청크 단위로 재작성: 멀티라인 블록 단일 항목 보장"
  - "voice_cards.json 27개 캐릭터: wiki_markdown + DB 공동 출현 화자 + 관계 정보 포함"

patterns-established:
  - "멀티라인 텍스트 프롬프트 출력: quoteForPrompt 사용 (절대 %q 사용 금지)"
  - "LLM 응답 파싱: [NN] 마커 기준 분할 후 stripQuotes 적용"

requirements-completed: []

duration: 4h
completed: 2026-04-12
---

# Phase 09 Plan 01: Voice Cards + RAG go-v2-pipeline 재통합 Summary

**삭제된 컨텍스트 주입 계층 전량 복원 + 멀티라인 파싱 버그 수정 + voice card/RAG 포함 VL_Visken 번역 샘플 검증 완료**

## Performance

- **Duration:** 약 4시간
- **Started:** 2026-04-12T18:00:00Z
- **Completed:** 2026-04-12T22:30:00Z
- **Tasks:** 4 (+ 2개 편의 수정)
- **Files modified:** 14

## Accomplishments

- Phase 07에서 삭제된 contracts/store/clustertranslate/v2pipeline 계층 전량 복원 (GetNextLines, GetAdjacentKO, ClusterTask 6 필드, 5개 컨텍스트 블록 주입)
- go-generate-voice-cards CLI 개선 + wiki_markdown 기반 27개 캐릭터 voice_cards.json 생성 (Kattegatt 고어체 포함)
- 멀티라인 파싱 버그 발견 및 3단계 수정: `%q` → `quoteForPrompt`, `[NN]` 청크 파서, `stripQuotes` 불균형 따옴표 처리
- VL_Visken 멀티라인 블록(최대 7줄) 완전 번역 + Visken 격식체 voice card 효과 확인

## Task Commits

1. **Task 1: contracts + store + clustertranslate 계층 복원** — `ba98999` (feat)
2. **Task 2: scorellm + v2pipeline Config/worker/run + main.go** — `c155dcc` (feat)
3. **Task 3: voice_cards.json 재생성** — `775ec39` (feat)
4. **Fix: watchdog restartOpenCode 포트 버그** — `5e9934a` (fix)
5. **Task 4-a: 멀티라인 파싱 버그 수정 (prompt + parser)** — `4c3e405` (fix)
6. **Task 4-b: stripQuotes 불균형 따옴표 제거** — `c1ced70` (fix)

## Files Created/Modified

- `workflow/internal/contracts/v2pipeline.go` — GetNextLines, GetAdjacentKO 인터페이스 추가
- `workflow/internal/v2pipeline/store.go` — PostgreSQL 구현, parent_choice_text 스캔
- `workflow/internal/clustertranslate/types.go` — ClusterTask 6개 컨텍스트 필드
- `workflow/internal/clustertranslate/voice_card.go` — Relationships 필드 추가
- `workflow/internal/clustertranslate/prompt.go` — 5개 컨텍스트 블록 + quoteForPrompt 함수
- `workflow/internal/clustertranslate/parser.go` — [NN] 청크 분할 파서 + stripQuotes 개선
- `workflow/internal/clustertranslate/parser_test.go` — 멀티라인 테스트 케이스 추가
- `workflow/internal/v2pipeline/worker.go` — voice card lazy load + RAG 주입
- `workflow/internal/v2pipeline/run.go` — ragCtx 로드 + worker 전달
- `workflow/cmd/go-v2-pipeline/main.go` — --voice-cards, --rag-context 플래그
- `workflow/cmd/go-generate-voice-cards/main.go` — --wiki-dir, relationships 생성 지시
- `projects/esoteric-ebb/context/voice_cards.json` — 27개 캐릭터 프로필 (신규)
- `projects/esoteric-ebb/project.json` — OpenCode 포트 4113 → 4115

## Decisions Made

- **quoteForPrompt 도입**: `%q`는 내부 `\n`을 `\\n`으로 이스케이프하여 LLM이 리터럴 `\n`을 반환하고 이것이 DB에 저장되는 근본 버그. `quoteForPrompt`는 실제 개행을 보존하고 내부 `"`만 이스케이프.
- **파서 청크 분할 재작성**: 기존 `strings.Split("\n")` 라인 단위 파서는 멀티라인 블록을 첫 줄만 파싱. `[NN]` 마커 기준 청크 분할로 멀티라인 전체를 단일 항목으로 처리.
- **stripQuotes 방어층 유지**: 프롬프트 수정에도 불구하고 LLM이 이스케이프 형태로 응답하는 경우 방어. 레거시 DB 항목 포맷팅 시에도 활용.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] 멀티라인 파싱 버그 — %q 이스케이프로 개행 소실**
- **Found during:** Task 4 (VL_Visken 샘플 번역 실행 후 결과 확인)
- **Issue:** `prompt.go`가 `%q` 포맷으로 텍스트 출력 → 멀티라인 블록의 `\n`이 `\\n`으로 이스케이프 → LLM이 리터럴 백슬래시-n 응답 → 파서가 첫 줄만 파싱하거나 이스케이프 문자 잔존
- **Fix:** `quoteForPrompt` 함수 추가 (실제 개행 보존), 파서를 [NN] 청크 분할 방식으로 재작성, stripQuotes에서 `\n` → 개행 복원 + 불균형 따옴표 제거
- **Files modified:** `workflow/internal/clustertranslate/prompt.go`, `workflow/internal/clustertranslate/parser.go`, `workflow/internal/clustertranslate/parser_test.go`
- **Verification:** 멀티라인 테스트 케이스 4개 추가 후 통과, VL_Visken 7줄 블록 완전 번역 확인
- **Committed in:** `4c3e405`, `c1ced70`

**2. [Rule 1 - Bug] watchdog restartOpenCode 잘못된 포트 사용**
- **Found during:** Task 2 이후 파이프라인 실행 시
- **Issue:** `manage-opencode-serve.ps1`이 포트 4112를 사용, 실제 서버는 4115에서 실행
- **Fix:** URL에서 포트 추출하여 직접 `opencode.exe serve --port` 실행
- **Files modified:** `workflow/internal/v2pipeline/run.go`
- **Committed in:** `5e9934a`

---

**Total deviations:** 2 auto-fixed (2 Rule 1 - Bug)
**Impact on plan:** 멀티라인 파싱 버그는 번역 품질에 직접 영향 — 수정 필수. 범위 확대 없음.

## Issues Encountered

- VL_Visken 첫 번역 시도에서 `ko_raw`에 따옴표 잔존 확인 → `stripQuotes` 불균형 따옴표 수정 후 재번역으로 해결
- `c-0/blk-0`, `c-1/blk-0` (`.CAUGHT_STEALING_Visken==0-` 형태) 2건은 계속 failed — 이는 ink 조건 코드로 번역 대상이 아닌 시스템 텍스트로 판단 (degenerate 검증에서 걸림)

## Known Stubs

없음 — 모든 컨텍스트 주입 로직이 실제 DB 쿼리 및 파일 로드로 연결됨.

## Next Phase Readiness

- go-v2-pipeline `--voice-cards`, `--rag-context` 플래그 완비, voice_cards.json 27개 캐릭터 준비됨
- 멀티라인 파싱 버그 수정 완료 → 전량 재번역(09-02) 진행 가능
- VL_Visken Visken 격식체 voice card 효과 확인, Kattegatt 고어체 검증 필요 (해당 씬 포함 여부 확인 필요)
- 09-02에서 전체 40,067건 재번역 시 `--voice-cards`와 `--rag-context` 반드시 포함

---
*Phase: 09-retranslation-execution*
*Completed: 2026-04-12*

## Self-Check: PASSED

- FOUND: `.planning/phases/09-retranslation-execution/09-01-SUMMARY.md`
- FOUND: `projects/esoteric-ebb/context/voice_cards.json`
- FOUND: `workflow/internal/clustertranslate/parser.go`
- FOUND: `workflow/internal/clustertranslate/prompt.go`
- FOUND commit: `ba98999` (contracts + store + clustertranslate 복원)
- FOUND commit: `c155dcc` (scorellm + v2pipeline 복원)
- FOUND commit: `775ec39` (voice_cards.json 재생성)
- FOUND commit: `5e9934a` (watchdog 포트 버그 수정)
- FOUND commit: `4c3e405` (멀티라인 파싱 버그 수정)
- FOUND commit: `c1ced70` (stripQuotes 불균형 따옴표 제거)
