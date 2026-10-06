package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sync/atomic"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/introspection"
	"github.com/vektah/gqlparser/v2/ast"
)

// executor holds per-operation state. It mirrors the structure of the code
// gqlgen generates (FieldSet, ResolveField, MarshalSliceConcurrently) so error
// paths, null propagation, middlewares and concurrency behave identically.
type executor struct {
	engine *Engine
	oc     *graphql.OperationContext
}

// root executes the selection set of a root operation type.
func (x *executor) root(ctx context.Context, def *ast.Definition, sel ast.SelectionSet, serial bool) graphql.Marshaler {
	fields := graphql.CollectFields(x.oc, sel, x.engine.implementors[def.Name])
	ctx = graphql.WithFieldContext(ctx, &graphql.FieldContext{Object: def.Name})

	out := graphql.NewFieldSet(fields)
	for i, field := range fields {
		if field.Name == "__typename" {
			out.Values[i] = graphql.MarshalString(def.Name)
			continue
		}
		innerCtx := graphql.WithRootFieldContext(ctx, &graphql.RootFieldContext{
			Object: field.Name,
			Field:  field,
		})
		inner := func(ctx context.Context) (res graphql.Marshaler) {
			defer func() {
				if r := recover(); r != nil {
					x.oc.Error(ctx, x.oc.Recover(ctx, r))
					res = graphql.Null
				}
			}()
			res = x.field(ctx, def, field, nil)
			if x.invalid(def, field, res) {
				atomic.AddUint32(&out.Invalids, 1)
			}
			return res
		}
		run := func(ctx context.Context) graphql.Marshaler {
			return x.oc.RootResolverMiddleware(innerCtx, inner)
		}
		if serial {
			out.Values[i] = run(innerCtx)
		} else {
			out.Concurrently(i, run)
		}
	}
	out.Dispatch(ctx)
	if atomic.LoadUint32(&out.Invalids) > 0 {
		return graphql.Null
	}
	return out
}

// subscription starts the single subscription stream of the operation.
func (x *executor) subscription(ctx context.Context, sel ast.SelectionSet) func(context.Context) graphql.Marshaler {
	def := x.engine.schema.Subscription
	fields := graphql.CollectFields(x.oc, sel, x.engine.implementors[def.Name])
	ctx = graphql.WithFieldContext(ctx, &graphql.FieldContext{Object: def.Name})
	if len(fields) != 1 {
		graphql.AddErrorf(ctx, "must subscribe to exactly one stream")
		return nil
	}
	field := fields[0]
	fd := x.fieldDefinition(def, field)
	stream := x.engine.streams[def.Name+"."+field.Name]
	if fd == nil || stream == nil {
		graphql.AddErrorf(ctx, "no stream resolver for %s.%s", def.Name, field.Name)
		return nil
	}
	return graphql.ResolveFieldStream(
		ctx, x.oc, field,
		x.fieldContextInit(def, fd, true),
		func(ctx context.Context) (any, error) {
			fc := graphql.GetFieldContext(ctx)
			ch, err := stream(ctx, Params{Args: fc.Args, Object: def, Field: fd})
			if err != nil {
				return nil, err
			}
			return ch, nil
		},
		nil,
		func(ctx context.Context, sel ast.SelectionSet, v any) graphql.Marshaler {
			return x.complete(ctx, fd.Type, sel, v)
		},
		true,
		fd.Type.NonNull,
	)
}

// object completes an object value against a selection set.
func (x *executor) object(ctx context.Context, def *ast.Definition, sel ast.SelectionSet, obj any) graphql.Marshaler {
	fields := graphql.CollectFields(x.oc, sel, x.engine.implementors[def.Name])

	out := graphql.NewFieldSet(fields)
	for i, field := range fields {
		if field.Name == "__typename" {
			out.Values[i] = graphql.MarshalString(def.Name)
			continue
		}
		resolve := func(ctx context.Context) graphql.Marshaler {
			res := x.field(ctx, def, field, obj)
			if x.invalid(def, field, res) {
				atomic.AddUint32(&out.Invalids, 1)
			}
			return res
		}
		// Like gqlgen: fields with a user resolver run concurrently, plain
		// property reads run inline.
		if _, ok := x.engine.resolvers[def.Name+"."+field.Name]; ok {
			out.Concurrently(i, resolve)
		} else {
			out.Values[i] = resolve(ctx)
		}
	}
	out.Dispatch(ctx)
	if atomic.LoadUint32(&out.Invalids) > 0 {
		return graphql.Null
	}
	return out
}

// invalid reports whether a completed field value must null its parent.
func (x *executor) invalid(def *ast.Definition, field graphql.CollectedField, res graphql.Marshaler) bool {
	if res == graphql.RequiredNull {
		return true
	}
	fd := x.fieldDefinition(def, field)
	return fd != nil && fd.Type.NonNull && res == graphql.Null
}

