# gospect-mcp

![Go](https://img.shields.io/badge/Go-1.21%2B-00ADD8?logo=go&logoColor=white)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)
![MCP](https://img.shields.io/badge/MCP-server-8A2BE2)
[![CI](https://github.com/backendArchitect/gospect-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/backendArchitect/gospect-mcp/actions/workflows/ci.yml)
[![Release](https://github.com/backendArchitect/gospect-mcp/actions/workflows/release.yml/badge.svg)](https://github.com/backendArchitect/gospect-mcp/actions/workflows/release.yml)

<img width="1024" height="559" alt="image" src="https://github.com/user-attachments/assets/c302ec7c-ce66-4429-b276-50c473d4c15c" />


**🌐 Website:** [backendarchitect.github.io/gospect-mcp](https://backendarchitect.github.io/gospect-mcp/)

**A Go-only, report-first code scanner exposed as an MCP server.** It indexes a Go module,
runs deterministic analyzers, and **reports** genuine bugs, dead code, stale docs and outdated
APIs. It **never modifies your code** — fixes are a separate, explicitly-invoked step.

Think of it as an *MRI for your Go code*: a deep, non-invasive scan that produces a diagnosis
your editor or AI agent acts on — not a tool that silently rewrites things.

---

## Quick start

Install the latest version straight from source (needs Go 1.21+):

```sh
go install github.com/backendArchitect/gospect-mcp@latest
```

This drops a `gospect-mcp` binary in your `$GOBIN` (usually `~/go/bin` — make sure it's on your
`PATH`). Or grab a prebuilt binary (Linux/macOS/Windows, amd64/arm64) from the
[Releases page](https://github.com/backendArchitect/gospect-mcp/releases). Then scan any Go module:

```sh
gospect-mcp scan /path/to/your/module ./...
```

You'll get a JSON report of findings, **ordered by importance — bugs first**, then medium, then low.
That's it — no config, no services, no code changes. Run `gospect-mcp help` for every command and flag.

Add `-verbose` to watch a long scan work (per-module load progress + a summary, all on stderr; the
JSON on stdout is unchanged):

```sh
gospect-mcp scan -verbose /path/to/monorepo
```

### Cutting the noise (filters + text output)

A big repo can surface hundreds of findings. Narrow the output — the same flags work on `scan` and
`check`:

```sh
gospect-mcp scan -format text -min-severity high .      # readable, only the serious stuff
gospect-mcp scan -category bug .                        # just bugs
gospect-mcp scan -detector nilness,unmarshal .          # specific detectors
gospect-mcp scan -exclude '*.pb.go,mocks/' .            # skip generated code / mocks
```

- `-min-severity low|medium|high` — drop everything below the given severity.
- `-category a,b` / `-detector a,b` — keep only those categories/detectors.
- `-exclude glob,glob` — drop findings whose file path matches a glob or substring.
- `-format text` (scan) — a readable grouped listing instead of JSON.

> **Speed & gotchas.** `scan` requires a path — `gospect-mcp scan` *alone* starts the MCP server
> (it waits on stdin, which can look like a hang). A whole large module scans in a few seconds
> (e.g. ~270 packages in ~5s), but the **first** scan of a big repo may take longer while Go
> compiles its dependencies once — subsequent scans are fast. Use `-verbose` to see progress.

### Monorepos (multiple modules)

Point `scan` at the **repo root** and it finds every nested `go.mod` and scans them all in one
run — no need to loop over services by hand:

```sh
gospect-mcp scan ./my-monorepo        # scans the root module + every nested service module
```

(Multi-module discovery kicks in only for the default `./...` pattern; pass an explicit pattern to
scope to a single module.) If one module can't be loaded — stale vendoring, a private dependency —
it's **skipped with a reason** printed to stderr and listed in the report's `skipped_modules`, and
the other modules still scan. A partial scan never masquerades as full coverage.

### Not a Go project?

gospect-mcp is **Go-only**. Point it at a repo with no Go and it says so — and names what it looks
like instead (`… looks like a JavaScript/TypeScript (Node/React) project. gospect-mcp is Go-only.`)
rather than failing cryptically.

### Suppressing intentional findings

Mark deliberate code with a `//gospect:ignore` comment on the flagged line (or the line directly
above it), mirroring the familiar `//nolint` idiom:

```go
resp.Body.Close() //gospect:ignore                 // suppress any finding on this line
_ = risky()       //gospect:ignore unchecked-error  // suppress only these detectors (comma/space separated)
```

Suppressed findings drop from the report; the count is reported as `suppressed`. To silence a whole
detector across a run instead, use `check -ignore <detector,...>`.

For repo-wide, checked-in suppression, add a **`.gospectignore`** at the module root:

```gitignore
# .gospectignore — one rule per line
*.pb.go              # skip generated protobuf
mocks/               # skip a whole directory (path glob or substring)
detector:todo        # never report TODO markers
detector:go-version  # we pin Go deliberately
```

It's honored by both the CLI and the MCP server; the count is reported as `ignored`.

**Generated code is skipped automatically.** Files carrying the standard `// Code generated … DO NOT
EDIT.` marker (protobuf, mocks, stringer, …) never contribute findings — the fix belongs in the
generator, not its output. The count is reported as `generated`; pass `-include-generated` to scan
them anyway.

### Adopt on a noisy repo: baseline mode

An existing codebase can surface hundreds of findings. Snapshot them once, then only see — or gate
on — what's **new**:

```sh
gospect-mcp scan . > gospect-baseline.json          # snapshot today's findings (commit this)
gospect-mcp scan  -baseline gospect-baseline.json .  # later: shows only NEW findings
gospect-mcp check -baseline gospect-baseline.json -fail-on medium .   # CI fails only on new ones
```

Matching is by a line-independent fingerprint (detector + path + message), so a pre-existing finding
that merely shifts lines stays baselined.

### Deeper analysis: staticcheck

By default gospect runs a fast, high-precision analyzer set. Add `-staticcheck` to also run the
[staticcheck](https://staticcheck.dev) `SA` analyzers — the canonical Go bug checks (`SA1019`
deprecated-API usage, dead assignments, impossible conditions, and ~100 more):

```sh
gospect-mcp scan -staticcheck .                       # much deeper bug detection
gospect-mcp check -staticcheck -since origin/main .   # deep, but only on the PR's changes (fast)
```

It's opt-in because it's **thorough but slow** — roughly an order of magnitude slower than the
default set on a large module. Pairing it with `-since` (diff mode) keeps PR checks fast while still
getting staticcheck depth on the changed code. Findings land under the `bug` category (and
`modernize` for deprecated-API `SA1019`), so existing severity gates and filters apply.

### Fast PR checks: diff mode

On a pull request you don't need to rescan the whole repo — only what changed. `-since <git-ref>`
loads and scans just the packages containing `.go` files changed since that ref:

```sh
gospect-mcp scan  -since origin/main .              # scan only changed packages
gospect-mcp check -since origin/main -fail-on high . # gate a PR on its own changes
```

On a large monorepo this turns a ~30s full scan into a **~1–2s** PR check (it loads only the
touched module, not every service). Use `origin/main...` (three dots) for merge-base semantics on a
branch. Outside a git repo, or without `-since`, it does a normal full scan. Diff mode skips the
whole-repo graph detectors, since a PR check should only surface findings in the code it touched.

### GitHub code scanning (SARIF) & vulnerabilities

```sh
gospect-mcp scan -format sarif . > gospect.sarif     # upload via github/codeql-action/upload-sarif
gospect-mcp scan -vuln .                             # also run govulncheck for known-CVE deps
```

`-format sarif` emits SARIF 2.1.0 so findings appear as inline PR annotations. `-vuln` is opt-in
(it's slow and needs the vulnerability database); if `govulncheck` isn't installed it says so
instead of failing.

> Prefer building it yourself? See [From source](#from-source).

### Keeping up to date

```sh
gospect-mcp version         # print the installed version
gospect-mcp update          # check GitHub for a newer release; update if one exists, else "up to date"
gospect-mcp uninstall       # remove the installed binary (asks to confirm; add --yes to skip)
```

`update` checks the latest GitHub release and, when a newer one exists, reinstalls via
`go install …@<tag>`. If no release is newer it prints that you're up to date; if none are
published yet it says so.

`uninstall` deletes the running binary from disk (it resolves its own path, so it also works for
a downloaded binary). It won't touch a `go run` temp build. After removing, delete the `gospect`
entry from your MCP client config and any `GOSPECT_GRAPH_*` env vars.

---

## Why gospect-mcp

- **Report-first.** The default output is a *report*. It will not touch your code. Fixes only
  happen when you explicitly ask for them.
- **Sensor, not oracle.** The server runs pure Go tooling and emits findings with evidence. It
  uses **no LLM** and detects **no** installed AI — the MCP host you connect it to (Claude Code,
  Cursor, any MCP client) supplies the intelligence. That makes it model- and vendor-agnostic.
- **Genuine over noisy.** It builds on the real Go toolchain (`go/packages`, `go/analysis`,
  SSA) so candidates are semantically backed, not grep guesses.
- **Universal.** Works on any Go module where `go build ./...` succeeds — single- or
  multi-module.

---

## Multi-agent support

`gospect-mcp` speaks the Model Context Protocol over stdio, so **any MCP client can use it** — no
plugin, no adapter. And because every scan is **stateless** (nothing persists between calls, no
shared index, no background daemon), you can point as many agents at it as you like with nothing to
coordinate. One binary, any number of assistants.

**Claude Code**

```sh
claude mcp add gospect gospect-mcp
```

**Any other MCP client** — Cursor, Windsurf, Cline, VS Code (Continue/Copilot), Zed, Codex CLI,
Gemini CLI, Claude Desktop — takes the same stdio config (`~/.claude.json`, a project `.mcp.json`,
or the client's own MCP settings):

```json
{
  "mcpServers": {
    "gospect": {
      "command": "gospect-mcp",
      "args": []
    }
  }
}
```

| Surface | How it connects | Notes |
|---|---|---|
| Claude Code | `claude mcp add gospect gospect-mcp` | one-liner |
| Cursor / Windsurf / Cline | `mcpServers` JSON above | stdio |
| VS Code (Continue, Copilot MCP) | `mcpServers` JSON above | stdio |
| Zed / Codex CLI / Gemini CLI | client's MCP config | stdio |
| Claude Desktop | `claude_desktop_config.json` | stdio |

Then ask your agent to scan a module. It calls the `scan` tool and reasons over the report — and,
because the server is report-only, it **can't** change your code unless you explicitly ask (via the
separate `propose_fix` tool, which still only emits guidance).

---

## Performance

gospect builds on the Go type-checker, so its runtime is bounded by `go build` — not by the tool.
Warm-cache, single machine:

| Scope | Packages | Time | Notes |
|---|---|---|---|
| Single small module | ~12 | ~1s | load ~0.9s / scan ~0.1s |
| Mid-size module | 272 | ~2.5s | load ~2.1s / scan ~0.5s |
| Full 9-module monorepo | 458 | ~26s | modules load in parallel |

The **first** scan of a big repo is slower while Go compiles its dependencies once; subsequent scans
are fast. Monorepo modules load concurrently. Scope with a package pattern (`./somepkg/...`) for
instant results, and use `-verbose` to watch progress on a long run.

---

## What it detects

All detectors are deterministic and report-only. Findings carry `category`, `detector`,
`severity`, `file:line`, and `message`; the report includes a `by_category` summary.

| Category | Detectors |
|---|---|
| **bug** | `nilness` (SSA nil-deref), `lostcancel` (leaked `context.CancelFunc`), `httpresponse`, `unmarshal`, `copylock`, `errorsas`, `nilfunc`, `unreachable` |
| **missing** | unimplemented stubs (`panic("not implemented")`), `TODO`/`FIXME` markers, unchecked error returns (errcheck-lite) |
| **modernize** | outdated `go.mod` go directive, `loopclosure` (pre-1.22 loop-var capture) |

The default set is built entirely on `golang.org/x/tools`. Opt into the deeper
[staticcheck](https://staticcheck.dev) `SA` analyzers with `-staticcheck` (see below).

**Planned** (see the design notes): over-engineering, stale swagger/OpenAPI vs. routes,
untested exported code, routes-with-no-handler, deprecated-API detection, and a `propose_fix`
tool that emits a *fix envelope* (still never applies code) — all via optional composition with
a code graph such as [codebase-memory-mcp](https://github.com/DeusData/codebase-memory-mcp).

---

## Optional: compose with a code graph

Some detectors need whole-repo relationships (call graph, routes, test coverage) that
single-package analysis can't see. `gospect-mcp` gets these by acting as an MCP **client** of a
code-intelligence graph such as [codebase-memory-mcp](https://github.com/DeusData/codebase-memory-mcp)
— no graph of its own, no duplicated index.

Enable it with three env vars (all optional; unset = graph disabled, local detectors still run):

```sh
export GOSPECT_GRAPH_CMD="codebase-memory-mcp"   # command to launch the graph MCP server
export GOSPECT_GRAPH_PROJECT="my-project"         # project name to query
export GOSPECT_GRAPH_SCOPE="internal/"            # optional file-path substring to scope queries
gospect-mcp scan /path/to/module
```

When configured, the report gains graph-backed findings:
- **untested-exports** — exported functions with no incoming test.
- **over-engineered / high-complexity** — functions/methods whose cyclomatic **or** cognitive
  complexity exceeds conservative thresholds.
- **unhandled-route** — HTTP routes registered with no handler.
- **stale-doc / swagger-drift** — endpoints documented in an OpenAPI/Swagger spec (JSON or YAML)
  with no matching registered route (heuristic path matching; report-first).

A graph connection failure never fails the scan; it's recorded in the report's `graph_error`
field and the local findings are still returned.

## The `scan` tool

**Input**

| field | type | required | default | description |
|---|---|---|---|---|
| `path` | string | ✅ | — | filesystem dir of the Go module |
| `patterns` | string[] | | `["./..."]` | package patterns to scan |

**Output** — a JSON `Report`: load stats + a flat, sorted list of `Finding`s. No mutations.

### Example

```sh
gospect-mcp scan ./testdata/buggy
```

```json
{
  "path": "./testdata/buggy",
  "packages_loaded": 1,
  "load_errors": 0,
  "load_millis": 8,
  "finding_count": 5,
  "by_category": { "bug": 1, "missing": 3, "modernize": 1 },
  "findings": [
    {
      "category": "bug",
      "detector": "nilness",
      "severity": "high",
      "file": ".../buggy.go",
      "line": 8,
      "message": "nil dereference in load"
    },
    {
      "category": "missing",
      "detector": "unchecked-error",
      "severity": "medium",
      "file": ".../stubs.go",
      "line": 14,
      "message": "error return value is not checked"
    }
  ]
}
```

---

## Use it in CI (gate on findings)

`gospect-mcp check` scans and exits **non-zero** when any finding is at or above a severity —
so a PR fails if it introduces (say) a nil dereference.

```sh
gospect-mcp check -fail-on high .          # exit 1 if any high-severity finding
gospect-mcp check -fail-on medium -ignore todo,go-version ./...
gospect-mcp check -format json .           # machine-readable
```

Flags go **before** the path: `check [flags] <dir> [patterns...]`. Exit codes: `0` clean,
`1` blocking findings, `2` error.

Drop-in GitHub Action:

```yaml
- uses: backendArchitect/gospect-mcp@v1
  with:
    path: .
    fail-on: high      # high | medium | low
    # ignore: todo,go-version
```

**Post findings as code-scanning annotations (SARIF).** Set `sarif: true` and grant the job
`security-events: write` — findings then show up inline on the PR and in the Security tab. SARIF is
generated and uploaded *before* the gate, so annotations appear even when the check fails:

```yaml
permissions:
  contents: read
  security-events: write   # required for SARIF upload
steps:
  - uses: actions/checkout@v4
  - uses: backendArchitect/gospect-mcp@v1
    with:
      path: .
      sarif: true          # generate + upload SARIF
      fail-on: high        # still gate the job; set gate: false to annotate only
```

Or run it directly:

```yaml
- run: |
    go install github.com/backendArchitect/gospect-mcp@latest
    gospect-mcp check -fail-on high .
```

## The `propose_fix` tool (report-first)

Fixes are opt-in and separate from scanning. `propose_fix` takes a finding and returns a **fix
envelope** — it never edits code:

- `root_cause`, `expected_scope`, and a `reuse_hint` (reuse before adding)
- a `verify_first` checklist led by an **adversarial** "default to *not* a real issue" prompt
- ponytail `constraints` (smallest root-cause fix, no unrequested abstractions, one runnable check)

The calling agent uses the envelope to make a minimal, verified fix. CLI form:

```sh
echo '{"detector":"unchecked-error","file":"x.go","line":14,"message":"..."}' | gospect-mcp propose-fix
```

## How it works

```
  Go module ──► go/packages (load + type-check) ──► detectors ──► Report (JSON)
                     the risky core                  bug / missing / modernize
```

1. **Load** the target packages with full types + syntax via `go/packages`.
2. **Detect** — run curated `go/analysis` passes plus lightweight AST checks. Each diagnostic
   becomes a `Finding`.
3. **Report** — aggregate, de-dupe, sort, summarize. Never edit.

The MCP layer is a hand-rolled JSON-RPC 2.0 stdio server (`initialize`, `tools/list`,
`tools/call`) — no SDK dependency.

---

## Installation

Pick whichever fits — all give you the same `gospect-mcp` binary.

**One-line install (macOS / Linux)** — downloads the latest release binary for your platform:

```sh
curl -fsSL https://raw.githubusercontent.com/backendArchitect/gospect-mcp/main/install.sh | bash
```

Override the target dir with `GOSPECT_INSTALL_DIR` or the version with `GOSPECT_VERSION`.

**Windows (PowerShell)** — installs to `%LOCALAPPDATA%\gospect-mcp` and adds it to your PATH:

```powershell
irm https://raw.githubusercontent.com/backendArchitect/gospect-mcp/main/install.ps1 | iex
```

**With Go** (any platform with a Go toolchain):

```sh
go install github.com/backendArchitect/gospect-mcp@latest
```

**Prebuilt binaries** — Linux / macOS / Windows, amd64 & arm64 — from the
[Releases page](https://github.com/backendArchitect/gospect-mcp/releases). Each is a
`gospect-mcp_<tag>_<os>_<arch>.tar.gz` with a `.sha256` checksum.

**From source:**

```sh
git clone git@github.com:backendArchitect/gospect-mcp.git
cd gospect-mcp
go build -o gospect-mcp .     # build the binary
go test ./...                 # run the tests
./gospect-mcp scan ./testdata/buggy
```

`testdata/buggy` is a separate module with deliberate issues (a nil-deref, a stub, a TODO, an
unchecked error, an old `go.mod`) that the test suite asserts each detector catches.

---

## Releases & CI

- **PRs** run `go vet` + `go test` + `go build` (`ci.yml`).
- **Pushes to `main`** run the tests and then auto-cut a release (`release.yml`): patch-bump a
  `vX.Y.Z` tag and publish cross-platform binaries + checksums to GitHub Releases. Add
  `[skip release]` to a commit message to skip releasing.

---

## License

[MIT License](LICENSE).
