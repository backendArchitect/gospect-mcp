package agent

import (
	"bytes"
	"context"
	"reflect"
	"runtime"
	"testing"
)

func TestParseAndSubstitute_ArgToken(t *testing.T) {
	a, err := Parse("mycli --edit {prompt} --yes")
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "mycli" {
		t.Fatalf("name = %q, want mycli", a.Name)
	}
	got, viaStdin := substitute(a.Args, "FIX THIS")
	want := []string{"mycli", "--edit", "FIX THIS", "--yes"}
	if !reflect.DeepEqual(got, want) || viaStdin {
		t.Fatalf("substitute = %v (stdin=%v), want %v (stdin=false)", got, viaStdin, want)
	}
}

func TestSubstitute_StdinWhenNoToken(t *testing.T) {
	a, _ := Parse("mycli --edit")
	_, viaStdin := substitute(a.Args, "p")
	if !viaStdin {
		t.Fatal("expected prompt piped via stdin when template has no {prompt}")
	}
}

func TestParse_Empty(t *testing.T) {
	if _, err := Parse("   "); err == nil {
		t.Fatal("expected error for empty template")
	}
}

func TestKnownNames(t *testing.T) {
	names := KnownNames()
	if len(names) == 0 || names[0] != "claude" {
		t.Fatalf("KnownNames = %v, want claude first", names)
	}
}

// TestRun_Stdin exercises Run end-to-end using a real system command (cat) as a stand-in agent.
func TestRun_Stdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses the POSIX 'cat'")
	}
	a, _ := Parse("cat") // no {prompt} -> prompt goes to stdin, cat echoes it
	var out bytes.Buffer
	if err := a.Run(context.Background(), t.TempDir(), "hello-prompt", &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "hello-prompt" {
		t.Fatalf("agent output = %q, want the piped prompt", got)
	}
}
