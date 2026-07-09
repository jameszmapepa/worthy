# Contributing to worthy

Thanks for helping improve `worthy`. Here's how to get a change merged.

## Getting started

Requires **Go 1.26+**.

```bash
git clone https://github.com/jameszmapepa/worthy
cd worthy
make run REPO=charmbracelet/bubbletea   # try it against any public repo
```

Run `make` (or `make help`) for every developer task.

## Workflow

This project uses [GitHub Flow](https://docs.github.com/en/get-started/using-github/github-flow):
branch from `main` (`feature/<name>` or `fix/<name>`) and open the pull request
against `main`. Keep branches short-lived and PRs small.

## Releasing

Releases are cut from the changelog. Open a PR that promotes the
`## [Unreleased]` section to `## [x.y.z] - YYYY-MM-DD`; when it merges, the
`tag-release` workflow tags `vx.y.z` on `main` and starts the release
workflow (binaries via GoReleaser, notes from that changelog section).
Publishing waits in the `release` environment until the maintainer approves
the run in the Actions UI. Versions follow
[SemVer](https://semver.org/spec/v2.0.0.html).

## Before you open a PR

Keep the local gate green:

```bash
make check   # gofmt + go vet + golangci-lint + race-enabled tests
```

`make check` and CI use [golangci-lint](https://golangci-lint.run) v2; install it
once with `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`.

- New behavior needs tests. Hold coverage where it is — the scoring engine near
  97%, the other packages 87–96%.
- Keep `internal/score` pure and network-free so it stays deterministically
  testable.
- Match the house style: small focused files, comments that say *why* not *how*,
  immutable data, and no emojis anywhere.
- User-facing changes get a `CHANGELOG.md` entry under `## [Unreleased]`
  ([Keep a Changelog](https://keepachangelog.com/en/1.1.0/)); a CI guard enforces it.

## Reporting bugs

Open an [issue](https://github.com/jameszmapepa/worthy/issues). For bugs, include
the repo you ran `worthy` against and the output you saw.
