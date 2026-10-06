package engine_test

import (
	"context"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/executor"
)

// BenchmarkEngines compares execution overhead of the generated and dynamic
// engines on the same operation with an in-memory store (no network I/O).
//
//	go test ./internal/engine -bench Engines -benchmem -run '^$'
func BenchmarkEngines(b *testing.B) {
	const query = `{
		findDocs(input: {selector: {type: "movie"}, limit: 10}) { docs { _id _rev data } bookmark warning }
		queryView(input: {designDoc: "movies", viewName: "by_genre", includeDocs: true}) { totalRows rows { id key value doc { _id data } } }
		documents(ids: ["` + m1 + `", "` + m2 + `", "` + d1 + `"]) { _id data }
	}`
	for _, eng := range []struct {
		name    string
		factory engineFactory
	}{
		{"gqlgen", gqlgenSchema},
		{"dynamic", dynamicSchema},
	} {
		b.Run(eng.name, func(b *testing.B) {
			exec := executor.New(eng.factory(b, newFakeStore()))
			b.ReportAllocs()
			for b.Loop() {
				ctx := graphql.StartOperationTrace(context.Background())
				opCtx, errs := exec.CreateOperationContext(ctx, &graphql.RawParams{Query: query})
				if errs != nil {
					b.Fatal(errs)
				}
				handler, ctx := exec.DispatchOperation(ctx, opCtx)
				if resp := handler(ctx); resp == nil || len(resp.Errors) > 0 {
					b.Fatalf("unexpected response: %+v", resp)
				}
			}
		})
	}
}
