# Phase 03: Execution Log

실행 중 발견된 문제, 계획 변경, 의사결정을 기록합니다.

## 파서 버그: 블록 ID 충돌 (168건)

**발견**: v2 ingest 시 PK 충돌 에러
**원인**: 여러 소스 파일이 같은 knot 구조 공유 (예: `HiddenSafe` → `AR_HiddenFreestriderSafe`, `TE_HiddenTeaShopSafe`)
**수정**: ID 포맷 `{path}/blk-{idx}` → `{sourceFile}/{path}/blk-{idx}`
**영향**: 파서 재실행 + ingest 재실행 필요했음
**커밋**: `de7c05a`

## OpenCode agent 필드 버그

**발견**: 모든 워커가 warmup에서 `empty response body` 에러
**원인**: OpenCode 1.2.26에서 `agent` 필드 포함 시 빈 응답 반환
**재현**: curl로 agent 필드 포함/미포함 비교 → agent 포함 시 100% 빈 응답
**수정**: EnsureContext, SendPrompt에서 agent 필드 제거
**커밋**: `7c50c68`

## OpenCode directory 파라미터 → 프로젝트 스캔 병목

**발견**: OpenCode serve가 요청마다 수 분씩 지연
**원인**: `directory=localize-agent` 파라미터로 인해 .claude/ 스킬, GSD 명령어 등을 매번 로딩
**수정**: LLM client에서 directory 파라미터 제거 + serve wrapper를 격리 디렉토리에서 실행
**커밋**: `bd6e726`, `299ef28`

## Superpowers 플러그인 제거

**발견**: OpenCode가 .claude/skills/의 Superpowers를 로딩하여 추가 지연
**판단**: 14개 스킬 전부 GSD와 중복이거나 불필요
**수정**: ~/.claude/skills/ 전체 삭제, deprecated commands 3개 삭제
**영향**: Claude Code 세션 오버헤드 감소

## Validation 전략 변경: 배치 전체 reject → ratio 기반

**발견**: failed 7,158건 누적 (전체 20%)
**원인**: 배치 10줄 중 1줄이 ascii_heavy면 전체 reject. 게임 태그/고유명사가 ASCII 비율을 올림
**수정**:
1. `degenerateReason`: HTML 태그(`<b>`, `<i>`)와 게임 토큰(`ROLL`, `SPELL`, `.변수명`) strip 후 ASCII 비율 계산
2. `ValidateTranslation`: ≤50% degenerate면 accept, >50%만 reject
**결과**: failed 비율 20% → 0.5% 이하로 감소
**커밋**: `3413976`

## S/F 단일 문자 passthrough

**발견**: `"S"`, `"F"` 블록이 번역 실패
**원인**: 스킬체크 Success/Fail 마커가 dialogue로 파싱됨
**수정**: `IsPassthrough`에 단일 대문자 체크 추가
**영향**: 4건 (향후 재파싱 시 자동 적용)
**커밋**: `dfc1fbf`

## Score 배치 처리 최적화

**발견**: score 단계가 병목 (1건당 1회 LLM 호출, 35K번 round-trip)
**수정**: `BuildBatchScorePrompt` + `ParseBatchScoreResponse` 추가, 10건/호출로 변경
**결과**: score 워커 8→4로 줄여도 처리 속도 유지
**커밋**: `ec28321`

## 능력치 화자(wis/str/int/con/dex/cha) 처리

**발견**: 번역 결과에 `con:`, `str:` prefix가 오염
**분석**:
- 게임에서 능력치는 내면의 목소리로 화자 역할 (핵심 시스템)
- ink `#con` 태그로 speaker 메타데이터 전달 → 게임 엔진이 UI에 표시
- `source_raw`에는 prefix 없음 → `ko_formatted`에 있으면 안 됨
- 캐릭터 speaker(Braxo, Snell 등)는 문제 없음 — 능력치만 해당
**수정**:
1. 프롬프트 규칙: "speaker labels are context only — do NOT include in output"
2. export: `abilityPrefixRe`로 잔여 prefix strip
3. 기존 done 능력치 항목 1,631건 리셋 → 재번역
**커밋**: `4ace692`

**추가 — 말투 가이드**:
- 각 능력치마다 고유한 성격/말투가 있음 (게임 핵심 콘텐츠)
- wis=직관적 관찰자, str=의지/육체, int=분석가, cha=사교꾼, dex=반사신경, con=신체감각
- warmup 프롬프트에 한국어 톤 가이드 추가
**커밋**: `c094591`

## GPT Pro 사용량 제한

**발견**: OpenCode serve가 정상 시작되지만 메시지 응답 없음 (60초 타임아웃)
**원인**: GPT Pro 계정 rate limit 도달
**해결**: 다른 계정으로 전환
**교훈**: OpenCode serve 모드에서 rate limit 시 에러 메시지 없이 조용히 타임아웃됨

## 계획 대비 변경 사항 요약

| 원래 계획 | 실제 | 이유 |
|-----------|------|------|
| 파서 출력 그대로 사용 | ID 포맷 변경 + 재파싱 | 크로스 파일 ID 충돌 |
| OpenCode 기존 설정 사용 | agent/directory 제거, 격리 실행 | 호환성 문제 |
| 단건 score | 배치 score (10건/호출) | 성능 병목 |
| 전량 실행 후 검증 | 실행 중 validation 전략 변경 | failed 비율 과다 |
| speaker 태그 그대로 보존 | 능력치 prefix 제거 + 말투 가이드 | 게임 렌더링 구조와 불일치 |
