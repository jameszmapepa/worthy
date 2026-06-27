# repo-health

A terminal UI that scores the health of a **public GitHub repository** and tells
you whether it's worth contributing to or depending on — then shows you *why*,
not just a number.

```
repohealth charmbracelet/log
```

`repo-health` reads ~16 signals from the public GitHub API, scores each against
a documented formula, rolls them into a weighted composite, and layers on five
**conditional gates** that catch the ways a naive health score gets gamed. It
runs unauthenticated by default — no token, no secrets required.

## Why not just one number?

A single weighted-average score is easy to game and conflates two different
questions — *is this project alive?* and *will it accept my PR?* A `.github`
template adds every governance doc in one commit; stale-bots fake issue-close
times; ~71% of merged PRs are self-merged; and the xz-utils backdoor
(CVE-2024-3094) would have scored green on every activity metric.

So `repo-health` answers **both questions as first-class scores** —
**"Will it last?"** (maintenance) and **"Will my PR land?"** (contribution) —
each graded on its own and **capped by the gates that govern it**, not just the
blended composite. A repo flagged "closed to newcomers" shows a depressed
"Will my PR land?" score even when its overall activity looks healthy. You still
get the weighted composite you'd expect, plus the gates that flag and cap it when
the underlying signals don't add up, plus a **confidence level** so a repo with
little data doesn't look falsely precise. Every sub-score shows its raw metric
and formula, following the CHAOSS principle of *definitions over rankings*. The
model is derived from CHAOSS, the OpenSSF Scorecard, and empirical
open-source-survival research.

## The scoring model

**Composite = 45% Activity + 45% Community/Governance + 10% Security/Integrity.**

| Category | Indicators |
|---|---|
| **Activity** (45%) | commit frequency, commit recency, release cadence, issue close-ratio, PR backlog |
| **Community** (45%) | newcomer merge rate (.25), issue first-response time / bot-filtered (.20), PR acceptance (.15), governance docs (.15), license (.10), **open-PR responsiveness** (.10), **newcomer signals** / good-first-issue & help-wanted (.05) |
| **Security** (10%) | CI present, signed releases, security policy, workflow `pull_request_target` safety |

Letter grade (A/B/C/D/F) is applied to the **gate-adjusted** composite.

**Open-PR responsiveness** closes the ghosting blind spot: counting only *closed*
PRs lets a maintainer who leaves newcomer PRs open forever look fine, so the score
also reads open-PR median age and stale (>30d) newcomer PRs. **Newcomer signals**
rewards curated entry points (`good first issue` / `help wanted`) at modest weight.

### The two questions

Beyond the blended composite, each report carries two gate-adjusted scores:

- **Will it last?** — the Activity score, capped by the **stale/archived** and
  **bus-factor** gates (a stale or single-maintainer project can't grade well on
  longevity no matter how its history looks).
- **Will my PR land?** — the Community score, capped by the **closed-to-newcomers**
  gate.

A **confidence** level (High / Medium / Low) reports how many sub-scores fell back
to neutral defaults for lack of data, so a brand-new or low-traffic repo is flagged
rather than scored as if its sparse signals were definitive.

### Gates (flag and cap the score)

- **Bus factor** — one contributor authored >80% of recent commits and the pool
  is ≤4 contributors (caps at 70; also caps "Will it last?").
- **Closed to strangers** — high overall PR acceptance but ~zero newcomer merges,
  the dead-but-busy founder-self-merge pattern (caps at 75).
- **Stale / archived** — no push in a year, or archived/disabled. Auto-downgraded
  to "mature, stable" for old repos with a high issue-close ratio and real releases.
- **Integrity risk** — activity looks green but `pull_request_target` is used and
  releases are unsigned (the xz pattern; caps at 80).
- **Vanity stars** — star/watcher ratio looks inflated.

## Install

Requires Go 1.26+.

From a clone (simplest):

```bash
git clone https://github.com/jameszmapepa/repo-health
cd repo-health
go install ./cmd/repohealth   # installs `repohealth` to $(go env GOPATH)/bin
```

Or via the module path with `go install`. The repository is currently
**private**, so Go's public proxy and checksum database can't read it — tell Go
to skip them for this namespace first (GitHub auth comes from your existing git
credentials; `gh auth setup-git` configures that if needed):

```bash
go env -w GOPRIVATE='github.com/jameszmapepa/*'
go install github.com/jameszmapepa/repo-health/cmd/repohealth@latest
```

