# Homebrew formula for gospect-mcp. Lives in a tap repo (backendArchitect/homebrew-tap) so users can:
#
#   brew install backendArchitect/tap/gospect-mcp
#
# It builds from the tagged source with the Go toolchain (no bottle to maintain). Bump `url` +
# `sha256` on each release — see packaging/README.md. `--HEAD` installs from main.
class GospectMcp < Formula
  desc "Report-first, Go-only code scanner (MCP server + CLI)"
  homepage "https://github.com/backendArchitect/gospect-mcp"
  url "https://github.com/backendArchitect/gospect-mcp/archive/refs/tags/v1.0.0.tar.gz"
  sha256 "REPLACE_WITH_TARBALL_SHA256"
  license "MIT"
  head "https://github.com/backendArchitect/gospect-mcp.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w"), "."
  end

  test do
    assert_match "gospect-mcp", shell_output("#{bin}/gospect-mcp version")
  end
end
