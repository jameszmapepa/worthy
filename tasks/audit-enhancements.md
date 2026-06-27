# repo-health audit & enhancement plan

Goal: sharpen the tool's answers to its two core questions — **is the repo maintained?**
and **how easy is it for me to contribute?** — plus UI, performance, footprint, security.
Approved scope: full (restructure + new signals), executed autonomously, report at end.

Dependency order: **Package A (github+metrics)** → **B (score)** → **C (tui)**;
**D (build tooling)** runs parallel to A. Each package: go-engineer implements →
build/test/vet verified → senior-engineer + tester gate before the next dependent package.

## Package A — github + metrics (data layer)  Req: security, signals, perf, quality
- [ ] A1 SECURITY: sanitize ANSI/OSC from API strings (Description, Language, LicenseSPDX)
      at the `applyRepo` boundary using charmbracelet `ansi.Strip`; add regression test.
- [ ] A2 New signal: open-PR staleness / ghosting — fetch open PRs, compute median open-PR
      age + count of stale (>30d) newcomer open PRs. New RawMetrics fields.
- [ ] A3 New signal: good-first-issue / help-wanted open-issue counts (label listing, cap 100).
- [ ] A4 Surface `Fork` status into RawMetrics (already fetched, never copied).
- [ ] A5 `getRaw` must classify 403+ratelimit as `*RateLimitError` (currently swallowed).
- [ ] A6 `bytes.Contains(body, ...)` instead of `string(body)` in PRT scan.
- [ ] A7 `busFactor` single-pass (drop unused login slice / intermediate allocs).
- [ ] A8 Replace hand-rolled `splitPath`/`joinPath` with `strings.Split/Join`.
- [ ] A9 `url.QueryEscape(state)` in RecentPulls/RecentIssues query building.
- [ ] A10 `sort.Float64s` → `slices.Sort`.

## Package B — score (model)  Req: core questions
- [ ] B1 Export `LetterGrade` (rename private `letterGrade`); single grade authority.
- [ ] B2 Make the two questions FIRST-CLASS on `Report` (Maintained, Contributable
      QuestionScores), computed AFTER routing each gate's cap to the question it answers:
      stale/bus_factor → maintained; closed_to_strangers → contributable.
- [ ] B3 New sub-scores consuming A2/A3: open-PR responsiveness (ghosting penalty) folded
      into the contribute question; good-first-issue presence as a newcomer-friendly signal.
- [ ] B4 Confidence/sample-size flag so tiny/new repos don't score a misleading ~62 on
      all-neutral defaults; expose on Report and surface in TUI.
- [ ] B5 Re-categorize `bus_factor` (sustainability, not liveness) + widen gate (count<=4).
- [ ] B6 Fix `pr_acceptance` "all-time" mislabel → document the real recent-cohort window.

## Package C — tui (presentation)  Req: UI
- [ ] C1 Persistent header shows grade + one-line verdict on every view.
- [ ] C2 Question-first layout: the two answers are the most prominent thing on screen.
- [ ] C3 Track terminal height; clip/truncate overflow with a sentinel.
- [ ] C4 Narrow-terminal: stack gauges vertically below ~70 cols.
- [ ] C5 Accessibility: text/letter tier alongside color bars (NO_COLOR safe).
- [ ] C6 Loading progress: per-stage status (activity/community/security) under spinner.
- [ ] C7 PERF: `renderBar` run-length gradient (<=24 styles vs ~per-cell); reuse/replace
      `progress.New` per render; dedup `categoryGrade` → `score.LetterGrade`.
- [ ] C8 FOOTPRINT: replace ntcharts sparkline with stdlib block-char sparkline; drop dep.
- [ ] C9 PERF: cancel fetch context on quit (goroutine leak) + overall collection deadline.
- [ ] C10 Polish: `?` help overlay, footer collapse hint, error-view retry affordance.

## Package D — build tooling (footprint)  Req: footprint
- [ ] D1 Taskfile.yml `build` with `-ldflags="-s -w" -trimpath` (-3.2 MB / 29%).

## Gate (Dev-QA)
- [ ] Tester: regression tests RED->GREEN for A1 (escape), B2/B3/B4 (scoring), gate routing.
- [ ] senior-engineer: BLOCK/WARNING/APPROVE on the full diff.
- [ ] Verify: `go build ./...`, `go vet ./...`, `go test -race ./...`, binary size before/after.

## Review (complete)

All packages A/B/C/D delivered. Dev-QA gate: senior-engineer APPROVE (0 CRITICAL/HIGH;
1 MEDIUM comment fix applied), tester TESTS PASS (added TestPRResponsiveness + both B2
gate-routing-direction tests). Final verification this turn:
- `go build ./...` clean, `go vet ./...` clean
- `go test -race ./...` all 5 packages pass
- coverage: cmd 87.8 / github 90.1 / metrics 96.1 / score 97.7 / tui 94.2 (floor 80)
- footprint: stripped binary 10.7M → 7.45M via build flags; dropped ntcharts + harmonica +
  bubblezone (the image/png chain) from the module graph.

Things intentionally NOT touched: the 45/45/10 category weights (kept; blended composite
retained for back-compat, just demoted behind the two questions); ETag/conditional-request
caching (rejected for a one-shot CLI); README prose (model now diverges from the doc table —
follow-up doc pass recommended); the pre-existing `ptr`→`new` modernizer nits.
