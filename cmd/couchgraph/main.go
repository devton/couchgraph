// Command couchgraph is the standalone CouchGraph CLI: it serves a GraphQL API
// over CouchDB from a couchgraph.yaml project (SDL + directives + design docs),
// without writing Go code. See docs/rfc-001-dynamic-schema-engine.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vektah/gqlparser/v2/ast"
	"go.uber.org/zap"

	"github.com/devton/couchgraph/internal/config"
	"github.com/devton/couchgraph/internal/couch"
	"github.com/devton/couchgraph/internal/engine"
	"github.com/devton/couchgraph/internal/project"
	"github.com/devton/couchgraph/internal/server"
)

// version is set at build time: -ldflags "-X main.version=v0.1.0".
var version = "dev"

const usage = `couchgraph - GraphQL over CouchDB, config-driven

Usage:
  couchgraph <command> [flags]

Commands:
  init [dir]   Scaffold a new project (couchgraph.yaml, schema/, couchdb/design/)
  serve        Start the GraphQL server for a project
  validate     Check schema, directives and referenced views without connecting
  sync         Push design documents to CouchDB
  version      Print the version

Flags (serve, validate, sync):
  -c, -config  Path to the project file (default "couchgraph.yaml")
  -port        Override the server port (serve only)

Environment variables (COUCHDB_URL, COUCHDB_USER, COUCHDB_PASSWORD, PORT, ...)
and a .env file in the working directory are honoured; values set in
couchgraph.yaml take precedence and may reference them as ${VAR:-default}.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]

	var err error
	switch cmd {
	case "init":
		err = runInit(args)
	case "serve":
		err = runServe(args)
	case "validate":
		err = runValidate(args)
	case "sync":
		err = runSync(args)
	case "version", "-v", "--version":
		fmt.Println("couchgraph", version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

type common struct {
	configPath string
	port       int
}

func parseFlags(name string, args []string) (*common, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	c := &common{}
	fs.StringVar(&c.configPath, "c", project.DefaultFile, "project file")
	fs.StringVar(&c.configPath, "config", project.DefaultFile, "project file")
	if name == "serve" {
		fs.IntVar(&c.port, "port", 0, "server port")
	}
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return c, nil
}

// load reads the env config and overlays the project file.
func load(c *common) (*config.Config, *project.Project, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	p, err := project.Load(c.configPath)
	if err != nil {
		return nil, nil, err
	}
	p.Apply(cfg)
	if c.port > 0 {
		cfg.Server.Port = c.port
	}
	return cfg, p, nil
}

func build(p *project.Project, store couch.Store) (*engine.Engine, error) {
	sources, err := p.SchemaSources()
	if err != nil {
		return nil, err
	}
	return engine.Build(engine.Options{Store: store, Core: p.CoreEnabled(), Sources: sources})
}

// ── serve ──────────────────────────────────────────────────────────────────

func runServe(args []string) error {
	c, err := parseFlags("serve", args)
	if err != nil {
		return err
	}
	cfg, p, err := load(c)
	if err != nil {
		return err
	}
	log, err := server.NewLogger(cfg)
	if err != nil {
		return err
	}
	defer log.Sync() //nolint:errcheck

	repo, err := server.Connect(context.Background(), cfg, log)
	if err != nil {
		return err
	}
	if p.SyncOnStart() {
		if err := syncDesignDocs(context.Background(), p, repo, func(file, id string, changed bool) {
			log.Info("design doc", zap.String("id", id), zap.String("file", file), zap.Bool("updated", changed))
		}); err != nil {
			return err
		}
	}

	eng, err := build(p, repo)
	if err != nil {
		return err
	}
	log.Info("schema loaded", zap.String("project", c.configPath), zap.Bool("core_api", p.CoreEnabled()))
	return server.Run(cfg, log, server.Handler(cfg, log, eng))
}

// ── validate ───────────────────────────────────────────────────────────────

func runValidate(args []string) error {
	c, err := parseFlags("validate", args)
	if err != nil {
		return err
	}
	_, p, err := load(c)
	if err != nil {
		return err
	}
	eng, err := build(p, nil) // resolvers are compiled, never invoked
	if err != nil {
		return err
	}
	docs, err := p.LoadDesignDocs()
	if err != nil {
		return err
	}

	views := map[string]bool{}
	for _, d := range docs {
		id, _ := d.Doc["_id"].(string)
		if vs, ok := d.Doc["views"].(map[string]any); ok {
			for name := range vs {
				views[strings.TrimPrefix(id, "_design/")+"/"+name] = true
			}
		}
	}

	schema := eng.Schema()
	var warnings []string
	fields, directives := 0, 0
	for _, def := range sortedTypes(schema) {
		for _, fd := range def.Fields {
			fields++
			for _, d := range fd.Directives {
				var view string
				switch d.Name {
				case "view":
					view, _ = d.ArgumentMap(nil)["name"].(string)
				case "hasMany":
					view, _ = d.ArgumentMap(nil)["view"].(string)
				case "field", "get", "find", "belongsTo":
				default:
					continue
				}
				directives++
				if view != "" && !views[strings.TrimPrefix(view, "_design/")] {
					warnings = append(warnings, fmt.Sprintf("%s.%s: view %q is not defined in local design docs (it must exist in CouchDB)", def.Name, fd.Name, view))
				}
			}
		}
	}

	fmt.Printf("ok: %d types, %d fields, %d directive bindings, %d design docs\n", len(sortedTypes(schema)), fields, directives, len(docs))
	for _, w := range warnings {
		fmt.Println("warning:", w)
	}
	return nil
}

func sortedTypes(s *ast.Schema) []*ast.Definition {
	var out []*ast.Definition
	for name, def := range s.Types {
		if def.Kind == ast.Object && !def.BuiltIn && !strings.HasPrefix(name, "__") {
			out = append(out, def)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ── sync ───────────────────────────────────────────────────────────────────

func runSync(args []string) error {
	c, err := parseFlags("sync", args)
	if err != nil {
		return err
	}
	cfg, p, err := load(c)
	if err != nil {
		return err
	}
	log, err := server.NewLogger(cfg)
	if err != nil {
		return err
	}
	repo, err := server.Connect(context.Background(), cfg, log)
	if err != nil {
		return err
	}
	return syncDesignDocs(context.Background(), p, repo, func(file, id string, changed bool) {
		state := "unchanged"
		if changed {
			state = "updated"
		}
		fmt.Printf("%-10s %s (%s)\n", state, id, file)
	})
}

func syncDesignDocs(ctx context.Context, p *project.Project, store couch.Store, report func(file, id string, changed bool)) error {
	docs, err := p.LoadDesignDocs()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, d := range docs {
		changed, err := couch.SyncDesignDoc(ctx, store, d.Doc)
		if err != nil {
			return fmt.Errorf("sync %s: %w", d.File, err)
		}
		id, _ := d.Doc["_id"].(string)
		report(d.File, id, changed)
	}
	return nil
}

// ── init ───────────────────────────────────────────────────────────────────

func runInit(args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	files := map[string]string{
		project.DefaultFile:         initConfig,
		"schema/notes.graphqls":     initSchema,
		"couchdb/design/notes.json": initDesign,
	}
	for name := range files {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return fmt.Errorf("%s already exists", filepath.Join(dir, name))
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(files[name]), 0o644); err != nil {
			return err
		}
		fmt.Println("created", path)
	}
	fmt.Printf("\nNext:\n  cd %s\n  couchgraph validate\n  couchgraph serve\n", dir)
	return nil
}

const initConfig = `couchdb:
  url: ${COUCHDB_URL:-http://localhost:5984}
  user: ${COUCHDB_USER:-admin}
  password: ${COUCHDB_PASSWORD:-password}
  database: ${COUCHDB_DATABASE:-notes}

schema:
  core: true            # also expose the generic API (document, findDocs, queryView, ...)
  paths:
    - schema/**/*.graphqls

designDocs:
  paths:
    - couchdb/design/*.json
  sync: on-start        # on-start | manual (couchgraph sync)

server:
  port: 8080
  playground: true
  readOnly: false
`

const initSchema = `"""A note document: { "type": "note", "title": "...", "tags": [...], "created_at": "..." }"""
type Note @collection(type: "note") {
  id: ID!
  title: String!
  body: String
  tags: [String!]!
  createdAt: String     # read from created_at automatically
}

extend type Query {
  note(id: ID!): Note @get
  notes(tag: String, limit: Int = 50): [Note!]!
    @find(selector: "{\"tags\": {\"$elemMatch\": {\"$eq\": \"$tag\"}}}")
  notesByTag(tag: String!): [Note!]! @view(name: "notes/by_tag", key: "$tag", includeDocs: true)
}
`

const initDesign = `{
  "views": {
    "by_tag": {
      "map": "function (doc) { if (doc.type === 'note' && Array.isArray(doc.tags)) { doc.tags.forEach(function (t) { emit(t, null); }); } }"
    }
  }
}
`
