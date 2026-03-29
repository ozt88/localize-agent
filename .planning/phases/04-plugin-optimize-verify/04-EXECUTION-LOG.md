# Phase 04 Execution Log

## Timeline

### 2026-03-26 — Wave 1 실행
- 04-01 (V3Sidecar contextual_entries): 에이전트 완료, 3 commits
- 04-02 (Plugin.cs 4-stage): 에이전트 Task 1 완료(TryTranslate 8→4), Task 2 권한 문제로 orchestrator가 인라인 완료
- Wave 1 완료, Wave 2(04-03 배포+검증) 시작

### 2026-03-26 — 04-03 Task 1: translations.json 재생성
- go-v2-export 실행 성공: 35,030 entries, contextual_entries 포함 확인
- TextAsset 경로 수정 필요 (TextAssets → TextAsset, 단수형)
- TextAsset 286 파일 생성, 4,656 블록 번역 적용

### 2026-03-26 — 04-03 Task 2: 배포 + 인게임 검증 시작
- v2 배포판 조립: v1의 BepInEx 프레임워크 기반 + v2 Plugin.dll + translations.json + textassets
- Plugin.dll 빌드 시 BepInExRoot 경로 문제 해결 (v1 build의 BepInEx core 사용)

### 2026-03-26~27 — 인게임 버그 발견 및 수정 (계획에 없던 작업)

#### Bug 1: 대사 블록 뭉침
- **발견:** 인트로 대사가 12줄 모두 한 텍스트 버블에 출력
- **원인:** inject.go가 한국어 번역 전체를 첫 ^text 노드에 넣고 나머지를 비움
- **수정:** \n split → 각 ^text 노드에 1줄씩 분배 (cdb9715)

#### Bug 2: LLM 이스케이프 아티팩트
- **발견:** `\"`, 리터럴 `\n`, `str:` 접두사가 화면에 노출
- **원인:** LLM이 JSON 이스케이프를 그대로 출력 (3,360건 리터럴 \n, 2,694건 \")
- **수정:** NormalizeLLMEscapes + CleanTarget 함수 추가 (4ade1c7)
- **2차 수정:** TextAsset injection 경로(hashToKO)에도 동일 적용 누락 → 수정 (d93d7b2)

#### Bug 3: 극히 낮은 히트율 (5.9%)
- **발견:** translations_loaded=6,917 / 35,030, hit rate 5.9%
- **원인 1 — ko_formatted 비어있음:** 35,030 done 중 ko_formatted 있는 건 6,918개만. 태그 없는 30,712건이 ko_raw만 있고 포맷터 스테이지 미적용
- **수정:** ko_formatted 비어있으면 ko_raw fallback (28084c0)
- **결과:** TextAsset missing 13,767 → 0
- **원인 2 — 블록 vs 줄 불일치:** v2가 블록 단위 번역, 게임은 줄 단위 조회
- **수정:** 멀티라인 블록을 \n split, 줄 수 일치(89.2%) 시 개별 entry 추가 (8e93072)
- **결과:** 6,917 → 75,244 로드 가능 entries

#### Bug 4: 볼드 태그 전부 제거
- **발견:** v2 번역에 의도적 `<b>` 태그가 있는데 전부 사라짐
- **원인:** CleanOrphanBoldTags가 v1 기준 "한국어에 <b>있으면 전부 누출이니 제거"
- **수정:** <b>/<\/b> 균형이면 유지, 불균형일 때만 제거 (bdf500d)

#### Bug 5: DC/FC 시스템 접두사 노출
- **발견:** 선택지에 `DC12 str-The Cleric? 마음에 드는데.` 원본 노출
- **원인:** 번역 target에 DC/FC 접두사 포함, TranslationMap 키에도 접두사 포함
- **수정:** export에서 target 접두사 제거 + body-only entry 추가. Plugin.cs Stage 2b 추가 (0f8a41a)

#### Bug 6: 선택지 번호 누락
- **발견:** 번역된 선택지에서 1, 2, 3, 4 번호가 사라짐
- **원인:** 게임이 `<link="N">N.   Body</link>` 형태로 전달 → TryTranslate가 전체 검색 → 번호 포함 래퍼 제거됨
- **수정:** DialogAddChoiceTextPrefix에서 link 태그+번호 분리→본문만 번역→재조립 (217dabe)

#### 폰트 weight 불일치 (미해결)
- **발견:** 같은 줄에서 한글 vs 영문/기호 글자 두께가 다름
- **원인:** Pretendard-Regular fallback 폰트와 게임 기본 폰트 간 weight 차이
- **임시 수정:** FindKoreanFontFile Bold→Regular 우선 변경 (e90555c)
- **상태:** 근본 해결 필요 (게임 기본 폰트와 weight 매칭되는 한국어 폰트 필요)

## 계획 대비 편차

04-03 Task 2(배포+검증)가 단순 검증이 아니라 **6개 critical bug 수정**으로 확대됨. 이 버그들은 Phase 01-03에서 발견 불가능했던 integration-level 이슈:

- inject.go의 줄 분배: 단위 테스트로는 잡을 수 없는 ink 런타임 동작
- ko_raw/ko_formatted 갭: 파이프라인 스테이지 간 데이터 흐름 누락
- Plugin.cs v1 가정: CleanOrphanBoldTags, FindKoreanFontFile 등이 v2 데이터와 호환 안 됨
- 선택지 렌더링: TMP link 태그 래핑은 인게임에서만 관찰 가능

### 2026-03-28 — 선택지 번호 문제 심층 조사
- **발견:** 스크린샷에서 2, 3번 선택지에 번호 없음
- **조사:** AddChoiceText 시그니처 확인 — 9개 파라미터 (text, color, holder, **index**, ...)
- **핵심:** `ink_choice` 캡처 101건 **전부** `<link>` 래핑 없음. 게임이 Plugin 훅 **이후** 번호+link를 추가
- **이전 수정 (217dabe):** `<link>` regex가 절대 매칭 안 됨 → 불필요한 코드
- **수정 (6977155):** regex 제거, 단순 TryTranslate로 변경. 번호는 게임이 독립적으로 처리
- **추가 발견 (f17185e):** `StripQuotationMarks`가 선택지 따옴표 제거 → 게임의 번호 매칭 실패. ink_choice에서는 따옴표 보존하도록 `stripQuotes: false` 추가

### 2026-03-28 — 번역 품질 리뷰 도구 추가
- **요청:** 유저가 v1 대비 v2 번역 품질이 낮다고 판단, 검토 도구 요청
- **1번 도구 (오프라인):** quality_review.tsv — source→v1_target→v2_target 비교 파일
- **2번 도구 (인게임):** Plugin.cs에 translation_hits.json 로그 추가 — ENABLE_FULL_CAPTURE 모드에서 source→target 쌍 기록
- **커밋:** 별도 커밋 예정

## 현재 상태

- translations.json: 75,789 entries (75,244 with target)
- TextAsset: 286 files, 18,423/18,423 blocks replaced (0 missing)
- DC/FC 접두사: 579개 선택지 처리
- 선택지 번호: link 태그 보존
- 미번역: ~320건 (untranslated_capture.json)
- 폰트: Pretendard-Regular 로드 확인, weight 차이 잔존
