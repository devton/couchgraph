// Package project loads a standalone CouchGraph project described by a
// couchgraph.yaml file: CouchDB connection, schema files, design documents and
// server settings (see docs/rfc-001-dynamic-schema-engine.md).
package project

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
	"go.yaml.in/yaml/v3"

	"github.com/devton/couchgraph/internal/config"
	"github.com/devton/couchgraph/internal/couch"
)

// DefaultFile is the project file looked up in the working directory.
const DefaultFile = "couchgraph.yaml"

// Project is the parsed couchgraph.yaml. Relative paths are resolved against
// the directory containing the file.
type Project struct {
	Dir string `yaml:"-"`

	CouchDB struct {
		URL      string `yaml:"url"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		Database string `yaml:"database"`
	} `yaml:"couchdb"`

	Schema struct {
		// Core exposes the generic core API next to the user schema (default true).
		Core  *bool    `yaml:"core"`
		Paths []string `yaml:"paths"`
	} `yaml:"schema"`

	DesignDocs struct {
		Paths []string `yaml:"paths"`
		// Sync: "on-start" (default) pushes design docs when serving; "manual"
		// only via `couchgraph sync`.
		Sync string `yaml:"sync"`
	} `yaml:"designDocs"`

	// Indexes lists Mango indexes to be created and kept in sync.
	Indexes []couch.IndexDefinition `yaml:"indexes"`

	Server struct {
		Port       int   `yaml:"port"`
		Playground *bool `yaml:"playground"`
		ReadOnly   *bool `yaml:"readOnly"`
	} `yaml:"server"`

	Auth struct {
		Enabled     *bool  `yaml:"enabled"`
		JWTSecret   string `yaml:"jwtSecret"`
		RequireAuth *bool  `yaml:"requireAuth"`
	} `yaml:"auth"`
}

// Load reads a project file. ${VAR} and ${VAR:-default} are expanded from the
// environment before parsing.
func Load(path string) (*Project, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("project: %w", err)
	}
	var p Project
	if err := yaml.Unmarshal([]byte(expandEnv(string(raw))), &p); err != nil {
		return nil, fmt.Errorf("project: parse %s: %w", path, err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	p.Dir = filepath.Dir(abs)
	if len(p.Schema.Paths) == 0 {
		p.Schema.Paths = []string{"schema/**/*.graphqls"}
	}
	if p.DesignDocs.Sync == "" {
		p.DesignDocs.Sync = "on-start"
	}
	return &p, nil
}

// CoreEnabled reports whether the generic core API is exposed.
func (p *Project) CoreEnabled() bool { return p.Schema.Core == nil || *p.Schema.Core }

// SyncOnStart reports whether design docs are pushed when serving.
func (p *Project) SyncOnStart() bool { return p.DesignDocs.Sync == "on-start" }

// Apply overlays the values set in the project file onto cfg (which already
// holds defaults and environment variables).
func (p *Project) Apply(cfg *config.Config) {
	setStr := func(dst *string, v string) {
		if v != "" {
			*dst = v
		}
	}
	setBool := func(dst *bool, v *bool) {
		if v != nil {
			*dst = *v
		}
	}
	setStr(&cfg.CouchDB.URL, p.CouchDB.URL)
	setStr(&cfg.CouchDB.User, p.CouchDB.User)
	setStr(&cfg.CouchDB.Password, p.CouchDB.Password)
	setStr(&cfg.CouchDB.Database, p.CouchDB.Database)
	if p.Server.Port > 0 {
		cfg.Server.Port = p.Server.Port
	}
	setBool(&cfg.Server.PlaygroundEnabled, p.Server.Playground)
	setBool(&cfg.Server.ReadOnly, p.Server.ReadOnly)
	setBool(&cfg.Auth.Enabled, p.Auth.Enabled)
	setStr(&cfg.Auth.JWTSecret, p.Auth.JWTSecret)
	setBool(&cfg.Auth.RequireAuth, p.Auth.RequireAuth)
}

// SchemaSources reads the user SDL files.
func (p *Project) SchemaSources() ([]*ast.Source, error) {
	files, err := p.glob(p.Schema.Paths)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("project: no schema files match %v", p.Schema.Paths)
	}
	sources := make([]*ast.Source, 0, len(files))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		sources = append(sources, &ast.Source{Name: p.rel(f), Input: string(b)})
	}
	return sources, nil
}

// DesignDoc is a design document read from disk.
type DesignDoc struct {
	File string
	Doc  map[string]any
}

// LoadDesignDocs reads the design documents. A missing "_id" defaults to
// "_design/<file name without extension>".
func (p *Project) LoadDesignDocs() ([]DesignDoc, error) {
	files, err := p.glob(p.DesignDocs.Paths)
	if err != nil {
		return nil, err
	}
	docs := make([]DesignDoc, 0, len(files))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var doc map[string]any
		if err := json.Unmarshal(b, &doc); err != nil {
			return nil, fmt.Errorf("project: %s: %w", p.rel(f), err)
		}
		if _, ok := doc["_id"].(string); !ok {
			doc["_id"] = "_design/" + strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
		}
		docs = append(docs, DesignDoc{File: p.rel(f), Doc: doc})
	}
	return docs, nil
}

func (p *Project) rel(path string) string {
	if r, err := filepath.Rel(p.Dir, path); err == nil {
		return r
	}
	return path
}

// glob resolves patterns relative to the project dir. "**" matches any number
// of directories.
func (p *Project) glob(patterns []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, pat := range patterns {
		if !filepath.IsAbs(pat) {
			pat = filepath.Join(p.Dir, pat)
		}
		matches, err := globDoubleStar(pat)
		if err != nil {
			return nil, fmt.Errorf("project: pattern %q: %w", pat, err)
		}
		for _, m := range matches {
			if !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

func globDoubleStar(pattern string) ([]string, error) {
	root, rest, ok := strings.Cut(filepath.ToSlash(pattern), "/**/")
	if !ok {
		return filepath.Glob(pattern)
	}
	root = filepath.FromSlash(root)
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		// Match the remainder against the tail of the relative path.
		parts := strings.Split(filepath.ToSlash(rel), "/")
		restParts := strings.Split(rest, "/")
		if len(parts) < len(restParts) {
			return nil
		}
		tail := strings.Join(parts[len(parts)-len(restParts):], "/")
		if ok, _ := filepath.Match(rest, tail); ok {
			out = append(out, path)
		}
		return nil
	})
	return out, err
}

// expandEnv expands ${VAR} and ${VAR:-default}.
func expandEnv(s string) string {
	return os.Expand(s, func(key string) string {
		name, def, hasDef := strings.Cut(key, ":-")
		if v, ok := os.LookupEnv(name); ok && v != "" {
			return v
		}
		if hasDef {
			return def
		}
		return ""
	})
}
