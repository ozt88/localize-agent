# Phase 3: 패치 출력 & 전량 실행 - Research

**Researched:** 2026-03-23
**Domain:** Go CLI export tooling, ink JSON manipulation, BepInEx patch artifacts
**Confidence:** HIGH

## Summary

Phase 3는 v2 파이프라인의 `pipeline_items_v2` 테이블에서 `state=done` 항목을 추출하여 세 가지 패치 아티팩트를 생성하는 export CLI(`go-v2-export`)를 구축하고, 40,067건+ 전량을 v2 파이프라인으로 처리하는 것이다. 기술적 핵심은 (1) translations.json v3 포맷 생성, (2) 285개 TextAsset ink JSON에 한국어 역삽입, (3) 8개 localizationtexts CSV 번역이다.

기존 코드베이스가 Phase 3에 필요한 대부분의 인프라를 이미 갖추고 있다. `v2pipeline.Store`에 bulk query 메서드를 추가하고, `inkparse` 패키지의 트리 워킹 로직을 역방향으로 재사용하여 `"^text"` 노드를 교체하면 된다. localizationtexts CSV는 총 698행(헤더 포함)으로 소규모이며, 별도 배치로 LLM 번역 후 CSV 파일을 직접 생성하는 방식이 적합하다.

**Primary recommendation:** `workflow/cmd/go-v2-export/main.go` CLI를 생성하여 translations.json, textassets, localizationtexts를 한 번에 출력. Store에 `QueryDone()` 메서드 추가, inkparse에 `Inject()` 함수 추가가 핵심 작업.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** v2 전용 포맷 (`esoteric-ebb-sidecar.v3`). 각 엔트리에 block ID, source_file, text_role, speaker_hint 포함.
- **D-02:** 동일 source text라도 block ID가 다르면 별도 엔트리로 전부 포함. ID로 고유 식별.
- **D-03:** passthrough 항목(번역 불필요 문자열)도 source=target으로 포함. 완전한 매핑 보장.
- **D-04:** 독립 CLI (`go-v2-export`)로 생성. v2 파이프라인 DB에서 `state=done` 항목 조회하여 출력.
- **D-05:** 전체 JSON 재생성 방식. 원본 ink JSON을 파싱한 후 `"^text"` 노드를 한국어(`ko_formatted`)로 교체하여 새로운 JSON 생성.
- **D-06:** 출력 파일 형식은 `.json` (ink JSON 그대로). Plugin.cs의 TextAssetOverrides가 `.text` getter를 후킹하므로 유효한 ink JSON이어야 함.
- **D-07:** 역삽입 검증 전략은 Claude 재량. 구조 보존, 블록 수 일치 등 적절한 검증 구현.
- **D-08:** 실패률 임계치 설정하여 시스템적 문제 조기 감지. 구체적 임계치는 Claude 재량.
- **D-09:** 모니터링 및 진행 보고 방식은 Claude 재량.
- **D-10:** 부분 완료 상태에서도 done 항목만으로 패치 빌드 가능. export CLI가 done 항목만 조회하여 출력.
- **D-11:** 8개 CSV 파일 전체 전량 번역 (Feats, ItemTexts, JournalTexts, Popups, QuestPoints, SheetInfo, SpellTexts, UIElements).
- **D-12:** CSV 번역의 파이프라인 통합 vs 별도 처리는 Claude 재량. 콘텐츠 유형별 최적 방식 선택.
- **D-13:** runtime_lexicon.json은 Phase 4로 연기. Phase 3에서는 생성하지 않음.

### Claude's Discretion
- 실패률 임계치 수치 설정
- 모니터링/진행 보고 구현 방식
- TextAsset 역삽입 검증 전략 상세
- CSV 번역의 파이프라인 통합 여부 및 방식
- export CLI 필터링 옵션 (content_type별, source_file별 등)

