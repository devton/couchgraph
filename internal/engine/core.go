package engine

import (
	"context"
	"fmt"

	"github.com/vektah/gqlparser/v2/ast"

	"github.com/devton/couchgraph/internal/couch"
	coreschema "github.com/devton/couchgraph/internal/graph/schema"
)

// CoreSource returns the embedded core SDL as a gqlparser source.
func CoreSource() *ast.Source {
	return &ast.Source{Name: coreschema.CoreFile, Input: coreschema.Core}
}

// NewCore builds an engine serving the agnostic core API (document, documents,
// findDocs, queryView, databases, serverInfo, upsertDoc, deleteDoc, bulkDocs,
// docChanges) backed by store. Extra SDL sources are merged with the core SDL.
func NewCore(store couch.Store, extra ...*ast.Source) (*Engine, error) {
	e, err := Load(append([]*ast.Source{CoreSource()}, extra...)...)
	if err != nil {
		return nil, err
	}
	RegisterCore(e, store)
	return e, nil
}

// RegisterCore wires the core root fields to store. Objects are returned as
// map[string]any shaped like the core SDL types and read by the default
// resolver.
func RegisterCore(e *Engine, store couch.Store) {
	// ── Queries ───────────────────────────────────────────────────────────
	e.Resolve("Query.document", func(ctx context.Context, p Params) (any, error) {
		id, _ := p.Args["id"].(string)
		doc, err := couch.NewLoader(store).LoadAndWait(ctx, id)
		if err != nil || doc == nil {
			return nil, err
		}
		return toDocument(doc), nil
	})

	e.Resolve("Query.documents", func(ctx context.Context, p Params) (any, error) {
		ids := stringList(p.Args["ids"])
		loader := couch.NewLoader(store)
		channels := make([]<-chan couch.LoadResult, len(ids))
		for i, id := range ids {
			channels[i] = loader.Load(id)
		}
		loader.Dispatch(ctx)
		docs := make([]any, len(ids))
		for i, ch := range channels {
			res := <-ch
			if res.Err != nil {
				return nil, res.Err
			}
			if res.Doc != nil {
				docs[i] = toDocument(res.Doc)
			}
		}
		return docs, nil
	})

	e.Resolve("Query.findDocs", func(ctx context.Context, p Params) (any, error) {
		input := object(p.Args["input"])
		selector, ok := input["selector"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("findDocs: 'selector' must be a JSON object")
		}
		opts := couch.FindOptions{
			Selector: selector,
			Fields:   stringList(input["fields"]),
			Limit:    intValue(input["limit"]),
			Skip:     intValue(input["skip"]),
		}
		if len(opts.Fields) == 0 {
			opts.Fields = nil
		}
		for _, s := range list(input["sort"]) {
			if m, ok := s.(map[string]any); ok {
				opts.Sort = append(opts.Sort, m)
			}
		}
		if b, ok := input["bookmark"].(string); ok {
			opts.Bookmark = b
		}

		res, err := store.Find(ctx, opts)
		if err != nil {
			return nil, err
		}
		docs := make([]any, 0, len(res.Docs))
		for _, raw := range res.Docs {
			docs = append(docs, toDocument(raw))
		}
		return map[string]any{
			"docs":     docs,
			"bookmark": nonEmpty(res.Bookmark),
			"warning":  nonEmpty(res.Warning),
		}, nil
	})

	e.Resolve("Query.queryView", func(ctx context.Context, p Params) (any, error) {
		input := object(p.Args["input"])
		designDoc, _ := input["designDoc"].(string)
		viewName, _ := input["viewName"].(string)
		opts := couch.ViewOptions{
			DesignDoc:   designDoc,
			ViewName:    viewName,
			Key:         input["key"],
			StartKey:    input["startKey"],
			EndKey:      input["endKey"],
			Limit:       intValue(input["limit"]),
			Skip:        intValue(input["skip"]),
			Descending:  boolValue(input["descending"]),
			IncludeDocs: boolValue(input["includeDocs"]),
			Reduce:      boolPtr(input["reduce"]),
			Group:       boolPtr(input["group"]),
			GroupLevel:  intPtr(input["groupLevel"]),
		}
		if keys, ok := input["keys"].([]any); ok {
			opts.Keys = keys
		}

		res, err := store.QueryView(ctx, opts)
		if err != nil {
			return nil, err
		}
		rows := make([]any, 0, len(res.Rows))
		for _, vr := range res.Rows {
			row := map[string]any{"id": nonEmpty(vr.ID), "key": vr.Key, "value": vr.Value, "doc": nil}
			if vr.Doc != nil {
				row["doc"] = toDocument(vr.Doc)
			}
			rows = append(rows, row)
		}
		return map[string]any{"rows": rows, "totalRows": res.TotalRows, "offset": res.Offset}, nil
	})

	e.Resolve("Query.databases", func(ctx context.Context, _ Params) (any, error) {
		return store.Databases(ctx)
	})

	e.Resolve("Query.serverInfo", func(ctx context.Context, _ Params) (any, error) {
		return store.ServerInfo(ctx)
	})

	// ── Mutations ─────────────────────────────────────────────────────────
	e.Resolve("Mutation.upsertDoc", func(ctx context.Context, p Params) (any, error) {
		input := object(p.Args["input"])
		data, ok := input["data"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("upsertDoc: 'data' must be a JSON object")
		}
		id, _ := input["_id"].(string)
		rev, _ := input["_rev"].(string)
		newID, newRev, err := store.Upsert(ctx, id, rev, data)
		if err != nil {
			return nil, err
		}
		return mutationResult(true, newID, newRev), nil
	})

	e.Resolve("Mutation.deleteDoc", func(ctx context.Context, p Params) (any, error) {
		input := object(p.Args["input"])
		id, _ := input["_id"].(string)
		rev, _ := input["_rev"].(string)
		newRev, err := store.Delete(ctx, id, rev)
		if err != nil {
			return nil, err
		}
		return mutationResult(true, id, newRev), nil
	})

	e.Resolve("Mutation.bulkDocs", func(ctx context.Context, p Params) (any, error) {
		input := object(p.Args["input"])
		entries := list(input["docs"])
		docs := make([]map[string]any, 0, len(entries))
		for _, entry := range entries {
			d := object(entry)
			data, ok := d["data"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("bulkDocs: each 'data' must be a JSON object")
			}
			doc := make(map[string]any, len(data)+2)
			for k, v := range data {
				doc[k] = v
			}
			if id, ok := d["_id"].(string); ok {
				doc["_id"] = id
			}
			if rev, ok := d["_rev"].(string); ok {
				doc["_rev"] = rev
			}
			docs = append(docs, doc)
		}

		results, err := store.BulkDocs(ctx, docs)
		if err != nil {
			return nil, err
		}
		out := make([]any, 0, len(results))
		for _, res := range results {
			ok, _ := res["ok"].(bool)
			id, _ := res["_id"].(string)
			rev, _ := res["_rev"].(string)
			out = append(out, mutationResult(ok, id, rev))
		}
		return map[string]any{"results": out}, nil
	})

	// ── Subscriptions ─────────────────────────────────────────────────────
	e.Subscribe("Subscription.docChanges", func(ctx context.Context, p Params) (<-chan any, error) {
		events, err := store.SubscribeChanges(ctx, "now")
		if err != nil {
			return nil, err
		}
		filter := make(map[string]bool)
		for _, id := range stringList(p.Args["docIds"]) {
			filter[id] = true
		}

		out := make(chan any, 16)
		go func() {
			defer close(out)
			for {
				select {
				case <-ctx.Done():
					return
				case event, ok := <-events:
					if !ok {
						return
					}
					if len(filter) > 0 && !filter[event.ID] {
						continue
					}
					change := map[string]any{
						"id":      event.ID,
						"seq":     event.Seq,
						"deleted": event.Deleted,
						"doc":     nil,
					}
					if event.Doc != nil {
						change["doc"] = toDocument(event.Doc)
					}
					select {
					case <-ctx.Done():
						return
					case out <- change:
					}
				}
			}
		}()
		return out, nil
	})
}

// toDocument shapes a raw CouchDB document as the core Document type.
func toDocument(raw map[string]any) map[string]any {
	id, _ := raw["_id"].(string)
	rev, _ := raw["_rev"].(string)
	data := make(map[string]any, len(raw))
	for k, v := range raw {
		if k != "_id" && k != "_rev" {
			data[k] = v
		}
	}
	return map[string]any{"_id": id, "_rev": rev, "data": data}
}

func mutationResult(ok bool, id, rev string) map[string]any {
	return map[string]any{"ok": ok, "_id": id, "_rev": rev}
}

// ── Coerced-argument accessors ─────────────────────────────────────────────

func object(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func list(v any) []any {
	l, _ := v.([]any)
	return l
}

func stringList(v any) []string {
	items := list(v)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func intValue(v any) int {
	i, _ := v.(int)
	return i
}

func intPtr(v any) *int {
	if i, ok := v.(int); ok {
		return &i
	}
	return nil
}

func boolValue(v any) bool {
	b, _ := v.(bool)
	return b
}

func boolPtr(v any) *bool {
	if b, ok := v.(bool); ok {
		return &b
	}
	return nil
}

// nonEmpty maps "" to null, matching the optional string fields of the core API.
func nonEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
