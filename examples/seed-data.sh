#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────────────────────────────────────
# CouchGraph Sample Data & Index Seeder
# Usage: ./examples/seed-data.sh [COUCHDB_URL] [USER] [PASSWORD] [DATABASE]
# ─────────────────────────────────────────────────────────────────────────────

COUCH_URL="${1:-http://localhost:5984}"
COUCH_USER="${2:-admin}"
COUCH_PASS="${3:-password}"
COUCH_DB="${4:-couchgraph}"

AUTH="$COUCH_USER:$COUCH_PASS"

echo "➡️ Checking CouchDB connectivity at $COUCH_URL..."
curl -s -u "$AUTH" "$COUCH_URL/" > /dev/null || {
  echo "❌ Error: Could not connect to CouchDB at $COUCH_URL with user $COUCH_USER."
  exit 1
}

echo "➡️ Ensuring database '$COUCH_DB' exists..."
curl -s -X PUT -u "$AUTH" "$COUCH_URL/$COUCH_DB" > /dev/null || true

echo "➡️ Creating Mango Index for type and name fields..."
curl -s -X POST -u "$AUTH" "$COUCH_URL/$COUCH_DB/_index" \
  -H "Content-Type: application/json" \
  -d '{
    "index": {
      "fields": ["type", "name", "active"]
    },
    "name": "idx_type_name_active",
    "type": "json"
  }' > /dev/null

echo "➡️ Creating MapReduce Design Document for view queries..."
curl -s -X PUT -u "$AUTH" "$COUCH_URL/$COUCH_DB/_design/users" \
  -H "Content-Type: application/json" \
  -d '{
    "views": {
      "by_email": {
        "map": "function (doc) { if (doc.type === \"user\" && doc.email) { emit(doc.email, { name: doc.name, role: doc.role }); } }"
      }
    }
  }' > /dev/null || true

echo "➡️ Seeding sample documents..."
curl -s -X POST -u "$AUTH" "$COUCH_URL/$COUCH_DB/_bulk_docs" \
  -H "Content-Type: application/json" \
  -d '{
    "docs": [
      {
        "_id": "01927f3a-1001-7000-8000-000000000001",
        "type": "user",
        "name": "Alice Silva",
        "email": "alice@example.com",
        "role": "admin",
        "active": true
      },
      {
        "_id": "01927f3a-1002-7000-8000-000000000002",
        "type": "user",
        "name": "Bob Santos",
        "email": "bob@example.com",
        "role": "engineer",
        "active": true
      },
      {
        "_id": "01927f3a-1003-7000-8000-000000000003",
        "type": "user",
        "name": "Carla Dias",
        "email": "carla@example.com",
        "role": "designer",
        "active": false
      },
      {
        "_id": "01927f3a-2001-7000-8000-000000000001",
        "type": "product",
        "name": "GraphQL Book",
        "price": 49.90,
        "active": true
      }
    ]
  }' > /dev/null

echo "✅ CouchDB seeded successfully!"
echo "   Database: $COUCH_DB"
echo "   Index: idx_type_name_active"
echo "   View: _design/users/_view/by_email"
echo "   Test via GraphQL at http://localhost:8080"
