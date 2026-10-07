package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/evanw/esbuild/pkg/api"

	"github.com/devton/couchgraph/internal/couch"
)

// DefaultTimeout is the maximum time a script resolver is allowed to run.
const DefaultTimeout = 3 * time.Second

// Script represents a pre-compiled TypeScript or JavaScript resolver file.
type Script struct {
	Path    string
	Program *goja.Program
	Export  string
}

// Runtime manages the compilation, caching and sandboxed execution of
// TypeScript/JavaScript resolvers.
type Runtime struct {
	dir     string
	store   couch.Store
	mu      sync.RWMutex
	scripts map[string]*Script
	pool    sync.Pool
}

// New creates a new script Runtime rooted at dir.
func New(dir string, store couch.Store) *Runtime {
	r := &Runtime{
		dir:     dir,
		store:   store,
		scripts: make(map[string]*Script),
	}
	r.pool.New = func() any {
		return goja.New()
	}
	return r
}

// Transpile converts TypeScript or modern JavaScript source code into CommonJS
// code runnable by Goja.
func Transpile(source string, filename string) (string, error) {
	loader := api.LoaderJS
	if strings.HasSuffix(filename, ".ts") || strings.HasSuffix(filename, ".tsx") {
		loader = api.LoaderTS
	}

	result := api.Transform(source, api.TransformOptions{
		Loader:            loader,
		Target:            api.ES2018,
		Format:            api.FormatCommonJS,
		Sourcemap:         api.SourceMapInline,
		Sourcefile:        filename,
		MinifyWhitespace:  false,
		MinifyIdentifiers: false,
		MinifySyntax:      false,
	})

	if len(result.Errors) > 0 {
		var msgs []string
		for _, err := range result.Errors {
			msgs = append(msgs, fmt.Sprintf("%s:%d:%d: %s", filename, err.Location.Line, err.Location.Column, err.Text))
		}
		return "", fmt.Errorf("transpile error:\n  %s", strings.Join(msgs, "\n  "))
	}

	return string(result.Code), nil
}

// Load compiles and caches a script file (TypeScript or JavaScript).
func (r *Runtime) Load(relPath string, exportName string) (*Script, error) {
	if exportName == "" {
		exportName = "default"
	}
	key := relPath + ":" + exportName

	r.mu.RLock()
	if s, ok := r.scripts[key]; ok {
		r.mu.RUnlock()
		return s, nil
	}
	r.mu.RUnlock()

	fullPath := relPath
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(r.dir, relPath)
	}

	raw, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("runtime: read script %q: %w", relPath, err)
	}

	jsCode, err := Transpile(string(raw), filepath.Base(fullPath))
	if err != nil {
		return nil, fmt.Errorf("runtime: transpile %q: %w", relPath, err)
	}

	// Wrap in CommonJS module scaffold so "export default" or "exports.foo =" works
	wrapped := fmt.Sprintf(`(function(exports, module) {
%s
return module.exports;
})({}, { exports: {} });`, jsCode)

	prog, err := goja.Compile(relPath, wrapped, true)
	if err != nil {
		return nil, fmt.Errorf("runtime: compile %q: %w", relPath, err)
	}

	s := &Script{
		Path:    relPath,
		Program: prog,
		Export:  exportName,
	}

	r.mu.Lock()
	r.scripts[key] = s
	r.mu.Unlock()

	return s, nil
}

// Context carries invocation data exposed to the script.
type Context struct {
	Args   map[string]any
	Parent any
	User   map[string]any
}

// Run executes a compiled script function with the provided execution context.
func (r *Runtime) Run(ctx context.Context, script *Script, c Context) (any, error) {
	vm := r.pool.Get().(*goja.Runtime)
	defer r.pool.Put(vm)

	// Clean VM scope
	for _, k := range vm.GlobalObject().Keys() {
		_ = vm.GlobalObject().Delete(k)
	}

	// Execution timeout
	timer := time.AfterFunc(DefaultTimeout, func() {
		vm.Interrupt(errors.New("execution timeout exceeded"))
	})
	defer timer.Stop()

	// Execute module wrapper
	modVal, err := vm.RunProgram(script.Program)
	if err != nil {
		return nil, fmt.Errorf("script execution error: %w", err)
	}

	modObj := modVal.ToObject(vm)
	var fnVal goja.Value

	if script.Export == "default" {
		fnVal = modObj.Get("default")
		if fnVal == nil || goja.IsUndefined(fnVal) {
			// Fallback: entire module export might be the function itself
			fnVal = modVal
		}
	} else {
		fnVal = modObj.Get(script.Export)
	}

	fn, ok := goja.AssertFunction(fnVal)
	if !ok {
		return nil, fmt.Errorf("script %q does not export a function named %q", script.Path, script.Export)
	}

	// Setup context object for the JS function
	jsCtx := vm.NewObject()
	_ = jsCtx.Set("args", c.Args)
	_ = jsCtx.Set("parent", c.Parent)
	if c.User != nil {
		_ = jsCtx.Set("user", c.User)
	}

	// Setup couch bridge if store is available
	if r.store != nil {
		couchObj := vm.NewObject()

		_ = couchObj.Set("get", func(call goja.FunctionCall) goja.Value {
			id := call.Argument(0).String()
			doc, err := r.store.Get(ctx, id)
			if err != nil || doc == nil {
				return goja.Null()
			}
			return vm.ToValue(doc)
		})

		_ = couchObj.Set("find", func(call goja.FunctionCall) goja.Value {
			arg := call.Argument(0).Export()
			m, ok := arg.(map[string]any)
			if !ok {
				panic(vm.ToValue("find() requires an options object"))
			}
			opts := couch.FindOptions{}
			if sel, ok := m["selector"].(map[string]any); ok {
				opts.Selector = sel
			}
			if lim, ok := m["limit"].(int64); ok {
				opts.Limit = int(lim)
			}
			res, err := r.store.Find(ctx, opts)
			if err != nil {
				panic(vm.ToValue(err.Error()))
			}
			return vm.ToValue(res.Docs)
		})

		_ = jsCtx.Set("couch", couchObj)
	}

	// Call the exported function: fn(ctx)
	res, err := fn(goja.Undefined(), jsCtx)
	if err != nil {
		return nil, fmt.Errorf("resolver %s:%s runtime error: %w", script.Path, script.Export, err)
	}

	if res == nil || goja.IsNull(res) || goja.IsUndefined(res) {
		return nil, nil
	}

	return res.Export(), nil
}
