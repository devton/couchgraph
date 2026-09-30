# CouchGraph

> A GraphQL API layer for CouchDB, written in Go.

CouchGraph exposes your CouchDB documents through a strongly-typed, schema-first GraphQL API — no REST endpoints, no boilerplate. Query documents by ID, run [Mango](https://docs.couchdb.org/en/stable/api/database/find.html) selectors, execute MapReduce views, and perform bulk writes — all through a single GraphQL endpoint.

---

## Features

- **Schema-first GraphQL** — powered by [gqlgen](https://gqlgen.com/)
- **Mango queries** — pass any `_find` selector directly as a GraphQL input
- **View queries** — query CouchDB MapReduce design documents
- **Batch loading** — `documents(ids: [...])` uses `_bulk_get` internally (no N+1)
- **Cursor pagination** — bookmark-based (native to CouchDB, no `OFFSET`)
- **Full CRUD** — create, update, delete via mutations
- **Bulk operations** — `bulkDocs` mirrors CouchDB's `_bulk_docs`
- **UUIDv7 IDs** — time-ordered document IDs generated automatically
- **GraphQL Playground** — available at `/` in development
- **Graceful shutdown** — SIGINT/SIGTERM handled cleanly
- **Structured logging** — via [zap](https://github.com/uber-go/zap)
- **Docker-ready** — distroless image, < 10 MB

---

## Quick Start

### 1. Start CouchDB locally

```bash
docker-compose up -d couchdb
```

### 2. Run the server

```bash
# Copy and edit environment config
cp .env.example .env

# Run
go run ./cmd/server
```

Open [http://localhost:8080](http://localhost:8080) for the GraphQL Playground.

### 3. Run everything with Docker Compose

```bash
docker-compose up -d
```

---

## Configuration

All settings are configured via environment variables (prefix: `COUCHGRAPH_`) or a `config.yaml` file in the working directory.

| Variable | Default | Description |
|---|---|---|
| `COUCHGRAPH_COUCHDB_URL` | `http://localhost:5984` | CouchDB endpoint |
| `COUCHGRAPH_COUCHDB_USER` | `admin` | CouchDB username |
| `COUCHGRAPH_COUCHDB_PASSWORD` | `password` | CouchDB password |
| `COUCHGRAPH_COUCHDB_DATABASE` | `couchgraph` | Database to use (created if missing) |
| `COUCHGRAPH_SERVER_PORT` | `8080` | HTTP port |
| `COUCHGRAPH_SERVER_PLAYGROUND_ENABLED` | `true` | Enable GraphQL Playground |
| `COUCHGRAPH_LOG_LEVEL` | `info` | Log level: `debug` \| `info` \| `warn` \| `error` |
| `COUCHGRAPH_LOG_FORMAT` | `json` | Log format: `json` \| `console` |

---

## GraphQL API

### Queries

#### Fetch a single document

```graphql
query {
  document(id: "01927f3a-b1c2-7e4d-a9f0-123456789abc") {
    _id
    _rev
    data
  }
}
```

#### Batch fetch (uses `_bulk_get` — no N+1)

```graphql
query {
  documents(ids: ["id-1", "id-2", "id-3"]) {
    _id
    _rev
    data
  }
}
```

#### Mango query (`_find`)

```graphql
query {
  findDocs(input: {
    selector: { type: "user", active: true }
    sort: [{ name: "asc" }]
    limit: 20
    bookmark: "g2wAAAABaANkAB..."  # cursor from previous response
  }) {
    docs {
      _id
      data
    }
    bookmark
    warning
  }
}
```

#### View query (MapReduce)

```graphql
query {
  queryView(input: {
    designDoc: "users"
    viewName:  "by_email"
    startKey:  "a"
    endKey:    "z"
    limit:     50
    includeDocs: true
  }) {
    totalRows
    rows {
      id
      key
      value
      doc { _id data }
    }
  }
}
```

#### Server utilities

```graphql
query {
  databases
  serverInfo
}
```

---

### Mutations

#### Create a document (UUIDv7 ID auto-generated)

```graphql
mutation {
  upsertDoc(input: {
    data: { type: "user", name: "Alice", active: true }
  }) {
    ok
    _id
    _rev
  }
}
```

#### Update a document

```graphql
mutation {
  upsertDoc(input: {
    _id:  "01927f3a-b1c2-7e4d-a9f0-123456789abc"
    _rev: "1-abc123"
    data: { type: "user", name: "Alice", active: false }
  }) {
    ok
    _id
    _rev
  }
}
```

#### Delete a document

```graphql
mutation {
  deleteDoc(input: {
    _id:  "01927f3a-b1c2-7e4d-a9f0-123456789abc"
    _rev: "2-def456"
  }) {
    ok
    _id
    _rev
  }
}
```

#### Bulk write

```graphql
mutation {
  bulkDocs(input: {
    docs: [
      { data: { type: "product", name: "Widget" } }
      { data: { type: "product", name: "Gadget" } }
    ]
  }) {
    results {
      ok
      _id
      _rev
    }
  }
}
```

---

## Architecture

```
Client (Browser / App / CLI)
        │  GraphQL query / mutation
        ▼
┌─────────────────────────┐
│   gqlgen HTTP server    │  :8080/query
│   Playground at /       │
└────────────┬────────────┘
             │ thin resolvers
             ▼
┌─────────────────────────┐
│   Repository layer      │  internal/couch/repository.go
│   DataLoader (batch)    │  internal/couch/dataloader.go
└────────────┬────────────┘
             │ Kivik v4 driver
             ▼
┌─────────────────────────┐
│   CouchDB  :5984        │
│   _find  _bulk_get      │
│   _design/_view         │
└─────────────────────────┘
```

### Document IDs

All documents created by CouchGraph use **UUIDv7** — time-ordered, globally unique identifiers. This means:

- Documents sort lexicographically by creation time
- You can range-scan by time using `startKey` / `endKey` in views
- No custom sequencing logic needed

---

## Project Layout

```
couchgraph/
├── cmd/server/main.go              # entry point
├── internal/
│   ├── config/config.go            # config loader (Viper)
│   ├── couch/
│   │   ├── client.go               # Kivik connection wrapper
│   │   ├── repository.go           # CRUD + Mango + Views
│   │   └── dataloader.go           # _bulk_get batch loader
│   └── graph/
│       ├── schema/schema.graphqls  # GraphQL schema (source of truth)
│       ├── generated/              # gqlgen-generated code (do not edit)
│       ├── model/models_gen.go     # generated models
│       ├── resolver/
│       │   ├── resolver.go         # dependency injection
│       │   └── schema.resolvers.go # all query/mutation resolvers
│       └── scalar/map.go           # custom Map JSON scalar
├── docker-compose.yml
├── Dockerfile
├── gqlgen.yml
└── .env.example
```

---

## Development

### Regenerate GraphQL code

After editing `internal/graph/schema/schema.graphqls`:

```bash
go install github.com/99designs/gqlgen@latest
gqlgen generate
```

### Run with live reload (optional)

```bash
# Install air
go install github.com/air-verse/air@latest

air
```

---

## Stack

| Component | Package |
|---|---|
| GraphQL server | [gqlgen](https://github.com/99designs/gqlgen) v0.17 |
| CouchDB driver | [kivik](https://github.com/go-kivik/kivik) v4 |
| Configuration | [viper](https://github.com/spf13/viper) |
| Logging | [zap](https://github.com/uber-go/zap) |
| ID generation | [google/uuid](https://github.com/google/uuid) (v7) |

---

## Roadmap

- [ ] GraphQL Subscriptions via CouchDB `_changes` feed
- [ ] Per-request authentication (JWT / CouchDB session tokens)
- [ ] Prometheus metrics endpoint
- [ ] Typed schema extensions (map CouchDB doc types to GraphQL types)
- [ ] Index management mutations (`createIndex`, `deleteIndex`)
- [ ] OpenTelemetry tracing

---

## License

MIT
