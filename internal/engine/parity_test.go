package engine_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/executor"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/devton/couchgraph/internal/auth"
	"github.com/devton/couchgraph/internal/config"
	"github.com/devton/couchgraph/internal/engine"
	"github.com/devton/couchgraph/internal/graph/generated"
	"github.com/devton/couchgraph/internal/graph/resolver"
)

// The parity suite runs identical operations through the gqlgen-generated
// schema and the dynamic engine and requires byte-identical responses and
// identical repository calls.

type engineFactory func(t testing.TB, store *fakeStore) graphql.ExecutableSchema

func gqlgenSchema(_ testing.TB, store *fakeStore) graphql.ExecutableSchema {
	return generated.NewExecutableSchema(generated.Config{Resolvers: &resolver.Resolver{Repo: store}})
}

func dynamicSchema(t testing.TB, store *fakeStore) graphql.ExecutableSchema {
	t.Helper()
	// The generated schema also contains the movies example; load it too so
	// introspection is comparable. Its fields are not exercised here (they
	// move to SDL directives in Step 2).
	movies, err := os.ReadFile("../graph/schema/movies.graphqls")
	if err != nil {
		t.Fatal(err)
	}
	e, err := engine.NewCore(store, &ast.Source{Name: "movies.graphqls", Input: string(movies)})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

type runOptions struct {
	readOnly bool
	maxResp  int
}

// run executes an operation like the HTTP handler does and returns every
// response of the stream, JSON-encoded.
func run(t *testing.T, es graphql.ExecutableSchema, query, variables string, opts runOptions) []string {
	t.Helper()
	exec := executor.New(es)
	exec.Use(extension.Introspection{})
	exec.AroundOperations(auth.MutationGuard(opts.readOnly, config.AuthConfig{}))

	var vars map[string]any
	if variables != "" {
		dec := json.NewDecoder(bytes.NewBufferString(variables))
		dec.UseNumber() // same as gqlgen's HTTP transports
		if err := dec.Decode(&vars); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = graphql.StartOperationTrace(ctx)
	now := graphql.Now()
	params := &graphql.RawParams{
		Query:     query,
		Variables: vars,
		ReadTime:  graphql.TraceTiming{Start: now, End: now},
	}

	encode := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	opCtx, errs := exec.CreateOperationContext(ctx, params)
	if errs != nil {
		return []string{encode(exec.DispatchError(graphql.WithOperationContext(ctx, opCtx), errs))}
	}
	handler, ctx := exec.DispatchOperation(ctx, opCtx)

	max := opts.maxResp
	if max == 0 {
		max = 20
	}
	var out []string
	for len(out) < max {
		resp := handler(ctx)
		if resp == nil {
			break
		}
		out = append(out, encode(resp))
	}
	return out
}

func assertParity(t *testing.T, query, variables string, opts runOptions) []string {
	t.Helper()
	gqlStore, dynStore := newFakeStore(), newFakeStore()
	want := run(t, gqlgenSchema(t, gqlStore), query, variables, opts)
	got := run(t, dynamicSchema(t, dynStore), query, variables, opts)

	if len(want) != len(got) {
		t.Fatalf("response count mismatch: gqlgen=%d dynamic=%d\ngqlgen:  %v\ndynamic: %v", len(want), len(got), want, got)
	}
	for i := range want {
		if want[i] != got[i] {
			t.Errorf("response %d mismatch\ngqlgen:  %s\ndynamic: %s", i, want[i], got[i])
			if dir := os.Getenv("PARITY_DUMP"); dir != "" {
				// PARITY_DUMP=/some/dir writes both responses for diffing.
				base := filepath.Join(dir, strings.ReplaceAll(t.Name(), "/", "_"))
				_ = os.WriteFile(base+".gqlgen.json", []byte(want[i]), 0o644)
				_ = os.WriteFile(base+".dynamic.json", []byte(got[i]), 0o644)
			}
		}
	}
	wantCalls, gotCalls := gqlStore.Calls(), dynStore.Calls()
	if len(wantCalls) != len(gotCalls) {
		t.Fatalf("store calls mismatch\ngqlgen:  %v\ndynamic: %v", wantCalls, gotCalls)
	}
	for i := range wantCalls {
		if wantCalls[i] != gotCalls[i] {
			t.Errorf("store call %d mismatch\ngqlgen:  %s\ndynamic: %s", i, wantCalls[i], gotCalls[i])
		}
	}
	return got
}

const (
	m1 = "0190a1b2-0000-7000-8000-000000000001"
	m2 = "0190a1b2-0000-7000-8000-000000000002"
	d1 = "0190a1b2-0000-7000-8000-0000000000d1"
)

func TestParityQueries(t *testing.T) {
	cases := []struct {
		name, query, variables string
	}{
		{"document", `{ document(id: "` + m1 + `") { __typename _id _rev data } }`, ""},
		{"document missing", `{ document(id: "nope") { _id } }`, ""},
		{"document alias and variables", `query Q($id: ID!) { a: document(id: $id) { id: _id } b: document(id: "` + d1 + `") { data } }`, `{"id":"` + m2 + `"}`},
		{"documents with gaps", `{ documents(ids: ["` + m1 + `", "nope", "` + d1 + `"]) { _id data } }`, ""},
		{"documents single value list coercion", `query Q($ids: [ID!]!) { documents(ids: $ids) { _id } }`, `{"ids":"` + m2 + `"}`},
		{"documents loader error", `{ documents(ids: ["` + m1 + `", "fail"]) { _id } }`, ""},
		{"document loader error", `{ document(id: "fail") { _id } }`, ""},
		{"findDocs literal", `{ findDocs(input: {selector: {type: "movie", rating: {$gte: 8.5}}, fields: ["title"], sort: [{rating: "desc"}], limit: 1, skip: 0}) { docs { _id data } bookmark warning } }`, ""},
		{"findDocs variables", `query F($in: FindInput!) { findDocs(input: $in) { docs { _id } bookmark warning } }`, `{"in":{"selector":{"type":"director","n":{"$gt":1.25}},"limit":10,"bookmark":"abc"}}`},
		{"findDocs empty", `{ findDocs(input: {selector: {type: "nothing"}}) { docs { _id } bookmark warning } }`, ""},
		{"findDocs bad selector", `{ findDocs(input: {selector: "oops"}) { docs { _id } } }`, ""},
		{"findDocs store error nulls data", `{ databases findDocs(input: {selector: {type: "fail"}}) { warning } }`, ""},
		{"queryView map", `{ queryView(input: {designDoc: "movies", viewName: "by_genre", key: "Drama", keys: ["Drama", 1999, null], startKey: ["a"], endKey: {x: 1}, limit: 5, skip: 1, descending: true, includeDocs: true, reduce: false}) { totalRows offset rows { id key value doc { _id data } } } }`, ""},
		{"queryView reduce", `query V($g: Int) { queryView(input: {designDoc: "movies", viewName: "box_office_by_genre", reduce: true, group: true, groupLevel: $g}) { totalRows offset rows { id key value doc { _id } } } }`, `{"g":2}`},
		{"server", `{ databases serverInfo __typename }`, ""},
		{"fragments and directives", `
			query Q($skip: Boolean!) {
				document(id: "` + m1 + `") { ...Doc ... on Document { rev: _rev } data @skip(if: $skip) }
				findDocs(input: {selector: {type: "movie"}}) { docs { ...Doc } bookmark @include(if: $skip) }
			}
			fragment Doc on Document { _id }`, `{"skip":true}`},
		{"type introspection", `{ __type(name: "ViewInput") { kind name description inputFields { name description defaultValue type { kind name ofType { kind name ofType { kind name } } } } } }`, ""},
		{"typename introspection", `{ __type(name: "Nope") { name } a: __type(name: "Map") { kind name specifiedByURL } }`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertParity(t, tc.query, tc.variables, runOptions{})
		})
	}
}

func TestParityMutations(t *testing.T) {
	cases := []struct {
		name, query, variables string
		readOnly               bool
	}{
		{"upsert new", `mutation { upsertDoc(input: {data: {type: "movie", title: "Dune", year: 2021, rating: 8.0, tags: ["a", 1]}}) { ok _id _rev } }`, "", false},
		{"upsert update variables", `mutation U($in: UpsertInput!) { upsertDoc(input: $in) { ok _id _rev } }`, `{"in":{"_id":"` + m1 + `","_rev":"1-a","data":{"title":"The Matrix Reloaded","year":2003}}}`, false},
		{"upsert conflict", `mutation { upsertDoc(input: {_id: "x", _rev: "conflict", data: {a: 1}}) { ok } }`, "", false},
		{"upsert bad data", `mutation U($d: Map!) { upsertDoc(input: {data: $d}) { ok } }`, `{"d":[1,2]}`, false},
		{"serial mutations", `mutation { a: upsertDoc(input: {data: {n: 1}}) { _id } b: upsertDoc(input: {data: {n: 2}}) { _id } c: deleteDoc(input: {_id: "` + m2 + `", _rev: "3-b"}) { ok _id _rev } }`, "", false},
		{"bulk", `mutation { bulkDocs(input: {docs: [{data: {a: 1}}, {_id: "k", _rev: "1-z", data: {b: true}}]}) { results { ok _id _rev } } }`, "", false},
		{"read-only guard", `mutation { deleteDoc(input: {_id: "a", _rev: "1"}) { ok } }`, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertParity(t, tc.query, tc.variables, runOptions{readOnly: tc.readOnly})
		})
	}
}

