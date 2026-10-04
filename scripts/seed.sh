#!/usr/bin/env bash
set -euo pipefail

# ─────────────────────────────────────────────────────────────────────────────
# CouchGraph IMDb Movies Seeder Runner
# Runs the Go seeder against the configured CouchDB instance in .env
# ─────────────────────────────────────────────────────────────────────────────

echo "🎬 Running CouchGraph IMDb Movies Seeder..."
go run ./cmd/seed