If the repository is made public, the `GOPRIVATE` step is unnecessary and
`go install ...@latest` works directly.

## Usage

```bash
repohealth owner/repo
repohealth github.com/owner/repo
repohealth https://github.com/owner/repo
```

### Views and keys

The TUI opens on the scorecard. Switch views and interact with:

| Key | Action |
|---|---|
| `tab` | cycle views |
| `1` / `2` / `3` / `4` | scorecard / questions / gauges / explain |
| `↑` `↓` or `j` `k` | move the selection (scorecard, questions, gauges) |
| `enter` / `→` | open the drill-down (scorecard, questions, gauges) |
| `esc` / `←` | close the drill-down (or quit when nothing is open) |
| `r` | re-fetch and re-score |
| `?` | toggle the keybinding help overlay |
| `q` · `ctrl-c` | quit |

- **Scorecard** — two headline cards answering "Will it last?" and "Will my PR
  land?" (each with its gate-adjusted grade), a confidence caveat when data is
  thin, then the composite + grade, per-indicator color-coded bars, and the
  triggered gates. Each bar carries a text grade alongside its color so meaning
  survives in monochrome / `NO_COLOR` terminals. Select any indicator and drill in
  to see its formula, raw metric, weight, contribution to its category, and the
  gates it feeds.
- **Questions** — the two questions a contributor asks — "Will it last?"
  (Activity) and "Will my PR land?" (Community) — as best-to-worst bars under a
  per-question verdict, plus a separate "Supply-chain integrity" section for the
  Security indicators. Select any indicator to drill into its formula, raw metric,
  weight, and gates.
- **Gauges** — category gauges plus a sparkline of the 52-week commit trend.
  Select a category to break it down into its constituent indicators.
- **Explain** — a plain-language verdict, the strongest and weakest indicators,
  and each triggered gate with how to clear it (or a clean bill of health).

## Rate limits

By default `repo-health` is **unauthenticated**, which GitHub caps at
**60 requests/hour per IP**. A full score costs roughly 15–35 requests, so a few
repos per hour is fine. When the limit is hit, the affected metrics degrade
gracefully (shown as neutral and listed as partial) rather than failing.

If a `GITHUB_TOKEN` environment variable is present, it is used automatically to
raise the ceiling to 5,000 requests/hour. It is **never required, prompted for,
or stored** — purely opt-in:

```bash
GITHUB_TOKEN=$(gh auth token) repohealth owner/repo
```

## Project layout

```
cmd/repohealth/     CLI entry point + argument parsing
internal/github/    dependency-free GitHub REST client (stdlib only)
internal/metrics/   collects raw signals from the API (bot-filtering, graceful degradation)
internal/score/     pure scoring engine: sub-scores, composite, gates (no network)
internal/tui/       Bubble Tea v2 UI: four views + drill-down on scorecard, questions, and gauges
docs/SPEC.md        the design contract (metrics, formulas, weights, endpoints)
```

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and
[Lip Gloss](https://github.com/charmbracelet/lipgloss). Sparklines and bars are
hand-rolled from Unicode block characters — no charting dependency.

## Running & developing locally

Requires Go 1.26+. A `Taskfile.yml` wraps the common tasks — run `task` for the
full list:

```bash
task run -- owner/repo   # run the TUI against any repo (no build step)
task build               # stripped release build to ./bin/repohealth (-s -w -trimpath)
task test:race           # full test suite with the race detector
task cover               # total test coverage
task size                # compare default vs stripped binary size
task check               # vet + test:race (pre-commit gate)
```

Without [Task](https://taskfile.dev), the raw equivalents are:

```bash
go run ./cmd/repohealth owner/repo                    # run it locally
go test ./... -race -cover                            # full suite
go vet ./...
go build -trimpath -ldflags="-s -w" ./cmd/repohealth  # stripped release build
```

The scoring engine is pure and tested above 97%; overall package coverage is
87–96%. Set `GITHUB_TOKEN` first to lift the unauthenticated 60 req/hr limit
to 5,000/hr.

## Caveats

- Scores describe how a project treats *strangers as a population* — they are a
  prior on openness, not a prediction of your individual PR's odds.
- Activity metrics cannot see supply-chain risk on their own; the integrity gate
  is a heuristic nudge, not a security audit. Verify before you depend.
