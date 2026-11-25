package preview

import (
	"context"
	"testing"
	"time"
)

func TestLatestDiscardsStaleWork(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	started := make(chan struct{})
	delivered := make(chan int, 4)
	w := New(ctx, 5*time.Millisecond, func(ctx context.Context, v int) int {
		if v == 1 {
			close(started)
			<-ctx.Done()
		}
		return v
	}, func(_ uint64, v int) { delivered <- v })
	defer w.Close()
	first := w.Submit(1)
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("first task did not start")
	}
	second := w.Submit(2)
	third := w.Submit(3)
	select {
	case got := <-delivered:
		if got != 3 || !w.IsCurrent(third) || w.IsCurrent(first) || w.IsCurrent(second) {
			t.Fatalf("stale value: %d", got)
		}
	case <-ctx.Done():
		t.Fatal("latest preview was not delivered")
	}
}
