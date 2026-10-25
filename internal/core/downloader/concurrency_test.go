package downloader

import (
	"context"
	"sync"
	"testing"
)

func TestConcurrencyIsScopedAndReturnsToZero(t *testing.T) {
	ctx, cancel := context.WithCancel(WithConcurrencyTracking(context.Background()))
	defer cancel()
	other := WithConcurrencyTracking(context.Background())
	started := make(chan struct{}, 4)
	var workers sync.WaitGroup
	for i := 0; i < 4; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer TrackTransfer(ctx)()
			started <- struct{}{}
			<-ctx.Done()
		}()
	}
	for i := 0; i < 4; i++ {
		<-started
	}
	if count := ActiveTransfers(ctx); count != 4 {
		t.Errorf("got %d active requests, want 4", count)
	}
	if count := ActiveTransfers(other); count != 0 {
		t.Errorf("another job inherited %d requests", count)
	}
	cancel()
	workers.Wait()
	if count := ActiveTransfers(ctx); count != 0 {
		t.Fatalf("cancelled requests left a count of %d", count)
	}
	TrackTransfer(context.Background())()
	if ActiveTransfers(context.Background()) != 0 {
		t.Fatal("an untracked context must report zero")
	}
}
