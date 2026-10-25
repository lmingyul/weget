package downloader

import (
	"context"
	"sync/atomic"
)

type concurrencyKey struct{}

// WithConcurrencyTracking gives a download and its child requests one counter.
func WithConcurrencyTracking(ctx context.Context) context.Context {
	return context.WithValue(ctx, concurrencyKey{}, new(atomic.Int64))
}

// ActiveTransfers returns the number of media requests currently in flight.
func ActiveTransfers(ctx context.Context) int {
	if ctx == nil {
		return 0
	}
	if count, ok := ctx.Value(concurrencyKey{}).(*atomic.Int64); ok {
		return int(count.Load())
	}
	return 0
}

// TrackTransfer counts one request until the returned function is called.
func TrackTransfer(ctx context.Context) func() {
	count, ok := ctx.Value(concurrencyKey{}).(*atomic.Int64)
	if !ok {
		return func() {}
	}
	count.Add(1)
	return func() { count.Add(-1) }
}
