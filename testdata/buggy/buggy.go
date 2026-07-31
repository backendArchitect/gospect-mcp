package buggy

// Deref dereferences p on a branch where it is statically known to be nil.
// nilness should flag the `*p` in the nil branch. This is the same bug class the
// scanner was born to catch.
func Deref(p *int) int {
	if p == nil {
		return *p // nil dereference
	}
	return *p
}
