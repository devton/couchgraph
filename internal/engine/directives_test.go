package engine_test

import (
	"os"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"

	"github.com/devton/couchgraph/internal/engine"
)

// moviesEngine builds the engine for the examples/movies project, so the
// example is also covered by the test suite.
func moviesEngine(t *testing.T, store *fakeStore) *engine.Engine {
	t.Helper()
	sdl, err := os.ReadFile("../../examples/movies/schema/movies.graphqls")
	if err != nil {
		t.Fatal(err)
	}
	e, err := engine.Build(engine.Options{
		Store:   store,
		Core:    true,
		Sources: []*ast.Source{{Name: "movies.graphqls", Input: string(sdl)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestDirectivesMoviesExample(t *testing.T) {
	cases := []struct {
		name, query, want string
		wantCalls         []string
	}{
		{
			name:  "@find drops absent filters and adds the @collection discriminator",
			query: `{ movies { id title boxOffice } }`,
			want:  `{"data":{"movies":[{"id":"` + m1 + `","title":"The Matrix","boxOffice":null},{"id":"` + m2 + `","title":"Parasite","boxOffice":null}]}}`,
			wantCalls: []string{
				`Find [{"selector":{"type":"movie"},"limit":20}]`,
			},
		},
		{
			name:  "@find substitutes arguments, keeping their JSON types",
			query: `{ movies(genre: "Drama", minRating: 8.5, limit: 1) { title } }`,
			wantCalls: []string{
				`Find [{"selector":{"genres":{"$elemMatch":{"$eq":"Drama"}},"rating":{"$gte":8.5},"type":"movie"},"limit":1}]`,
			},
		},
		{
			name:  "@belongsTo across list items is batched into one _bulk_get",
			query: `{ movies { title director { name } } }`,
			want:  `{"data":{"movies":[{"title":"The Matrix","director":{"name":"Lana Wachowski"}},{"title":"Parasite","director":{"name":"Bong Joon-ho"}}]}}`,
		},
		{
			name:  "@get enforces the @collection type",
			query: `{ movie(id: "` + d1 + `") { title } director(id: "` + d1 + `") { name } }`,
			want:  `{"data":{"movie":null,"director":{"name":"Lana Wachowski"}}}`,
		},
		{
			name:  "@view with $arg key and includeDocs",
			query: `{ moviesByYear(year: 1999) { title } }`,
			want:  `{"data":{"moviesByYear":[{"title":"The Matrix"}]}}`,
			wantCalls: []string{
				`QueryView [{"DesignDoc":"movies","ViewName":"by_year","Key":1999,"Keys":null,"StartKey":null,"EndKey":null,"Limit":0,"Skip":0,"Descending":false,"IncludeDocs":true,"Reduce":false,"Group":null,"GroupLevel":null}]`,
			},
		},
		{
			name:  "non-null violations propagate like gqlgen",
			query: `{ movies { title director { birthYear } } }`,
			want:  `{"errors":[{"message":"must not be null","path":["movies",0,"director","birthYear"]}],"data":{"movies":[{"title":"The Matrix","director":null},{"title":"Parasite","director":{"birthYear":1969}}]}}`,
		},
		{
			name:  "@hasMany queries the view with the parent id",
			query: `{ director(id: "` + d1 + `") { movies { title } } }`,
			wantCalls: []string{
				`BulkGet [["` + d1 + `"]]`,
				`QueryView [{"DesignDoc":"movies","ViewName":"by_director_id","Key":"` + d1 + `","Keys":null,"StartKey":null,"EndKey":null,"Limit":0,"Skip":0,"Descending":false,"IncludeDocs":false,"Reduce":false,"Group":null,"GroupLevel":null}]`,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeStore()
			got := run(t, moviesEngine(t, store), tc.query, "", runOptions{})
			if tc.want != "" && (len(got) != 1 || got[0] != tc.want) {
				t.Errorf("response mismatch\nwant: %s\ngot:  %v", tc.want, got)
			}
			calls := store.Calls()
			if tc.wantCalls != nil && strings.Join(calls, "\n") != strings.Join(tc.wantCalls, "\n") {
				t.Errorf("store calls mismatch\nwant: %v\ngot:  %v", tc.wantCalls, calls)
			}
			if strings.Contains(tc.name, "batched") {
				bulk := 0
				for _, c := range calls {
					if strings.HasPrefix(c, "BulkGet") {
						bulk++
					}
				}
				if bulk != 1 {
					t.Errorf("expected exactly 1 BulkGet, got %d: %v", bulk, calls)
				}
			}
		})
	}
}

func TestDirectiveValidation(t *testing.T) {
	cases := []struct {
		name, sdl, wantErr string
	}{
		{
			"unknown placeholder",
			`extend type Query { xs(a: String): [Document!]! @find(selector: "{\"a\": \"$b\"}") }`,
			`unknown argument placeholder(s) $b`,
		},
		{
			"invalid selector JSON",
			`extend type Query { xs: [Document!]! @find(selector: "{nope") }`,
			`selector: must be a JSON object`,
		},
		{
			"invalid view name",
			`extend type Query { xs: [Document!]! @view(name: "by_genre") }`,
			`view must be "designDoc/viewName"`,
		},
		{
			"@get without id argument",
			`extend type Query { x(key: ID!): Document @get }`,
			`field has no "id" argument`,
		},
		{
			"two resolving directives",
			`extend type Query { x(id: ID!): Document @get @view(name: "a/b") }`,
			`only one of`,
		},
		{
			"unknown directive is a schema error",
			`extend type Query { x: Document @nope }`,
			`Undefined directive nope`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := engine.Build(engine.Options{
				Store:   newFakeStore(),
				Core:    true,
				Sources: []*ast.Source{{Name: "x.graphqls", Input: tc.sdl}},
			})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestLookupKey(t *testing.T) {
	doc := map[string]any{"_id": "x", "runtime_minutes": 1, "title": "t", "value": map[string]any{"min": 2}}
	for name, want := range map[string]any{"id": "x", "runtimeMinutes": 1, "title": "t", "missing": nil} {
		if got := engine.LookupKey(doc, name); got != want {
			t.Errorf("LookupKey(%q) = %v, want %v", name, got, want)
		}
	}
	if got := engine.LookupPath(doc, "value.min"); got != 2 {
		t.Errorf("LookupPath(value.min) = %v", got)
	}
}
