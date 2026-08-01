package stdout

import (
	"io"
	"os"
	"sync"
)

var (
	w  io.Writer = os.Stdout
	mu sync.RWMutex
)

// Writer returns the current stdout writer (real stdout or discard).
func Writer() io.Writer {
	mu.RLock()
	defer mu.RUnlock()
	return w
}

// Quiet replaces stdout with io.Discard and returns a cleanup function.
// Usage:
//
//	defer stdout.Quiet()()
//
// WARNING: Quiet must ONLY be called from the main goroutine, before spawning
// any concurrent work that writes to stdout, and its returned cleanup must be
// deferred in the same goroutine. Never call Quiet from multiple goroutines
// concurrently — it is not designed for nested or parallel silencing.
func Quiet() func() {
	mu.Lock()
	old := w
	w = io.Discard
	mu.Unlock()
	return func() {
		mu.Lock()
		w = old
		mu.Unlock()
	}
}
