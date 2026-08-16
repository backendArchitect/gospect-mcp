# gospect-mcp — usage & reference

The full reference. New here? Start with the [README](README.md); this page is the deep end.
Run `gospect-mcp help` for every command and flag at any time.

- [Commands](#commands)
- [Filtering the output](#filtering-the-output)
- [Monorepos](#monorepos)
- [Suppressing findings](#suppressing-findings)
- [Baseline mode (adopt on a noisy repo)](#baseline-mode)
- [Deeper analysis: staticcheck](#deeper-analysis-staticcheck)
- [Fast PR checks: diff mode](#fast-pr-checks-diff-mode)
- [SARIF & vulnerabilities](#sarif--vulnerabilities)
- [Use it in CI](#use-it-in-ci)
- [Fixing: the full story](#fixing-the-full-story)
- [Connect an AI editor (MCP)](#connect-an-ai-editor-mcp)
- [Whole-repo detectors & the code graph](#whole-repo-detectors--the-code-graph)
- [What it detects (full list)](#what-it-detects-full-list)
- [The `scan` tool (JSON shape)](#the-scan-tool)
- [Performance](#performance)
- [Staying up to date](#staying-up-to-date)
- [How it works](#how-it-works)

---

## Commands

```
gospect-mcp scan  [flags] <dir> [patterns...]   print a report (JSON by default)
gospect-mcp check [flags] <dir> [patterns...]   CI gate: exit non-zero on findings
gospect-mcp fix   [flags] <dir> [patterns...]   apply a verified fix (opt-in)
gospect-mcp propose-fix                          read a finding on stdin, print fix guidance
gospect-mcp version | update | uninstall | help
gospect-mcp                                      start the MCP server (stdio)
```

Progress streams to **stderr** by default (so a long scan never looks hung); JSON stays clean on
**stdout** for piping. Silence progress with `-quiet`.

> `gospect-mcp scan` *without a path* starts the MCP server (it waits on stdin, which can look like a
> hang). Always pass a directory to scan.

## Filtering the output

A big repo can surface hundreds of findings. The same flags work on `scan` and `check`:

```sh
gospect-mcp scan -format text -min-severity high .   # readable, only the serious stuff
gospect-mcp scan -category bug .                     # just bugs
gospect-mcp scan -detector nilness,unmarshal .       # specific detectors
gospect-mcp scan -exclude '*.pb.go,mocks/' .         # skip generated code / mocks
```

- `-min-severity low|medium|high` — drop everything below the given severity.
- `-min-confidence low|medium|high` — drop low-confidence heuristics.
- `-category a,b` / `-detector a,b` — keep only those categories/detectors.
- `-exclude glob,glob` — drop findings whose file path matches a glob or substring.
- `-format text` — a readable grouped listing instead of JSON.

## Monorepos

Point `scan` at the **repo root** and it finds every nested `go.mod` and scans them all in one run:

```sh
gospect-mcp scan ./my-monorepo
```

Multi-module discovery kicks in only for the default `./...` pattern. If a module can't be loaded
(stale vendoring, a private dependency), it's **skipped with a reason** on stderr and listed in the
report's `skipped_modules` — the rest still scan. A partial scan never masquerades as full coverage.

**Not a Go project?** gospect is Go-only. Point it at a non-Go repo and it says so and names what it
looks like (`… looks like a JavaScript/TypeScript project. gospect-mcp is Go-only.`).

## Suppressing findings

Mark deliberate code with a `//gospect:ignore` comment on the flagged line (or the line above it):

```go
resp.Body.Close() //gospect:ignore                  // suppress any finding on this line
_ = risky()       //gospect:ignore unchecked-error  // suppress only these detectors
```

For repo-wide, checked-in suppression, add a **`.gospectignore`** at the module root:

```gitignore
*.pb.go              # skip generated protobuf
mocks/               # skip a whole directory
detector:todo        # never report TODO markers
detector:go-version  # we pin Go deliberately
```

To silence a whole detector for one run, use `check -ignore <detector,...>`.

**Generated code is skipped automatically** — files with the standard `// Code generated … DO NOT
EDIT.` marker never contribute findings. Pass `-include-generated` to scan them anyway.

## Baseline mode

Adopt gospect on an existing codebase without drowning in old findings — snapshot them once, then
only see (or gate on) what's **new**:

```sh
gospect-mcp scan . > gospect-baseline.json                          # snapshot (commit this)
gospect-mcp scan  -baseline gospect-baseline.json .                 # later: only NEW findings
gospect-mcp check -baseline gospect-baseline.json -fail-on medium . # CI fails only on new ones
```

Matching uses a line-independent fingerprint, so a pre-existing finding that merely shifts lines
stays baselined.

## Deeper analysis: staticcheck

By default gospect runs a fast, high-precision set. Add `-staticcheck` to also run the
[staticcheck](https://staticcheck.dev) `SA` analyzers (the canonical Go bug checks — deprecated-API
usage, dead assignments, impossible conditions, ~100 more):

```sh
gospect-mcp scan -staticcheck .                       # much deeper
gospect-mcp check -staticcheck -since origin/main .   # deep, but only on the PR's changes (fast)
```

It's opt-in because it's thorough but slow (~10× the default set on a large module). Pair it with
`-since` to keep PR checks fast.

## Fast PR checks: diff mode

`-since <git-ref>` loads and scans only the packages containing `.go` files changed since that ref:

```sh
gospect-mcp check -since origin/main -fail-on high .   # gate a PR on its own changes
```

On a large monorepo this turns a ~30s full scan into a **~1–2s** PR check. Use `origin/main...`
(three dots) for merge-base semantics. Diff mode skips whole-repo graph detectors.

## SARIF & vulnerabilities

```sh
gospect-mcp scan -format sarif . > gospect.sarif   # upload via github/codeql-action/upload-sarif
gospect-mcp scan -vuln .                           # also run govulncheck for known-CVE deps
```

`-format sarif` emits SARIF 2.1.0 for inline PR annotations. `-vuln` is opt-in (slow, needs the vuln
database); if `govulncheck` isn't installed it says so instead of failing.

## Use it in CI

`gospect-mcp check` exits non-zero when a finding is at/above a severity. Exit codes: `0` clean,
`1` blocking findings, `2` error. Flags may appear anywhere around the path.

```sh
gospect-mcp check -fail-on high .
gospect-mcp check -fail-on medium -ignore todo,go-version ./...
```

The drop-in GitHub Action (full workflow: [examples/gospect.yml](examples/gospect.yml)):

```yaml
- uses: backendArchitect/gospect-mcp@v1
  with:
    path: .
    fail-on: high        # high | medium | low
    comment: true        # post + update a findings comment on the PR (needs pull-requests: write)
    sarif: true          # upload SARIF annotations (needs security-events: write)
    # ignore: todo,go-version
    # gate: false        # report only, never fail the job
```

Both the PR comment and SARIF are produced **before** the gate, so they appear even when the check
fails. See [`packaging/README.md`](packaging/README.md) for Marketplace/publishing details.

## Fixing: the full story

gospect finds; it doesn't fix — unless you run `fix`, which is always explicit. Even then it never
trusts the change: it applies one fix, **re-scans**, checks the module still builds, and **rolls the
working tree back** if anything's off.

```sh
gospect-mcp fix -detector nilness .          # auto-detect an installed agent, fix one finding
gospect-mcp fix -min-severity high -n 5 .    # fix up to 5 (each verified fix is committed)
gospect-mcp fix -safe -staticcheck .         # deterministic analyzer fixes only — no AI
gospect-mcp fix -agent "aider --yes {prompt}" .
gospect-mcp fix -dry-run -detector nilness . # just print the prompt gospect would send
```

**`-safe` (no AI).** Applies an analyzer's mechanical fix directly — but only when it offers
**exactly one** (an ambiguous choice like `!!b` → `!b` *or* `b` is left to you). Others are skipped.

**Agent mode** auto-detects `claude`, `aider`, `cursor-agent`, `gemini`, `opencode` on your PATH
(override with `-agent`). Its output streams to stderr so a multi-minute fix visibly progresses;
`-timeout` (default `5m`) kills a stuck agent and rolls back.

**The safety contract** — a fix is *kept* only if all hold, else the tree is restored exactly:

1. A **clean git tree** to start (so any change is cleanly reversible).
2. On re-scan, the target finding is **gone** and **no new findings** appear.
3. The module still **builds** (`-test` also requires `go test`).
4. On success the change is left **uncommitted** for you to review.

Exit codes: `0` fixed, `1` rolled back, `2` error.

**Guarded MCP `fix` tool.** By default the MCP server only exposes `scan` and `propose_fix` and
never edits code. Start it with `--allow-fix` (or `GOSPECT_ALLOW_FIX=1`) to expose a `fix` tool that
is deterministic-only (no AI in the server) and self-verifying — same safety contract as above.

**`propose_fix`** (report-first) returns a *fix envelope* for a finding — root cause, an adversarial
"verify it's real first" checklist, and constraints — without editing anything:

```sh
echo '{"detector":"unchecked-error","file":"x.go","line":14,"message":"..."}' | gospect-mcp propose-fix
```

## Connect an AI editor (MCP)

gospect speaks the Model Context Protocol over stdio, so **any MCP client can use it** — no plugin.
Every scan is stateless, so point as many agents at it as you like.

**Claude Code:**

```sh
claude mcp add gospect gospect-mcp
```

**Any other client** (Cursor, Windsurf, Cline, VS Code, Zed, Codex CLI, Gemini CLI, Claude Desktop)
takes the same stdio config:

```json
{
  "mcpServers": {
    "gospect": { "command": "gospect-mcp", "args": [] }
  }
}
```

Because the server is report-only, an agent **can't** change your code through gospect — only read
findings and, on request, a fix envelope.

## Whole-repo detectors & the code graph

Two detectors need a repo-wide view and run off a **built-in graph** of the loaded packages:

- **over-engineered / high-complexity** — on by default; functions past conservative cyclomatic/
  cognitive thresholds.
- **untested-exports** — opt-in with `-untested` (exported functions with no test); noisy on large
  repos, so it's off by default.
- **stale-doc / swagger-drift** — runs automatically when an OpenAPI/Swagger spec is present and
  routes are found in the code; flags documented endpoints with no matching route.

One more, **unhandled-route**, needs a real call/route graph. gospect can act as an MCP client of a
graph server like [codebase-memory-mcp](https://github.com/DeusData/codebase-memory-mcp) — no graph
of its own. Enable with three optional env vars:

```sh
export GOSPECT_GRAPH_CMD="codebase-memory-mcp"   # command to launch the graph server
export GOSPECT_GRAPH_PROJECT="my-project"         # project name to query
export GOSPECT_GRAPH_SCOPE="internal/"            # optional path-substring scope
gospect-mcp scan /path/to/module
```

A graph failure never fails the scan — it's recorded in the report's `graph_error` and local
findings are still returned.

## What it detects (full list)

Every finding carries `category`, `detector`, `severity`, `confidence`, `file:line`, and `message`.

**The default run is deliberately high-signal — genuine bugs (plus `stub`) only.** The opinionated
hygiene heuristics that fire on clean code (they produced ~99% of the findings on the Go stdlib in
testing) are **off by default** and enabled together with **`-pedantic`**.

### Default (always on)

| Category | Detectors |
|---|---|
| **bug** | `nilness` (nil deref), `lostcancel` (leaked `context.CancelFunc`), `bodyclose` (unclosed HTTP body), `httpresponse`, `unmarshal`, `copylock`, `errorsas`, `printf` (format/arg mismatch), `atomic` (lost `sync/atomic` update), `sortslice` (non-slice to `sort.Slice`), `unusedresult` (ignored `errors.New`/`fmt.Errorf`), `stringintconv` (`string(int)`), `timeformat` (wrong time layout), `sigchanyzer` (unbuffered signal channel), `appends` (empty `append`), `shift` (over-wide shift), `bools` (redundant boolean), `nilfunc`, `unreachable`, `ineffassign` (dead assignment) |
| **missing** | unimplemented stubs (`panic("not implemented")`) |
| **modernize** | `loopclosure` (pre-1.22 loop-var capture) |

### Opt-in

| Flag | Adds |
|---|---|
| `-pedantic` | `unchecked-error` (unchecked error returns), `high-complexity` (complexity hotspots, capped at medium severity), `todo` (`TODO`/`FIXME` markers), `go-version` (outdated `go.mod` directive) |
| `-staticcheck` | ~100 `SA` checks from [staticcheck](https://staticcheck.dev) |
| `-untested` | exported functions with no test |
| `-vuln` | `govulncheck` known-CVE dependencies |

The default set is built entirely on `golang.org/x/tools`.

## The `scan` tool

**Input:** `path` (string, required) — the Go module dir; `patterns` (string[], default `["./..."]`).

**Output:** a JSON `Report` — load stats plus a flat, severity-sorted list of findings:

```json
{
  "path": "./testdata/buggy",
  "packages_loaded": 1,
  "finding_count": 5,
  "by_category": { "bug": 1, "missing": 3, "modernize": 1 },
  "findings": [
    { "category": "bug", "detector": "nilness", "severity": "high",
      "file": ".../buggy.go", "line": 8, "message": "nil dereference in load" }
  ]
}
```

## Performance

Bounded by `go build`, not the tool. Warm-cache, single machine:

| Scope | Packages | Time |
|---|---|---|
| Single small module | ~12 | ~1s |
| Mid-size module | 272 | ~2.5s |
| Full 9-module monorepo | 458 | ~26s |

The **first** scan of a big repo is slower while Go compiles dependencies once; later scans are
fast. Scope with a package pattern (`./somepkg/...`) for instant results.

## Staying up to date

```sh
gospect-mcp version     # print the installed version
gospect-mcp update      # update to the latest release if one exists
gospect-mcp uninstall   # remove the binary (add --yes to skip the prompt)
```

## How it works

```
Go module ──► go/packages (load + type-check) ──► detectors ──► Report (JSON)
```

1. **Load** the packages with full types + syntax via `go/packages`.
2. **Detect** — run curated `go/analysis` passes plus lightweight AST checks.
3. **Report** — aggregate, de-dupe, sort, summarize. Never edit.

The MCP layer is a hand-rolled JSON-RPC 2.0 stdio server — no SDK dependency.
