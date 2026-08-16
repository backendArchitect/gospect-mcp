package buggy

// NilCond has an impossible condition: p is provably nil, so `p != nil` can never be true.
// nilness reports this as a *condition* diagnostic (not a dereference), so gospect classifies it as
// the low, -pedantic-only "nil-condition" detector — it false-positives on correct defensive code.
func NilCond() int {
	var p *int
	if p != nil {
		return *p
	}
	return 0
}
