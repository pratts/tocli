package engine

import (
	"context"
	"errors"
	"testing"
	"time"
)

// unreachableMagnet has a syntactically valid, non-zero (an all-zero hash
// is rejected outright by the library) but essentially made-up info hash
// with no real swarm behind it, so GotInfo() never resolves -- exactly the
// "dead magnet" case MetadataTimeout exists for.
const unreachableMagnet = "magnet:?xt=urn:btih:" + "deadbeef00deadbeef00deadbeef00deadbeef00"

// TestOpenPreview_RespectsContextTimeout proves the fix for the bug flagged
// in review: OpenPreview used to block on <-t.GotInfo() unconditionally,
// hanging forever against a dead magnet. It must now return promptly once
// ctx expires, rather than after the real (2-minute) MetadataTimeout.
func TestOpenPreview_RespectsContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	session, err := OpenPreview(ctx, unreachableMagnet)
	elapsed := time.Since(start)

	if session != nil {
		t.Fatal("expected no session for a resolution that never completes")
	}
	if err == nil {
		t.Fatal("expected an error once the context expired")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want it to wrap context.DeadlineExceeded", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("OpenPreview took %s to return, want it bounded by the ~200ms context timeout", elapsed)
	}
}

// TestOpenPreview_RespectsExplicitCancellation proves Esc-style cancellation
// (calling a stored cancel func, not waiting out a deadline) also returns
// promptly -- this is what closes the addflow leak: pressing Esc during
// the loading screen now actually stops the in-flight resolution instead
// of abandoning it to run for the rest of MetadataTimeout.
func TestOpenPreview_RespectsExplicitCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	var session *PreviewSession
	var err error
	go func() {
		session, err = OpenPreview(ctx, unreachableMagnet)
		close(done)
	}()

	// Give it a moment to actually start (create the client, add the
	// magnet) before cancelling, closer to the real Esc-mid-resolution
	// scenario than cancelling before it even begins.
	time.Sleep(50 * time.Millisecond)
	cancelledAt := time.Now()
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("OpenPreview did not return within 2s of explicit cancellation")
	}
	elapsed := time.Since(cancelledAt)

	if session != nil {
		t.Fatal("expected no session once cancelled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want it to wrap context.Canceled", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("OpenPreview took %s to return after cancellation, want near-immediate", elapsed)
	}
}
