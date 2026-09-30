# Agent Role System Runbook

> Operational playbooks and execution protocols for agents working on **CouchGraph**.

---

## 1. Quick Start for Agents

When you receive a prompt in this repository:

1. Read [`skills/couchgraph/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/SKILL.md) to understand project invariants.
2. Check the [`Routing Matrix`](file:///Users/ton/work/couchgraph/skills/couchgraph/reference/routing-matrix.md) to pick the appropriate workflow.
3. For non-trivial features, trigger `/couchgraph radioactive` and follow the 14-phase pipeline.
4. Always verify that:
   - All document IDs use UUIDv7 (`uuid.NewV7()`).
   - Resolvers in `internal/graph/resolver/schema.resolvers.go` stay thin and call `internal/couch/repository.go`.
   - Batch lookups by ID use `couch.Loader` (`_bulk_get`).
   - `go build ./...` and `go test ./...` pass with 0 errors.

---

## 2. Command Reference

```bash
# Full feature delivery with Matt Pocock discovery & quality loop
/couchgraph radioactive "Add CouchDB _changes subscription support"

# Codebase mapping and orientation
/couchgraph wayfinder "Where are Mango queries translated?"

# Requirements grilling
/couchgraph grilling "We need to support per-database authentication"

# Code review
/couchgraph review
/couchgraph thermo-review

# Fix review findings
/couchgraph thermo-fix

# Generate release notes
/couchgraph changelog
```

---

## 3. Operational Gates

| Phase | Gate | Requirement |
|---|---|---|
| **Phase 2 (Grilling)** | Decision Log | User must confirm Socratic questions |
| **Phase 7 (To-Spec)** | Spec Approval | User must explicitly approve `docs/tasks/task.<ref>.md` |
| **Phase 10 (Tests)** | Test Pass | 100% test pass on `go test ./...` |
| **Phase 12 (Fix Loop)** | Review Verdict | 0 🔴 Blocker and 0 🟠 High findings remaining |
