# Packaging & distribution

How gospect-mcp reaches users, and the one-time / per-release steps each channel needs.

| Channel | Automated? | Setup |
|---|---|---|
| `go install` | ✅ | Nothing — works from source. |
| Prebuilt binaries | ✅ | `release.yml` publishes them per tag. |
| Docker (GHCR) | ✅ | `release.yml` builds + pushes `ghcr.io/backendarchitect/gospect-mcp` per tag. |
| pre-commit | ✅ | `.pre-commit-hooks.yaml` — consumers pin a tag. |
| Homebrew | manual | Needs a tap repo + a per-release formula bump (below). |
| GitHub Marketplace | one-time | Publish the Action from a release (below). |

## Docker

Built and pushed automatically on release to `ghcr.io/backendarchitect/gospect-mcp:{vX.Y.Z,latest}`
(linux/amd64 + arm64). To build locally:

```sh
docker build -t gospect-mcp .
docker run --rm -v "$PWD":/work gospect-mcp scan ./...
```

The image intentionally ships the Go toolchain — gospect shells out to `go list` to load the target
module, so a toolchain-less image can't scan.

## Homebrew tap

Homebrew needs a tap repo. One-time:

1. Create a public repo named **`homebrew-tap`** under the `backendArchitect` org.
2. Copy [`homebrew/gospect-mcp.rb`](homebrew/gospect-mcp.rb) into it as `Formula/gospect-mcp.rb`.

Users then run:

```sh
brew install backendArchitect/tap/gospect-mcp
```

Per release, bump the formula's `url` and `sha256`:

```sh
tag=v1.2.3
url="https://github.com/backendArchitect/gospect-mcp/archive/refs/tags/${tag}.tar.gz"
sha=$(curl -sL "$url" | shasum -a 256 | cut -d' ' -f1)
echo "url $url"; echo "sha256 $sha"
```

(Or automate it later with `brew bump-formula-pr`, or a `release.yml` job that opens a PR to the tap
using a `HOMEBREW_TAP_TOKEN` secret.)

## GitHub Marketplace (the Action)

`action.yml` already carries the required `name`, `description`, and `branding`. To list it:

1. Open the repo's **Releases** → the tag you want to publish.
2. Edit the release; check **"Publish this Action to the GitHub Marketplace"**.
3. Accept the agreement, pick a primary + secondary category (e.g. *Code quality*, *Continuous
   integration*), and publish.

After that, consumers can find it in the Marketplace and use `backendArchitect/gospect-mcp@vX`.
