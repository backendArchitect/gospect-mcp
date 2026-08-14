package buggy

import (
	"fmt"
	"os"
	"os/signal"
	"sort"
	"sync/atomic"
	"time"
)

// PrintfMismatch passes a string where %d expects an int (printf).
func PrintfMismatch() {
	fmt.Printf("%d\n", "not-a-number")
}

// AtomicLostUpdate assigns an atomic result back to the same variable (atomic).
func AtomicLostUpdate() int64 {
	var n int64
	n = atomic.AddInt64(&n, 1)
	return n
}

// SortNonSlice sorts a value that isn't a slice (sortslice) — panics at runtime.
func SortNonSlice() {
	sort.Slice(42, func(i, j int) bool { return false })
}

// IgnoredError discards the error that fmt.Errorf builds (unusedresult).
func IgnoredError() {
	fmt.Errorf("boom")
}

// StringFromInt converts an int with string() instead of strconv.Itoa (stringintconv — has a fix).
func StringFromInt() string {
	i := 65
	return string(i)
}

// BadTimeLayout swaps month and day in the layout (timeformat — has a fix).
func BadTimeLayout() string {
	return time.Now().Format("2006-02-01")
}

// UnbufferedSignal passes an unbuffered channel to signal.Notify (sigchanyzer).
func UnbufferedSignal() {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt)
}

// EmptyAppend calls append with no values to append (appends).
func EmptyAppend() []int {
	s := []int{1}
	s = append(s)
	return s
}

// OverShift shifts an 8-bit value further than its width (shift).
func OverShift() int8 {
	var x int8 = 1
	return x << 9
}

// RedundantBool is a tautological boolean expression (bools).
func RedundantBool(b bool) bool {
	return b && b
}
