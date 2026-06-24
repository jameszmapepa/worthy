# repo-health — Design Specification (ground truth)

A Go TUI that scores the health of a **public** GitHub repository and visualizes
it. This document is the authoritative contract. Do not invent metrics, weights,
or thresholds outside it; they are derived from a research briefing grounded in
CHAOSS, OpenSSF Scorecard, and empirical OSS-survival literature.

## Hard constraints

- **Unauthenticated GitHub REST only.** 60 req/hr per IP. A `GITHUB_TOKEN` env
  var, if present, is used as a pure opt-in (5,000 req/hr) — never required,
  prompted for, or persisted. (Implemented in `internal/github`.)
- **Standard library for HTTP/JSON.** No GitHub SDK dependency.
- **TUI stack — the v2 Charm line (verified go-gettable, mutually compatible):**
  - `charm.land/bubbletea/v2` v2.0.7
  - `charm.land/lipgloss/v2` v2.0.4
  - `charm.land/bubbles/v2` v2.1.0
  - `github.com/NimbleMarkets/ntcharts/v2` v2.2.0 (sparkline + barchart; **no
    gauge type exists** — use `bubbles/v2/progress` for the category gauges).
  - NOTE the v1→v2 API changes (confirmed via context7): key messages are
    `tea.KeyPressMsg` (NOT `tea.KeyMsg`); `Update` returns `(tea.Model, tea.Cmd)`;
    run via `tea.NewProgram(m).Run()`. `progress.New(progress.WithColors(c1,c2),
    progress.WithWidth(n))`, render a fixed percent with `prog.ViewAs(0.0..1.0)`
    (do NOT animate — we render a static snapshot). `spinner.New()` + `m.spinner.Tick()`.
    lipgloss: `lipgloss.NewStyle()`, `lipgloss.Color("#rrggbb")`.
    ntcharts: `sparkline.New(w,h)` → `.PushAll([]float64)` → `.Draw()`/`.DrawBraille()`
    → `.View()`; `barchart.New(w,h)` → `.Push(barchart.BarData{Label, Values:
    []barchart.BarValue{{Name, Value, Style}}})` → `.Draw()` → `.View()`.
- **Immutability, small files (<400 lines), errors handled explicitly, no
  panics in library code, table-driven tests, 80%+ coverage.**
- **Go 1.26**, idiomatic: typed errors (`errors.Is/As`), `context.Context` first
  arg on I/O, options pattern, no global state, exported symbols documented,
  `gofmt`/`go vet`/`golangci-lint` clean. Use the Go LSP for diagnostics as you write.

## Module layout

```
cmd/repohealth/main.go     parse owner/repo (and github.com URL forms), run TUI
internal/github/           DONE: REST client, types, api methods (the plumbing)
internal/metrics/          collectors: endpoint -> RawMetrics struct
internal/score/            sub-scores, weights, composite, gates  (PURE, test first)
internal/tui/              bubbletea model + 3 views + styles
```

## Data flow

`main` -> `metrics.Collect(ctx, client, owner, repo, now)` returns a `RawMetrics`
value (pure data, no scoring). -> `score.Evaluate(raw)` returns a `Report`
(sub-scores + category scores + composite + gate flags). -> `tui` renders the
`Report`. Keep `score` free of any `github`/network imports so it is unit-testable
in isolation against hand-built `RawMetrics`.

## RawMetrics (what metrics.Collect produces)

All time-based metrics computed relative to an injected `now time.Time` (never
`time.Now()` inside pure functions — inject it, so tests are deterministic).

