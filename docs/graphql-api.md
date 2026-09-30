# CouchGraph — GraphQL API Reference

> Complete reference of all Types, Queries, Mutations, and Scalars supported by CouchGraph.

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
- `selector: Map!` (Required): JSON selector object, e.g. `{ "type": "user", "active": true }`.
- `fields: [String!]`: Projection list (optional).
- `sort: [Map!]`: Sort array, e.g. `[{ "name": "asc" }]`.
- `limit: Int`: Max documents to return.
- `skip: Int`: Number of documents to skip.
- `bookmark: String`: Opaque cursor from a previous `FindResult` for efficient pagination.

**Example Request:**
```graphql
query {
  findDocs(input: {
    selector: {
      type: "user",
      role: { "$in": ["admin", "engineer"] }
    }
    sort: [{ name: "asc" }]
    limit: 20
  }) {
    docs {
      _id
      _rev
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
- `designDoc: String!` (e.g. `"users"`)
- `viewName: String!` (e.g. `"by_email"`)
- `startKey: Map` / `endKey: Map`: Key range filtering.
- `limit: Int` / `skip: Int`: Pagination parameters.
- `descending: Boolean`: Reverse key ordering.
- `includeDocs: Boolean`: Embed the full document payload in `row.doc`.
- `reduce: Boolean`: Enable/disable reduce function.
- `groupLevel: Int`: Grouping level for reduce views.

**Example Request:**
```graphql
query {
  queryView(input: {
    designDoc: "users"
    viewName: "by_email"
    startKey: "a"
    endKey: "z\ufff0"
    includeDocs: true
    limit: 10
  }) {
    totalRows
    offset
    rows {
      id
      key
      value
      doc {
        _id
        data
      }
    }
  }
}
```

---

### 5. `databases: [String!]!` & `serverInfo: Map!`
Provides diagnostic metadata about the connected CouchDB cluster.

**Example Request:**
```graphql
query {
  databases
  serverInfo
}
```

---

## 3. Mutations Reference

### 1. `upsertDoc(input: UpsertInput!): MutationResult!`
Creates or updates a single document.
- **Create**: Omit `_id` to generate a new **UUIDv7** ID, or provide a custom `_id` without `_rev`.
- **Update**: Provide `_id` and the current `_rev` token.

**Example Create:**
```graphql
mutation {
  upsertDoc(input: {
    data: {
      type: "order"
      customer: "Alice"
      total: 199.50
      status: "pending"
    }
  }) {
    ok
    _id
    _rev
  }
}
```

**Example Update:**
```graphql
mutation {
  upsertDoc(input: {
    _id: "01927f3a-1001-7000-8000-000000000001"
    _rev: "1-abc123456789"
    data: {
      type: "order"
      customer: "Alice"
      total: 199.50
      status: "completed"
    }
  }) {
    ok
    _id
    _rev
  }
}
```

---

### 2. `deleteDoc(input: DeleteInput!): MutationResult!`
Deletes a document from CouchDB using MVCC revision matching.

**Example Request:**
```graphql
mutation {
  deleteDoc(input: {
    _id: "01927f3a-1001-7000-8000-000000000001"
    _rev: "2-def987654321"
  }) {
    ok
    _id
    _rev
  }
}
```

---

### 3. `bulkDocs(input: BulkDocsInput!): BulkResult!`
Performs bulk creates, updates, and deletes in a single atomic-like CouchDB `_bulk_docs` call.

**Example Request:**
```graphql
mutation {
  bulkDocs(input: {
    docs: [
      {
        data: { type: "item", name: "Item A" }
      },
      {
        data: { type: "item", name: "Item B" }
      }
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