### Deferred Ideas (OUT OF SCOPE)
- runtime_lexicon.json 생성 -- Phase 4 게임 검증에서 동적 텍스트 패턴 확인 후 처리
- Plugin.cs v3 포맷 인식 로직 -- Phase 4 (PLUGIN-01)
- 패치 빌드 스크립트 수정 (BepInEx/doorstop 보존) -- 프로젝트 범위 밖
- 고유명사 음역/의역 정책 개선 -- 별도 작업
- 품질 스코어 기반 선택적 재번역 -- Phase 3 전량 실행 이후 분석
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PATCH-01 | v2 대사 블록 단위 소스로 translations.json 생성 (BepInEx TranslationLoader 호환) | Store.QueryDone() 메서드 추가 + v3 포맷 JSON 생성. Plugin.cs AddEntry()가 `source`/`target` 키를 읽으므로 호환 가능 |
| PATCH-02 | 285개 textassets 파일에 한국어 삽입된 ink JSON 생성 | inkparse 트리 워킹 로직 역방향 적용 -- `"^text"` 노드 위치를 찾아 ko_formatted로 교체 |
| PATCH-03 | localizationtexts CSV 및 runtime_lexicon.json 생성 | CSV 8개 파일 총 ~690행(ENGLISH). LLM 번역 후 CSV 직접 출력. runtime_lexicon.json은 D-13에 의해 Phase 4로 연기 |
| VERIFY-01 | v2 파이프라인으로 전량(40,067건+) 재번역 실행 완료 | 기존 go-v2-pipeline CLI 활용, 모니터링은 CountByState() 주기적 조회로 구현 |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `database/sql` | stdlib | DB 접근 (pipeline_items_v2 조회) | 기존 Store 패턴 그대로 사용 |
| `encoding/json` | stdlib | translations.json 생성, ink JSON 읽기/쓰기 | 프로젝트 전체 표준 |
| `encoding/csv` | stdlib | localizationtexts CSV 읽기/쓰기 | Go 표준, BOM 처리만 주의 |
| `github.com/jackc/pgx/v5` | v5.7.6 | PostgreSQL 백엔드 | 기존 의존성 |
| `modernc.org/sqlite` | v1.38.2 | SQLite 백엔드 | 기존 의존성 |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `workflow/internal/inkparse` | local | ink JSON 파싱 + 역삽입 | TextAsset 역삽입 시 |
| `workflow/internal/v2pipeline` | local | Store, 상태 상수 | done 항목 조회 시 |
| `workflow/pkg/shared` | local | AtomicWriteFile, LoadProjectConfig | 파일 쓰기, 설정 로드 |

**Installation:** 새로운 외부 의존성 없음. 기존 `go.mod` 그대로 사용.

## Architecture Patterns

### Recommended Project Structure
```
workflow/
├── cmd/go-v2-export/main.go       # Export CLI 진입점
├── internal/v2pipeline/
│   ├── store.go                    # QueryDone() 메서드 추가
│   └── export.go                   # (신규) export 도메인 로직
├── internal/inkparse/
│   ├── inject.go                   # (신규) ink JSON 역삽입
│   └── inject_test.go             # 역삽입 테스트
└── internal/contracts/v2pipeline.go # QueryDone 인터페이스 추가
```

### Pattern 1: Export CLI (D-04)
**What:** `go-v2-export` CLI가 DB에서 done 항목을 조회하여 세 가지 아티팩트를 생성
**When to use:** 파이프라인 실행 완료 후 (또는 부분 완료 시 `--min-coverage` 옵션과 함께)
**Example:**
```go
// cmd/go-v2-export/main.go -- established CLI pattern
func main() {
    os.Exit(run())
}

func run() int {
    var cfg exportConfig
    // flag parsing...
    flag.StringVar(&cfg.outDir, "out-dir", "", "output directory")
    flag.Float64Var(&cfg.minCoverage, "min-coverage", 0.0, "minimum done ratio (0-1)")
    flag.StringVar(&cfg.contentType, "content-type", "", "filter by content_type")
    flag.Parse()

    // LoadProjectConfig -> OpenStore -> QueryDone -> generate artifacts
    store, err := v2pipeline.OpenStore(cfg.backend, cfg.dbPath, cfg.dsn)
    // ...
    items, err := store.QueryDone()
    // Generate translations.json, textassets, CSV
}
```

