---
created: 2026-03-25T03:34:24.960Z
title: OpenCode scaleout orchestration
area: infrastructure
files:
  - workflow/internal/v2pipeline/run.go
  - scripts/manage-opencode-serve.ps1
---

## Problem

단일 OpenCode 인스턴스가 장시간 실행 시 세션 누적으로 성능 저하/타임아웃 발생.
밤새 실행 중 OpenCode가 응답 불능 상태 → 1,000건+ failed 누적.

## Solution

Option C: 자동 스케일
1. 역할별 OpenCode 인스턴스 자동 생성 (translate:4112, format:4113, score:4114)
2. 백그라운드 goroutine으로 주기적 헬스체크 (probeServer 재사용)
3. 응답 없으면 자동 kill → restart → stale claim 복구
4. N시간마다 인스턴스 교체 (세션 누적 방지)
5. Config에 TranslateServerURL/FormatServerURL/ScoreServerURL 이미 분리되어 있음
