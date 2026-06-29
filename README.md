# worthy

[![CI](https://github.com/jameszmapepa/worthy/actions/workflows/ci.yml/badge.svg)](https://github.com/jameszmapepa/worthy/actions/workflows/ci.yml)
![Version](https://img.shields.io/badge/version-0.1.0-blue)
![Go](https://img.shields.io/badge/go-1.26%2B-00ADD8)
[![License: MIT](https://img.shields.io/badge/license-MIT-green)](LICENSE)

**Looking for an open-source project to contribute to?** Point `worthy` at any
public GitHub repo and it tells you, in your terminal, whether it's worth your
time — and *why*, not just a number.

```
worthy charmbracelet/log
```

Before you sink hours into a project, you want to know two things: **will my
pull request actually get merged**, and **will the project still be alive** next
year? `worthy` checks the signals you'd otherwise dig up by hand — how fast
issues get a reply, whether outsiders' PRs get merged, how active the maintainers
are, how often it ships — and grades them in seconds. It also catches the tricks
that fool a quick glance, like a repo that looks busy but never merges a
stranger's PR. No login or token needed.

![worthy demo](demo.gif)

## Install

Requires Go 1.26+.

```bash
go install github.com/jameszmapepa/worthy/cmd/worthy@latest
```

Or clone and build, or grab a prebuilt binary (macOS / Linux / Windows,
amd64 / arm64) from the [releases](https://github.com/jameszmapepa/worthy/releases):

```bash
git clone https://github.com/jameszmapepa/worthy
cd worthy
go install ./cmd/worthy
```

## Usage

```bash
worthy owner/repo
worthy github.com/owner/repo
worthy https://github.com/owner/repo
worthy --ascii owner/repo    # plain language tags instead of Nerd Font icons
```

The header shows the primary language as a Nerd Font devicon; pass `--ascii`
(or set `WORTHY_ASCII=1`) if your terminal lacks a Nerd Font.

### Views and keys

The TUI opens on the scorecard. `←`/`→` (or `tab`) switch views, `enter` drills
into any indicator, `?` lists every key.

| Key | Action |
|---|---|
| `←` `→` · `tab` / `shift+tab` | switch view |
| `1` `2` `3` `4` | scorecard · questions · gauges · explain |
| `↑` `↓` · `j` `k` | move selection |
| `enter` / `esc` | open / close drill-down |
| `r` | re-fetch and re-score |
| `?` · `q` | help · quit |

- **Scorecard** — the two headline grades, composite, per-indicator bars, and gates.
- **Questions** — "Will it last?" and "Will my PR land?" as ranked bars.
- **Gauges** — category gauges plus a 52-week commit sparkline.
- **Explain** — a plain-language verdict and how to clear each gate.

Bars carry a text grade alongside their color, so meaning survives in
`NO_COLOR` / monochrome terminals.

## How it scores

worthy answers two questions, then blends them into one overall grade:

- **Will my PR land?** — how the project treats outside contributors: newcomer
  merge rate, how fast issues get a first reply, PR acceptance, docs, and license.
- **Will it last?** — its momentum: commit frequency and recency, release
  cadence, issue close-ratio, and PR backlog.

A **confidence** level flags repos with too little history to judge fairly.

**Composite = 47.5% Activity + 45% Community + 7.5% Security.**

| Category | Indicators (sub-weights) |
|---|---|
| **Activity** (47.5%) | commit frequency, recency, release cadence, issue close-ratio, PR backlog |
| **Community** (45%) | newcomer merge rate (.25), first-response time (.20), PR acceptance (.15), governance docs (.15), license (.10), open-PR responsiveness (.10), newcomer signals (.05) |
| **Security** (7.5%) | CI present, signed releases, security policy, `pull_request_target` safety |

**Gates** catch what a simple average misses, and cap the score: a lone-maintainer
project, one that takes PRs but rarely from strangers, a stale or archived repo,
an unsigned release using risky workflows (the xz-utils pattern), or inflated
star counts.

A score reflects how a project treats newcomers *as a group* — a read on its
openness, not a promise about your specific PR. It's a heuristic, not a security
audit, so verify before you depend.

→ Full formulas, thresholds, and endpoints: [`docs/SPEC.md`](docs/SPEC.md).

## Rate limits

Unauthenticated, GitHub allows **60 requests/hour per IP**; a full score costs
~15–35, so a few repos per hour is fine. Over the limit, affected metrics
degrade to neutral rather than failing. Set `GITHUB_TOKEN` to lift the ceiling
to 5,000/hour — it's never required, prompted for, or stored:

```bash
GITHUB_TOKEN=$(gh auth token) worthy owner/repo
```

## Roadmap

- `--json` / `--plain` non-interactive output for scripting and CI.
- Pagination beyond one page for very large repos.
- Response caching to stretch the unauthenticated rate limit.

See [`CHANGELOG.md`](CHANGELOG.md) for released changes.

## Contributing

Contributions are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md) for the
development loop, the quality gate (`make check`), and the branching model.
Report bugs or request features at
[Issues](https://github.com/jameszmapepa/worthy/issues).

## License

[MIT](LICENSE). The scoring model draws on [CHAOSS](https://chaoss.community/),
the [OpenSSF Scorecard](https://github.com/ossf/scorecard), and empirical
open-source-survival research. Built with the [Charm](https://charm.sh/) stack
(Bubble Tea + Lip Gloss).

> **Status:** active, pre-1.0 (`v0.1.0`) — the scoring model and TUI may still
> change between minor versions while the public contract stabilizes.
