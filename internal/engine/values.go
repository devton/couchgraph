package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/devton/couchgraph/internal/graph/scalar"
)

// ── Output helpers ─────────────────────────────────────────────────────────

// normalizeNil turns typed nils (nil pointers, maps, interfaces) into an
// untyped nil. A nil slice for a non-null list is kept so it completes to []
// (gqlgen semantics), while a nil slice for a nullable list becomes null.
func normalizeNil(v any, typ *ast.Type) any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice:
		if rv.IsNil() && !(typ.Elem != nil && typ.NonNull) {
			return nil
		}
	case reflect.Pointer, reflect.Map, reflect.Interface, reflect.Func, reflect.Chan:
		if rv.IsNil() {
			return nil
		}
	}
	return v
}

// deref follows pointers so scalars can be returned as *string, *int, etc.
func deref(v any) any {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if !rv.IsValid() {
		return nil
	}
	return rv.Interface()
}

func toInt(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case float64:
		if n != math.Trunc(n) {
			return 0, fmt.Errorf("Int cannot represent non-integer value %v", n)
		}
		return int(n), nil
	case json.Number:
		i, err := n.Int64()
		return int(i), err
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(rv.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int(rv.Uint()), nil
	case reflect.Float32:
		return toInt(rv.Float())
	}
	return 0, fmt.Errorf("Int cannot represent %T", v)
}

func toFloat(v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case json.Number:
		return n.Float64()
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Float32, reflect.Float64:
		return rv.Float(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(rv.Uint()), nil
	}
	return 0, fmt.Errorf("Float cannot represent %T", v)
}

// ── Default resolver ───────────────────────────────────────────────────────

// defaultResolve reads a field from the parent value when no resolver is
// registered: map key lookup for map[string]any, otherwise an exported method
// (called with the field arguments in declaration order) or struct field
// matched by name / json tag. This also serves gqlgen's introspection types.
func defaultResolve(parent any, fd *ast.FieldDefinition, args map[string]any) (any, error) {
	switch p := parent.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		return LookupKey(p, fd.Name), nil
	}

	rv := reflect.ValueOf(parent)
	if rv.Kind() != reflect.Pointer {
		// Make the value addressable so pointer-receiver methods are visible.
		ptr := reflect.New(rv.Type())
		ptr.Elem().Set(rv)
		rv = ptr
	} else if rv.IsNil() {
		return nil, nil
	}

	exported := upperFirst(fd.Name)
	if m := rv.MethodByName(exported); m.IsValid() {
		return callMethod(m, fd, args)
	}

	el := rv.Elem()
	switch el.Kind() {
	case reflect.Struct:
		t := el.Type()
		if sf, ok := t.FieldByName(exported); ok && sf.IsExported() {
			return el.FieldByIndex(sf.Index).Interface(), nil
		}
		for i := 0; i < t.NumField(); i++ {
			sf := t.Field(i)
			if !sf.IsExported() {
				continue
			}
			tag, _, _ := strings.Cut(sf.Tag.Get("json"), ",")
			if tag == fd.Name || (tag == "" && strings.EqualFold(sf.Name, fd.Name)) {
				return el.Field(i).Interface(), nil
			}
		}
	case reflect.Map:
		if el.Type().Key().Kind() == reflect.String {
			if v := el.MapIndex(reflect.ValueOf(fd.Name).Convert(el.Type().Key())); v.IsValid() {
				return v.Interface(), nil
			}
		}
	}
	return nil, nil
}

func callMethod(m reflect.Value, fd *ast.FieldDefinition, args map[string]any) (any, error) {
	mt := m.Type()
	if mt.NumIn() > len(fd.Arguments) || mt.NumOut() == 0 || mt.NumOut() > 2 {
		return nil, fmt.Errorf("cannot resolve %s: incompatible method signature %s", fd.Name, mt)
	}
	in := make([]reflect.Value, mt.NumIn())
	for i := range in {
		pt := mt.In(i)
		av := args[fd.Arguments[i].Name]
		if av == nil {
			in[i] = reflect.Zero(pt)
			continue
		}
		v := reflect.ValueOf(av)
		if !v.Type().ConvertibleTo(pt) {
			return nil, fmt.Errorf("cannot resolve %s: argument %s is %T, method expects %s", fd.Name, fd.Arguments[i].Name, av, pt)
		}
		in[i] = v.Convert(pt)
	}
	out := m.Call(in)
	if len(out) == 2 && !out[1].IsNil() {
		err, _ := out[1].Interface().(error)
		return nil, err
	}
	return out[0].Interface(), nil
}

func upperFirst(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}

// ── Input coercion ─────────────────────────────────────────────────────────

