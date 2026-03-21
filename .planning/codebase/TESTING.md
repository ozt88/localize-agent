# Testing Patterns

**Analysis Date:** 2026-03-22

## Test Framework

**Runner (Go — primary):**
- Go's built-in `testing` package — no external test framework
- Config: none (uses `go test ./...`)
- Assertion: no assertion library; raw `if` + `t.Fatalf` exclusively

**Runner (TypeScript — github-docs subtree only):**
- Vitest
- Config: `github-docs/vitest.config.ts`
- Path alias `@/` resolved via `alias` config

**Run Commands (Go):**
```bash
go test ./...                        # Run all tests
go test ./workflow/internal/...      # Run internal package tests
go test -run TestName ./pkg/...      # Run specific test by name
go test -v ./...                     # Verbose output
RUN_LIVE_RECOVERY=1 go test -run TestResidualRecoverabilitySmoke ./workflow/internal/translation/  # Live integration test (skipped by default)
```

---

## Test File Organization

**Location:** Co-located with source files in the same package directory and same package namespace.

**Naming:** `<source_file>_test.go` — e.g. `batch_builder_test.go` next to `batch_builder.go`, `pipeline_mock_test.go` next to `pipeline.go`.

**Special file: `pipeline_mock_test.go`** — Each major package has one of these containing all fake/mock implementations and shared test helpers. These are in the same package (not `_test` external package), so they have access to unexported types.

**Package declaration:** Tests use `package <packagename>` (white-box testing), not `package <packagename>_test`. This gives access to all unexported functions and types.

**Structure:**
```
workflow/internal/translation/
├── batch_builder.go
├── batch_builder_test.go
├── pipeline_mock_test.go          # fakes + test server helper
├── contamination_guard_test.go
├── tags_test.go
├── structured_text_test.go
├── postprocess_validation_test.go
└── testdata/
    ├── failed_quote_fixture.json
    ├── low_score_failed_fixture.json
    └── residual_no_row_fixture.json
```

---

## Test Structure

**Suite Organization — simple functions:**
```go
package translation

import "testing"

func TestMaskRestoreTags_RichTextAndPlaceholders(t *testing.T) {
    src := `Do <i>you</i> like {food} and $NAME?`
    masked, maps := maskTags(src)
    if masked != `Do [T0]you[T1] like [T2] and [T3]?` {
        t.Fatalf("masked=%q", masked)
    }
    got, err := restoreTags(`너는 [T0]정말[T1] [T2]와 [T3]를 좋아해?`, maps)
    if err != nil {
        t.Fatalf("restoreTags error: %v", err)
    }
    if got != want {
        t.Fatalf("got=%q want=%q", got, want)
    }
}
```

**Suite Organization — table-driven tests:**
Used when testing multiple input/output pairs for the same function:
```go
func TestParseCSV(t *testing.T) {
    tests := []struct {
        name string
        give string
        want []string
    }{
        {name: "empty", give: "", want: nil},
        {name: "normal", give: "pass, revise,reject", want: []string{"pass", "revise", "reject"}},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := parseCSV(tt.give)
            if !reflect.DeepEqual(got, tt.want) {
                t.Fatalf("parseCSV(%q) = %v, want %v", tt.give, got, tt.want)
            }
        })
    }
}
```

Also seen as `map[string]string` for simple single-value transforms:
```go
func TestNormalizeStatCheck(t *testing.T) {
    tests := map[string]string{
        "ROLL14 str-": "STR 14",
        "DC13 wis-":   "WIS 13",
    }
    for in, want := range tests {
        if got := normalizeStatCheck(in); got != want {
            t.Fatalf("normalizeStatCheck(%q)=%q want %q", in, got, want)
        }
    }
}
```

**Test naming convention:** `Test<TargetFunction>_<ScenarioDescription>` using PascalCase for each segment:
- `TestBuildBatch_FiltersAndCounts`
- `TestCollectProposals_BatchErrorFallsBackToSingles`
- `TestValidateRestoredOutput_RejectsPoliteChoice`
- `TestLoadDoneItems_PrefersSourceRaw`

**Assertion pattern:** All assertions use `t.Fatalf` (never `t.Errorf`). The format is `field=<got>, want <expected>`:
```go
if result.completedCount != 1 {
    t.Fatalf("result=%+v", result)
}
if got := done["id-1"]["Text"]; got != "localized {name}" {
    t.Fatalf("restored text=%v", got)
}
```

---

## Mocking

**Framework:** No external mock framework. All fakes are hand-written structs implementing interfaces.

**Pattern:** Fake structs capture calls and return configurable values:
```go
type fakeCheckpointStore struct {
    enabled bool
    upserts []string
}

func (f *fakeCheckpointStore) IsEnabled() bool { return f.enabled }
func (f *fakeCheckpointStore) UpsertItem(...) error {
    f.upserts = append(f.upserts, entryID+":"+status)
    return nil
}

// Compile-time interface assertion:
var _ contracts.TranslationCheckpointStore = (*fakeCheckpointStore)(nil)
```

**HTTP mocking:** `net/http/httptest.NewServer` is used to spin up real HTTP test servers for testing LLM client interactions. This is the primary integration technique:
```go
func newServerClientForTest(t *testing.T, responder llmPromptResponder) *serverClient {
    t.Helper()
    ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch {
        case r.Method == http.MethodPost && r.URL.Path == "/session":
            _, _ = w.Write([]byte(`{"id":"s1"}`))
        case r.Method == http.MethodPost && r.URL.Path == "/session/s1/message":
            status, payload := responder(prompt)
            // write response
        }
    }))
    t.Cleanup(ts.Close)
    return &serverClient{llm: platform.NewSessionLLMClient(ts.URL, ...), ...}
}
```

