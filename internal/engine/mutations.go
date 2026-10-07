package engine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	kivik "github.com/go-kivik/kivik/v4"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/devton/couchgraph/internal/couch"
)

// Key styles for @collection(keys:), used when input fields are written to a
// document.
const (
	keysSnakeCase = "snake_case" // runtimeMinutes → runtime_minutes (default)
	keysAsIs      = "as_is"      // runtimeMinutes → runtimeMinutes
)

// conflictRetries bounds the read-merge-write loop of @update/@delete when the
// client did not pin a revision.
const conflictRetries = 3

// ErrConflict is returned (wrapped) when a document changed between read and
// write, or when the client-supplied revision is stale.
var ErrConflict = errors.New("conflict")

// collectionInfo describes the @collection of a mutation's return type.
type collectionInfo struct {
	typeName string
	field    string
	value    string
	keys     string
}

// inputPlan maps the fields of an input object type to document keys. It is
// built once per mutation at compile time.
type inputPlan struct {
	fields []inputFieldPlan
}

type inputFieldPlan struct {
	name   string     // GraphQL input field name
	key    string     // document key
	nested *inputPlan // non-nil for (lists of) input objects
}

// requireMutation checks the field is a Mutation root field returning a
// @collection object, and returns that collection.
func (c *compiler) requireMutation() (collectionInfo, error) {
	if m := c.e.schema.Mutation; m == nil || m.Name != c.def.Name {
		return collectionInfo{}, fmt.Errorf("only allowed on Mutation fields")
	}
	typeName := c.fd.Type.Name()
	if c.fd.Type.Elem != nil {
		return collectionInfo{}, fmt.Errorf("must return a single %s, not a list", typeName)
	}
	def := c.e.schema.Types[typeName]
	if def == nil || def.Kind != ast.Object {
		return collectionInfo{}, fmt.Errorf("must return an object type annotated with @collection")
	}
	d := def.Directives.ForName("collection")
	if d == nil {
		return collectionInfo{}, fmt.Errorf("return type %s has no @collection", typeName)
	}
	args := d.ArgumentMap(nil)
	info := collectionInfo{typeName: typeName}
	info.field, _ = args["field"].(string)
	info.value, _ = args["type"].(string)
	info.keys, _ = args["keys"].(string)
	if info.keys != keysSnakeCase && info.keys != keysAsIs {
		return collectionInfo{}, fmt.Errorf("%s @collection(keys: %q): must be %q or %q", typeName, info.keys, keysSnakeCase, keysAsIs)
	}
	return info, nil
}

// requireIDArg checks the field declares the ID argument named argName.
func (c *compiler) requireIDArg(argName string) error {
	if c.fd.Arguments.ForName(argName) == nil {
		return fmt.Errorf("field has no %q argument", argName)
	}
	return nil
}

// inputArg returns the input object type of the argument named argName.
func (c *compiler) inputArg(argName string) (*ast.Definition, error) {
	ad := c.fd.Arguments.ForName(argName)
	if ad == nil {
		return nil, fmt.Errorf("field has no %q argument", argName)
	}
	def := c.e.schema.Types[ad.Type.Name()]
	if def == nil || def.Kind != ast.InputObject || ad.Type.Elem != nil {
		return nil, fmt.Errorf("argument %q must be an input object type", argName)
	}
	return def, nil
}