| Field | Meaning | Source endpoint(s) |
|---|---|---|
| `CommitsLast52Weeks []int` | weekly commit counts | `/stats/commit_activity` |
| `DaysSinceLastPush int` | recency | repo `pushed_at` |
| `RepoAgeDays int` | first-commit proxy | repo `created_at` |
| `MergedPRs, ClosedUnmergedPRs int` | all-time PR outcome split (used by `pr_acceptance`) | `/pulls?state=closed&sort=updated` |
| `RecentIssuesClosed, RecentIssuesOpen int` | non-PR issues created in last 90d, closed vs open (zero = neutral no-data) | reused from same `/issues?state=all&sort=created` fetch as TTFR — no extra call |
| `RecentPRsMerged, RecentPRsOpen int` | PRs created in last 90d, merged vs open; closed-unmerged excluded (zero = neutral no-data) | `/pulls?state=all&sort=created&direction=desc` (new call, net -3 calls vs prior design: 4 CountByState removed, 1 added) |
| `MedianIssueFirstResponseHours float64` | bot-filtered TTFR | `/issues?state=all&sort=created` + `/issues/{n}/comments` |
| `NewcomerPRsMerged, NewcomerPRsClosedUnmerged int` | author_association in {FIRST_TIME_CONTRIBUTOR, NONE, CONTRIBUTOR}, last ~90d, self-merge excluded | `/pulls?state=closed` |
| `TopContributorRecentShare float64` | top login's fraction of last-12-week commits (0..1) | `/stats/contributors` |
| `ContributorCount int` | contributors with >0 recent commits | `/stats/contributors` |
| `ReleaseCount int`, `DaysSinceLastRelease int` | cadence (exclude draft/prerelease) | `/releases` |
| `HasCI bool` | >=1 workflow with state=active | `/actions/workflows` |
| `HasSignedReleaseAssets bool` | any release asset matching `.asc/.sig/.sigstore/.intoto.jsonl` | `/releases` |
| `HasSecurityPolicy bool` | community files.security present | `/community/profile` |
| `HealthPercentage int` | presence-only doc score | `/community/profile` |
| `HasReadme, HasContributing, HasCodeOfConduct, HasLicense bool` | governance docs | `/community/profile` |
| `LicenseSPDX string` | license id ("NOASSERTION"/"" = none) | repo `license` |
| `UsesPullRequestTarget bool` | any workflow file uses `pull_request_target` | `/contents/.github/workflows/*` (best-effort; false if not fetched) |
| `Stars, Forks, Watchers int` | vanity + sanity ratios | repo |
| `Archived, Disabled bool` | dead flags | repo |
| `Partial []string` | names of metrics skipped due to rate-limit/404 | — |

`metrics.Collect` MUST degrade gracefully: a `RateLimitError` or `NotFoundError`
on one endpoint records the metric name in `Partial` and continues; it never
aborts the whole collection. Bot-filter: exclude users whose login ends in
`[bot]` or whose `type == "Bot"`; first-response must be by a login != author.

## Scoring model (internal/score) — PURE, TDD

### Sub-scores (each 0..100, with a documented formula)

Each sub-score is a named function `func(raw RawMetrics) SubScore` where
`SubScore{ Key, Label, Value float64 (0..100), Raw string (human metric),
Formula string, Weight, Gates []string }`.

- `Formula` is the human-readable scoring formula (the documented forms below),
  surfaced verbatim in the scorecard drill-down and Explain view.
- `Gates` lists the keys of gates whose trigger condition references this
  sub-score, derived declaratively from the gate predicates: `pr_acceptance`
  and `newcomer_merge_rate` → `closed_to_strangers`; `commit_recency`,
  `issue_close_ratio`, `release_cadence` → `stale_or_archived`;
  `workflow_safety`, `signed_releases` → `integrity_risk`. Other sub-scores
  carry an empty slice. `bus_factor` and `vanity_stars` reference only raw
  metrics, so no sub-score links to them.

**Activity category (weight 0.45):**
- `commit_frequency`: median weekly commits over last 12 weeks. Saturating curve:
  0 -> 0; >=15/wk -> 100; linear between: `min(100, median12/15*100)`.
- `commit_recency`: `DaysSinceLastPush`. 0d -> 100; >=365d -> 0:
  `max(0, 100 - DaysSinceLastPush/365*100)`.
- `release_cadence`: `ReleaseCount==0` -> 40 (neutral-low). Else by
  `DaysSinceLastRelease`: <=90d ->100, >=730d ->0, linear.
- `issue_close_ratio`: `RecentIssuesClosed/(RecentIssuesClosed+RecentIssuesOpen)*100`; 90-day creation cohort (non-PR issues with `CreatedAt >= now-90d`); zero in-cohort issues → 50 (neutral).
- `pr_backlog`: `RecentPRsMerged/(RecentPRsMerged+RecentPRsOpen)*100`; 90-day creation cohort (PRs with `CreatedAt >= now-90d`); closed-unmerged excluded; zero in-cohort PRs → 50 (neutral).

**Community/Governance category (weight 0.45):**
- `issue_responsiveness`: from `MedianIssueFirstResponseHours`. <=24h ->100;
  24-168h ->100..60; 168-720h ->60..0; >720h ->0; no issue data at all -> 50.
- `pr_acceptance`: `MergedPRs/(MergedPRs+ClosedUnmergedPRs)*100`; no closed PRs -> 50.
- `newcomer_merge_rate`: `NewcomerPRsMerged/(merged+closedUnmerged)*100`; no
  newcomer PRs -> 50 (unknown/neutral).
- `governance_docs`: weighted presence — README .25, CONTRIBUTING .25,
  CODE_OF_CONDUCT .2, LICENSE .3, *100.
