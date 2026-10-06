// Package engine implements a dynamic GraphQL execution engine: a runtime
// implementation of gqlgen's graphql.ExecutableSchema driven by a parsed SDL
// (*ast.Schema) and a registry of resolvers keyed by "Type.field".
//
// It plugs into the regular gqlgen handler (handler.NewDefaultServer), so all
// transports, introspection, the playground, operation middlewares (mutation
// guard) and extensions keep working unchanged.
//
// See docs/rfc-001-dynamic-schema-engine.md.
package engine

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

// Params carries everything a resolver needs for one field invocation.
type Params struct {
	// Parent is the resolved value of the enclosing object (nil for root fields).
	Parent any
	// Args are the coerced field arguments (Int→int, Float→float64,
	// String/ID/Enum→string, Boolean→bool, lists→[]any, inputs→map[string]any).
	// Absent optional arguments without defaults are not present in the map.
	Args map[string]any
	// Object is the type that owns the field.
	Object *ast.Definition
	// Field is the schema definition of the field being resolved.
	Field *ast.FieldDefinition
}

// ResolverFunc resolves a field to a plain Go value. Objects are typically
// map[string]any (or structs), lists are slices, scalars are Go primitives.
type ResolverFunc func(ctx context.Context, p Params) (any, error)

// StreamFunc resolves a subscription field to a channel of events. Each event
// is completed against the selection set and emitted as one response. The
// channel must be closed when the stream ends or ctx is done.
type StreamFunc func(ctx context.Context, p Params) (<-chan any, error)

// ScalarCodec customises input coercion and output serialisation of a custom
// scalar. Scalars without a codec are passed through as raw JSON values.
type ScalarCodec struct {
	Marshal   func(v any) (graphql.Marshaler, error)
	Unmarshal func(v any) (any, error)
}

// Engine is a dynamic graphql.ExecutableSchema.
type Engine struct {
	schema       *ast.Schema
	resolvers    map[string]ResolverFunc
	streams      map[string]StreamFunc
	scalars      map[string]ScalarCodec
	implementors map[string][]string
	inline       map[string]bool
	scopes       []func(context.Context) context.Context
}

var _ graphql.ExecutableSchema = (*Engine)(nil)

// Load parses and validates SDL sources and returns an engine for them.
func Load(sources ...*ast.Source) (*Engine, error) {
	schema, err := gqlparser.LoadSchema(sources...)
	if err != nil {
		return nil, fmt.Errorf("engine: load schema: %w", err)
	}
	return New(schema), nil
}

// New creates an engine for an already parsed schema.
func New(schema *ast.Schema) *Engine {
	e := &Engine{
		schema:       schema,
		resolvers:    make(map[string]ResolverFunc),
		streams:      make(map[string]StreamFunc),
		scalars:      make(map[string]ScalarCodec),
		implementors: make(map[string][]string),
		inline:       make(map[string]bool),
	}
	// Precompute the "satisfies" list used by graphql.CollectFields for each
	// object type: the type itself, its interfaces and the unions containing it.
	for name, def := range schema.Types {
		if def.Kind != ast.Object {
			continue
		}
		sat := append([]string{name}, def.Interfaces...)
		for _, other := range schema.Types {
			if other.Kind == ast.Union {
				for _, member := range other.Types {
					if member == name {
						sat = append(sat, other.Name)
					}
				}
			}
		}
		e.implementors[name] = sat
	}
	return e
}

// Resolve registers a resolver for a field coordinate such as "Query.document".
func (e *Engine) Resolve(coord string, fn ResolverFunc) { e.resolvers[coord] = fn }

// ResolveProperty registers a cheap, non-blocking resolver (e.g. a renamed or
// nested property read). Unlike Resolve, it runs inline instead of in its own
// goroutine.
func (e *Engine) ResolveProperty(coord string, fn ResolverFunc) {
	e.resolvers[coord] = fn
	e.inline[coord] = true
}

// WithRequestScope registers a function that decorates the context of every
// operation before execution (e.g. to attach a per-request DataLoader).
func (e *Engine) WithRequestScope(fn func(context.Context) context.Context) {
	e.scopes = append(e.scopes, fn)
}