// buildInputPlan compiles the key mapping of input type in for documents of
// object type obj (which may be nil for nested inputs without a matching
// object field). Key rules, in order: @field(from:) on the input field,
// @field(from:) on the same-named object field, then the @collection key style.
func (c *compiler) buildInputPlan(in, obj *ast.Definition, keys string, seen map[string]*inputPlan) (*inputPlan, error) {
	if p, ok := seen[in.Name]; ok {
		return p, nil // recursive input types share their plan
	}
	plan := &inputPlan{}
	seen[in.Name] = plan
	for _, f := range in.Fields {
		if f.Name == "id" || strings.HasPrefix(f.Name, "_") {
			continue // _id/_rev are managed by CouchGraph
		}
		key, err := writeKey(f, obj, keys)
		if err != nil {
			return nil, fmt.Errorf("%s.%s: %w", in.Name, f.Name, err)
		}
		fp := inputFieldPlan{name: f.Name, key: key}
		if nestedIn := c.e.schema.Types[f.Type.Name()]; nestedIn != nil && nestedIn.Kind == ast.InputObject {
			var nestedObj *ast.Definition
			if obj != nil {
				if of := obj.Fields.ForName(f.Name); of != nil {
					nestedObj = c.e.schema.Types[of.Type.Name()]
				}
			}
			if fp.nested, err = c.buildInputPlan(nestedIn, nestedObj, keys, seen); err != nil {
				return nil, err
			}
		}
		plan.fields = append(plan.fields, fp)
	}
	return plan, nil
}

func writeKey(f *ast.FieldDefinition, obj *ast.Definition, keys string) (string, error) {
	from := fieldFrom(f.Directives)
	if from == "" && obj != nil {
		if of := obj.Fields.ForName(f.Name); of != nil {
			// Dot paths on object fields (e.g. value.min) are read-only views:
			// fall back to the key style for those.
			if objFrom := fieldFrom(of.Directives); !strings.Contains(objFrom, ".") {
				from = objFrom
			}
		}
	}
	switch {
	case strings.Contains(from, "."):
		return "", fmt.Errorf("@field(from: %q): dot paths cannot be written", from)
	case from == "_id" || from == "_rev":
		return "", fmt.Errorf("@field(from: %q): reserved key", from)
	case from != "":
		return from, nil
	case keys == keysAsIs:
		return f.Name, nil
	default:
		return toSnake(f.Name), nil
	}
}

func fieldFrom(dl ast.DirectiveList) string {
	d := dl.ForName("field")
	if d == nil {
		return ""
	}
	from, _ := d.ArgumentMap(nil)["from"].(string)
	return from
}

// apply writes the input values into doc. Absent input fields are left
// untouched. With merge (updates), an explicit null removes the key; on create
// nulls are skipped.
func (p *inputPlan) apply(doc, in map[string]any, merge bool) {
	for _, f := range p.fields {
		v, present := in[f.name]
		if !present {
			continue
		}
		if v == nil {
			if merge {
				delete(doc, f.key)
			}
			continue
		}
		if f.nested != nil {
			v = f.nested.convert(v)
		}
		doc[f.key] = v
	}
}

// convert maps a nested input value (object or list of objects) to document
// form. Nested objects are replaced as a whole, not merged.
func (p *inputPlan) convert(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		p.apply(out, t, false)
		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = p.convert(x)
		}
		return out
	}
	return v
}

// ── @create ────────────────────────────────────────────────────────────────

func (c *compiler) compileCreate(args map[string]any) error {
	coll, err := c.requireMutation()
	if err != nil {
		return err
	}
	inputName, _ := args["input"].(string)
	inDef, err := c.inputArg(inputName)
	if err != nil {
		return err
	}
	plan, err := c.buildInputPlan(inDef, c.e.schema.Types[coll.typeName], coll.keys, map[string]*inputPlan{})
	if err != nil {
		return err
	}
	store := c.store
	c.e.Resolve(c.coord(), func(ctx context.Context, p Params) (any, error) {
		in, _ := p.Args[inputName].(map[string]any)
		doc := map[string]any{}
		plan.apply(doc, in, false)
		doc[coll.field] = coll.value
		id, rev, err := store.Upsert(ctx, "", "", doc) // empty id → UUIDv7
		if err != nil {
			return nil, err
		}
		doc["_id"], doc["_rev"] = id, rev
		return doc, nil
	})
	return nil
}

// ── @update ────────────────────────────────────────────────────────────────

