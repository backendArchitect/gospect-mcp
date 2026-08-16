# Changelog

All notable changes are noted here. Releases are cut automatically from `main`
(patch-bumped `vX.Y.Z` tags); this file records the meaningful, human-facing
changes grouped by theme rather than every tag.

The format loosely follows [Keep a Changelog](https://keepachangelog.com), and
the project uses [Conventional Commits](https://www.conventionalcommits.org).

## Unreleased

### Changed
- **Default scan is now genuine bugs only.** The opinionated hygiene heuristics
  (`unchecked-error`, `high-complexity`, `todo`, `go-version`) moved behind a new
  `-pedantic` flag after a real-world shakedown showed they drowned the signal
  (~99% of findings on the Go stdlib). `high-complexity` also caps at `medium`
  severity so it never fails a `-fail-on high` gate.
- Lowered the `go.mod` floor to `go 1.25.0` (the true dependency minimum) and
  corrected the previously inaccurate "Go 1.21+" claim.

### Added
- `misspell` detector (under `-pedantic`): common typos in comments and
  function/type names, using a curated known-typo dictionary.
- Distribution: Docker image (`ghcr.io/backendarchitect/gospect-mcp`), Homebrew
  formula, and a `pre-commit` hook.
- CI: a `check` GitHub Action that gates PRs, posts a findings comment, and
  uploads SARIF; a `selftest` job; and a live site-preview link on docs PRs.
- `fix`: verified auto-fix (agent-driven or deterministic `-safe`), batch `-n`,
  `-timeout`, and a guarded MCP `fix` tool behind `--allow-fix`.
- 10 more `go vet`-grade bug detectors (printf, atomic, sortslice, unusedresult,
  stringintconv, timeformat, sigchanyzer, appends, shift, bools).
