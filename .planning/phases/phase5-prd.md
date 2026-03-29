# Phase 5: 미번역 커버리지 개선 — PRD

**작성일:** 2026-03-29
**기준 데이터:** untranslated_capture.json (2026-03-29T11:51:37, Plugin v2.0.0)
**현재 상태:** 838건 캡처, 341건 오탐(이미 번역됨), **497건 실제 미번역**

## 배경

Phase 04.2 완료 후 PLUGIN-03 커버리지 98.8% 달성. 나머지 미번역 497건은 5가지 근본 원인으로 분류됨.
이 Phase의 목표는 미번역 건수를 최대한 줄여 플레이어 체감 품질을 높이는 것.

---

## 카테고리 1: 렌더링 래퍼 (136건)

**근본 원인:** 게임 엔진이 런타임에 텍스트를 TMP 태그로 감싸서 렌더링. DB의 clean source와 불일치.

### 1a. Color 래퍼 (77건)
게임이 대사/선택지에 `<#hex>...</color>` 또는 `<color=X>...</color>` 태그를 씌움.

패턴:
```
<#DB5B2CFF>"Ragn."</color>           → inner: "Ragn."
<#DB5B2C44>"Which font should I choose?"</color>  → inner: "Which font..."
<color=#B9C03B>a ton of loot.</color> → inner: a ton of loot.
```

**해결:** Plugin.cs TryTranslate에서 color 래퍼 strip → inner text로 lookup → re-wrap.
단, `<#DB5B2CFF>` 중 54건은 `<noparse></noparse>` 접두사도 함께 붙어 있음(1b와 복합).

### 1b. Noparse 래퍼 (30건)
게임이 `<noparse></noparse>` 빈 태그를 텍스트 앞에 붙임.

패턴:
```
<noparse></noparse>!
<noparse></noparse>"Here's what I told them."
<noparse></noparse>20
<noparse></noparse><#DB5B2C44>"<i>Propaganda</i>?"</color>
```

**해결:** Plugin.cs에서 `<noparse></noparse>` 빈 쌍 제거 후 lookup.

### 1c. Line-indent 래퍼 (29건)
Phase 04.1에서 `ChoiceWrapperRegex`로 대부분 해결했으나, 29건은 여전히 영어로 남음.

패턴:
```
<#FFFFFFFF><line-indent=-10%><link="0">1.   "The Nationalists."</link></line-indent></color>
```

**해결:** `ChoiceWrapperRegex` 디버깅 — 왜 이 29건이 매칭 실패하는지 조사.
가능성: DB에 해당 선택지 텍스트 자체가 없음 (번역 안 된 원문).

---

## 카테고리 2: UI 라벨 (248건)

**근본 원인:** 게임 UI 텍스트가 runtime_lexicon에 등록되지 않음.

### 2a. 게임 설정/메뉴 (번역 필요, ~60건)
```
Ambient Volume, Master Volume, Music Volume, UI Volume, Voice Volume, World Volume
Fullscreen, Borderless, Resolution, VSYNC (120), Display Mode
Dialog Fonts, Dialog Text Size, UI Size, Colorblind Mode
Continue, Resume, Save, Quit to Menu, Return to Menu, Press Any Button
Settings, Audio, Visuals, RESET SETTINGS
Quality, Normal, High, Large, Off
Yes, No, Skip, SKIP
```

### 2b. 게임 용어/메카닉 (번역 필요, ~50건)
```
Hit Dice, Hit Point +X, Spell Difficulty, Spellbook, Prepared Spells, Concentration
Proficiency, Proficient, Proficiencies, Experience, Level Advancement
Stats, Inventory, Journal, Feats, Lore, Spellcasting Opportunity
Examine, Take All, Speak with Dead, Darkvision
Dead, Locked, COMPLETED, Journal Updated
```

### 2c. 직업/종족/배경 이름 (번역 필요, ~30건)
```
Barbarian, Bard, Cleric, Druid, Rogue, Wizard
Arcanist, Agrarian, Scholar Cleric, Primal Cleric, Trickster Cleric, Unstable Cleric
Beefy Cleric, Level X Cleric, Level X Mercenary
Nationalist, Freestrider, Azgalist, Apolitical
```

### 2d. 지역 이름 (번역 여부 결정 필요, ~30건)
```
Darrow's Nest, Goblin Garden, Guard Tower, Guild Warehouse
North Caverns, South Caverns, Secret Tunnel, Old Prison
Temple of Urth, Tolstad, Tolstad East, Rollermill
Tea Shop, Snell's Pad, Snurre's Office, Visken's Lair
Living Library (Lost), The City Below the City, The Infinite Wastes
```

### 2e. 고유명사/NPC (원문 유지 가능, ~10건)
```
Christoffer Bodegård, A Strange Human, Kiosk Kid, Punching Bag, Drunk Sphinx
```

