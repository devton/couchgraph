package couch

import (
	"context"
	"sync"
	"time"
)

// BatchLoader coalesces document loads issued concurrently (e.g. by sibling
// list items resolving a relation) into a single _bulk_get, and caches results
// for its lifetime. Create one per GraphQL operation; it is safe for
// concurrent use.
type BatchLoader struct {
	repo     BulkGetter
	wait     time.Duration
	maxBatch int

	mu    sync.Mutex
	cur   *loadBatch
	cache map[string]*loadBatch
}

type loadBatch struct {
	ids   []string
	index map[string]int
	once  sync.Once
	done  chan struct{}
	docs  []map[string]any
	err   error
}

// NewBatchLoader creates a loader that waits up to wait for more keys before
// dispatching (or dispatches immediately when maxBatch keys are queued).
func NewBatchLoader(repo BulkGetter, wait time.Duration, maxBatch int) *BatchLoader {
	if maxBatch <= 0 {
		maxBatch = 100
	}
	return &BatchLoader{repo: repo, wait: wait, maxBatch: maxBatch, cache: make(map[string]*loadBatch)}
}

// Load returns the document with the given ID, or nil if it does not exist.
func (l *BatchLoader) Load(ctx context.Context, id string) (map[string]any, error) {
	l.mu.Lock()
	b, ok := l.cache[id]
	if !ok {
		if l.cur == nil {
			l.cur = &loadBatch{index: make(map[string]int), done: make(chan struct{})}
			go l.dispatchAfter(ctx, l.cur)
		}
		b = l.cur
		b.index[id] = len(b.ids)
		b.ids = append(b.ids, id)
		l.cache[id] = b
		if len(b.ids) >= l.maxBatch {
			l.cur = nil
			go l.dispatch(ctx, b)
		}
	}
	l.mu.Unlock()

	select {
	case <-b.done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if b.err != nil {
		return nil, b.err
	}
	return b.docs[b.index[id]], nil
}

// LoadMany loads several documents concurrently (batched), preserving order.
func (l *BatchLoader) LoadMany(ctx context.Context, ids []string) ([]map[string]any, error) {
	out := make([]map[string]any, len(ids))
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i], errs[i] = l.Load(ctx, id)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (l *BatchLoader) dispatchAfter(ctx context.Context, b *loadBatch) {
	timer := time.NewTimer(l.wait)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
	l.mu.Lock()
	if l.cur == b {
		l.cur = nil
	}
	l.mu.Unlock()
	l.dispatch(ctx, b)
}

func (l *BatchLoader) dispatch(ctx context.Context, b *loadBatch) {
	b.once.Do(func() {
		defer close(b.done)
		l.mu.Lock()
		ids := append([]string(nil), b.ids...)
		l.mu.Unlock()
		b.docs, b.err = l.repo.BulkGet(ctx, ids)
		if b.err == nil && len(b.docs) != len(ids) {
			// Defensive: keep index alignment even if the store misbehaves.
			docs := make([]map[string]any, len(ids))
			copy(docs, b.docs)
			b.docs = docs
		}
	})
}
