# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Progressive loading: the header renders as soon as the repository answers
  and every collection stage reports running, retrying, done or degraded,
  so a GitHub "still computing statistics" retry no longer looks like a hang.
- On-disk response cache with ETag revalidation (one-hour freshness, `r`
  forces a refresh, `--no-cache` / `WORTHY_NO_CACHE=1` disables it). A repeat
  score drops from seconds to a fraction of a second.
- `--json` and `--plain` output; plain is automatic when stdout is not a
  terminal. `--help` and `--version` flags.
- Light-background palette selected from the terminal's reported background.
- Scrolling body with mouse wheel, page keys and a scroll indicator; the
  selected row stays on screen.
- Keys: `g`/`G` first and last row, `o` open on GitHub, `y` copy a one-line
  summary. Footer hints and the help overlay now come from one keymap.
- Window title names the repository and grade; the terminal's native
  progress indicator follows the fetch.
- Header badge shows the live remaining API budget.
- Header shows a language bar and legend (top six plus Other) from GitHub's
  languages endpoint, and the primary language as a filled colour badge.
  `--json` gains `repo.languages`.
- Dependabot patch and minor updates queue themselves for auto-merge once the
  required checks pass; majors still wait for a human.

### Changed

- Question answers and the headline verdict are now decision sentences built
  from the readings ("Likely: 4 of 5 newcomer PRs merged, first reply in
  about 27h") instead of grade labels. Gate text is written for the
  contributor and carries the triggering numbers. Quiet readings say "no
  issues in 90d" rather than "0/0 issues closed".
- GitHub 5xx responses are retried twice and reported without the HTML
  error page.

### Fixed

- Gauges view overflowed the terminal by two columns at every width.
- Footer wrapped on terminals narrower than 100 columns.
- Gate details, drill-down text and error messages ran past the right edge
  on narrow terminals.
- Header badge wrapped onto a second line.

## [0.2.0] - 2026-07-09

### Added

- Automatic release tagging: merging a changelog version bump to `main` tags
  the release and publishes it via the existing GoReleaser workflow.
- Release publishing is gated behind the `release` environment: the GoReleaser
  job waits for maintainer approval in the Actions UI before anything is
  published, on both the auto-tag and manual tag-push paths.

### Changed

- Switched the branching model from Gitflow to GitHub Flow: `main` is the
  only long-lived branch and the default; `develop` is retired.

### Fixed

- The changelog guard's Dependabot exemption now keys on the PR author instead
  of the workflow actor, so "Update branch" on a Dependabot PR no longer makes
  the check fail.

## [0.1.0] - 2026-06-28

First tagged release: a terminal UI that scores the health of a public GitHub
repository and answers "Will it last?" and "Will my PR land?".

### Added

- Repository health scoring over the unauthenticated GitHub REST API (optional
  `GITHUB_TOKEN` lifts the 60 req/hr limit to 5,000), with a weighted composite
  across Activity, Community, and Security plus research-backed gates that flag
  and cap the score.
- Prebuilt cross-platform binaries (macOS/Linux/Windows, amd64/arm64) published
  to GitHub Releases on each `v*` tag via a GoReleaser workflow, with release
  notes drawn from this changelog. A VHS workflow records the demo GIF, and a PR
  check requires a changelog entry.
- Four switchable TUI views — scorecard, questions, gauges (with a 52-week
  commit sparkline), and a plain-language explain/verdict view — each with
  per-indicator drill-down (formula, raw metric, weight, category share, gates).
- Arrow-key view navigation: `←`/`→` (and `h`/`l`, `tab`/`shift+tab`) cycle
  views; `enter` opens a drill-down and `esc` closes it.
- Header language shown as a Nerd Font devicon in the language's brand color,
  with an ASCII-tag fallback via the `--ascii` flag or `WORTHY_ASCII`
  environment variable for terminals without a Nerd Font.
- Labeled API rate-limit badge and colorized star/fork/watcher meta row.
- Commit-frequency fallback that counts commits via the `/commits` endpoint when
  GitHub's stats endpoint returns no data for very large repositories.
- Repo-wide newcomer-friendliness signal (open "good first issue" / "help
  wanted" counts and their unassigned subset) via the GitHub Search API.
- Open-source release under the MIT license, with a contributing guide and a
  golangci-lint gate (in CI and `make check`) for outside contributors.
- Newcomer-friendliness scored in availability tiers: an unassigned beginner
  issue scores full marks, a present-but-claimed one scores partial, and the
  absence of such labels is neutral rather than failing.
- Issue-responsiveness sampled from issues that actually have comments (not the
  newest issues, which rarely have a first reply yet), so active repositories
  get a real responsiveness signal instead of "no data".

[Unreleased]: https://github.com/jameszmapepa/worthy/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/jameszmapepa/worthy/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/jameszmapepa/worthy/releases/tag/v0.1.0