### 2f. 인게임 대사 (선택지 텍스트, ~10건)
이 항목들은 UI 라벨이 아니라 선택지/대사인데 래퍼 없이 표시됨:
```
"Actually, I'm apolitical."
"The Nationalists."
"The Freestriders."
"Here's what I told them."
Okay. Thank you, voices in my head. (Leave.)
```
DB에 해당 source_raw가 있는지 확인 필요.

---

## 카테고리 3: UI 템플릿/패턴 (52건)

**근본 원인:** 게임이 런타임에 변수를 치환하여 표시. 번역 불가능하거나 불필요.

### 3a. Passthrough (번역 불필요, ~40건)
```
숫자/퍼센트: 1%, 10%, 100%, +0, +1, +2, -1, -2
주사위: 1d20, 1d4, Xd6, Xd10, Xd4
해상도: 2560 x 1440
버전: v1.1.3, vX.XX
시간/날짜 템플릿: 00:00, Day XX - XX:XX, 20XX-XX-XX
기타: 0/300xp, XX | XX | XX | XX | XX | XX
```

### 3b. 부분 번역 필요 (~12건)
```
+X Attribute / -X Attribute  → +X 속성 / -X 속성
+X Reason for getting        → +X 획득 사유
- Cast SpellName -            → - 주문 시전 -
- Show Quest Branch -         → - 퀘스트 분기 -
- Show Unlocked Feat -        → - 해금된 특기 -
Day X, Day 01, Round X        → X일차, 1일차, X라운드
Level X reached               → 레벨 X 도달
```
regex lexicon 규칙으로 처리 가능.

---

## 카테고리 4: 게임 대사 (41건)

**근본 원인:** DB에 source_raw가 존재하지만, 런타임 텍스트에 inline `<color>`, `<size>`, `<b>` 등이 포함되어 정확 매칭 실패. 또는 DB에 아예 없는 텍스트.

샘플:
```
(And the <i>Trickster</i>, huh? I'll be sure to prep some nice loot for you to steal.)
Be violent and/or manly <size=33>Useful in Encounters
Become a <b>ZEALOT</b>. Regain your honor. Set out on your <b><size=33>DIVINE MISSION</b>.
And remember to get <color=#B9C03B>a ton of loot.</color>
Alright, so- last time you got through the whole intro sequence.
Five days until the first ever election.
A loner, a thinker, a man of ideas.
```

**해결:**
- DB에 있는 41건: inline 태그 strip 후 lookup (Plugin.cs)
- DB에 없는 건: intro/메타 텍스트로 별도 번역 후 lexicon 추가

---

## 카테고리 5: 멀티라인 텍스트 (20건)

**근본 원인:** 주문 설명, 능력치 설명, 시 인용 등 여러 줄로 구성된 텍스트. DB 매칭 구조와 불일치.

샘플:
```
1st-level enchantment\nCasting Time: 1 action\nRange: Self\n...
<b><color=#FF3959>STRENGTH</color></b> measures your natural bodily prowess...
<i>"Every shepherd knows the song..."</i>\n-Torna's Lament, from...
An explosion erupts at the heart of the city.\nNo one wants to deal with it...
```

**해결:** TextAsset 오버라이드로 처리하거나, 멀티라인 source로 DB에 추가.

---

## 부가 작업: 캡처 로직 수정

**문제:** 번역 완료된 텍스트 341건이 untranslated로 잡힘. 렌더링 래퍼가 씌워진 상태에서 캡처 로직이 "wrapper 포함 전체 텍스트"를 miss로 기록.

**해결:** Plugin.cs의 miss 캡처 로직에서, wrapper unwrap 후 inner text가 번역된 경우 캡처에서 제외.

---

## 수치 요약

| 카테고리 | 건수 | 해결 방법 | 우선순위 |
|----------|------|-----------|----------|
| 1. 렌더링 래퍼 | 136 | Plugin.cs wrapper strip | 높음 |
| 2. UI 라벨 | 248 | runtime_lexicon 확장 | 높음 |
| 3. UI 템플릿 | 52 | passthrough + regex lexicon | 중간 |
| 4. 게임 대사 | 41 | inline 태그 strip + 추가 번역 | 중간 |
| 5. 멀티라인 | 20 | TextAsset 또는 DB 추가 | 낮음 |
| 부가: 캡처 버그 | 341 (오탐) | 캡처 로직 수정 | 중간 |
| **합계** | **497 실제 미번역** | | |

## 결정 필요 사항

1. **지역 이름 번역 여부** — 원문 유지 vs 한국어 번역 (예: "Darrow's Nest" → "대로우의 둥지"?)
2. **고유명사 처리** — NPC 이름은 원문 유지, 직업명은 번역?
3. **passthrough 범위** — 숫자/퍼센트/주사위를 일괄 passthrough 처리?
4. **멀티라인 처리 방식** — TextAsset vs DB 추가 vs 무시?
5. **캡처 버그 수정 시점** — Phase 5에 포함 vs 별도 작업?