### Pattern 2: ink JSON Injection (D-05)
**What:** 원본 ink JSON의 `"^text"` 노드를 한국어로 교체하여 새 JSON 생성
**When to use:** TextAsset 역삽입 시
**Critical insight:** 파서의 `walkFlatContent`가 `"^text"` 문자열을 읽는 것과 동일한 경로로 역추적하되, 이번에는 읽기 대신 교체해야 함. ink JSON은 `map[string]any` 트리이므로 in-place 수정 가능.

```go
// inkparse/inject.go
// InjectTranslations walks the ink JSON tree and replaces "^text" nodes
// with Korean translations, matching blocks by SourceHash.
func InjectTranslations(data []byte, sourceFile string, translations map[string]string) ([]byte, *InjectReport, error) {
    // 1. Parse JSON into map[string]any
    // 2. Walk tree same as Parse(), but instead of collecting text,
    //    replace "^text" strings with Korean equivalents
    // 3. Re-serialize to JSON
    // 4. Return report with replaced/skipped/missing counts
}
```

**Key detail about `"^text"` structure:**
- ink JSON에서 텍스트는 `"^Some text here"` 형식의 문자열로 저장됨
- 연속된 `"^text"` 문자열들이 하나의 블록을 구성 (파서가 이를 병합)
- 역삽입 시: 원본 블록의 텍스트를 병합하여 source_hash를 계산 -> ko_formatted 매핑 조회 -> `"^text"` 노드를 한국어 텍스트로 교체
- **중요:** 한 블록이 여러 `"^text"` 노드로 구성될 수 있으므로, 첫 번째 노드에 전체 한국어 텍스트를 넣고 나머지는 빈 문자열로 설정하거나, 줄바꿈 기준으로 분할하여 원래 노드 수에 맞춰야 함

### Pattern 3: translations.json v3 Format (D-01, D-02, D-03)
**What:** `esoteric-ebb-sidecar.v3` 포맷으로 모든 done 항목 export
**Format:**
```json
{
  "format": "esoteric-ebb-sidecar.v3",
  "entries": [
    {
      "id": "KnotName/g-0/blk-0",
      "source": "Original English text",
      "target": "한국어 번역",
      "source_file": "TS_SomeScene",
      "text_role": "dialogue",
      "speaker_hint": "Braxo"
    }
  ]
}
```

**Plugin.cs 호환 주의:** 현재 Plugin.cs의 `AddEntry()`는 `source`/`target` 키를 읽어 `TranslationMap[source] = target`으로 저장. v3 포맷의 추가 필드(`id`, `source_file`, `text_role`, `speaker_hint`)는 Phase 4(PLUGIN-01)에서 처리. 현재 Plugin.cs는 v3의 `source`/`target`을 인식하므로 기본 기능은 동작.

