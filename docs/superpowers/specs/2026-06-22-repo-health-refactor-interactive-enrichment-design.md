# repo-health — Refactor + Interactive Enrichment (Design)

Date: 2026-06-22
Status: Approved design, pending implementation plan.
Supersedes nothing; complements `docs/SPEC.md` (the scoring contract).

## Context

`repo-health` is a Go TUI that scores a public GitHub repo's health and
visualizes it (3 views: scorecard, radar, gauges). The scoring engine
(`internal/score`) is pure and well-tested; the data plumbing
(`internal/github`, `internal/metrics`) is complete. A prior session left an
uncommitted "polish" pass in the tree (braille header, `verdict.go` one-liner,
richer radar/scorecard/gauges) that builds, vets, and tests green. Per the
session decision, that polish pass is the **working baseline** — this work
builds on top of it and does not commit or revert it separately.

This design covers two things in one pass:
1. A behavior-preserving **internal refactor** to fix structural debt.
2. Two **interactive TUI enrichments**: indicator drill-down (A) and an
   Explain/Verdict view (B).

It explicitly does **not** add JSON/`--plain` output, pagination, caching, or
new scoring metrics/gates (deferred; see Out of Scope).

## Decisions locked during brainstorming

- Baseline = current working tree (keep the uncommitted polish pass).
- Build **A + B** (drill-down and Explain view).
- Drill-down detail presentation = **inline expansion** below the selected
  indicator (not split-pane, not modal).
- Sequence = refactor first (safety net), then score-model enrichment, then A,
  then B; each through the senior-engineer + tester Dev-QA gate.

## Architectural principle

Both enrichments must render **from data already on the `Report`, with zero new
network calls and zero scoring logic in the TUI.** Today `score` computes the
"why" (formula, raw inputs, weight) and the TUI discards it at render time. So
the spine of this design is: **enrich the score model once; both features render
off it.** `score` stays the single source of truth; the TUI stays a pure
renderer. This preserves the SPEC's package boundary (`score` imports no
`github`/network code).

## Score model changes (`internal/score`)

These are public-shape changes and require a reference-search of all
construction/test sites plus a `docs/SPEC.md` update.

1. `SubScore` gains:
   - `Formula string` — human-readable formula matching the documented form in
     `SPEC.md §Sub-scores`, e.g. `commit_frequency` →
     `"min(100, median12/15 × 100)"`. One per sub-score.
   - `Gates []string` — keys of gates whose **trigger condition references this
     sub-score**. Sub-scores not used by any gate condition carry an empty
     slice. Linkage (derived declaratively, not by ad-hoc strings):
     - `closed_to_strangers` ← `pr_acceptance`, `newcomer_merge_rate`
     - `stale_or_archived`   ← `commit_recency`, `issue_close_ratio`,
       `release_cadence` (the phase-downgrade inputs)
     - `integrity_risk`      ← `workflow_safety`, `signed_releases`
     - `bus_factor`, `vanity_stars` reference raw metrics only (no sub-score) →
       no sub-score links to them; they still appear in view B on their own.

2. `Gate` gains:
   - `HowToClear string` — one advisory line on what would clear the gate. This
     is guidance, not scoring (introduces no new thresholds). Per gate:
     - `bus_factor`: "Distribute commits beyond the top author / grow the
       contributor base."
     - `closed_to_strangers`: "Merge PRs from first-time or non-member
       contributors."
     - `stale_or_archived`: stale → "Resume commits or cut a release."
       archived/disabled → "Archived in place; informational only."
     - `integrity_risk`: "Sign release assets, or drop `pull_request_target`
       from workflows."
     - `vanity_stars`: "Informational: stars are high relative to watchers."

3. New helper `Drivers(r Report) (strong, weak []SubScore)` — returns the
   top-N and bottom-N sub-scores by `Value` (N defined as a package constant,
   default 3), with deterministic tie-breaking by sub-score order. Lives in
   `score` (testable) rather than as an ad-hoc sort in the view.

No change to formulas, weights, gate conditions, composite math, or grade
bands. The numbers a repo scores today do not change.

## Refactor plan (behavior-preserving, done first)

Every step keeps the existing test suites green as the safety net.

1. **`internal/metrics/collect.go` (473 lines) → split** to satisfy the SPEC's
   <400-line rule and one-responsibility-per-file:
   - `collect.go`: the `Collect()` orchestrator, `RawMetrics` assembly, and
     `Partial` degradation handling (unchanged semantics).
   - `collect_activity.go`: `busFactor`, `processReleases`, commit-activity.
   - `collect_community.go`: `processPulls`, `medianTTFR`, issue/PR helpers.
   - `collect_security.go`: `processWorkflows`, signed-asset / security-policy.
   Move the matching tests alongside; no behavior change.

2. **`internal/github/client.go`** — extract `doJSONRequest(ctx, path, out)` and
   `doRawRequest(ctx, path)` to centralize request construction, the 202-retry
   loop, header handling, and error typing. Refactor api.go's call sites onto
   them. Covered by existing `client_test.go`; no behavior change.

