# Getting Started with CouchGraph

> Step-by-step guide to run CouchGraph locally and make your first GraphQL queries.

---

## 1. Prerequisites

- **Go 1.26+** (or Docker)
- **Docker & Docker Compose** (for running CouchDB locally)
- `curl` (for testing and seeding)

---

## 2. Quick Start (5 Minutes)

### Step 1: Clone and Setup Configuration

```bash
git clone https://github.com/ton/couchgraph.git
cd couchgraph

# Copy sample environment configuration
cp .env.example .env
```

### Step 2: Start CouchDB

Start a local CouchDB 3.4 instance with persistent storage:

```bash
docker-compose up -d couchdb
```

Verify CouchDB is running:

```bash
curl http://localhost:5984/
# Response: {"couchdb":"Welcome","version":"3.4.x",...}
```

### Step 3: Seed Sample Data (Optional)

Run the included seed script to create sample documents, a Mango index, and a MapReduce view:

```bash
./examples/seed-data.sh
```

### Step 4: Run the CouchGraph Server

```bash
go run ./cmd/server
```

The server will start on port `8080`:

```
{"level":"info","ts":"...","msg":"connected to CouchDB","url":"http://localhost:5984","database":"couchgraph"}
{"level":"info","ts":"...","msg":"GraphQL Playground enabled","url":"http://localhost:8080"}
{"level":"info","ts":"...","msg":"starting CouchGraph server","addr":":8080"}
```

---

## 3. Explore with GraphQL Playground

Open your browser at [http://localhost:8080](http://localhost:8080) to interact with the GraphQL Playground.

### Example 1: Create a Document

```graphql
mutation {
  upsertDoc(input: {
    data: {
      type: "user"
      name: "Alice Developer"
      email: "alice@couchgraph.dev"
      active: true
    }
  }) {
    ok
    _id
    _rev
  }
}
```

> [!NOTE]
> CouchGraph automatically generates a **UUIDv7** time-ordered ID for your document (e.g., `01927f3a-1001-7000-8000-...`).

### Example 2: Query by Mango Selector

```graphql
query {
  findDocs(input: {
    selector: {
      type: "user"
      active: true
    }
    limit: 10
  }) {
    docs {
      _id
      _rev
      data
    }
    bookmark
  }
}
```

### Example 3: Batch Fetch with DataLoader

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

## 4. Running the Entire Stack with Docker

If you prefer not to install Go locally, you can run both CouchDB and CouchGraph using Docker Compose:

```bash
docker-compose up -d
```

To view logs:

```bash
docker-compose logs -f couchgraph
```

To stop:

```bash
docker-compose down
```

---

## 5. Configuration Reference

Environment variables can be provided in `.env` or passed directly to the container:

| Variable | Type | Default | Description |
|---|---|---|---|
| `COUCHGRAPH_COUCHDB_URL` | String | `http://localhost:5984` | CouchDB base URL |
| `COUCHGRAPH_COUCHDB_USER` | String | `admin` | CouchDB admin username |
| `COUCHGRAPH_COUCHDB_PASSWORD` | String | `password` | CouchDB admin password |
| `COUCHGRAPH_COUCHDB_DATABASE` | String | `couchgraph` | Active database name (auto-created if missing) |
| `COUCHGRAPH_SERVER_PORT` | Integer | `8080` | Port for HTTP server |
| `COUCHGRAPH_SERVER_PLAYGROUND_ENABLED` | Boolean | `true` | Enable GraphQL Playground at `/` |
| `COUCHGRAPH_LOG_LEVEL` | String | `info` | Logging verbosity (`debug`, `info`, `warn`, `error`) |
| `COUCHGRAPH_LOG_FORMAT` | String | `console` | Format for logs (`console` or `json`) |
