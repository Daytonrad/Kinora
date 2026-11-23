// Package safe runs Kinora's long-lived background loops so that a panic in one of them (an
// unexpected file, a broken indexer definition) is logged and the loop restarts, instead of the
// whole server going down. net/http already does this for request handlers.
package safe

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"
)

// Backoff bounds for restarts after a panic.
var (
	MinBackoff = 5 * time.Second
	MaxBackoff = 5 * time.Minute
)

// Go starts fn in a goroutine and restarts it after a panic, with growing pauses, until ctx ends.
// A normal return of fn ends the loop.
func Go(ctx context.Context, log *slog.Logger, name string, fn func(context.Context)) {
	go Run(ctx, log, name, fn)
}

// Run is Go without the goroutine; it returns when fn returns normally or ctx ends.
func Run(ctx context.Context, log *slog.Logger, name string, fn func(context.Context)) {
	wait := MinBackoff
	for {
		err := Call(func() error { fn(ctx); return nil })
		if err == nil || ctx.Err() != nil {
			return
		}
		log.Error("background task panicked, restarting", "task", name, "err", err, "restart_in", wait.String(),
			"stack", string(err.(*PanicError).Stack))
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(wait*2, MaxBackoff)
	}
}

// PanicError carries a recovered panic and its stack.
type PanicError struct {
	Value any
	Stack []byte
}

func (e *PanicError) Error() string { return fmt.Sprintf("panic: %v", e.Value) }

// Call runs fn and turns a panic into a *PanicError, so one bad input (a file, an indexer answer)
// fails on its own instead of taking the process down.
func Call(fn func() error) (err error) {
	defer func() {
		if v := recover(); v != nil {
			err = &PanicError{Value: v, Stack: debug.Stack()}
		}
	}()
	return fn()
}

// Recover is deferred at the top of a one-off goroutine: it logs a panic instead of crashing.
func Recover(log *slog.Logger, name string) {
	if v := recover(); v != nil {
		log.Error("background task panicked", "task", name, "err", fmt.Sprint(v), "stack", string(debug.Stack()))
	}
}