func TestParitySubscriptions(t *testing.T) {
	cases := []struct {
		name, query string
		wantResp    int
	}{
		{"all changes", `subscription { docChanges { id seq deleted doc { _id data } } }`, 3},
		{"filtered", `subscription { docChanges(docIds: ["` + d1 + `"]) { __typename id doc { _id } } }`, 1},
		{"invalid two streams", `subscription { a: docChanges { id } b: docChanges { seq } }`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := assertParity(t, tc.query, "", runOptions{})
			if len(got) != tc.wantResp {
				t.Fatalf("expected %d responses, got %d: %v", tc.wantResp, len(got), got)
			}
		})
	}
}

func TestParityValidationErrors(t *testing.T) {
	assertParity(t, `{ document(id: 1.5) { nope } }`, "", runOptions{})
	assertParity(t, `query Q($n: Int) { queryView(input: {designDoc: "a", viewName: "b", limit: $n}) { offset } }`, `{"n":"x"}`, runOptions{})
}

func TestCoreCheck(t *testing.T) {
	e, err := engine.NewCore(newFakeStore())
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Check(); err != nil {
		t.Fatalf("core engine should have every root field wired: %v", err)
	}

	e.Resolve("Query.nope", func(context.Context, engine.Params) (any, error) { return nil, nil })
	if err := e.Check(); err == nil {
		t.Fatal("expected Check to reject a resolver for an unknown field")
	}
}