- `license`: 100 if recognized SPDX present (not ""/"NOASSERTION"); else 0.

**Security/Integrity category (weight 0.10):**
- `ci_present`: `HasCI` -> 100 else 0.
- `signed_releases`: `HasSignedReleaseAssets` ->100; `ReleaseCount==0` ->40; else 0.
- `security_policy`: `HasSecurityPolicy` ->100 else 0.
- `workflow_safety`: `UsesPullRequestTarget` ->30; workflows not fetched ->70; else 100.

### Composite

```
categoryScore(cat) = weighted average of its sub-scores by per-sub weights
                     (equal within category unless noted above)
composite = 0.45*Activity + 0.45*Community + 0.10*Security   (0..100)
```

Round to one decimal. Letter grade on the gate-ADJUSTED composite:
A >=85, B >=70, C >=55, D >=40, F <40.

### Gates (conditional flags that annotate AND may cap the composite)

Returned as `[]Gate{ Key, Severity (info|warn|critical), Title, Detail,
HowToClear string, CapTo *float64 }`.
`AdjustedComposite = min(rawComposite, min(all gate CapTo))`.
`HowToClear` is a one-line advisory on what would clear the gate (guidance only;
it introduces no new thresholds and does not affect scoring).

1. **bus_factor** (`TopContributorRecentShare > 0.80` AND `ContributorCount<=2`):
   warn, CapTo 70.
2. **closed_to_strangers** (`pr_acceptance >= 70` AND `newcomer_merge_rate <= 15`
   AND newcomer sample > 0): warn, CapTo 75.
3. **stale_or_archived** (`Archived || Disabled || DaysSinceLastPush > 365`):
   critical if archived/disabled else warn; CapTo 40 (archived/disabled) / 60 (stale).
   PHASE downgrade: if `RepoAgeDays>365` AND `issue_close_ratio>=70` AND
   `ReleaseCount>0` AND not archived -> info "Mature/stable, low cadence", no cap.
4. **integrity_risk** (`UsesPullRequestTarget && !HasSignedReleaseAssets` AND
   rawComposite>70): warn, CapTo 80.
5. **vanity_stars** (`Stars>5000 && Watchers*200 < Stars`): info, no cap.

Every gate carries a one-line plain-language `Detail`.

## TUI (internal/tui)

Bubble Tea program. On start: spinner while an async `tea.Cmd` runs
`metrics.Collect` + `score.Evaluate`; store the `Report` on completion. On a
`RateLimitError`, show the reset time + a `GITHUB_TOKEN` hint.

Three views, switch with `tab` (cycle) or `1`/`2`/`3`; `q`/`ctrl+c` quits;
`r` re-runs the fetch.

- **View 1 Scorecard:** gate-adjusted composite + letter grade + repo identity;
  per-indicator horizontal bars grouped by category, green (>=70)/amber
  (40-69)/red (<40), each with label, raw metric, bar. Gates listed with glyphs.
- **View 2 Questions:** the two contributor questions — "Will it last?"
  (Activity only) and "Will my PR land?" (Community only) — as best-to-worst
  horizontal bars under a per-question verdict, followed by a separate
  "Supply-chain integrity" section displaying the Security category's
  sub-scores. Security is presented as an integrity section, not a third
  contributor question. All indicators across both questions and the integrity
  section are selectable (`j`/`k`/arrows); an inline detail panel (the same
  formula/raw/weight/gates as the scorecard) opens on `enter`.
- **View 3 Gauges + Sparklines:** `bubbles/v2/progress` bars (static, via
  `ViewAs`) for the 3 categories + composite; ntcharts `sparkline` of
  `CommitsLast52Weeks` (commit trend). Category gauges are selectable; `enter`
  breaks the selected category down into its constituent indicators.

Responsive to width; `lipgloss` styling; header shows effective rate-limit mode.

## Testing (TDD — non-negotiable)

- `internal/github`: httptest server; 200 / 202-retry / 404 / rate-limit / Link-count.
- `internal/score`: table-driven, one case per sub-score formula incl. boundaries
  (0, saturation, neutral/no-data), composite math, and EVERY gate trigger +
  non-trigger + the phase downgrade. The heart of the suite — aim high.
- `internal/metrics`: httptest fixtures; bot-filtering, self-merge exclusion,
  author_association filtering, graceful `Partial` degradation.
- `internal/tui`: model update/transition tests (teatest where practical) +
  pure view-render snapshot of a fixed Report.

Run `gofmt`, `go vet`, `go test ./... -race -cover` clean before the Dev-QA gate.
```