// coerceArgs coerces the arguments of a field (variables substituted and SDL
// defaults applied by gqlparser) into plain Go values.
func (e *Engine) coerceArgs(ctx context.Context, fd *ast.FieldDefinition, field graphql.CollectedField, vars map[string]any) (map[string]any, error) {
	raw := field.ArgumentMap(vars)
	args := make(map[string]any, len(raw))
	for _, ad := range fd.Arguments {
		v, ok := raw[ad.Name]
		if !ok {
			continue
		}
		argCtx := graphql.WithPathContext(ctx, graphql.NewPathWithField(ad.Name))
		c, err := e.coerceInput(argCtx, ad.Type, v)
		if err != nil {
			return nil, graphql.ErrorOnPath(argCtx, err)
		}
		args[ad.Name] = c
	}
	return args, nil
}

func (e *Engine) coerceInput(ctx context.Context, typ *ast.Type, v any) (any, error) {
	if v == nil {
		if typ.NonNull {
			return nil, fmt.Errorf("must not be null")
		}
		return nil, nil
	}

	if typ.Elem != nil {
		rv := reflect.ValueOf(v)
		if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
			// Input coercion: a single value is accepted as a list of one.
			c, err := e.coerceInput(ctx, typ.Elem, v)
			if err != nil {
				return nil, err
			}
			return []any{c}, nil
		}
		out := make([]any, rv.Len())
		for i := range out {
			elCtx := graphql.WithPathContext(ctx, graphql.NewPathWithIndex(i))
			c, err := e.coerceInput(elCtx, typ.Elem, rv.Index(i).Interface())
			if err != nil {
				return nil, graphql.ErrorOnPath(elCtx, err)
			}
			out[i] = c
		}
		return out, nil
	}

	def := e.schema.Types[typ.NamedType]
	if def == nil {
		return nil, fmt.Errorf("unknown input type %s", typ.NamedType)
	}
	switch def.Kind {
	case ast.Scalar:
		return e.unmarshalScalar(def.Name, v)
	case ast.Enum:
		s, ok := v.(string)
		if !ok || def.EnumValues.ForName(s) == nil {
			return nil, fmt.Errorf("%v is not a valid %s", v, def.Name)
		}
		return s, nil
	case ast.InputObject:
		m, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s must be an object, got %T", def.Name, v)
		}
		out := make(map[string]any, len(def.Fields))
		for _, f := range def.Fields {
			fv, present := m[f.Name]
			if !present {
				if f.DefaultValue == nil {
					if f.Type.NonNull {
						return nil, fmt.Errorf("%s.%s must be defined", def.Name, f.Name)
					}
					continue
				}
				dv, err := f.DefaultValue.Value(nil)
				if err != nil {
					return nil, err
				}
				fv = dv
			}
			fieldCtx := graphql.WithPathContext(ctx, graphql.NewPathWithField(f.Name))
			c, err := e.coerceInput(fieldCtx, f.Type, fv)
			if err != nil {
				return nil, graphql.ErrorOnPath(fieldCtx, err)
			}
			out[f.Name] = c
		}
		return out, nil
	default:
		return nil, fmt.Errorf("type %s cannot be used as an input type", def.Name)
	}
}

// unmarshalScalar uses gqlgen's built-in unmarshalers so inputs are accepted
// exactly like the generated engine accepts them.
func (e *Engine) unmarshalScalar(name string, v any) (any, error) {
	if codec, ok := e.scalars[name]; ok && codec.Unmarshal != nil {
		return codec.Unmarshal(v)
	}
	switch name {
	case "Int":
		return graphql.UnmarshalInt(v)
	case "Float":
		return graphql.UnmarshalFloat(v)
	case "String":
		return graphql.UnmarshalString(v)
	case "ID":
		return graphql.UnmarshalID(v)
	case "Boolean":
		return graphql.UnmarshalBoolean(v)
	default:
		// Map and unknown custom scalars: arbitrary JSON.
		return scalar.UnmarshalMap(v)
	}
}

// LookupKey reads a GraphQL field from a CouchDB document map: the exact key
// first, then "_id" for "id", then the snake_case form of a camelCase name
// (runtimeMinutes → runtime_minutes).
func LookupKey(m map[string]any, name string) any {
	if v, ok := m[name]; ok {
		return v
	}
	if name == "id" {
		return m["_id"]
	}
	if snake := toSnake(name); snake != name {
		return m[snake]
	}
	return nil
}

// LookupPath reads a dot-separated path ("value.min") from nested maps; each
// segment uses LookupKey semantics.
func LookupPath(v any, path string) any {
	for _, seg := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = LookupKey(m, seg)
	}
	return v
}

func toSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
