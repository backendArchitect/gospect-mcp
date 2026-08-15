# Contributing to gospect-mcp

Thanks for your interest in improving gospect-mcp! It's a Go-only, report-first
code scanner exposed as an MCP server. This guide covers how to build it, test
it, and get a change merged.

By participating you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md).

## Build from source

You need **Go 1.25+** (the analysis libraries gospect wraps — `golang.org/x/tools` and
`staticcheck` — require it). No C compiler, no external services.

```sh
git clone https://github.com/backendArchitect/gospect-mcp
cd gospect-mcp
go build ./...
go install .        # drops a `gospect-mcp` binary in $GOBIN
```

## Run tests

```sh
go test ./...       # the whole suite
go test ./internal/fixer -run TestFix_Integration -v   # a single package
```

Some tests shell out to `git` and use POSIX shell "agents" to exercise the
apply / verify / rollback loop; they skip automatically on Windows or when `git`
isn't on `PATH`. Keep the suite green — a change without a test for non-trivial
logic won't be merged.

## Lint

```sh
go vet ./...
gofmt -l .          # must print nothing; run `gofmt -w .` to fix
```

If you have [staticcheck](https://staticcheck.dev) installed, run it too — this
project literally wraps it, so we hold ourselves to it:

```sh
staticcheck ./...
```

Dogfooding is encouraged: `go run . scan . ./...` should come back clean.

## Project structure

```
main.go                  CLI entry point + commands (scan/check/fix/help/...) and the MCP server
internal/
  loader/                loads Go modules via go/packages (single- and multi-module, diff mode)
  scan/                  orchestrates detectors, filtering, baseline, SARIF, .gospectignore
  detect/                the analyzers: go/analysis bug checks, staticcheck, govulncheck, suggested fixes
  graph/                 code-graph interface + local/ AST implementation (routes, complexity, untested)
  gate/                  the non-Go guard (identifies non-Go input and bails)
  spec/                  OpenAPI/swagger spec parsing for stale-doc detection
  fixer/                 the verified apply → re-scan → keep-or-rollback harness
  fix/                   the fix "envelope" (root cause, verify-first prompt, constraints)
  agent/                 detects & drives an installed AI agent (claude/aider/cursor-agent/...)
  mcp/                   the JSON-RPC stdio MCP server (report-only)
  selfupdate/            `gospect-mcp update`
```

## Design principles

Two rules shape almost every decision here — please respect them in a PR:

1. **The MCP server is a sensor, not an actuator.** It *reports* findings and
   never touches your code. Fixing is a separate, CLI-only, explicitly-invoked
   step (`gospect-mcp fix`). Don't add code-mutation to the server path.
2. **A finding must be real.** We'd rather miss a shaky finding than emit a
   false positive. New detectors should carry a confidence level and suppress
   intentional/generated code. Deterministic fixes are applied *only* when the
   analyzer offers exactly one unambiguous fix.

We also follow a "minimal, boring, deletion-over-addition" style: match the
naming and comment density of the surrounding code, and don't add abstractions
that weren't asked for.

## Adding a detector

Most detectors are `go/analysis` analyzers wired up in `internal/detect`:

1. Add the analyzer to the appropriate set (e.g. `bugAnalyzers` in
   `internal/detect/bugs.go`) with its category, severity, and confidence.
2. If it can emit a `SuggestedFix`, verify it produces **exactly one** fix so it
   can flow through the deterministic `-safe` path — otherwise it's report-only.
3. Add a small fixture package and a test proving it fires (and doesn't
   false-positive on the intentional/generated cases).

Open an issue first for anything that changes output format, the MCP protocol
surface, or adds a heavy dependency — those need a maintainer's sign-off before
you sink time into them.

## Commit format

We use [Conventional Commits](https://www.conventionalcommits.org):

```
feat(scan): add .gospectignore support
fix(fixer): restore working tree wholesale on rollback
docs(readme): document the -safe flag
perf(loader): load monorepo modules in parallel
```

Type is one of `feat`, `fix`, `docs`, `perf`, `refactor`, `test`, `chore`.
Keep the subject imperative and under ~72 characters. Pushing to `main`
triggers an automated release, so keep history clean and meaningful.

## Pull requests

- **Open an issue first** for features or anything that changes public behavior,
  output, or the MCP surface. Bug fixes can go straight to a PR.
- **Keep it focused.** One logical change per PR. A 40-line diff that's easy to
  review beats a 400-line one that isn't.
- **Include a test** for non-trivial logic and make sure `go test ./...`,
  `go vet ./...`, and `gofmt -l .` are all clean.
- **Explain the "why."** Describe the problem and how the change addresses it at
  the root cause, not just the symptom.

## Security

Because `fix` runs analyzer-suggested edits and can drive an external AI agent
against your working tree, correctness of the verify/rollback harness is
security-critical. If you find a way for a fix to leave a repo in a broken or
unexpected state — or any other security concern — please **do not** open a
public issue. Email **chauhanvatsal55@gmail.com** instead.

## License

By contributing, you agree that your contributions will be licensed under the
project's [MIT License](LICENSE).

## Good first issues

New here? Look for issues tagged
[`good first issue`](https://github.com/backendArchitect/gospect-mcp/labels/good%20first%20issue)
— small detectors, doc fixes, and test coverage are great places to start.
