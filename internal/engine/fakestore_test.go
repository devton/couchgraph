package engine_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/devton/couchgraph/internal/couch"
)

// fakeStore is a deterministic in-memory couch.Store. It records every call
// (method + options as JSON) so tests can assert that both engines pass
// identical, correctly coerced inputs to the repository.
type fakeStore struct {
	mu      sync.Mutex
	docs    map[string]map[string]any
	calls   []string
	changes []couch.ChangeEvent
	seq     int
}

func newFakeStore() *fakeStore {
	docs := map[string]map[string]any{
		"0190a1b2-0000-7000-8000-000000000001": {
			"_id": "0190a1b2-0000-7000-8000-000000000001", "_rev": "1-a",
			"type": "movie", "title": "The Matrix", "year": float64(1999), "rating": 8.7,
			"genres": []any{"Sci-Fi", "Action"}, "director_id": "0190a1b2-0000-7000-8000-0000000000d1",
		},
		"0190a1b2-0000-7000-8000-000000000002": {
			"_id": "0190a1b2-0000-7000-8000-000000000002", "_rev": "3-b",
			"type": "movie", "title": "Parasite", "year": float64(2019), "rating": 8.5,
			"genres": []any{"Drama"}, "nested": map[string]any{"z": 1.5, "a": []any{true, nil, "x"}},
		},
		"0190a1b2-0000-7000-8000-0000000000d1": {
			"_id": "0190a1b2-0000-7000-8000-0000000000d1", "_rev": "2-c",
			"type": "director", "name": "Lana Wachowski",
		},
	}
	return &fakeStore{
		docs: docs,
		changes: []couch.ChangeEvent{
			{ID: "0190a1b2-0000-7000-8000-000000000001", Seq: "1-x", Doc: docs["0190a1b2-0000-7000-8000-000000000001"]},
			{ID: "0190a1b2-0000-7000-8000-000000000002", Seq: "2-x", Deleted: true},
			{ID: "0190a1b2-0000-7000-8000-0000000000d1", Seq: "3-x", Doc: docs["0190a1b2-0000-7000-8000-0000000000d1"]},
		},
	}
}

const failID = "fail"

func (s *fakeStore) record(method string, args ...any) {
	b, _ := json.Marshal(args)
	s.mu.Lock()
	s.calls = append(s.calls, method+" "+string(b))
	s.mu.Unlock()
}

func (s *fakeStore) Calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := append([]string(nil), s.calls...)
	sort.Strings(out) // root query fields run concurrently
	return out
}

func (s *fakeStore) Get(_ context.Context, id string) (map[string]any, error) {
	s.record("Get", id)
	return s.docs[id], nil
}

func (s *fakeStore) BulkGet(_ context.Context, ids []string) ([]map[string]any, error) {
	s.record("BulkGet", ids)
	out := make([]map[string]any, len(ids))
	for i, id := range ids {
		if id == failID {
			return nil, errors.New("couch: bulk_get failed")
		}
		out[i] = s.docs[id]
	}
	return out, nil
}

func (s *fakeStore) Upsert(_ context.Context, id, rev string, data map[string]any) (string, string, error) {
	s.record("Upsert", id, rev, data)
	if rev == "conflict" {
		return "", "", errors.New("couch: document update conflict")
	}
	if id == "" {
		s.mu.Lock()
		s.seq++
		id = fmt.Sprintf("0190a1b2-0000-7000-8000-%012d", 900+s.seq)
		s.mu.Unlock()
	}
	return id, "1-new", nil
}

func (s *fakeStore) Delete(_ context.Context, id, rev string) (string, error) {
	s.record("Delete", id, rev)
	return "2-deleted", nil
}

func (s *fakeStore) BulkDocs(_ context.Context, docs []map[string]any) ([]map[string]any, error) {
	s.record("BulkDocs", docs)
	out := make([]map[string]any, len(docs))
	for i, d := range docs {
		id, _ := d["_id"].(string)
		if id == "" {
			id = fmt.Sprintf("bulk-%d", i)
		}
		out[i] = map[string]any{"ok": true, "_id": id, "_rev": "1-bulk"}
	}
	return out, nil
}

func (s *fakeStore) Find(_ context.Context, opts couch.FindOptions) (*couch.FindResult, error) {
	s.record("Find", opts)
	if opts.Selector["type"] == failID {
		return nil, errors.New("couch: invalid selector")
	}
	ids := make([]string, 0, len(s.docs))
	for id := range s.docs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	res := &couch.FindResult{}
	for _, id := range ids {
		doc := s.docs[id]
		if t, ok := opts.Selector["type"].(string); ok && doc["type"] != t {
			continue
		}
		res.Docs = append(res.Docs, doc)
		if opts.Limit > 0 && len(res.Docs) == opts.Limit {
			break
		}
	}
	if len(res.Docs) > 0 {
		res.Bookmark = "bm-" + strings.Repeat("x", len(res.Docs))
	} else {
		res.Warning = "No matching index found, create an index to optimize query time."
	}
	return res, nil
}

func (s *fakeStore) QueryView(_ context.Context, opts couch.ViewOptions) (*couch.ViewResult, error) {
	s.record("QueryView", opts)
	if opts.Reduce != nil && *opts.Reduce {
		return &couch.ViewResult{
			Rows: []couch.ViewRow{
				{Key: "Drama", Value: float64(1)},
				{Key: "Sci-Fi", Value: map[string]any{"sum": 17.2, "count": float64(2)}},
			},
			TotalRows: 2,
		}, nil
	}
	row := couch.ViewRow{ID: "0190a1b2-0000-7000-8000-000000000001", Key: []any{"Sci-Fi", float64(1999)}, Value: nil}
	if opts.IncludeDocs {
		row.Doc = s.docs[row.ID]
	}
	return &couch.ViewResult{Rows: []couch.ViewRow{row}, TotalRows: 42, Offset: 7}, nil
}

func (s *fakeStore) Databases(context.Context) ([]string, error) {
	s.record("Databases")
	return []string{"_users", "couchgraph", "couchgraph_movies"}, nil
}

func (s *fakeStore) ServerInfo(context.Context) (map[string]any, error) {
	s.record("ServerInfo")
	return map[string]any{"couchdb": "Welcome", "version": "3.5.1", "features": []any{"nouveau", "partitioned"}}, nil
}

func (s *fakeStore) SubscribeChanges(ctx context.Context, since string) (<-chan couch.ChangeEvent, error) {
	s.record("SubscribeChanges", since)
	ch := make(chan couch.ChangeEvent)
	go func() {
		defer close(ch)
		for _, ev := range s.changes {
			select {
			case <-ctx.Done():
				return
			case ch <- ev:
			}
		}
	}()
	return ch, nil
}

var _ couch.Store = (*fakeStore)(nil)
