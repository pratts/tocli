package engine

import (
	"testing"
	"time"
)

// TestRunWithTimeout_ReturnsPromptlyOnSlowOperation confirms the shutdown
// path can't be blocked indefinitely by a slow/hung client.Close(): if the
// wrapped operation doesn't finish within the timeout, runWithTimeout must
// return anyway rather than waiting for it.
func TestRunWithTimeout_ReturnsPromptlyOnSlowOperation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})

	go func() {
		start := time.Now()
		runWithTimeout(50*time.Millisecond, func() {
			close(started)
			<-release // simulates an operation that hangs past the timeout
		})
		elapsed := time.Since(start)
		if elapsed > 500*time.Millisecond {
			t.Errorf("runWithTimeout blocked for %s, want it to return near the 50ms timeout", elapsed)
		}
		close(finished)
	}()

	<-started
	select {
	case <-finished:
	case <-time.After(1 * time.Second):
		t.Fatal("runWithTimeout did not return within 1s of its 50ms timeout")
	}

	close(release) // let the leaked goroutine's fn finish so it doesn't outlive the test
}

// TestRunWithTimeout_WaitsForFastOperation is the control case: a fast
// operation should be waited for normally, not cut off.
func TestRunWithTimeout_WaitsForFastOperation(t *testing.T) {
	ran := false
	runWithTimeout(1*time.Second, func() { ran = true })
	if !ran {
		t.Fatal("fast operation did not run to completion before runWithTimeout returned")
	}
}