**Custom http.RoundTripper:** Used to inject network-level errors without a real server (see `roundTripperFunc` in `workflow/pkg/platform/llm_client_test.go`):
```go
type roundTripperFunc func(*http.Request) (*http.Response, error)
func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
```

**What to mock:**
- External HTTP servers (LLM backends) — use `httptest.NewServer` with a `responder` function
- Database stores — implement the contracts interface with a `fake*` struct
- File stores — implement `contracts.FileStore` with a `fakeFileStore` that holds an in-memory `jsonData` map

**What NOT to mock:**
- SQLite database — real SQLite is used in tests via `t.TempDir()` paths
- Business logic functions — tested directly (white-box)

---

## Fixtures and Factories

**Test Data (JSON fixtures in `testdata/`):**
Located in `workflow/internal/translation/testdata/`:
- `failed_quote_fixture.json` — fixture for quote-parsing edge cases
- `low_score_failed_fixture.json` — fixture for low-scoring translation results
- `residual_no_row_fixture.json` — fixture for items that produced no output row

**Loading pattern:**
```go
func loadFailedQuoteFixture(t *testing.T) failedQuoteFixture {
    t.Helper()
    path := filepath.Join("testdata", "failed_quote_fixture.json")
    raw, err := os.ReadFile(path)
    if err != nil {
        t.Fatalf("read fixture: %v", err)
    }
    var fx failedQuoteFixture
    if err := json.Unmarshal(raw, &fx); err != nil {
        t.Fatalf("decode fixture: %v", err)
    }
    return fx
}
```

**Inline struct literals:** For most tests, input data is constructed inline as struct literals rather than loaded from files. This keeps tests self-contained and readable.

**Temporary files and directories:** `t.TempDir()` is used for SQLite DBs and JSON files that need to exist on disk during the test:
```go
dbPath := filepath.Join(t.TempDir(), "ckpt.db")
chunkPath := filepath.Join(t.TempDir(), "chunks.json")
os.WriteFile(chunkPath, []byte(raw), 0o644)
```

**Helper functions:** Marked with `t.Helper()`. Examples: `newServerClientForTest`, `loadFailedQuoteFixture`, `makeRow` (defined inside the test function).

---

## Coverage

**Requirements:** No enforced coverage targets. No CI coverage threshold detected.

**View Coverage:**
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

---

## Test Types

**Unit Tests:**
- The dominant form; test individual functions in isolation
- Located in `*_test.go` files alongside source
- Use inline struct data or `testdata/` JSON fixtures

**Integration-style Tests (with real HTTP servers):**
- Use `httptest.NewServer` to test full LLM client request/response cycles
- Test files: `workflow/internal/translation/pipeline_mock_test.go`, `workflow/internal/evaluation/pipeline_mock_test.go`, `workflow/internal/semanticreview/api_test.go`, `workflow/pkg/platform/llm_client_test.go`
- These spin up real HTTP servers in-process and exercise the full request path

**Database Integration Tests:**
- Use real SQLite via `t.TempDir()` for temp DB paths
- Test files: `workflow/internal/semanticreview/reader_test.go`, `workflow/internal/translation/checkpoint_writer_test.go`
- Includes concurrency tests: `TestCheckpointBatchWriter_ConcurrentEnqueue` spawns 32 goroutines writing 100 items each

**Live/Smoke Tests (skipped by default):**
- Guarded by environment variable: `if os.Getenv("RUN_LIVE_RECOVERY") != "1" { t.Skip(...) }`
- Example: `TestResidualRecoverabilitySmoke` in `workflow/internal/translation/residual_recoverability_test.go`
- These require actual LLM backends and real project data

**E2E Tests:**
- Not detected in the Go or Python codebase
- `github-docs/vitest.config.ts` excludes `playwright-*.spec.ts` from its test runner, suggesting Playwright E2E tests exist but are excluded from vitest runs

---

## Common Patterns

**Async / Concurrent Testing:**
```go
func TestCheckpointBatchWriter_ConcurrentEnqueue(t *testing.T) {
    var wg sync.WaitGroup
    for w := 0; w < workers; w++ {
        wg.Add(1)
        go func(worker int) {
            defer wg.Done()
            // concurrent writes
        }(w)
    }
    wg.Wait()
    // verify results after all goroutines finish
}
```

**Error Testing (positive — expect error):**
```go
if _, err := restoreTags(`안녕`, maps); err == nil {
    t.Fatal("expected placeholder mismatch error")
}
// With specific message check:
if !strings.Contains(err.Error(), "no text in response") {
    t.Fatalf("error=%v", err)
}
```

**Error Testing (negative — expect no error):**
```go
got, err := restorePreparedText("...", meta)
if err != nil {
    t.Fatalf("restorePreparedText error=%v", err)
}
```

**Prompt content assertions:** Many tests assert that generated prompts contain specific required substrings:
```go
for _, want := range []string{
    "Return one Korean line per input line.",
    "Do not merge lines.",
    `"cluster_join_hint":"single_action_chain"`,
} {
    if !strings.Contains(prompt, want) {
        t.Fatalf("prompt missing %q:\n%s", want, prompt)
    }
}
```

**Deferred cleanup:**
```go
t.Cleanup(ts.Close)   // preferred for httptest servers
defer db.Close()      // used for SQLite connections
defer store.Close()
```

**Helper pointer utility** (in `workflow/internal/translation/pipeline_mock_test.go`):
```go
func intPtr(v int) *int {
    return &v
}
```

---

*Testing analysis: 2026-03-22*
