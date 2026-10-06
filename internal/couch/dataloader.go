package couch

import (
	"context"
	"sync"
)

// Loader batches document fetches within a single GraphQL request execution
// using CouchDB's _bulk_get endpoint, eliminating N+1 query problems.
//
// Usage:
//  1. Call loader.Load(id) from each resolver (returns a future channel).
//  2. Call loader.Dispatch(ctx) once after all Load() calls are registered.
//  3. Each Load() channel receives a LoadResult with the resolved document.
//
// A new Loader must be created per-request (not shared across requests).
type Loader struct {
	repo BulkGetter

	mu    sync.Mutex
	batch []loadRequest
	once  sync.Once
	done  chan struct{} // closed when Dispatch completes
}

type loadRequest struct {
	id     string
	result chan LoadResult
}

// LoadResult is the result of a single document load. It is exported so
// callers of Load() can receive and inspect it directly.
type LoadResult struct {
	Doc map[string]any
	Err error
}

// NewLoader creates a per-request document loader.
func NewLoader(repo BulkGetter) *Loader {
	return &Loader{
		repo: repo,
		done: make(chan struct{}),
	}
}

// Load schedules a document fetch by ID and returns a channel that will
// receive a LoadResult once Dispatch is called.
func (l *Loader) Load(id string) <-chan LoadResult {
	ch := make(chan LoadResult, 1)

	l.mu.Lock()
	l.batch = append(l.batch, loadRequest{id: id, result: ch})
	l.mu.Unlock()

	return ch
}

// LoadAndWait is a convenience wrapper that loads a single document and blocks
// until the result is available. It calls Dispatch internally, so it should
// only be used for single-document queries where batching is not needed.
func (l *Loader) LoadAndWait(ctx context.Context, id string) (map[string]any, error) {
	ch := l.Load(id)
	l.Dispatch(ctx)
	res := <-ch
	return res.Doc, res.Err
}

// Dispatch executes the batch fetch exactly once. Subsequent calls are no-ops.
// It is safe to call Dispatch from multiple goroutines concurrently.
func (l *Loader) Dispatch(ctx context.Context) {
	l.once.Do(func() {
		defer close(l.done)

		l.mu.Lock()
		batch := l.batch
		l.batch = nil
		l.mu.Unlock()

		if len(batch) == 0 {
			return
		}

		// Deduplicate IDs while preserving the order for result mapping.
		ids, idIdx := deduplicateIDs(batch)

		docs, err := l.repo.BulkGet(ctx, ids)

		// Fan out results to each waiting channel.
		for i, id := range ids {
			var result LoadResult
			if err != nil {
				result.Err = err
			} else {
				result.Doc = docs[i]
			}
			for _, req := range idIdx[id] {
				req.result <- result
			}
		}
	})
}

// deduplicateIDs returns a deduplicated slice of IDs and a map from each
// unique ID to all requests that asked for it.
func deduplicateIDs(batch []loadRequest) ([]string, map[string][]loadRequest) {
	seen := make(map[string]bool, len(batch))
	ids := make([]string, 0, len(batch))
	idx := make(map[string][]loadRequest, len(batch))

	for _, req := range batch {
		if !seen[req.id] {
			seen[req.id] = true
			ids = append(ids, req.id)
		}
		idx[req.id] = append(idx[req.id], req)
	}

	return ids, idx
}
