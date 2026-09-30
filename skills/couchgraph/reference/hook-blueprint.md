# Hook Blueprint

> Automation checklist and quality hooks for **CouchGraph**.

---

## 1. Automated Verification Checks

Before completing any task or opening a pull request, run the standard quality verification pipeline:

```bash
# 1. Verify Go module and format
go mod tidy
go vet ./...

# 2. Compile all packages
go build ./...

# 3. Run all tests
go test -v -race ./...
```

---

## 2. Schema Generation Hook

Whenever modifying `internal/graph/schema/*.graphqls`:

```bash
# Regenerate gqlgen code
gqlgen generate

# Verify generated code compiles
go build ./...
```

---

## 3. Pre-Commit Checklist

- [ ] All new entity IDs generated via UUIDv7 (`uuid.NewV7()`).
- [ ] No database operations inside `internal/graph/resolver/schema.resolvers.go`.
- [ ] Multi-document lookups by ID use `couch.Loader` for `_bulk_get` batching.
- [ ] `go vet ./...` reports 0 issues.
- [ ] `go build ./...` compiles with 0 errors.