func (e *Engine) scope(ctx context.Context) context.Context {
	for _, fn := range e.scopes {
		ctx = fn(ctx)
	}
	return ctx
}

// Subscribe registers a stream resolver for a subscription field coordinate
// such as "Subscription.docChanges".
func (e *Engine) Subscribe(coord string, fn StreamFunc) { e.streams[coord] = fn }

// Scalar registers a codec for a custom scalar.
func (e *Engine) Scalar(name string, codec ScalarCodec) { e.scalars[name] = codec }

// Check verifies the registry against the schema: every registered coordinate
// must exist, and every root Query/Mutation/Subscription field must be backed
// by a resolver (root fields have no parent value to read from).
func (e *Engine) Check() error {
	var problems []string
	for coord := range e.resolvers {
		if e.fieldDef(coord) == nil {
			problems = append(problems, "resolver for unknown field "+coord)
		}
	}
	for coord := range e.streams {
		if e.fieldDef(coord) == nil {
			problems = append(problems, "stream for unknown field "+coord)
		}
	}
	for _, root := range []*ast.Definition{e.schema.Query, e.schema.Mutation} {
		if root == nil {
			continue
		}
		for _, f := range root.Fields {
			if strings.HasPrefix(f.Name, "__") {
				continue
			}
			if _, ok := e.resolvers[root.Name+"."+f.Name]; !ok {
				problems = append(problems, "missing resolver for root field "+root.Name+"."+f.Name)
			}
		}
	}
	if sub := e.schema.Subscription; sub != nil {
		for _, f := range sub.Fields {
			if _, ok := e.streams[sub.Name+"."+f.Name]; !ok {
				problems = append(problems, "missing stream for subscription field "+sub.Name+"."+f.Name)
			}
		}
	}
	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("engine: %s", strings.Join(problems, "; "))
}

func (e *Engine) fieldDef(coord string) *ast.FieldDefinition {
	typeName, fieldName, ok := strings.Cut(coord, ".")
	if !ok {
		return nil
	}
	def := e.schema.Types[typeName]
	if def == nil {
		return nil
	}
	return def.Fields.ForName(fieldName)
}

// Schema implements graphql.ExecutableSchema.
func (e *Engine) Schema() *ast.Schema { return e.schema }

// Complexity implements graphql.ExecutableSchema. Returning false makes gqlgen
// fall back to its default complexity calculation.
func (e *Engine) Complexity(_ context.Context, _, _ string, _ int, _ map[string]any) (int, bool) {
	return 0, false
}

// Exec implements graphql.ExecutableSchema.
func (e *Engine) Exec(ctx context.Context) graphql.ResponseHandler {
	oc := graphql.GetOperationContext(ctx)
	x := &executor{engine: e, oc: oc}

	switch oc.Operation.Operation {
	case ast.Query, ast.Mutation:
		root, serial := e.schema.Query, false
		if oc.Operation.Operation == ast.Mutation {
			// Spec: root mutation fields execute serially.
			root, serial = e.schema.Mutation, true
		}
		first := true
		return func(ctx context.Context) *graphql.Response {
			if !first {
				return nil
			}
			first = false
			ctx = e.scope(ctx)
			data := x.root(ctx, root, oc.Operation.SelectionSet, serial)
			var buf bytes.Buffer
			data.MarshalGQL(&buf)
			return &graphql.Response{Data: buf.Bytes()}
		}

	case ast.Subscription:
		ctx = e.scope(ctx)
		next := x.subscription(ctx, oc.Operation.SelectionSet)
		if next == nil {
			// Errors were recorded on ctx and are attached by the gqlgen executor.
			return graphql.OneShot(&graphql.Response{})
		}
		return func(ctx context.Context) *graphql.Response {
			data := next(ctx)
			if data == nil {
				return nil
			}
			var buf bytes.Buffer
			data.MarshalGQL(&buf)
			return &graphql.Response{Data: buf.Bytes()}
		}

	default:
		return graphql.OneShot(graphql.ErrorResponse(ctx, "unsupported GraphQL operation"))
	}
}