func (c *compiler) compileUpdate(args map[string]any) error {
	coll, err := c.requireMutation()
	if err != nil {
		return err
	}
	idArg, _ := args["arg"].(string)
	if err := c.requireIDArg(idArg); err != nil {
		return err
	}
	inputName, _ := args["input"].(string)
	inDef, err := c.inputArg(inputName)
	if err != nil {
		return err
	}
	plan, err := c.buildInputPlan(inDef, c.e.schema.Types[coll.typeName], coll.keys, map[string]*inputPlan{})
	if err != nil {
		return err
	}
	store := c.store
	c.e.Resolve(c.coord(), func(ctx context.Context, p Params) (any, error) {
		id, _ := p.Args[idArg].(string)
		pinned, _ := p.Args["rev"].(string)
		in, _ := p.Args[inputName].(map[string]any)
		return withConflictRetry(pinned, func() (any, error) {
			existing, rev, err := loadForWrite(ctx, store, coll, id, pinned)
			if err != nil {
				return nil, err
			}
			doc := make(map[string]any, len(existing)+len(in))
			for k, v := range existing {
				if k != "_id" && k != "_rev" {
					doc[k] = v
				}
			}
			plan.apply(doc, in, true)
			doc[coll.field] = coll.value
			_, newRev, err := store.Upsert(ctx, id, rev, doc)
			if err != nil {
				return nil, writeError(err, coll, id)
			}
			doc["_id"], doc["_rev"] = id, newRev
			return doc, nil
		})
	})
	return nil
}

// ── @delete ────────────────────────────────────────────────────────────────

func (c *compiler) compileDelete(args map[string]any) error {
	coll, err := c.requireMutation()
	if err != nil {
		return err
	}
	idArg, _ := args["arg"].(string)
	if err := c.requireIDArg(idArg); err != nil {
		return err
	}
	store := c.store
	c.e.Resolve(c.coord(), func(ctx context.Context, p Params) (any, error) {
		id, _ := p.Args[idArg].(string)
		pinned, _ := p.Args["rev"].(string)
		return withConflictRetry(pinned, func() (any, error) {
			existing, rev, err := loadForWrite(ctx, store, coll, id, pinned)
			if err != nil {
				return nil, err
			}
			if _, err := store.Delete(ctx, id, rev); err != nil {
				return nil, writeError(err, coll, id)
			}
			return existing, nil // the document as it was before deletion
		})
	})
	return nil
}

// loadForWrite reads the current document (bypassing the request cache, which
// may hold a stale revision) and checks collection and pinned revision.
func loadForWrite(ctx context.Context, store couch.Store, coll collectionInfo, id, pinned string) (map[string]any, string, error) {
	if id == "" {
		return nil, "", fmt.Errorf("%s id must not be empty", coll.typeName)
	}
	existing, err := store.Get(ctx, id)
	if err != nil && kivik.HTTPStatus(err) != http.StatusNotFound {
		return nil, "", err
	}
	if existing == nil || existing[coll.field] != coll.value {
		return nil, "", fmt.Errorf("%s %q not found", coll.typeName, id)
	}
	rev, _ := existing["_rev"].(string)
	if pinned != "" && pinned != rev {
		return nil, "", fmt.Errorf("%w: %s %q is at revision %s, not %s", ErrConflict, coll.typeName, id, rev, pinned)
	}
	return existing, rev, nil
}

func writeError(err error, coll collectionInfo, id string) error {
	if kivik.HTTPStatus(err) == http.StatusConflict {
		return fmt.Errorf("%w: %s %q was modified concurrently", ErrConflict, coll.typeName, id)
	}
	return err
}

// withConflictRetry re-runs a read-merge-write when it lost a race, unless the
// client pinned a revision (then the conflict is the client's to resolve).
func withConflictRetry(pinned string, fn func() (any, error)) (any, error) {
	var (
		res any
		err error
	)
	for attempt := 0; attempt < conflictRetries; attempt++ {
		res, err = fn()
		if err == nil || pinned != "" || !errors.Is(err, ErrConflict) {
			return res, err
		}
	}
	return res, err
}
