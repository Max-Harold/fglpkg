package installer

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestRunParallelEmpty: empty input → nil, fn never called.
func TestRunParallelEmpty(t *testing.T) {
	var calls int32
	err := runParallel([]int{}, 4, func(int) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})
	if err != nil {
		t.Fatalf("runParallel([]) error: %v", err)
	}
	if calls != 0 {
		t.Errorf("fn called %d times, want 0", calls)
	}
}

// TestRunParallelSingle: single item bypasses goroutine machinery and
// propagates the returned error directly.
func TestRunParallelSingle(t *testing.T) {
	wantErr := errors.New("boom")
	err := runParallel([]int{42}, 4, func(x int) error {
		if x != 42 {
			t.Errorf("item = %d, want 42", x)
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

// TestRunParallelHonoursCap: with N items and cap=4, the maximum
// number of goroutines simultaneously inside fn never exceeds 4.
func TestRunParallelHonoursCap(t *testing.T) {
	const (
		nItems = 20
		cap    = 4
	)
	items := make([]int, nItems)
	for i := range items {
		items[i] = i
	}

	var (
		inFlight int32
		maxSeen  int32
	)
	err := runParallel(items, cap, func(int) error {
		now := atomic.AddInt32(&inFlight, 1)
		// Track the high-water mark.
		for {
			cur := atomic.LoadInt32(&maxSeen)
			if now <= cur || atomic.CompareAndSwapInt32(&maxSeen, cur, now) {
				break
			}
		}
		// Hold the goroutine briefly so concurrency is visible.
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return nil
	})
	if err != nil {
		t.Fatalf("runParallel error: %v", err)
	}
	if maxSeen > int32(cap) {
		t.Errorf("maxSeen = %d, want <= %d", maxSeen, cap)
	}
	if maxSeen == 0 {
		t.Errorf("maxSeen = 0, expected at least 1 in-flight")
	}
}

// TestRunParallelAllRun: every item's fn is called, even when some
// fail. This guarantees we don't leave half-finished downloads hanging.
func TestRunParallelAllRun(t *testing.T) {
	items := []int{0, 1, 2, 3, 4, 5}
	var (
		mu      sync.Mutex
		visited = map[int]bool{}
	)
	_ = runParallel(items, 2, func(i int) error {
		mu.Lock()
		visited[i] = true
		mu.Unlock()
		if i == 2 {
			return errors.New("simulated failure")
		}
		return nil
	})
	for _, i := range items {
		if !visited[i] {
			t.Errorf("item %d was not processed; failures must not skip siblings", i)
		}
	}
}

// TestRunParallelFirstErrorReturned: with multiple failures, exactly
// one error is returned. Index-order winning is documented behavior.
func TestRunParallelFirstErrorReturned(t *testing.T) {
	err1 := errors.New("first")
	err2 := errors.New("second")
	got := runParallel([]int{0, 1, 2}, 4, func(i int) error {
		switch i {
		case 1:
			return err1
		case 2:
			return err2
		}
		return nil
	})
	if !errors.Is(got, err1) {
		t.Errorf("got %v, want err1 (lowest-index error)", got)
	}
}

func TestInstallConcurrencyDefault(t *testing.T) {
	t.Setenv("FGLPKG_INSTALL_CONCURRENCY", "")
	if got := installConcurrency(); got != defaultInstallConcurrency {
		t.Errorf("unset env: got %d, want %d", got, defaultInstallConcurrency)
	}
}

func TestInstallConcurrencyOverride(t *testing.T) {
	t.Setenv("FGLPKG_INSTALL_CONCURRENCY", "8")
	if got := installConcurrency(); got != 8 {
		t.Errorf("env=8: got %d, want 8", got)
	}
}

func TestInstallConcurrencyBadValueFallsBack(t *testing.T) {
	for _, bad := range []string{"-3", "0", "garbage", "1.5"} {
		t.Setenv("FGLPKG_INSTALL_CONCURRENCY", bad)
		if got := installConcurrency(); got != defaultInstallConcurrency {
			t.Errorf("env=%q: got %d, want fallback %d", bad, got, defaultInstallConcurrency)
		}
	}
}
