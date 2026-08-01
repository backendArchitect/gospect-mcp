package detect

import "testing"

func TestVulnMessage(t *testing.T) {
	got := vulnMessage("GO-2024-1", "buffer overflow", "example.com/x", "v1.2.3")
	want := "GO-2024-1: buffer overflow (module example.com/x, fixed in v1.2.3)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	// No summary, no fix version still reads cleanly.
	if got := vulnMessage("GO-2024-2", "", "m", ""); got != "GO-2024-2 (module m)" {
		t.Fatalf("minimal case = %q", got)
	}
}
