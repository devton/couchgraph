package engine

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/vektah/gqlparser/v2/ast"

	"github.com/devton/couchgraph/internal/couch"
)

//go:embed directives.graphqls
var directivesSDL string

// DirectivesSource returns the SDL defining the CouchGraph directives.
func DirectivesSource() *ast.Source {
	return &ast.Source{Name: "couchgraph/directives.graphqls", Input: directivesSDL, BuiltIn: true}
}

// Options configures Build.
type Options struct {
	// Store backs core and directive resolvers.
	Store couch.Store
	// Core includes the agnostic core API (document, findDocs, queryView, ...).
	Core bool
	// Sources are the user SDL files (types annotated with directives).
	Sources []*ast.Source
}

// Build assembles a complete engine: directive definitions, optional core API,
// user SDL, compiled directive resolvers, and a registry Check.
func Build(opts Options) (*Engine, error) {
	sources := []*ast.Source{DirectivesSource()}
	if opts.Core {
		sources = append(sources, CoreSource())
	}
	sources = append(sources, opts.Sources...)

	e, err := Load(sources...)
	if err != nil {
		return nil, err
	}
	if opts.Core {
		RegisterCore(e, opts.Store)
	}
	if err := CompileDirectives(e, opts.Store); err != nil {
		return nil, err
	}
	if err := e.Check(); err != nil {
		return nil, err
	}
	return e, nil
}

type loaderKey struct{}

// loader returns the per-operation BatchLoader (or a fresh one outside an
// operation, e.g. in unit tests).
func loader(ctx context.Context, store couch.Store) *couch.BatchLoader {
	if l, ok := ctx.Value(loaderKey{}).(*couch.BatchLoader); ok {
		return l
	}
	return couch.NewBatchLoader(store, batchWindow, batchMax)
}

const (
	batchWindow = 2 * time.Millisecond
	batchMax    = 100
)

var directiveNames = map[string]bool{
	"field": true, "get": true, "find": true, "view": true, "belongsTo": true, "hasMany": true,
}

