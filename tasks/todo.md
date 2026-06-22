# repo-health — build plan

A Go TUI that scores the health of a public GitHub repo and visualizes it.
Unauthenticated GitHub REST only (optional GITHUB_TOKEN opt-in, never stored/prompted).

## Design source
Scoring model derived from a research briefing (CHAOSS, OpenSSF Scorecard,
empirical OSS-survival literature). Key decisions:
- Weighted composite (Activity 40% / Community 30% / Security 30%) AS REQUESTED,
  PLUS conditional gates that flag/cap the score (anti-gaming).
- Every sub-score exposes its raw metric + formula + weight (definitions over rankings).
- Bot-filter all responsiveness metrics.

## Tasks

### Stage 1 — GitHub API client (testable core)
- [ ] Define API response types (repo, issue, pull, contributor stats, community)
- [ ] HTTP client: unauth + optional token, JSON decode, error handling
- [ ] Handle 202 (stats recompute) with bounded retry
- [ ] Handle rate-limit (403 + X-RateLimit-Remaining=0) gracefully
- [ ] Link-header total-count trick (cheap issue/PR counts)
- [ ] Unit tests with httptest server (no live API)

### Stage 2 — Scoring engine (testable core)
- [ ] Sub-score normalizers (0-100) per indicator with documented formulas
- [ ] Category weights + composite weighted average
- [ ] Conditional gates: bus-factor, closed-to-strangers, phase, integrity
- [ ] Unit tests for every formula + gate boundary

### Stage 3 — Metric collectors
- [ ] One collector per indicator wiring endpoint -> raw metric
- [ ] Bot-filtering for first-response + self-merge
- [ ] Degrade gracefully when a call is rate-limited

### Stage 4 — TUI (3 switchable views)
- [ ] Bubble Tea root model + async fetch + loading state
- [ ] View 1: scorecard + per-indicator color-coded bars
- [ ] View 2: ASCII radar across categories (hand-rolled)
- [ ] View 3: gauge dials + sparklines (ntcharts)
- [ ] Key handling: tab/1/2/3 to switch, q to quit

### Stage 5 — Wiring + polish
- [ ] cmd/repohealth/main.go: parse owner/repo, run TUI
- [ ] README with usage + rate-limit caveat
- [ ] senior-engineer review (Dev-QA gate)
- [ ] go vet + gofmt + full test run

## Review (complete)

All 5 packages built TDD via parallel specialist agents (go-engineer pair),
gated by a senior-engineer Dev-QA review and a live smoke against the real API.

Final state — `go build/vet ./...` + `gofmt` clean; `go test ./... -race`:
- internal/score   100.0%   (pure scoring engine + 5 gates)
- internal/tui      93.5%   (3 views, Bubble Tea v2)
- internal/metrics  91.1%   (collectors, bot-filtering, graceful degradation)
- internal/github   90.5%   (stdlib REST client, 202-retry, rate-limit, Link-count)
- cmd/repohealth    87.8%   (arg parsing + boundary validation)

Decisions / deviations from the original ask (all surfaced to user):
- Build the weighted composite AS ASKED, plus 5 research-backed gates that flag
  and cap it (a naive additive score is gameable — CHAOSS/OpenSSF evidence).
- Migrated to the latest v2 Charm stack (bubbletea 2.0.7 / lipgloss 2.0.4 /
  bubbles 2.1.0 / ntcharts v2.2.0) per the latest-standards directive.
- Unauthenticated by default (60/hr); GITHUB_TOKEN is pure opt-in.

Dev-QA findings (WARNING, none blocking) — all fixed + tested + re-verified:
- HIGH: URL injection — added owner/repo charset validation at the cmd boundary
  + url.PathEscape in the client (defense in depth).
- Live-smoke bug: missing commit data scored 0 ("worst"); now neutral 50.
- Dead duplicate branch + tautological isSoftError → context-cancel now aborts,
  all else degrades to Partial, nothing silently swallowed.
- Workflow scan now tolerates per-file failures (was voiding the whole scan).
- Removed dead import-retainer blanks.

Verified live against charmbracelet/log: composite 65.1 (grade C), gates correctly
empty, rate-limit tail degraded gracefully. Throwaway smoke harness removed.

Not done (offered as next steps): git init + first commit; optional `--plain`
non-interactive output mode (V2); pagination beyond one page for very large repos.

## Refactor + Interactive Enrichment (complete)

Design: `docs/superpowers/specs/2026-06-22-repo-health-refactor-interactive-
enrichment-design.md`. Shipped on branch `refactor/interactive-enrichment` as
four focused, Dev-QA-gated commits (each: tester + senior-engineer, full
`go test ./... -race` green). The prior uncommitted TUI polish pass rode in as
the agreed working baseline (per the design), folded into the Stage 1 commit.

- Stage 1 — `refactor:` decompose `metrics/collect.go` (473→230) into per-domain
  files (+ split the 1814-line test into 6); add `tui/util.go`
  (truncate/clampWidth/barColor/renderBar); extract `score.ratioScore`. Behavior
  preserving — scores numerically unchanged.
- Stage 2 — `feat:` score-model enrichment: `SubScore.Formula` + `SubScore.Gates`
  (declarative gate linkage), `Gate.HowToClear`, `score.Drivers`. Metadata only;
  no score/gate/grade changes. `SPEC.md` updated.
- Stage 3 — `feat:` scorecard indicator drill-down (Enhancement A): j/k/↑/↓
  selection, enter/→ expand, esc/← collapse; inline detail panel (formula, raw,
  weight, category share, linked gates). Renders from `Report`, no new I/O.
- Stage 4 — `feat:` Explain/Verdict view (Enhancement B, 4th view): verdict +
  strongest/weakest drivers + per-gate how-to-clear; healthy-repo empty state.

Acceptance (all met): build/vet/gofmt clean; `-race` green; coverage ≥80% per
package (score 100, tui 95.5, metrics 91.1, github 90.5, cmd 87.8); no source
file >400 lines (`collect.go` 230); repo scores identical to the pre-change
baseline (enrichment adds explanation, not new scoring); `SPEC.md` reflects the
new fields.

Out of scope (deferred per design): `--json`/`--plain`, pagination, caching,
new scoring metrics/gates, gate-inspector interactivity. Open follow-ons:
drill-down for the radar/gauge views (split-pane/modal — V1 is scorecard-only);
push branch + open PR into `develop`.