const introspectionQuery = `
query IntrospectionQuery {
  __schema {
    description
    queryType { name }
    mutationType { name }
    subscriptionType { name }
    types { ...FullType }
    directives { name description isRepeatable locations args { ...InputValue } }
  }
}
fragment FullType on __Type {
  kind name description specifiedByURL
  fields(includeDeprecated: true) {
    name description
    args { ...InputValue }
    type { ...TypeRef }
    isDeprecated deprecationReason
  }
  inputFields { ...InputValue }
  interfaces { ...TypeRef }
  enumValues(includeDeprecated: true) { name description isDeprecated deprecationReason }
  possibleTypes { ...TypeRef }
}
fragment InputValue on __InputValue { name description type { ...TypeRef } defaultValue }
fragment TypeRef on __Type {
  kind name
  ofType { kind name ofType { kind name ofType { kind name ofType { kind name ofType { kind name ofType { kind name } } } } } }
}`

// federationArtifacts are added to the gqlgen schema by the federation plugin
// (gqlgen.yml `federation:` block). The dynamic engine does not implement
// Apollo Federation yet, so they are excluded from the introspection parity.
var federationArtifacts = map[string]bool{
	"FieldSet": true, "_Any": true, "_Service": true, "federation__Policy": true, "federation__Scope": true,
	"authenticated": true, "composeDirective": true, "computedRequires": true, "extends": true, "external": true,
	"inaccessible": true, "interfaceObject": true, "key": true, "link": true, "override": true, "policy": true,
	"provides": true, "requires": true, "requiresScopes": true, "shareable": true, "tag": true, "_service": true,
}

// stripFederation removes federation types, directives and Query._service
// from an introspection response and returns its canonical JSON encoding.
func stripFederation(t *testing.T, resp string) string {
	t.Helper()
	var doc map[string]any
	dec := json.NewDecoder(strings.NewReader(resp))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	schema := doc["data"].(map[string]any)["__schema"].(map[string]any)
	keep := func(items []any) []any {
		out := items[:0]
		for _, it := range items {
			if !federationArtifacts[it.(map[string]any)["name"].(string)] {
				out = append(out, it)
			}
		}
		return out
	}
	schema["types"] = keep(schema["types"].([]any))
	schema["directives"] = keep(schema["directives"].([]any))
	for _, typ := range schema["types"].([]any) {
		if typ := typ.(map[string]any); typ["name"] == "Query" {
			typ["fields"] = keep(typ["fields"].([]any))
		}
	}
	return canonical(t, doc)
}

func canonical(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParityIntrospection(t *testing.T) {
	canonicalize := func(t *testing.T, resp string) string {
		var doc any
		dec := json.NewDecoder(strings.NewReader(resp))
		dec.UseNumber()
		if err := dec.Decode(&doc); err != nil {
			t.Fatal(err)
		}
		return canonical(t, doc)
	}
	gqlStore, dynStore := newFakeStore(), newFakeStore()
	want := run(t, gqlgenSchema(t, gqlStore), introspectionQuery, "", runOptions{})
	got := run(t, dynamicSchema(t, dynStore), introspectionQuery, "", runOptions{})
	if len(want) != 1 || len(got) != 1 {
		t.Fatalf("expected one response each, got %d and %d", len(want), len(got))
	}
	if w, g := stripFederation(t, want[0]), canonicalize(t, got[0]); w != g {
		t.Errorf("introspection mismatch (federation excluded)\ngqlgen:  %.2000s\ndynamic: %.2000s", w, g)
	}
}
