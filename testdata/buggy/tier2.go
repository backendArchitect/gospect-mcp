package buggy

import (
	"log/slog"
	"reflect"
	"sync"
	"time"
)

// WGAddInGoroutine calls wg.Add inside the goroutine (waitgroup).
func WGAddInGoroutine() {
	var wg sync.WaitGroup
	go func() {
		wg.Add(1)
		wg.Done()
	}()
	wg.Wait()
}

// DeepEqualErrors compares errors with reflect.DeepEqual (deepequalerrors).
func DeepEqualErrors(a, b error) bool {
	return reflect.DeepEqual(a, b)
}

// CompareReflectValue compares reflect.Value with == (reflectvaluecompare).
func CompareReflectValue(a, b reflect.Value) bool {
	return a == b
}

// BadSlog passes an odd number of key/value args to slog (slog).
func BadSlog() {
	slog.Info("hello", "key")
}

// DeferSince evaluates time.Since immediately instead of at return (defers).
func DeferSince() {
	t := time.Now()
	defer time.Since(t)
}

// ShadowErr shadows the outer err (shadow — pedantic only).
func ShadowErr() error {
	err := mightFail()
	if err != nil {
		err := mightFail()
		_ = err
	}
	return err
}
