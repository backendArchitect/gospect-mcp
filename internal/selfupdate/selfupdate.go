// Package selfupdate implements the `gospect-mcp version` / `update` commands: it reads the
// installed version from Go build info, checks GitHub for the latest release, and (on request)
// updates via `go install module@tag`.
package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

const (
	repo   = "backendArchitect/gospect-mcp"
	module = "github.com/backendArchitect/gospect-mcp"
)

// Current returns the version embedded by `go install module@vX` (from build info), or "dev"
// for local `go build` / `go run` builds.
func Current() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}

type release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// Latest fetches the latest published release. Returns ("","",nil) when none exist (HTTP 404).
// apiBase defaults to the public GitHub API; tests override it.
func Latest(ctx context.Context, apiBase string) (tag, url string, err error) {
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("github api: %s", resp.Status)
	}
	var r release
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", "", err
	}
	return r.TagName, r.HTMLURL, nil
}

// Newer reports whether latest is a newer semver than current (both like "v1.2.3" or "1.2.3").
// A non-release current (e.g. "dev") is always considered older.
func Newer(current, latest string) bool {
	if !strings.HasPrefix(strings.TrimSpace(current), "v") && parse(current) == [3]int{} {
		return true
	}
	return compare(latest, current) > 0
}

func compare(a, b string) int {
	as, bs := parse(a), parse(b)
	for i := 0; i < 3; i++ {
		switch {
		case as[i] > bs[i]:
			return 1
		case as[i] < bs[i]:
			return -1
		}
	}
	return 0
}

func parse(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	v = strings.SplitN(v, "-", 2)[0] // drop any pre-release suffix
	var out [3]int
	for i, p := range strings.SplitN(v, ".", 3) {
		if i > 2 {
			break
		}
		out[i], _ = strconv.Atoi(p)
	}
	return out
}

// Install runs `go install module@tag`, streaming output. Requires the Go toolchain on PATH.
func Install(ctx context.Context, tag string) error {
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("go toolchain not found on PATH; download a binary from https://github.com/%s/releases", repo)
	}
	cmd := exec.CommandContext(ctx, "go", "install", module+"@"+tag)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
