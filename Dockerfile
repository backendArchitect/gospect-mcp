# gospect-mcp container image.
#
# IMPORTANT: the runtime stage keeps the Go toolchain on purpose — gospect loads a target module
# with `go/packages`, which shells out to `go list`/the compiler. A scratch/distroless image without
# `go` on PATH cannot scan anything. `git` is included for `-since` diff mode and the `fix` command.
#
# Usage:
#   docker run --rm -v "$PWD":/work ghcr.io/backendarchitect/gospect-mcp scan ./...
#   docker run --rm -v "$PWD":/work ghcr.io/backendarchitect/gospect-mcp check -fail-on high ./...
ARG GO_VERSION=1.26

FROM golang:${GO_VERSION}-alpine AS build
ARG VERSION=dev   # release.yml passes the tag so `gospect-mcp version` matches the image tag
WORKDIR /src
# Cache modules first for faster rebuilds.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X github.com/backendArchitect/gospect-mcp/internal/selfupdate.injected=${VERSION}" \
    -o /out/gospect-mcp .

FROM golang:${GO_VERSION}-alpine
RUN apk add --no-cache git ca-certificates
COPY --from=build /out/gospect-mcp /usr/local/bin/gospect-mcp
WORKDIR /work
# No CMD: with no args gospect starts the MCP server over stdio (matching the binary). Pass a
# subcommand (scan/check/…) to use it as a CLI.
ENTRYPOINT ["gospect-mcp"]
