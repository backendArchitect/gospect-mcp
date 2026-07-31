package selfupdate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
