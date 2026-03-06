# Go Style Guide (v1)

Project applicability: This guide governs Go review decisions for the localization pipeline and its supporting tooling in this repository.

## Scope
- Applies to `workflow/cmd`, `workflow/internal`, and associated helper packages that implement translation, evaluation, validation, or localization data pipelines inside this repository.
- Covers in-repo Go code reviews, documentation, and informal notes that influence how reviewers comment on naming, formatting, or error-handling without introducing automated tooling enforcement.
- Excludes anything outside this workspace, any third-party Go code, and future automation rollouts (those belong to later versions).

## Version
- v1.0.0: locked review-mode artifact that trades enforcement scripts for prose guidance and maintains traceability to upstream references.

## Source
- Primary reference: https://google.github.io/styleguide/go/guide and the adjacent best-practices sections.
- Supplemental context: this repository’s architecture notes (`ARCHITECTURE.md`) and workflow guidance (`workflow/context/agent_context.md`, `workflow/context/ops.md`).

## Formatting
- Use `gofmt`/`go fmt` to normalize indentation, imports, and alignment before any review, then keep editorial changes within 100 columns when practical.
- Break literals, struct literals, and composite literals into multi-line forms with trailing commas to keep diffs clean and reviewer focus sharp.
- Prefer blank lines to delineate logical sections inside functions, but do not insert artificial spacing after `gofmt` has stabilized the block.

## Naming
- Exported symbols follow MixedCaps (no underscores) and favor nouns for types, verbs for functions, with descriptive suffixes only when they help readers understand exported behavior.
- Unexported identifiers stay lowerCamelCase, stay concise, and avoid redundant package or feature prefixes unless they clarify a domain boundary.
- Package names come from the repository layout (translation, evaluation, platform, etc.) and should stay short, easily pronounceable, and aligned with their directory purpose.

## Comments
- Write doc comments only for exported declarations; start with the identifier and keep the verb active (`GenerateReport returns...`).
- Inline comments document why a reviewer should care (non-obvious trade-offs, history, or future caution) rather than restating the code.
- Link rationale-heavy comments to review traces or reviewer notebooks when they explain deviation from upstream guidance.

## Errors
- Wrap errors at package boundaries with `%w` so callers retain context and can inspect causes later.
- Include actionable context (`"load pack: %w"`) instead of generic strings; keep error paths symmetrical with success paths to avoid swallowed failures.
- Handle expected control-flow signals explicitly (e.g., `io.EOF`, disabled jobs) and surface domain-specific details in returned errors instead of panicking inside pipelines.

## Concurrency
- Keep goroutine ownership explicit: one component spawns, one schema closes, and every worker respects the incoming `context.Context` cancellation.
- Guard shared mutable state with channels, mutexes, or other synchronizers, and document any invariants near the synchronization points.
- Avoid unbounded goroutines or leaking channels inside exported APIs; prefer context-aware loops and `sync.WaitGroup` patterns shown in the repository.

## Maintainability
- Favor small, focused functions that do one job and delegate helpers for repeated logic instead of sprawling monolithic methods.
- Limit interface surfaces to what downstream callers need; avoid exposing implementation detail through exported signatures unless the detail is part of the contract.
- Replace inline repetition with well-named helpers once duplication exceeds two occurrences, but document any exception to keep reviewers aligned.

## Testing
- Use table-driven tests for handlers, parsers, and controllers so reviewers can easily extend coverage for new cases.
- Cover both success and failure paths for each behavior; assert on returned values and errors rather than internal state when possible.
- Keep test helpers deterministic (no sleeps or goroutine races) and easy to reason about inside the translation/evaluation context described in `workflow/context/ops.md`.

## Documentation
- Keep package-level docs focused on intent, contracts, and side effects; do not repeat implementation details already obvious from the code.
- Reviewable examples should show how a caller interacts with the API, what errors may be returned, and how they are handled.
- Update `context.md` or relevant workflow docs only when behavior changes require Project-level note adjustments (this guide stays review-focused, not doc-heavy).

## Consistency
- Prefer existing repository conventions (file layout, helper naming, comment tone) when no higher-priority rule conflicts with Google’s guidance.
- Record any proposed change in behavior (naming, error wrapping, concurrency) inside review notes or notebooks before adjusting the Source Mapping entries.
- Treat consistency violations as review discussion points rather than immediate code rewrite targets unless they cause bugs.

## Source Mapping
| Rule | Why | Source URL | Local Policy |
| --- | --- | --- | --- |
| Formatting baseline | Keeps all reviewed Go snippets aligned with Google readability expectations and minimizes diff noise. | https://google.github.io/styleguide/go/guide#formatting | Apply `gofmt`/`go fmt` and keep wrap decisions to 100 columns so translation logic stays review-friendly. |
| Naming conventions | MixedCaps and short descriptive identifiers prevent ambiguity for pipeline owners. | https://google.github.io/styleguide/go/guide#mixed-caps | Align exported helpers in `workflow/internal` with the domain they affect (translation, evaluation) and document departures in review notebooks. |
| Comments | Clear doc comments help reviewers reason about public contracts; inline notes highlight non-obvious trade-offs. | https://google.github.io/styleguide/go/guide#comments | Attach review trace links to explanations touching domain-specific decisions (glossary usage, localization rules). |
| Error handling | `%w` wrapping plus contextual messages keep telemetry and logs actionable. | https://google.github.io/styleguide/go/best-practices#error-handling | Require wrapped errors at module boundaries (e.g., translation ↔ evaluation) and note any intentional unwraps in review notes. |
| Concurrency | Explicit ownership and context-aware goroutines prevent leaks inside long-running batches. | https://google.github.io/styleguide/go/best-practices#concurrency | Limit goroutines to the short-lived translation/evaluation workers and document shutdown assumptions in comments. |
| Testing | Table-driven tests and failure-path coverage make translation nuances reviewable. | https://google.github.io/styleguide/go/best-practices#testing | Keep test helpers deterministic, avoid CI-specific tooling references, and mention new failure cases in evaluation notes. |

## Must Have
- Clear prose sections covering formatting, naming, comments, errors, concurrency, maintainability, testing, documentation, and consistency.
- Explicit Source Mapping table that traces each local decision back to a Google guideline and captures the local policy context.
- Project applicability line near the top so reviewers know this document targets this repository's localization pipeline.

## Must Not Have
- No automation rollouts, CI/linter definitions, or tooling demands tied to this v1 doc (those belong to future versions).
- No mentions that assume `go.mod` is absent; this repository uses Go modules, so module-aware references are acceptable but not enforced here.
- No runtime scripts or commands masquerading as policy; keep the tone review-only and documentation-based.

## Local Policy
- The repository already ships with `go.mod` and module-aware workflows, so this guide accepts module semantics without prescribing new tooling.
- Focus on the translation/evaluation domain described in `ARCHITECTURE.md` and `workflow/context/ops.md`, ensuring naming and error patterns align with those layers.
- Keep every review comment traceable: document exceptions in notebooks (per the workflow/ops guidance) before updating Source Mapping rows.

## Validation Status
- v1 artifact: ready for review-only adoption within this repository.
- Content verifies scope, formatting, naming, errors, concurrency, maintainability, testing, documentation, and consistency, with traceability to Google references.
- Known constraint: documentation-only focus (no enforcement tooling), module-aware policy matches the existing `go.mod`, and local policies keep references confined to translation/evaluation code.