// CompileDirectives registers a resolver for every field annotated with a
// CouchGraph directive. It reports all problems at once.
func CompileDirectives(e *Engine, store couch.Store) error {
	e.WithRequestScope(func(ctx context.Context) context.Context {
		return context.WithValue(ctx, loaderKey{}, couch.NewBatchLoader(store, batchWindow, batchMax))
	})

	names := make([]string, 0, len(e.schema.Types))
	for name, def := range e.schema.Types {
		if def.Kind == ast.Object && !strings.HasPrefix(name, "__") {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	var problems []string
	for _, name := range names {
		def := e.schema.Types[name]
		for _, fd := range def.Fields {
			var found []*ast.Directive
			for _, d := range fd.Directives {
				if directiveNames[d.Name] {
					found = append(found, d)
				}
			}
			if len(found) == 0 {
				continue
			}
			coord := def.Name + "." + fd.Name
			if len(found) > 1 {
				problems = append(problems, fmt.Sprintf("%s: only one of @field/@get/@find/@view/@belongsTo/@hasMany is allowed", coord))
				continue
			}
			c := &compiler{e: e, store: store, def: def, fd: fd}
			if err := c.compile(found[0]); err != nil {
				problems = append(problems, fmt.Sprintf("%s @%s: %v", coord, found[0].Name, err))
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("engine: invalid directives:\n  %s", strings.Join(problems, "\n  "))
	}
	return nil
}

type compiler struct {
	e     *Engine
	store couch.Store
	def   *ast.Definition
	fd    *ast.FieldDefinition
}

func (c *compiler) coord() string { return c.def.Name + "." + c.fd.Name }

func (c *compiler) compile(d *ast.Directive) error {
	args := d.ArgumentMap(nil) // applies directive defaults
	switch d.Name {
	case "field":
		from, _ := args["from"].(string)
		c.e.ResolveProperty(c.coord(), func(_ context.Context, p Params) (any, error) {
			return LookupPath(p.Parent, from), nil
		})
		return nil
	case "get":
		return c.compileGet(args)
	case "find":
		return c.compileFind(args)
	case "view":
		return c.compileView(args)
	case "belongsTo":
		return c.compileBelongsTo(args)
	case "hasMany":
		return c.compileHasMany(args)
	}
	return nil
}

func (c *compiler) compileGet(args map[string]any) error {
	argName, _ := args["arg"].(string)
	if c.fd.Arguments.ForName(argName) == nil {
		return fmt.Errorf("field has no %q argument", argName)
	}
	typeName := c.fd.Type.Name()
	store := c.store
	c.e.Resolve(c.coord(), func(ctx context.Context, p Params) (any, error) {
		id, _ := p.Args[argName].(string)
		if id == "" {
			return nil, nil
		}
		doc, err := loader(ctx, store).Load(ctx, id)
		if err != nil || doc == nil || !c.inCollection(typeName, doc) {
			return nil, err
		}
		return doc, nil
	})
	return nil
}

func (c *compiler) compileFind(args map[string]any) error {
	selector, err := parseJSONObject(args["selector"])
	if err != nil {
		return fmt.Errorf("selector: %w", err)
	}
	var sortSpec []any
	if s, ok := args["sort"].(string); ok && s != "" {
		if err := json.Unmarshal([]byte(s), &sortSpec); err != nil {
			return fmt.Errorf("sort must be a JSON array: %w", err)
		}
	}
	if err := c.checkPlaceholders(selector); err != nil {
		return err
	}
	if field, value, ok := c.collectionOf(c.fd.Type.Name()); ok {
		if _, has := selector[field]; !has {
			selector[field] = value
		}
	}
	defaultLimit := toIntOr(args["limit"], 0)
	store := c.store
	c.e.Resolve(c.coord(), func(ctx context.Context, p Params) (any, error) {
		sel, _ := fill(selector, p)
		selMap, _ := sel.(map[string]any)
		if selMap == nil {
			selMap = map[string]any{}
		}
		opts := couch.FindOptions{Selector: selMap, Limit: defaultLimit}
		for _, s := range sortSpec {
			if m, ok := s.(map[string]any); ok {
				opts.Sort = append(opts.Sort, m)
			}
		}
		if v, ok := p.Args["limit"].(int); ok {
			opts.Limit = v
		}
		if v, ok := p.Args["skip"].(int); ok {
			opts.Skip = v
		}
		if v, ok := p.Args["bookmark"].(string); ok {
			opts.Bookmark = v
		}
		res, err := store.Find(ctx, opts)
		if err != nil {
			return nil, err
		}
		items := make([]any, len(res.Docs))
		for i, d := range res.Docs {
			items[i] = d
		}
		return c.shape(items), nil
	})
	return nil
}

func (c *compiler) compileView(args map[string]any) error {
	ddoc, view, err := splitView(args["name"])
	if err != nil {
		return err
	}
	var key any
	hasKey := false
	if k, ok := args["key"].(string); ok {
		key, hasKey = parseTemplate(k), true
		if err := c.checkPlaceholders(key); err != nil {
			return err
		}
	}
	reduce, _ := args["reduce"].(bool)
	includeDocs, _ := args["includeDocs"].(bool)
	base := couch.ViewOptions{
		DesignDoc:   ddoc,
		ViewName:    view,
		IncludeDocs: includeDocs && !reduce,
		Reduce:      &reduce,
	}
	if g, ok := args["group"].(bool); ok {
		base.Group = &g
	}
	if gl, ok := args["groupLevel"].(int64); ok {
		v := int(gl)
		base.GroupLevel = &v
	}
	if desc, ok := args["descending"].(bool); ok {
		base.Descending = desc
	}
	c.registerView(base, key, hasKey)
	return nil
}

func (c *compiler) compileHasMany(args map[string]any) error {
	ddoc, view, err := splitView(args["view"])
	if err != nil {
		return err
	}
	keyTmpl, _ := args["key"].(string)
	key := parseTemplate(keyTmpl)
	if err := c.checkPlaceholders(key); err != nil {
		return err
	}
	noReduce := false
	c.registerView(couch.ViewOptions{DesignDoc: ddoc, ViewName: view, Reduce: &noReduce}, key, true)
	return nil
}

func (c *compiler) registerView(base couch.ViewOptions, key any, hasKey bool) {
	store := c.store
	reduce := base.Reduce != nil && *base.Reduce
	c.e.Resolve(c.coord(), func(ctx context.Context, p Params) (any, error) {
		opts := base
		if hasKey {
			k, ok := fill(key, p)
			if !ok && c.isParentKey(key) {
				// Relation key missing on the parent: no related rows.
				return c.shape(nil), nil
			}
			if ok {
				opts.Key = k
			}
		}
		if v, ok := p.Args["limit"].(int); ok {
			opts.Limit = v
		}
		if v, ok := p.Args["skip"].(int); ok {
			opts.Skip = v
		}
		if v, ok := p.Args["descending"].(bool); ok {
			opts.Descending = v
		}
		res, err := store.QueryView(ctx, opts)
		if err != nil {
			return nil, err
		}
		return c.shape(shapeRows(res, reduce)), nil
	})
}

func (c *compiler) compileBelongsTo(args map[string]any) error {
	field, _ := args["field"].(string)
	typeName := c.fd.Type.Name()
	store := c.store
	c.e.Resolve(c.coord(), func(ctx context.Context, p Params) (any, error) {
		ref := LookupPath(p.Parent, field)
		l := loader(ctx, store)
		switch v := ref.(type) {
		case string:
			if v == "" {
				return nil, nil
			}
			doc, err := l.Load(ctx, v)
			if err != nil || doc == nil || !c.inCollection(typeName, doc) {
				return nil, err
			}
			return c.shape([]any{doc}), nil
		case []any:
			ids := make([]string, 0, len(v))
			for _, item := range v {
				if s, ok := item.(string); ok && s != "" {
					ids = append(ids, s)
				}
			}
			docs, err := l.LoadMany(ctx, ids)
			if err != nil {
				return nil, err
			}
			items := make([]any, 0, len(docs))
			for _, d := range docs {
				if d != nil && c.inCollection(typeName, d) {
					items = append(items, d)
				}
			}
			return c.shape(items), nil
		}
		return c.shape(nil), nil
	})
	return nil
}

// shape returns the item list for list fields, or the first item (or nil).
func (c *compiler) shape(items []any) any {
	if c.fd.Type.Elem != nil {
		if items == nil {
			items = []any{}
		}
		return items
	}
	if len(items) == 0 {
		return nil
	}
	return items[0]
}

// collectionOf returns the discriminator of a type annotated with @collection.
func (c *compiler) collectionOf(typeName string) (field, value string, ok bool) {
	def := c.e.schema.Types[typeName]
	if def == nil {
		return "", "", false
	}
	d := def.Directives.ForName("collection")
	if d == nil {
		return "", "", false
	}
	args := d.ArgumentMap(nil)
	field, _ = args["field"].(string)
	value, _ = args["type"].(string)
	return field, value, field != "" && value != ""
}

func (c *compiler) inCollection(typeName string, doc map[string]any) bool {
	field, value, ok := c.collectionOf(typeName)
	return !ok || doc[field] == value
}

func (c *compiler) isParentKey(tmpl any) bool {
	s, ok := tmpl.(string)
	return ok && strings.HasPrefix(s, "$parent.")
}

// checkPlaceholders ensures every "$name" refers to a field argument.
func (c *compiler) checkPlaceholders(tmpl any) error {
	var bad []string
	walkStrings(tmpl, func(s string) {
		name, ok := placeholder(s)
		if !ok || strings.HasPrefix(name, "parent.") {
			return
		}
		if c.fd.Arguments.ForName(name) == nil {
			bad = append(bad, "$"+name)
		}
	})
	if len(bad) > 0 {
		return fmt.Errorf("unknown argument placeholder(s) %s", strings.Join(bad, ", "))
	}
	return nil
}

// ── Templates ──────────────────────────────────────────────────────────────

var placeholderRe = regexp.MustCompile(`^\$((?:parent\.)?[A-Za-z_][A-Za-z0-9_.]*)$`)

func placeholder(s string) (string, bool) {
	m := placeholderRe.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// fill substitutes placeholders with argument / parent values. Entries whose
// value is absent are dropped, and containers left empty by drops are dropped
// too, so optional filters disappear from the query when not provided.
func fill(v any, p Params) (any, bool) {
	switch t := v.(type) {
	case string:
		name, ok := placeholder(t)
		if !ok {
			return t, true
		}
		if rest, isParent := strings.CutPrefix(name, "parent."); isParent {
			val := LookupPath(p.Parent, rest)
			return val, val != nil
		}
		val, present := p.Args[name]
		return val, present && val != nil
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			if fv, ok := fill(x, p); ok {
				out[k] = fv
			}
		}
		if len(out) == 0 && len(t) > 0 {
			return nil, false
		}
		return out, true
	case []any:
		out := make([]any, 0, len(t))
		for _, x := range t {
			if fv, ok := fill(x, p); ok {
				out = append(out, fv)
			}
		}
		if len(out) == 0 && len(t) > 0 {
			return nil, false
		}
		return out, true
	default:
		return v, true
	}
}

func walkStrings(v any, fn func(string)) {
	switch t := v.(type) {
	case string:
		fn(t)
	case map[string]any:
		for _, x := range t {
			walkStrings(x, fn)
		}
	case []any:
		for _, x := range t {
			walkStrings(x, fn)
		}
	}
}

// parseTemplate parses s as JSON, or keeps it as a plain string ("$arg").
func parseTemplate(s string) any {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err == nil {
		return v
	}
	return s
}

func parseJSONObject(v any) (map[string]any, error) {
	s, _ := v.(string)
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, fmt.Errorf("must be a JSON object: %w", err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

func splitView(v any) (ddoc, view string, err error) {
	s, _ := v.(string)
	s = strings.TrimPrefix(s, "_design/")
	ddoc, view, ok := strings.Cut(s, "/")
	if !ok || ddoc == "" || view == "" || strings.Contains(view, "/") {
		return "", "", fmt.Errorf("view must be \"designDoc/viewName\", got %q", s)
	}
	return ddoc, view, nil
}

// shapeRows maps view rows to items: reduce rows → {key, value}; otherwise the
// included doc, the emitted object value, or {id, key, value}.
func shapeRows(res *couch.ViewResult, reduce bool) []any {
	items := make([]any, 0, len(res.Rows))
	for _, r := range res.Rows {
		switch {
		case reduce:
			items = append(items, map[string]any{"key": r.Key, "value": r.Value})
		case r.Doc != nil:
			items = append(items, r.Doc)
		default:
			if m, ok := r.Value.(map[string]any); ok {
				items = append(items, m)
			} else {
				items = append(items, map[string]any{"id": r.ID, "key": r.Key, "value": r.Value})
			}
		}
	}
	return items
}

func toIntOr(v any, def int) int {
	switch n := v.(type) {
	case int64:
		return int(n)
	case int:
		return n
	}
	return def
}
