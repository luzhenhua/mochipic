// Package preview coalesces asynchronous work and discards stale results.
package preview

import (
	"context"
	"sync"
	"time"
)

// Latest owns one worker. Submit replaces queued work and cancels running
// work; the consumer must compare the delivered revision on its own thread.
type Latest[Q, R any] struct {
	ctx      context.Context
	cancel   context.CancelFunc
	delay    time.Duration
	work     func(context.Context, Q) R
	deliver  func(uint64, R)
	wake     chan struct{}
	done     chan struct{}
	mu       sync.Mutex
	version  uint64
	query    Q
	due      time.Time
	pending  bool
	inflight context.CancelFunc
}

func New[Q, R any](ctx context.Context, delay time.Duration, work func(context.Context, Q) R, deliver func(uint64, R)) *Latest[Q, R] {
	ctx, cancel := context.WithCancel(ctx)
	w := &Latest[Q, R]{ctx: ctx, cancel: cancel, delay: delay, work: work, deliver: deliver, wake: make(chan struct{}, 1), done: make(chan struct{})}
	go w.loop()
	return w
}

func (w *Latest[Q, R]) Submit(q Q) uint64 { return w.replace(q, w.delay, true) }

// SubmitNow coalesces queued work but skips the delay used for continuous edits.
func (w *Latest[Q, R]) SubmitNow(q Q) uint64 { return w.replace(q, 0, true) }
func (w *Latest[Q, R]) Cancel()              { var zero Q; w.replace(zero, 0, false) }

func (w *Latest[Q, R]) replace(q Q, delay time.Duration, pending bool) uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.version++
	w.query, w.due, w.pending = q, time.Now().Add(delay), pending
	if w.inflight != nil {
		w.inflight()
	}
	select {
	case w.wake <- struct{}{}:
	default:
	}
	return w.version
}

func (w *Latest[Q, R]) IsCurrent(revision uint64) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.version == revision && w.ctx.Err() == nil
}

func (w *Latest[Q, R]) Close() { w.cancel(); <-w.done }

func (w *Latest[Q, R]) loop() {
	defer close(w.done)
	for {
		select {
		case <-w.ctx.Done():
			return
		case <-w.wake:
		}
		for {
			w.mu.Lock()
			if w.ctx.Err() != nil {
				w.mu.Unlock()
				return
			}
			if !w.pending {
				w.mu.Unlock()
				break
			}
			if delay := time.Until(w.due); delay > 0 {
				w.mu.Unlock()
				timer := time.NewTimer(delay)
				select {
				case <-w.ctx.Done():
					timer.Stop()
					return
				case <-w.wake:
					timer.Stop()
				case <-timer.C:
				}
				continue
			}
			q, rev := w.query, w.version
			w.pending = false
			var zero Q
			w.query = zero
			ctx, cancel := context.WithCancel(w.ctx)
			w.inflight = cancel
			w.mu.Unlock()
			result := w.work(ctx, q)
			ok := ctx.Err() == nil
			cancel()
			w.mu.Lock()
			w.inflight = nil
			w.mu.Unlock()
			if ok && w.IsCurrent(rev) {
				w.deliver(rev, result)
			}
		}
	}
}
