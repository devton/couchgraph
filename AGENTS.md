# AGENTS.md

> Agent onboarding, system invariants, and workflow directory for **CouchGraph**.

---

## Start Here

- **Primary Project Skill**: [`skills/couchgraph/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/SKILL.md)
  - Contains architecture invariants, orchestrator logic, and command routing.

---

## Core Invariants (Summary)

1. **UUIDv7 Primary Keys**: All document IDs MUST use UUIDv7 (`uuid.NewV7()`). Never use v4/v5.
2. **Schema-First GraphQL**: Edit `internal/graph/schema/*.graphqls` and run `gqlgen generate`. Do not edit generated code.
3. **Thin Resolvers**: Resolvers only marshal requests/responses; database logic lives in `internal/couch/repository.go`.
4. **DataLoader Batching**: Multi-document lookups by ID use `couch.Loader` (`_bulk_get`) to avoid N+1 queries.
5. **Zero-Warning Code**: All changes must compile with `go build ./...` and pass `go vet ./...`.

---

## Skill & Workflow Directory

### Orchestrator & References
- [`skills/couchgraph/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/SKILL.md) — Primary project skill & command router
- [`skills/couchgraph/reference/routing-matrix.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/reference/routing-matrix.md) — Task classification matrix
- [`skills/couchgraph/reference/role-contracts.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/reference/role-contracts.md) — Agent roles and handoffs
- [`skills/couchgraph/reference/hook-blueprint.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/reference/hook-blueprint.md) — Quality verification hooks

### Workflows (`/couchgraph <cmd>`)
- [`workflows/radioactive/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/radioactive/SKILL.md) — 14-phase lifecycle embedding Matt Pocock skills
- [`workflows/mattpocock/wayfinder/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/mattpocock/wayfinder/SKILL.md) — Codebase mapping & orientation
- [`workflows/mattpocock/grilling/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/mattpocock/grilling/SKILL.md) — Requirements grilling & decision log
- [`workflows/mattpocock/domain-modeling/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/mattpocock/domain-modeling/SKILL.md) — Domain entities & UUIDv7 modeling
- [`workflows/mattpocock/research/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/mattpocock/research/SKILL.md) — Technical research & spikes
- [`workflows/mattpocock/prototype/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/mattpocock/prototype/SKILL.md) — POC prototyping
- [`workflows/mattpocock/to-spec/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/mattpocock/to-spec/SKILL.md) — Task specification generator
- [`workflows/c4-architecture/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/c4-architecture/SKILL.md) — Mermaid C4 architecture diagrams
- [`workflows/document/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/document/SKILL.md) — Evidence-based documentation
- [`workflows/review/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/review/SKILL.md) — Code review & findings table
- [`workflows/thermo-nuclear-code-quality-review/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/thermo-nuclear-code-quality-review/SKILL.md) — Deep code quality review
- [`workflows/thermo-fix/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/thermo-fix/SKILL.md) — Iterative review fix loop
- [`workflows/changelog/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/changelog/SKILL.md) — Release notes & changelog
- [`workflows/plan-to-blueprint/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/plan-to-blueprint/SKILL.md) — Plan-to-workflow converter
- [`workflows/brainstorming/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/brainstorming/SKILL.md) — Socratic discovery
- [`workflows/plan-writing/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/plan-writing/SKILL.md) — Implementation planning

---

## Runbooks & Architecture Guides
- [`docs/relations-and-domain-modeling.md`](file:///Users/ton/work/couchgraph/docs/relations-and-domain-modeling.md) — Relations, domain modeling, and virtual collections guide
- [`docs/auth-and-user-scoped-documents.md`](file:///Users/ton/work/couchgraph/docs/auth-and-user-scoped-documents.md) — JWT auth, user-scoped documents, and Row-Level Security
- [`docs/rfc-001-dynamic-schema-engine.md`](file:///Users/ton/work/couchgraph/docs/rfc-001-dynamic-schema-engine.md) — Dynamic schema engine RFC (standalone config-driven mode, directives, JS resolvers, CLI)
- [`docs/runbooks/agent-role-system.md`](file:///Users/ton/work/couchgraph/docs/runbooks/agent-role-system.md) — Operational execution playbook
- [`docs/runbooks/plan-to-blueprint.md`](file:///Users/ton/work/couchgraph/docs/runbooks/plan-to-blueprint.md) — Plan-to-blueprint transformation
