package selfupdate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUninstallRemovesFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "gospect-mcp")
	if err := os.WriteFile(f, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Uninstall(f); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Fatal("binary should be gone after Uninstall")
	}
}

func TestIsEphemeralBuild(t *testing.T) {
	if !IsEphemeralBuild("/root/.cache/go-build/ab/cd/exe/main") {
		t.Error("a go-build path should be ephemeral")
	}
	if IsEphemeralBuild("/home/u/go/bin/gospect-mcp") {
		t.Error("an installed GOBIN path should NOT be ephemeral")
	}
}

func TestNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"v1.0.0", "v1.0.1", true},
		{"v1.2.0", "v1.10.0", true}, // numeric, not lexical
		{"v2.0.0", "v1.9.9", false},
		{"v1.0.0", "v1.0.0", false},
		{"dev", "v0.0.1", true}, // non-release is always older
	}
	for _, c := range cases {
		if got := Newer(c.current, c.latest); got != c.want {
			t.Errorf("Newer(%q,%q)=%v want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v1.2.3","html_url":"https://example/releases/v1.2.3"}`))
	}))
	defer srv.Close()

	tag, url, err := Latest(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if tag != "v1.2.3" || url != "https://example/releases/v1.2.3" {
		t.Fatalf("got tag=%q url=%q", tag, url)
	}
}

func TestLatest_NoReleases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	tag, _, err := Latest(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if tag != "" {
		t.Fatalf("want empty tag for 404, got %q", tag)
	}
}
