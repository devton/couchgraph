# CouchGraph — GraphQL API Reference

> Complete reference of all Types, Queries, Mutations, Subscriptions, Security Guards, and Metrics supported by CouchGraph.

---

## 1. Core Types & Scalars

### `scalar Map`
Arbitrary JSON value (object, array, string, number, boolean, or null). Used for flexible document schemas and dynamic payloads.

### `type Document`
Represents a stored CouchDB document:

```graphql
type Document {
  """Unique document identifier (UUIDv7 by default)"""
  _id: ID!

  """CouchDB MVCC revision token"""
  _rev: String!

  """Raw JSON map containing all document fields (excluding _id and _rev)"""
  data: Map!
}
```

---

## 2. Queries Reference

### 1. `document(id: ID!): Document`
Fetches a single document by its `_id`. Uses the internal DataLoader to resolve the document.

**Example Request:**
```graphql
query {
  document(id: "01927f3a-1001-7000-8000-000000000001") {
    _id
    _rev
    data
  }
}
```

---

### 2. `documents(ids: [ID!]!): [Document]!`
Fetches multiple documents by their IDs in a **single CouchDB HTTP roundtrip** via `_bulk_get`.

> [!TIP]
> This query completely prevents N+1 database calls when resolving batches of entities.

**Example Request:**
```graphql
query {
  documents(ids: [
    "01927f3a-1001-7000-8000-000000000001",
    "01927f3a-1002-7000-8000-000000000002"
  ]) {
    _id
    data
  }
}
```

---

### 3. `findDocs(input: FindInput!): FindResult!`
Executes a declarative [Mango Query](https://docs.couchdb.org/en/stable/api/database/find.html) (`_find`) against CouchDB.

**Input Fields (`FindInput`):**
- `selector: Map!` (Required): JSON selector object, e.g. `{ "type": "movie", "rating": { "$gte": 8.5 } }`.
- `fields: [String!]`: Projection list (optional).
- `sort: [Map!]`: Sort array, e.g. `[{ "rating": "desc" }]`.
- `limit: Int`: Max documents to return.
- `skip: Int`: Number of documents to skip.
- `bookmark: String`: Opaque cursor from a previous `FindResult` for efficient pagination.

**Example Request:**
```graphql
query {
  findDocs(input: {
    selector: {
      type: "movie"
      rating: { "$gte": 8.5 }
    }
    sort: [{ rating: "desc" }]
    limit: 10
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

---

### 4. `queryView(input: ViewInput!): ViewResult!`
Executes a CouchDB MapReduce View query (`_design/<ddoc>/_view/<view>`).

**Input Fields (`ViewInput`):**
- `designDoc: String!` (e.g. `"movies"`)
- `viewName: String!` (e.g. `"by_genre"`)
- `key: Map`: Exact single key match (e.g. `"Drama"` or `1994`).
- `keys: [Map!]`: Multiple exact keys match (e.g. `["Action", "Sci-Fi"]`).
- `startKey: Map` / `endKey: Map`: Key range filtering.
- `limit: Int` / `skip: Int`: Pagination parameters.
- `descending: Boolean`: Reverse key ordering.
- `includeDocs: Boolean`: Embed the full document payload in `row.doc`.
- `reduce: Boolean`: Enable/disable reduce function.
- `group: Boolean`: Group by key in reduce queries.
- `groupLevel: Int`: Grouping level for array keys in reduce views.

**Example Request (Grouped Count Aggregation):**
```graphql
query {
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

## 3. Subscriptions Reference (Real-Time `_changes`)

### `docChanges(docIds: [ID!]): DocumentChange!`
Streams real-time document insertions, updates, and deletions directly from CouchDB's continuous `_changes` feed via WebSockets.

**Example Subscription:**
```graphql
subscription OnLiveChanges {
  docChanges {
    id
    seq
    deleted
    doc {
      _id
      _rev
      data
    }
  }
}
```

---

## 4. Security & Mutation Protection

CouchGraph provides two layers of mutation control:

### 1. Global Read-Only Mode (`READ_ONLY=true`)
When `READ_ONLY=true` (or `MUTATIONS_ENABLED=false`) is set in the environment:
- All mutations (`upsertDoc`, `deleteDoc`, `bulkDocs`, domain mutations) are immediately blocked.
- Any mutation execution returns: `"server is in read-only mode: mutations are disabled"`.
- All queries and subscriptions remain fully active.

### 2. JWT Authentication & Role-Based Mutation Control (`AUTH_ENABLED=true`)
When `AUTH_ENABLED=true` is configured with `JWT_SECRET`:
- Requests supply an HTTP `Authorization: Bearer <jwt-token>` header.
- Token claims are verified via HMAC-SHA256.
- **Write Permissions**: A user must have role `"admin"`, `"editor"`, `"writer"`, `"write"`, or scope `"write"`.
- **Read-Only Users**: Users with role `"reader"` or `"viewer"` are restricted to queries and cannot execute mutations (rejected with `"forbidden: write permissions required to execute mutations"`).
- **Unauthenticated Requests**: Blocked from mutations when Auth is enabled (or blocked from queries if `REQUIRE_AUTH=true`).

---

## 5. Prometheus Metrics (`/metrics`)

CouchGraph exposes Prometheus metrics at `/metrics` (configurable via `METRICS_PATH` and `METRICS_ENABLED=true`):
- Memory allocations and heap statistics
- Go runtime garbage collection cycles and pauses
- Goroutine count and thread allocations
