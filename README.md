# CouchGraph

> High-performance, schema-first GraphQL API layer for CouchDB, written in Go.

CouchGraph exposes CouchDB through a modern GraphQL API — eliminating REST boilerplate and N+1 query bottlenecks. Query documents by UUIDv7 ID, run [Mango](https://docs.couchdb.org/en/stable/api/database/find.html) selectors, execute MapReduce views with built-in reduce aggregations (`_count`, `_stats`, `_sum`), and cross documents with nested 1:1 and 1:N relational modeling.

---

## Key Features

- **Schema-first GraphQL** — powered by [gqlgen](https://gqlgen.com/) with full introspection and GraphiQL Explorer.
- **Generic & Agnostic Core** — full schemaless support for any CouchDB database (`scalar Map`, `findDocs`, `queryView`).
- **Domain Modeling & Relations** — define strongly-typed GraphQL entities (`Movie`, `Director`, etc.) with nested 1:1 and 1:N relations.
- **Zero N+1 Query Engine** — internal `couch.Loader` batches lookups into single CouchDB `_bulk_get` requests.
- **MapReduce Views & Aggregations** — exact single key (`key`), multi-key lookup (`keys`), ranges (`startKey`/`endKey`), and grouped reduce aggregations (`group: true`).
- **Mango Queries** — pass declarative `_find` selectors, projections, and sort descriptors.
- **Cursor Pagination** — native CouchDB bookmark pagination without `OFFSET` performance traps.
- **Full CRUD & Bulk Writes** — single `upsertDoc`/`deleteDoc` and atomic `bulkDocs` (`_bulk_docs`).
- **UUIDv7 Primary Keys** — time-ordered, sortable document identifiers generated automatically (`uuid.NewV7()`).
- **Docker & Dokploy Ready** — ultra-lightweight distroless container image (< 15 MB).

---

## 📚 Documentation & Guides

- 📘 [Relations & Domain Modeling Guide](file:///Users/ton/work/couchgraph/docs/relations-and-domain-modeling.md) — 1:1, 1:N relations, virtual collections, and foreign keys.
- 🚀 [Getting Started Guide](file:///Users/ton/work/couchgraph/docs/getting-started.md) — Local setup, environment config, and quickstart.
- 📖 [GraphQL API Reference](file:///Users/ton/work/couchgraph/docs/graphql-api.md) — Detailed reference for all queries, mutations, scalars, and inputs.
- 🏛️ [Architecture Deep Dive](file:///Users/ton/work/couchgraph/docs/architecture.md) — DataLoader batching, UUIDv7 indexing, and layer design.
- 🚢 [Dokploy Deployment Guide](file:///Users/ton/work/couchgraph/docs/deployment-dokploy.md) — Production deployment with Traefik and auto SSL.
- 💡 [IMDb Dataset & Query Examples](file:///Users/ton/work/couchgraph/examples/movies_queries.graphql) — 12 ready-to-use GraphQL queries.

---

## ⚡ Quick Start

### 1. Start CouchDB locally

```bash
docker-compose up -d couchdb
```

### 2. Configure environment

Copy the environment file and configure your CouchDB credentials:

```bash
cp .env.example .env
```

```ini
COUCHDB_URL=http://localhost:5984
COUCHDB_USER=admin
COUCHDB_PASSWORD=password
COUCHDB_DATABASE=couchgraph
PORT=8080
PLAYGROUND_ENABLED=true
```

### 3. Run the server

```bash
go run ./cmd/server
```

Open [http://localhost:8080](http://localhost:8080) for the interactive **GraphQL Playground** with the GraphiQL Explorer and Docs Explorer.

---

## 🎬 Testing with the IMDb Dataset Seeder

CouchGraph includes an on-demand dataset seeder (`cmd/seed/main.go`) that populates classic movies, directors, foreign key relations, Mango indexes, and MapReduce views:

```bash
# Seed the database configured in your .env:
go run ./cmd/seed

# Or seed into a dedicated test database:
go run ./cmd/seed -db couchgraph_movies
```

Once seeded, open [http://localhost:8080](http://localhost:8080) and run queries from [`examples/movies_queries.graphql`](file:///Users/ton/work/couchgraph/examples/movies_queries.graphql).

---

## 🛠️ Usage Modes

### Mode 1: Standalone Schemaless Service (Agnostic)

Use CouchGraph out-of-the-box as a high-performance GraphQL bridge for any CouchDB database without writing Go code:

#### Mango Query (`_find`)
```graphql
query FindSciFiMovies {
  findDocs(input: {
    selector: {
      type: "movie"
      genres: { "$in": ["Sci-Fi"] }
      rating: { "$gte": 8.5 }
    }
    sort: [{ rating: "desc" }]
    limit: 5
  }) {
    docs {
      _id
      data
    }
  }
}
```

#### MapReduce View with Grouped Aggregation
```graphql
query MovieCountPerGenre {
  queryView(input: {
    designDoc: "movies"
    viewName: "by_genre"
    reduce: true
    group: true
  }) {
    totalRows
    rows {
      key
      value
    }
  }
}
```

---

### Mode 2: Custom Domain Modeling & Schema Extensions

You can easily extend CouchGraph with strongly-typed domain schemas, business queries, and nested document relations.

#### Step 1: Define your domain schema (`internal/graph/schema/ecommerce.graphqls`)
```graphql
type Product {
  id: ID!
  title: String!
  price: Float!
  categoryId: ID!
  
  # 1:1 relation resolved via DataLoader (_bulk_get):
  category: Category
}

type Category {
  id: ID!
  name: String!
  
  # 1:N relation resolved via MapReduce View:
  products: [Product!]!
}

extend type Query {
  product(id: ID!): Product
  products(limit: Int): [Product!]!
  categories: [Category!]!
}
```

#### Step 2: Configure relation resolvers in `gqlgen.yml`
```yaml
models:
  Product:
    fields:
      category:
        resolver: true
  Category:
    fields:
      products:
        resolver: true
```

#### Step 3: Generate and implement resolvers
```bash
gqlgen generate
```

In `internal/graph/resolver/ecommerce.resolvers.go`:
```go
// 1:1 Lookups use DataLoader (Zero N+1):
func (r *productResolver) Category(ctx context.Context, obj *model.Product) (*model.Category, error) {
    loader := couch.NewLoader(r.Repo)
    doc, err := loader.LoadAndWait(ctx, obj.CategoryID)
    if err != nil || doc == nil {
        return nil, err
    }
    return mapToCategory(doc), nil
}

// 1:N Lookups query the indexed foreign key view:
func (r *categoryResolver) Products(ctx context.Context, obj *model.Category) ([]*model.Product, error) {
    falseVal := false
    res, err := r.Repo.QueryView(ctx, couch.ViewOptions{
        DesignDoc: "catalog",
        ViewName:  "by_category_id",
        Key:       obj.ID,
        Reduce:    &falseVal,
    })
    // map rows to []*model.Product...
}
```

For full details and patterns, read the [Relations & Domain Modeling Guide](file:///Users/ton/work/couchgraph/docs/relations-and-domain-modeling.md).

---

## ⚙️ Configuration Reference

All settings can be configured via environment variables (e.g. in `.env` or Docker) or a `config.yaml` file:

| Variable | Default | Description |
|---|---|---|
| `COUCHDB_URL` / `COUCHGRAPH_COUCHDB_URL` | `http://localhost:5984` | CouchDB endpoint URL |
| `COUCHDB_USER` / `COUCHGRAPH_COUCHDB_USER` | `admin` | CouchDB BasicAuth username |
| `COUCHDB_PASSWORD` / `COUCHGRAPH_COUCHDB_PASSWORD` | `password` | CouchDB BasicAuth password |
| `COUCHDB_DATABASE` / `COUCHGRAPH_COUCHDB_DATABASE` | `couchgraph` | Default CouchDB database |
| `PORT` / `COUCHGRAPH_SERVER_PORT` | `8080` | Server HTTP port |
| `PLAYGROUND_ENABLED` / `COUCHGRAPH_SERVER_PLAYGROUND_ENABLED` | `true` | Enable GraphiQL Playground (`/`) |
| `LOG_LEVEL` / `COUCHGRAPH_LOG_LEVEL` | `info` | Logging level (`debug`, `info`, `warn`, `error`) |
| `LOG_FORMAT` / `COUCHGRAPH_LOG_FORMAT` | `console` | Log format (`console` or `json`) |

---

## 🏛️ Architecture

```
GraphQL Client (Browser / Mobile / Microservice)
        │
        ▼  POST /query
┌─────────────────────────────────────────┐
│   gqlgen Server + GraphiQL Explorer     │  :8080/
└────────────────────┬────────────────────┘
                     │
        ┌────────────┴────────────┐
        ▼                         ▼
┌──────────────────┐    ┌──────────────────┐
│ Schemaless Core  │    │ Typed Extensions │
│ (findDocs, Views)│    │ (Movie, Director)│
└────────┬─────────┘    └────────┬─────────┘
         │                       │
         ▼                       ▼
┌──────────────────────────────────────────┐
│   couch.Repository + couch.Loader        │  DataLoader (_bulk_get batching)
└────────────────────┬─────────────────────┘
                     │ BasicAuth HTTP
                     ▼
┌──────────────────────────────────────────┐
│   CouchDB Cluster (:5984)                │
│   _find, _bulk_get, _design/_view        │
└──────────────────────────────────────────┘
```

---

## 📦 Project Layout

```
couchgraph/
├── cmd/
│   ├── server/main.go                  # Server entry point
│   └── seed/main.go                    # IMDb movies & directors seeder
├── internal/
│   ├── config/config.go                # Environment & YAML config loader
│   ├── couch/
│   │   ├── client.go                   # Kivik CouchDB client wrapper
│   │   ├── repository.go               # CouchDB CRUD, Views, and Mango operations
│   │   └── dataloader.go               # Request-level _bulk_get batcher
│   └── graph/
│       ├── schema/
│       │   ├── schema.graphqls         # Generic core GraphQL schema
│       │   └── movies.graphqls         # Typed domain schema extension (IMDb demo)
│       ├── resolver/
│       │   ├── resolver.go             # Dependency injection root
│       │   ├── schema.resolvers.go     # Generic query/mutation resolvers
│       │   └── movies.resolvers.go     # Typed relational domain resolvers
│       ├── model/models_gen.go         # Generated Go GraphQL models
│       └── scalar/map.go               # Custom Map JSON scalar
├── docs/                               # Guides & Architecture runbooks
├── examples/movies_queries.graphql     # Query examples
├── docker-compose.yml
├── Dockerfile
└── gqlgen.yml
```

---

## 🧪 Testing & Code Quality

```bash
# Run all unit tests
go test ./...

# Run static analysis / vetting
go vet ./...

# Compile entire project
go build ./...
```

---

## 🗺️ Roadmap

- [x] Strongly-typed domain schema extensions (1:1 & 1:N relations)
- [x] DataLoader batching via `_bulk_get` (Zero N+1)
- [x] MapReduce Reduce aggregations (`_count`, `_stats`, `_sum`, `group`)
- [ ] GraphQL Subscriptions via CouchDB `_changes` feed
- [ ] Per-request authentication (JWT / CouchDB session tokens)
- [ ] Prometheus metrics endpoint (`/metrics`)
- [ ] OpenTelemetry tracing

---

## 📄 License

MIT
