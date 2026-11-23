package safe

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestRunRestartsAfterPanic(t *testing.T) {
	MinBackoff, MaxBackoff = time.Millisecond, 4*time.Millisecond
	defer func() { MinBackoff, MaxBackoff = 5*time.Second, 5*time.Minute }()
	var calls atomic.Int32
	done := make(chan struct{})
	go func() {
		Run(context.Background(), quiet, "test", func(ctx context.Context) {
			if calls.Add(1) < 3 {
				panic("boom")
			}
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not finish")
	}
	if calls.Load() != 3 {
		t.Fatalf("calls %d, want 3 (two panics, then a normal return)", calls.Load())
	}
}

func TestRunStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Run(ctx, quiet, "test", func(ctx context.Context) { panic("always") })
		close(done)
	}()
	time.Sleep(10 * time.Millisecond) // first panic, now waiting MinBackoff (5 s)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run must return when the context ends during the backoff")
	}
}

func TestRecover(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer Recover(quiet, "one-off")
		panic("boom")
	}()
	<-done
}