### Pattern 4: CSV Translation for localizationtexts (D-11, D-12)
**What:** 8개 CSV 파일의 ENGLISH 칼럼을 LLM 번역하여 KOREAN 칼럼 채우기
**Recommendation (Claude's Discretion -- D-12):** v2 파이프라인에 통합하지 않고 별도 처리.

근거:
1. CSV 항목은 총 ~690행으로 소규모 (vs 77K+ ink 블록)
2. CSV는 단순 `ID,ENGLISH,KOREAN` 구조로 ink 블록의 복잡한 태그/배칭/스코어링 불필요
3. v2 파이프라인의 3-role worker pool (translate/format/score)은 CSV에 과도한 오버헤드
4. CSV 내용은 Feats/Spells/UI 등 콘텐츠별로 묶어서 LLM에 한 번에 보내는 것이 품질에 유리

**방식:** export CLI 내에서 LLM 번역 호출하거나, 별도 CLI로 CSV 번역 후 export CLI가 결과를 취합. 가장 단순한 접근: export CLI에 `--translate-csv` 플래그로 CSV 번역을 포함하되, 이미 번역된 행은 건너뜀.

### Anti-Patterns to Avoid
- **ink JSON 문자열 치환으로 역삽입:** 텍스트에 JSON 특수문자가 포함될 수 있어 regex/string replace는 깨짐. 반드시 JSON 파싱 후 트리 조작 방식 사용
- **source_hash 대신 ID로 매칭:** 동일 source_hash가 여러 ID에 매핑됨 (dedup). 역삽입 시 source text -> source_hash -> ko_formatted 방식이 정확
- **한 번에 모든 done 항목 메모리 로드:** 77K건 모두 로드해도 ~200MB 이하이므로 문제 없지만, TextAsset별 그룹핑을 위한 map 구조가 더 효율적

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| JSON 파싱/생성 | Custom JSON serializer | `encoding/json` | 표준 라이브러리가 ink JSON의 다양한 타입(string, array, map, null, number)을 모두 처리 |
| CSV 파싱 | 수동 comma split | `encoding/csv` | 따옴표 내 쉼표, 줄바꿈 등 에지 케이스 처리 (Feats.txt에 이미 quoted 필드 존재) |
| ink JSON 트리 워킹 | 새로운 walker | `inkparse` 패키지 로직 재사용 | 이미 검증된 `"^text"` 수집 로직을 역방향으로 적용 |
| UTF-8 BOM 처리 | 수동 byte 비교 | 기존 `bytes.TrimPrefix(data, utf8BOM)` 패턴 | 프로젝트 전체 일관성 |

## Common Pitfalls

### Pitfall 1: ink JSON `"^text"` 노드 분할/병합 불일치
**What goes wrong:** 파서는 연속 `"^text"` 노드를 하나의 블록으로 병합하지만, 역삽입 시 한국어 텍스트를 다시 여러 노드로 분할해야 함. 잘못 분할하면 게임 렌더링이 깨짐.
**Why it happens:** 원본에서 `"^Hello "`, `"^world\n"` 두 노드가 `"Hello world\n"`으로 병합됨. 역삽입 시 한국어를 어디서 끊을지 결정 필요.
**How to avoid:** 가장 안전한 접근은 첫 번째 `"^text"` 노드에 전체 한국어 텍스트를 넣고, 나머지 노드는 빈 `"^"` prefix만 유지하는 방식. 또는 원본 노드 경계를 기억해두고 글자 수 비례로 분배.
**Warning signs:** 역삽입 후 블록 수가 원본과 다르면 즉시 감지 가능.

### Pitfall 2: source_hash 충돌과 역삽입 매핑
**What goes wrong:** 동일 텍스트가 여러 파일/위치에 나타나면 source_hash가 같고 pipeline DB에는 하나의 항목만 존재 (ON CONFLICT DO NOTHING). 역삽입 시 어떤 번역을 사용할지 혼란.
**Why it happens:** Seed 시 dedup 정책에 의해 동일 source_hash는 첫 번째만 INSERT.
**How to avoid:** 역삽입은 source_raw -> source_hash -> ko_formatted 매핑 사용. 동일 source_hash면 동일 번역이 적용되므로 문제 없음. DB에서 source_hash -> ko_formatted 매핑을 먼저 구축하고, 각 TextAsset의 `"^text"` 블록에서 source_hash를 계산하여 조회.

### Pitfall 3: BOM 처리 일관성
**What goes wrong:** 원본 TextAsset .txt 파일에 UTF-8 BOM이 있고, 출력에서 BOM을 누락하면 게임이 파싱 실패할 수 있음. 반대로 BOM을 추가하면 다른 도구가 깨질 수 있음.
**Why it happens:** 원본 ink JSON 파일은 BOM으로 시작 (첫 바이트 `0xEF 0xBB 0xBF`). CSV도 BOM 포함.
**How to avoid:** 입력 시 BOM strip, 출력 시 원본과 동일하게 BOM 포함. 기존 `bytes.TrimPrefix` 패턴 활용.

### Pitfall 4: CSV 따옴표 처리
**What goes wrong:** Feats.txt에 이미 따옴표로 감싼 필드 존재 (콤마 포함 텍스트). 수동 split하면 필드 경계 오인식.
**Why it happens:** `"Lone Cleric - At the start of your Short Rest, heal..."` 같이 쉼표 포함 텍스트.
**How to avoid:** `encoding/csv` 패키지 사용 필수. 출력 시에도 한국어에 쉼표가 포함되면 자동 따옴표 처리.

### Pitfall 5: TextAsset 출력 포맷 -- `.txt` vs `.json`
**What goes wrong:** Plugin.cs의 `LoadTextAssetOverrides()`가 `"*.txt"` 패턴으로 파일을 검색함. `.json` 확장자로 출력하면 로드되지 않음.
**Why it happens:** D-06에서 `.json` 형식이라 했지만, Plugin.cs 코드를 보면 `Directory.EnumerateFiles(dir, "*.txt")` 패턴 사용.
**How to avoid:** **출력 파일 확장자는 `.txt`여야 함.** 내용은 ink JSON이지만 파일명은 `{AssetName}.txt`. Plugin.cs가 파일명(확장자 제외)으로 `TextAssetOverrides[name]`에 저장하고 게임의 TextAsset 이름과 매칭하므로, 파일명은 원본 TextAsset 이름과 정확히 일치해야 함.

### Pitfall 6: localizationtexts와 textassets 경로 혼동
**What goes wrong:** Plugin.cs의 `LoadTextAssetOverrides()`가 `localizationtexts/`와 `textassets/` 디렉토리 **모두를** `*.txt` 패턴으로 스캔하여 `TextAssetOverrides`에 로드함.
**Why it happens:** Plugin.cs 1209-1215행에서 candidateDirs에 localizationtexts와 textassets 모두 포함.
**How to avoid:** 두 디렉토리의 파일을 별도 용도로 관리. localizationtexts CSV는 `LoadLocalizationIdOverrides()`에서 별도로 ID,ENGLISH,KOREAN 파싱. textassets의 `.txt` 파일은 전체 텍스트로 TextAssetOverrides에 로드. 두 메커니즘이 독립적으로 동작하므로 파일 충돌 주의.

## Code Examples

### QueryDone 메서드 (Store 확장)
```go
// Source: 기존 store.go 패턴 기반, CountByState()와 동일 구조
func (s *Store) QueryDone() ([]contracts.V2PipelineItem, error) {
    rows, err := s.db.Query(s.rebind(`
        SELECT id, sort_index, source_file, knot, content_type, speaker, choice, gate,
               source_raw, source_hash, has_tags, state, ko_raw, ko_formatted,
               translate_attempts, format_attempts, score_attempts, score_final,
               failure_type, last_error, attempt_log, claimed_by, batch_id
        FROM pipeline_items_v2
        WHERE state = ?
        ORDER BY sort_index`),
        StateDone,
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var items []contracts.V2PipelineItem
    for rows.Next() {
        item, err := s.scanItem(rows)
        if err != nil {
            return nil, err
        }
        items = append(items, item)
    }
    return items, rows.Err()
}
```

### translations.json v3 생성
```go
// Source: v1 translations_v1_deploy.json 포맷 참고, D-01/D-02/D-03 반영
type V3Sidecar struct {
    Format  string       `json:"format"`
    Entries []V3Entry    `json:"entries"`
}

type V3Entry struct {
    ID          string `json:"id"`
    Source      string `json:"source"`
    Target      string `json:"target"`
    SourceFile  string `json:"source_file"`
    TextRole    string `json:"text_role"`
    SpeakerHint string `json:"speaker_hint"`
}
```

### ink JSON 역삽입 핵심 로직
```go
// Source: inkparse/parser.go walkFlatContent 로직 역방향 적용
// 원본과 동일한 트리 워킹으로 "^text" 노드를 수집하되, 교체 모드에서 동작
func injectWalkFlatContent(arr []any, knot, gate, choice string,
    translations map[string]string, // source_hash -> ko_formatted
    report *InjectReport) {
    // 1. 원본과 동일하게 "^text" 노드들을 순회하며 텍스트 버퍼에 수집
    // 2. divert 또는 컨테이너 끝에서 flush: 수집된 텍스트의 source_hash 계산
    // 3. translations map에서 ko_formatted 조회
    // 4. 찾으면: 첫 번째 "^text" 노드를 "^" + ko_formatted로 교체,
    //    나머지 노드는 "^"로 설정 (빈 텍스트)
    // 5. 못 찾으면: 원본 유지, report에 missing 카운트 증가
}
```

### CSV 번역 출력
```go
// Source: 기존 localizationtexts 포맷 (BOM + ID,ENGLISH,KOREAN)
func writeCSV(path string, rows [][]string) error {
    var buf bytes.Buffer
    buf.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
    w := csv.NewWriter(&buf)
    for _, row := range rows {
        w.Write(row)
    }
    w.Flush()
    return shared.AtomicWriteFile(path, buf.Bytes(), 0o644)
}
```

## State of the Art

| Old Approach (v1) | Current Approach (v2) | Impact |
|---|---|---|
| source text를 key로 TranslationMap 직접 매칭 | block ID 기반 고유 식별 + source text 매칭 폴백 | 동명이사 텍스트 문맥별 번역 가능 |
| translations_v1_deploy.json (flat source/target) | esoteric-ebb-sidecar.v3 (id + metadata) | Phase 4에서 Plugin.cs가 ID 기반 매칭 활용 |
| Python 스크립트로 패치 빌드 | Go CLI (`go-v2-export`)로 통합 | 빌드 체인 단순화, 타입 안전성 |

## Open Questions

1. **ink JSON `"^text"` 분할 전략**
   - What we know: 파서가 연속 `"^text"` 노드를 병합하여 하나의 블록으로 만듦
   - What's unclear: 역삽입 시 한국어를 원본과 동일한 수의 `"^text"` 노드로 분할해야 하는지, 아니면 첫 노드에 전체를 넣어도 게임이 정상 렌더링하는지
   - Recommendation: 첫 번째 노드에 전체 텍스트, 나머지는 빈 `"^"` (가장 안전). 게임 테스트로 검증 (Phase 4 VERIFY-02 범위)

2. **CSV 번역 LLM 호출 방식**
   - What we know: 8개 파일, 총 ~690행 (ENGLISH 기준)
   - What's unclear: export CLI 내에서 LLM 호출할지, 별도 사전 작업으로 처리할지
   - Recommendation: 별도 Go CLI 또는 export CLI의 서브커맨드로 CSV 번역 실행. 번역 결과를 중간 파일이나 DB에 저장한 뒤 export CLI가 취합. CSV 규모가 작으므로 한 번에 시트별로 LLM 호출 가능.

3. **전량 실행 예상 소요 시간**
   - What we know: 40,067건 중 passthrough(이미 done)를 제외한 실제 LLM 처리 대상 수 미확인
   - What's unclear: OpenCode 서버의 처리 속도에 따른 총 소요 시간
   - Recommendation: CountByState()로 초기 상태 확인 후 예상 시간 산출. 실패률 임계치로 조기 중단 가능하게 설계.

## Sources

### Primary (HIGH confidence)
- `workflow/internal/v2pipeline/store.go` -- DB 스키마, Seed/ClaimPending/MarkState 패턴
- `workflow/internal/inkparse/parser.go` -- 트리 워킹, `"^text"` 수집 로직
- `workflow/internal/contracts/v2pipeline.go` -- V2PipelineItem DTO, 상태 상수
- `projects/esoteric-ebb/patch/mod-loader/EsotericEbb.TranslationLoader/Plugin.cs` -- LoadEntriesFromJson, LoadTextAssetOverrides, LoadLocalizationIdOverrides
- `workflow/cmd/go-v2-ingest/main.go` -- 인제스트 패턴 (역방향이 export)
- `projects/esoteric-ebb/extract/1.1.3/ExportedProject/Assets/TextAsset/*.txt` -- 원본 ink JSON
- `projects/esoteric-ebb/extract/1.1.3/ExportedProject/Assets/StreamingAssets/TranslationPatch/localizationtexts/*.txt` -- CSV 포맷 확인

### Secondary (MEDIUM confidence)
- `projects/esoteric-ebb/output/batches/canonical_full_retranslate_dual_score_20260311_1/translations_v1_deploy.json` -- v1 포맷 참조
- `projects/esoteric-ebb/patch/tools/build_patch_package_unified.ps1` -- 패치 디렉토리 구조

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - 기존 프로젝트 의존성 그대로 사용, 새 외부 라이브러리 없음
- Architecture: HIGH - 기존 CLI/Store/inkparse 패턴 재사용, 신규 패턴 최소
- Pitfalls: HIGH - Plugin.cs 코드와 원본 데이터 직접 검증 완료
- TextAsset 역삽입: MEDIUM - `"^text"` 분할 전략은 게임 테스트 필요

**Research date:** 2026-03-23
**Valid until:** 2026-04-23 (게임 버전 1.1.3 고정이므로 안정적)