func (x *executor) fieldDefinition(def *ast.Definition, field graphql.CollectedField) *ast.FieldDefinition {
	if fd := def.Fields.ForName(field.Name); fd != nil {
		return fd
	}
	return field.Definition
}

// field resolves and completes one field of def.
func (x *executor) field(ctx context.Context, def *ast.Definition, field graphql.CollectedField, parent any) graphql.Marshaler {
	fd := x.fieldDefinition(def, field)
	if fd == nil {
		graphql.AddErrorf(ctx, "unknown field %s.%s", def.Name, field.Name)
		return graphql.Null
	}
	resolver := x.resolverFor(def, field.Name)
	return graphql.ResolveField(
		ctx, x.oc, field,
		x.fieldContextInit(def, fd, resolver != nil),
		func(ctx context.Context) (any, error) {
			fc := graphql.GetFieldContext(ctx)
			var (
				v   any
				err error
			)
			if resolver != nil {
				v, err = resolver(ctx, Params{Parent: parent, Args: fc.Args, Object: def, Field: fd})
			} else {
				v, err = defaultResolve(parent, fd, fc.Args)
			}
			if err != nil {
				return nil, err
			}
			return normalizeNil(v, fd.Type), nil
		},
		nil,
		func(ctx context.Context, sel ast.SelectionSet, v any) graphql.Marshaler {
			return x.complete(ctx, fd.Type, sel, v)
		},
		true,
		fd.Type.NonNull,
	)
}

// resolverFor returns the registered resolver or a built-in one (introspection
// root fields); nil means "read the property from the parent value".
func (x *executor) resolverFor(def *ast.Definition, name string) ResolverFunc {
	if fn, ok := x.engine.resolvers[def.Name+"."+name]; ok {
		return fn
	}
	if def == x.engine.schema.Query {
		switch name {
		case "__schema":
			return func(context.Context, Params) (any, error) {
				if x.oc.DisableIntrospection {
					return nil, fmt.Errorf("introspection disabled")
				}
				return introspection.WrapSchema(x.engine.schema), nil
			}
		case "__type":
			return func(_ context.Context, p Params) (any, error) {
				if x.oc.DisableIntrospection {
					return nil, fmt.Errorf("introspection disabled")
				}
				name, _ := p.Args["name"].(string)
				return introspection.WrapTypeFromDef(x.engine.schema, x.engine.schema.Types[name]), nil
			}
		}
	}
	return nil
}

// fieldContextInit builds the gqlgen FieldContext for a field and coerces its
// arguments, reporting coercion errors on the field path.
func (x *executor) fieldContextInit(def *ast.Definition, fd *ast.FieldDefinition, isResolver bool) func(context.Context, graphql.CollectedField) (*graphql.FieldContext, error) {
	return func(ctx context.Context, field graphql.CollectedField) (*graphql.FieldContext, error) {
		fc := &graphql.FieldContext{
			Object:     def.Name,
			Field:      field,
			IsMethod:   isResolver,
			IsResolver: isResolver,
		}
		fc.Child = func(ctx context.Context, child graphql.CollectedField) (*graphql.FieldContext, error) {
			childDef := x.engine.schema.Types[fd.Type.Name()]
			if childDef == nil {
				return nil, fmt.Errorf("field of type %s does not have child fields", fd.Type.Name())
			}
			childFd := x.fieldDefinition(childDef, child)
			if childFd == nil {
				return nil, fmt.Errorf("no field named %q was found under type %s", child.Name, childDef.Name)
			}
			_, isRes := x.engine.resolvers[childDef.Name+"."+child.Name]
			return x.fieldContextInit(childDef, childFd, isRes)(ctx, child)
		}
		if len(fd.Arguments) == 0 {
			return fc, nil
		}
		ctx = graphql.WithFieldContext(ctx, fc)
		args, err := x.engine.coerceArgs(ctx, fd, field, x.oc.Variables)
		if err != nil {
			x.oc.Error(ctx, err)
			return fc, err
		}
		fc.Args = args
		return fc, nil
	}
}

// complete serialises a resolved value according to its schema type.
func (x *executor) complete(ctx context.Context, typ *ast.Type, sel ast.SelectionSet, v any) graphql.Marshaler {
	v = normalizeNil(v, typ)
	if v == nil {
		if typ.NonNull && !graphql.HasFieldError(ctx, graphql.GetFieldContext(ctx)) {
			graphql.AddErrorf(ctx, "the requested element is null which the schema does not allow")
		}
		return graphql.Null
	}

	if typ.Elem != nil {
		return x.list(ctx, typ, sel, v)
	}

	def := x.engine.schema.Types[typ.NamedType]
	if def == nil {
		graphql.AddErrorf(ctx, "unknown type %s", typ.NamedType)
		return graphql.Null
	}
	switch def.Kind {
	case ast.Scalar:
		return x.scalar(ctx, def, typ, v)
	case ast.Enum:
		return x.enum(ctx, def, typ, v)
	case ast.Object:
		return x.object(ctx, def, sel, v)
	case ast.Interface, ast.Union:
		concrete := x.concreteType(def, v)
		if concrete == nil {
			graphql.AddErrorf(ctx, "cannot determine concrete type of abstract type %s (return a map with \"__typename\")", def.Name)
			return graphql.Null
		}
		return x.object(ctx, concrete, sel, v)
	default:
		graphql.AddErrorf(ctx, "type %s cannot be used as an output type", def.Name)
		return graphql.Null
	}
}

