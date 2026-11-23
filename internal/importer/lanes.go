package importer

import (
	"context"
	"sync"
)

// lanes run work one at a time per key and keys independently: one import at
// a time per root folder, so a slow disk never blocks another. A lane's
// goroutine exists only while it has work; a job queued twice while waiting
// runs once.
type lanes struct {
	mu      sync.Mutex
	pending map[string][]int64
	queued  map[string]map[int64]bool
	wg      sync.WaitGroup
}

func newLanes() *lanes {
	return &lanes{pending: map[string][]int64{}, queued: map[string]map[int64]bool{}}
}

// add queues id on key's lane and starts the lane when it was idle.
func (l *lanes) add(ctx context.Context, key string, id int64, run func(context.Context, int64)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.queued[key][id] {
		return
	}
	if l.queued[key] == nil {
		l.queued[key] = map[int64]bool{}
	}
	l.queued[key][id] = true
	idle := len(l.pending[key]) == 0
	l.pending[key] = append(l.pending[key], id)
	if idle {
		l.wg.Add(1)
		go l.drain(ctx, key, run)
	}
}

// drain runs the lane's jobs in order until it is empty.
func (l *lanes) drain(ctx context.Context, key string, run func(context.Context, int64)) {
	defer l.wg.Done()
	for {
		l.mu.Lock()
		if len(l.pending[key]) == 0 {
			delete(l.pending, key)
			delete(l.queued, key)
			l.mu.Unlock()
			return
		}
		id := l.pending[key][0]
		delete(l.queued[key], id)
		l.mu.Unlock()
		if ctx.Err() == nil {
			run(ctx, id)
		}
		l.mu.Lock()
		l.pending[key] = l.pending[key][1:]
		l.mu.Unlock()
	}
}

// wait blocks until every lane is idle.
func (l *lanes) wait() { l.wg.Wait() }
