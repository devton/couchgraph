package runtime_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devton/couchgraph/internal/couch"
	"github.com/devton/couchgraph/internal/engine/runtime"
)

type mockStore struct {
	couch.Store
	docs map[string]map[string]any
}

func (m *mockStore) Get(_ context.Context, id string) (map[string]any, error) {
	return m.docs[id], nil
}

func TestTranspileTypeScript(t *testing.T) {
	tsCode := `
interface Movie {
  title: string;
  rating: number;
}

export default function(ctx: any): Movie {
  return {
    title: "Inception",
    rating: 8.8
  };
}
`
	js, err := runtime.Transpile(tsCode, "test.ts")
	if err != nil {
		t.Fatalf("transpile error: %v", err)
	}
	if strings.Contains(js, "interface") {
		t.Errorf("expected interface to be stripped by esbuild, got: %s", js)
	}
	if !strings.Contains(js, "exports.default") && !strings.Contains(js, "default:") {
		t.Errorf("expected export in output, got: %s", js)
	}
}

func TestRuntimeExecution(t *testing.T) {
	tmp := t.TempDir()

	tsFile := filepath.Join(tmp, "calc.ts")
	err := os.WriteFile(tsFile, []byte(`
export default function(ctx: any) {
  const a: number = ctx.args.a;
  const b: number = ctx.args.b;
  return a + b;
}
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	rt := runtime.New(tmp, nil)
	script, err := rt.Load("calc.ts", "default")
	if err != nil {
		t.Fatalf("load script error: %v", err)
	}

	res, err := rt.Run(context.Background(), script, runtime.Context{
		Args: map[string]any{"a": 10, "b": 32},
	})
	if err != nil {
		t.Fatalf("run script error: %v", err)
	}

	if val, ok := res.(int64); !ok || val != 42 {
		if valFloat, ok := res.(float64); !ok || valFloat != 42 {
			t.Errorf("expected 42, got %v (%T)", res, res)
		}
	}
}

func TestRuntimeCouchBridge(t *testing.T) {
	tmp := t.TempDir()

	tsFile := filepath.Join(tmp, "movie_lookup.ts")
	err := os.WriteFile(tsFile, []byte(`
export default function(ctx: any) {
  const doc = ctx.couch.get(ctx.args.id);
  if (!doc) return null;
  return doc.title.toUpperCase();
}
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	store := &mockStore{
		docs: map[string]map[string]any{
			"m1": {"_id": "m1", "title": "The Matrix"},
		},
	}

	rt := runtime.New(tmp, store)
	script, err := rt.Load("movie_lookup.ts", "default")
	if err != nil {
		t.Fatalf("load script error: %v", err)
	}

	res, err := rt.Run(context.Background(), script, runtime.Context{
		Args: map[string]any{"id": "m1"},
	})
	if err != nil {
		t.Fatalf("run script error: %v", err)
	}

	if str, ok := res.(string); !ok || str != "THE MATRIX" {
		t.Errorf("expected 'THE MATRIX', got %v", res)
	}
}
