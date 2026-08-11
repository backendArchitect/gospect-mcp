package buggy

import (
	"context"
	"sync"
	"time"
)

// Guarded holds a lock; passing it by value copies the lock (copylocks — bug).
type Guarded struct {
	mu sync.Mutex
	n  int
}

// CopyLock takes Guarded by value, copying its Mutex.
func CopyLock(g Guarded) int {
	return g.n
}

// LeakCancel discards the cancel func from context.WithTimeout (lostcancel — bug).
func LeakCancel() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), time.Second)
	return ctx
}
