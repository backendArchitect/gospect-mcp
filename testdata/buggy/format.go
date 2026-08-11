package buggy

import "fmt"

// Echo passes a dynamic string as the format directive (SA1006 — staticcheck-only). It also has
// exactly one suggested fix (Printf -> Print), so it exercises the deterministic `-safe` path.
func Echo(msg string) {
	fmt.Printf(msg)
}