func (x *executor) list(ctx context.Context, typ *ast.Type, sel ast.SelectionSet, v any) graphql.Marshaler {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		graphql.AddErrorf(ctx, "expected a list for type %s, got %T", typ.String(), v)
		return graphql.Null
	}

	var ret graphql.Array
	elemDef := x.engine.schema.Types[typ.Elem.Name()]
	if typ.Elem.Elem == nil && elemDef != nil && (elemDef.Kind == ast.Scalar || elemDef.Kind == ast.Enum) {
		// Leaf lists are completed inline, like gqlgen's generated marshalers.
		ret = make(graphql.Array, rv.Len())
		for i := range ret {
			ret[i] = x.complete(ctx, typ.Elem, sel, rv.Index(i).Interface())
		}
	} else {
		ret = graphql.MarshalSliceConcurrently(ctx, rv.Len(), 0, false, func(ctx context.Context, i int) graphql.Marshaler {
			el := rv.Index(i).Interface()
			graphql.GetFieldContext(ctx).Result = el
			return x.complete(ctx, typ.Elem, sel, el)
		})
	}

	if typ.Elem.NonNull {
		for _, m := range ret {
			if m == graphql.Null {
				return graphql.Null
			}
		}
	}
	return ret
}

func (x *executor) enum(ctx context.Context, def *ast.Definition, typ *ast.Type, v any) graphql.Marshaler {
	s := fmt.Sprint(deref(v))
	if def.EnumValues.ForName(s) == nil {
		graphql.AddErrorf(ctx, "%q is not a valid value for enum %s", s, def.Name)
		return x.nullFor(ctx, typ)
	}
	return graphql.MarshalString(s)
}

func (x *executor) scalar(ctx context.Context, def *ast.Definition, typ *ast.Type, v any) graphql.Marshaler {
	m, err := x.engine.marshalScalar(ctx, def.Name, deref(v))
	if err != nil {
		graphql.AddError(ctx, graphql.ErrorOnPath(ctx, err))
		return x.nullFor(ctx, typ)
	}
	if m == graphql.Null {
		return x.nullFor(ctx, typ)
	}
	return m
}

// nullFor returns Null, recording the non-null violation like gqlgen does.
func (x *executor) nullFor(ctx context.Context, typ *ast.Type) graphql.Marshaler {
	if typ.NonNull && !graphql.HasFieldError(ctx, graphql.GetFieldContext(ctx)) {
		graphql.AddErrorf(ctx, "the requested element is null which the schema does not allow")
	}
	return graphql.Null
}

// concreteType resolves the object type of a value for an interface or union.
func (x *executor) concreteType(abstract *ast.Definition, v any) *ast.Definition {
	var name string
	if m, ok := v.(map[string]any); ok {
		name, _ = m["__typename"].(string)
	}
	if name == "" {
		return nil
	}
	concrete := x.engine.schema.Types[name]
	if concrete == nil || concrete.Kind != ast.Object {
		return nil
	}
	for _, p := range x.engine.schema.GetPossibleTypes(abstract) {
		if p.Name == concrete.Name {
			return concrete
		}
	}
	return nil
}

// marshalScalar serialises a scalar value using gqlgen's built-in marshalers so
// the wire format is byte-identical to the generated engine.
func (e *Engine) marshalScalar(ctx context.Context, name string, v any) (graphql.Marshaler, error) {
	if codec, ok := e.scalars[name]; ok && codec.Marshal != nil {
		return codec.Marshal(v)
	}
	switch name {
	case "Int":
		i, err := toInt(v)
		if err != nil {
			return nil, err
		}
		return graphql.MarshalInt(i), nil
	case "Float":
		f, err := toFloat(v)
		if err != nil {
			return nil, err
		}
		return graphql.WrapContextMarshaler(ctx, graphql.MarshalFloatContext(f)), nil
	case "String":
		s, ok := v.(string)
		if !ok {
			s = fmt.Sprint(v)
		}
		return graphql.MarshalString(s), nil
	case "ID":
		return graphql.MarshalID(fmt.Sprint(v)), nil
	case "Boolean":
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("Boolean cannot represent %T", v)
		}
		return graphql.MarshalBoolean(b), nil
	default:
		// Map and unknown custom scalars: raw JSON passthrough.
		b, err := json.Marshal(v)
		if err != nil {
			return graphql.Null, nil
		}
		return graphql.WriterFunc(func(w io.Writer) { _, _ = w.Write(b) }), nil
	}
}
