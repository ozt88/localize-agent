# Decisions

## RAG 구현 방식: PageIndex 대신 enriched termbank [active]
**Attempt:** PageIndex(VectifyAI) — 인덱싱+쿼리 모두 LLM 호출 필수
**Result:** 매 쿼리마다 LLM이므로 40K 배치에 비실용적. OpenCode 서버가 custom session API 사용해서 LiteLLM(PageIndex 의존)과 비호환
**Decision:** wiki(283p) + GlossaryTerms.txt(610건) 통합 enriched termbank + word-boundary regex 매칭으로 batch_id → top-3 hints JSON 사전 생성
**Status:** [active] (2026-04-10)

## v2pipeline에 lore 통합 방식 [active]
**Attempt:** v1 translation 패키지의 lore.go 패턴 재사용
**Result:** v2pipeline에는 lore 통합이 전혀 없음 (v1에만 존재). 신규 ragcontext 패키지로 구현
**Decision:** `workflow/internal/ragcontext/` 패키지 신규 생성, `--rag-context` CLI 플래그로 batch_id→hints JSON 파일 경로 주입
**Status:** [active] (2026-04-10)

## Phase 09 voice card 재설계 방향 [active]
**Attempt:** DB 대화 샘플만 사용하는 go-generate-voice-cards CLI (speech_style/honorific/personality 3필드)
**Result:** Kattegatt 등 특수 말투 캐릭터 미포함, wiki 세계관 정보 미활용 → 고어체 소실
**Decision:** wiki 캐릭터 페이지 + DB 공동 출현 화자 → LLM이 `relationships` 필드(화자→청자 관계) 포함 생성. 배치 내 등장 speaker로 relationships 필터링(토큰 압박 방지). speech_style에 구체적 어미 예시("어미: ~야, ~잖아") 추가.
**Status:** [active] (2026-04-12)

## watchdog probe 방식: deepProbe → probeServer [active]
**Attempt:** watchdog deepProbe — LLM 요청으로 서버 활성 확인
**Result:** 서버 바쁠 때 deepProbe가 timeout → false kill → 모든 worker 연결 끊김
**Decision:** `probeServer`(단순 HTTP GET)로 교체. TCP/LLM 응답은 probe에 부적합.
**Status:** [active] (2026-04-12, commit 일부)

## applyProjectDefaults checkpoint_backend 처리 [active]
**Attempt:** project.json의 `translation.checkpoint_backend` 값을 v2pipeline에 그대로 상속
**Result:** `shared/project.go`에서 checkpoint_backend 미지정 시 기본값 "sqlite" 반환 → v2pipeline이 빈 SQLite DB에 연결 → `total=0` 버그 발생
**Decision:** `applyProjectDefaults`에서 `projBackend != "" && projBackend != "sqlite"` 조건 추가, 그 외엔 "postgres"로 강제 설정
**Status:** [active] (2026-04-11, commit 1265ac3)