3. **`internal/tui`** — consolidate the duplicated `truncate`, `clampWidth`,
   `barColor`, `renderBar` helpers into a new `internal/tui/util.go`. Promote
   magic numbers (`gradientSteps`, `radarRingsNum`, `radarRows`, scorecard/gauge
   label widths) to named constants.

4. **`internal/score/subscores.go`** — extract a `ratioScore(closed, total, key,
   label, formula)` helper to de-duplicate `issue_close_ratio` and `pr_backlog`.

## Enhancement A — Indicator drill-down (scorecard view only, V1)

- `internal/tui/model.go`: add `selected int` selection state and key handling —
  `j`/`k`/`↑`/`↓` move selection, `enter`/`→` expand detail, `esc`/`←` collapse.
  Index is clamped to the indicator count. Selection keys are **inert** while
  loading, on error, and when the active view is not the scorecard. View
  switching does not lose the data but selection resets to 0 on entering the
  scorecard.
- `internal/tui/view_scorecard.go`: highlight the selected indicator's row;
  when expanded, render an **inline detail panel** directly below that row
  showing: `Label`, `Value`, `Raw`, `Formula`, `Weight`, the indicator's
  contribution to its category score (its weighted share = `Weight × Value`
  expressed against the category total), and any linked `Gates`. Responsive to
  width; no split-pane layout.

## Enhancement B — Explain / Verdict view (4th view)

- `internal/tui/view_explain.go` (new): renders
  - the verdict headline (reusing the existing `verdict.go` output),
  - strongest and weakest drivers from `score.Drivers`,
  - each **triggered** gate with its `Detail` and `HowToClear`; healthy repos
    (no gates) show an explicit "No gates triggered" state.
- `internal/tui/model.go` + `render.go`: add the view to the `tab` cycle and the
  `4` key; update the header/footer view indicators.

## Data flow

Unchanged backbone: `main → metrics.Collect → score.Evaluate → Report → tui`.
The enrichment rides on the existing `Report` (richer `SubScore`/`Gate` + the
`Drivers` helper). The TUI Model gains selection state only. No new I/O.

## Error handling

No new network paths ⇒ no new error paths in `score`/`metrics`. The refactor
must preserve `Partial`/graceful-degradation behavior exactly (validated by the
existing collector tests). New key handling must bound the selection index and
no-op safely in loading/error/non-scorecard states — never panic, never index
out of range.

## Testing strategy (TDD, 80%+)

- **Refactor:** existing suites (`collect_test.go` ~1814 lines, `score` tables,
  `client_test.go`) are the regression net and must stay green; relocated
  helpers keep their unit tests.
- **Score model:** table tests for `Formula` (one case per sub-score), the
  gate-linkage map, `HowToClear` presence per gate, and `Drivers`
  (ordering, ties, neutral/no-data inputs).
- **A:** model tests for selection movement (clamp at both bounds, no-op while
  loading, inert off the scorecard) + a render snapshot of the scorecard with an
  indicator selected and expanded.
- **B:** render snapshots of the Explain view for two fixtures — a healthy repo
  (no gates) and an unhealthy repo (gates with `HowToClear`).
- Strengthen the currently-weak `tui` view tests as part of this; run `gofmt`,
  `go vet`, `go test ./... -race -cover` clean before each Dev-QA gate.

## Sequencing + gates

1. Refactor (steps 1–4) → tester confirms suites green, senior-engineer review.
2. Score-model enrichment → tests RED then GREEN, review.
3. Enhancement A → tests RED then GREEN, review.
4. Enhancement B → tests RED then GREEN, review.
Each stage is a focused commit. `SPEC.md` updated in step 2.

## Risks / ceilings

- TUI selection is the main iteration point — mitigated by scoping to the
  scorecard view and inline expansion (no split-pane width math).
- Extending `SubScore`/`Gate` ripples to every construction and test site —
  mitigated by a reference-search before the change and updating `SPEC.md` in
  the same step.
- `ceiling:` drill-down selection is scorecard-only; radar/gauge selection and
  split-pane/modal presentations are deliberate follow-ons, not V1.

## Out of scope (explicit YAGNI / deferred)

- Non-interactive output (`--json`, `--plain`) and exit codes.
- Pagination beyond one page for very large repos.
- Disk caching of API responses.
- New scoring metrics or anti-gaming gates.
- Gate *selection/inspector* interactivity (B shows gates statically; full
  selectable gate inspection is a later option).

## Acceptance criteria

- `go build ./...`, `go vet ./...`, `gofmt -l` clean; `go test ./... -race`
  green; coverage ≥80% per package.
- No file exceeds 400 lines; `collect.go` is decomposed.
- Repo scores are numerically identical to the pre-change baseline (refactor +
  enrichment add explanation, not new scoring).
- Scorecard supports keyboard selection + inline drill-down with formula, raw,
  weight, contribution, and linked gates.
- A 4th Explain view renders the verdict, strongest/weakest drivers, and
  triggered gates with how-to-clear guidance, with a healthy-repo empty state.
- `docs/SPEC.md` reflects the new `SubScore`/`Gate` fields.
