package installer

import (
	"fmt"
	"os"
	"strconv"
	"sync"
)

// printMu serializes stdout writes from concurrent install workers so
// progress lines stay atomic and never interleave mid-message.
var printMu sync.Mutex

// printSync is fmt.Printf guarded by printMu — every install worker
// uses it when writing progress lines.
func printSync(format string, a ...any) {
	printMu.Lock()
	defer printMu.Unlock()
	fmt.Printf(format, a...)
}

// defaultInstallConcurrency caps concurrent downloads when the env
// override is unset or invalid. A small fixed pool keeps the registry
// and Maven Central honest and gives most projects the speedup they
// care about without flooding the local network stack.
const defaultInstallConcurrency = 4

// installConcurrency returns the worker cap for one install phase.
// Reads FGLPKG_INSTALL_CONCURRENCY each call so tests can override
// per-test via t.Setenv.
func installConcurrency() int {
	raw := os.Getenv("FGLPKG_INSTALL_CONCURRENCY")
	if raw == "" {
		return defaultInstallConcurrency
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		// Bad value — fall back rather than refuse to install. Env
		// tuning should never break a working install.
		return defaultInstallConcurrency
	}
	return n
}

// runParallel calls fn(item) for each item, bounded by cap workers.
// All goroutines run to completion regardless of where an error
// originated; the first non-nil error fn returned (in goroutine-start
// order) is the one this function returns.
//
// cap <= 0 is treated as 1 (sequential).
//
// The function is intentionally small and dependency-free so that it
// can be reused for non-install parallelism later (e.g. audit's
// per-JAR OSV.dev queries) without dragging in errgroup or similar.
func runParallel[T any](items []T, cap int, fn func(T) error) error {
	if cap < 1 {
		cap = 1
	}
	if len(items) == 0 {
		return nil
	}
	if len(items) == 1 {
		return fn(items[0])
	}

	sem := make(chan struct{}, cap)
	errs := make([]error, len(items))
	var wg sync.WaitGroup

	for i, item := range items {
		i, item := i, item
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			errs[i] = fn(item)
		}()
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
