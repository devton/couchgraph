---
name: radioactive
description: |
  14-phase end-to-end development lifecycle for CouchGraph embedding Matt Pocock AI Hero skills (wayfinder, grilling, domain-modeling, research, prototype, to-spec). Phase 1 executes CouchGraph project skill FIRST before wayfinder orientation.
---

# couchgraph.workflow.radioactive

### Goal

Provide a rigorous 14-phase development lifecycle for CouchGraph that embeds Matt Pocock's AI Hero skills, enforces core architectural invariants (UUIDv7, schema-first gqlgen, thin resolvers, DataLoader batching), and guarantees quality from discovery to release notes.

---

### Scope

- **Applies to**: Complex features, domain additions (e.g. Subscriptions via `_changes`, Auth middleware, dynamic indexing), or cross-layer refactors in CouchGraph.
- **Does not cover**: Single-line typo fixes or pure documentation formatting (use `/couchgraph document`).

---

### Triggers

- `/couchgraph radioactive [feature description]`
- `/radioactive [feature description]`
- "Implement new feature with full quality lifecycle"
- "Full-cycle development for CouchGraph"

---

### Inputs

- `featureDescription`: Short text describing the feature or task.
- `projectSlug`: `couchgraph`
- `baseBranch`: `main`
- `techStack`: `Go 1.26 + gqlgen + Kivik v4 + CouchDB 3.x + UUIDv7`
- `existingRootDoc`: `AGENTS.md`
- `uiInvolved`: Boolean (default: `false` for backend GraphQL service)
- `needsPrototype`: Boolean (default: `false`)
- `maxFixAttempts`: Maximum iterations for Phase 12 fix loop (default: 3)

---

### Invariants (Guardrails)

1. **Phase 1 Execution Order**: Phase 1 MUST ALWAYS execute the primary CouchGraph project skill ([`skills/couchgraph/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/SKILL.md)) **FIRST** to classify the request and extract hard constraints, and **THEN** execute `/wayfinder` for codebase mapping.
2. **Sequential Phase Pipeline**: All 14 phases must execute in exact numerical order without skipping.
3. **UUIDv7 Primary Key Mandatory Rule**: Every entity primary key or generated document ID MUST use UUIDv7 (`uuid.NewV7()`).
4. **Socratic Grilling Gate**: Phase 2 must present grilling questions and wait for explicit user confirmation before Phase 3 domain modeling.
5. **Spec Approval Gate**: Phase 7 must generate a structured task specification at `docs/tasks/task.<reference>.md` and obtain explicit user approval before Phase 8/9 execution.
6. **Executable Blueprint Contract**: Phase 8 must scaffold a reusable workflow contract at `skills/couchgraph/workflows/<feature-slug>/SKILL.md`.
7. **Pass/Fail Quality Gate**: Phase 10 tests and builds must pass 100% (`go test ./...`, `go vet ./...`, `go build ./...`). Phase 12 fix loop must resolve all 🔴 (Blocker) and 🟠 (High) findings before declaration of completion.
8. **Institutional Memory**: Phase 13 must capture newly discovered patterns into workflow contracts.

---

### Procedure

```
Phase 1  — CLASSIFY & WAYFIND  → CouchGraph Skill FIRST ➔ /mattpocock/wayfinder
Phase 2  — GRILLING            → /mattpocock/grilling (Socratic interrogation & decision log)
Phase 3  — DOMAIN MODELING     → /mattpocock/domain-modeling (Entities, UUIDv7 keys, state machines)
Phase 4  — TECHNICAL RESEARCH  → /mattpocock/research (Spikes, Kivik/CouchDB API investigation)
Phase 5  — PROTOTYPE / SPIKE   → /mattpocock/prototype (Conditional: POC for high-risk logic)
Phase 6  — UX DESIGN           → /ui-ux-pro-max (Conditional: if UI involved)
Phase 7  — TO-SPEC             → /mattpocock/to-spec (Synthesize into docs/tasks/task.<ref>.md)
Phase 8  — BLUEPRINT CONTRACT  → /workflow-blueprint (Scaffold reusable feature contract)
Phase 9  — EXECUTE             → Implement tasks sequentially with evidence-based edits
Phase 10 — TESTS               → Run go test ./..., go vet ./..., go build ./... (100% pass)
Phase 11 — REVIEW              → /thermo-nuclear-code-quality-review or /review
Phase 12 — FIX LOOP            → /thermo-fix × up to 3 (resolve 🔴→🟠, re-verify build)
Phase 13 — BLUEPRINTS UPDATE   → Update skills/couchgraph/ with institutional memory
Phase 14 — CHANGELOG           → /changelog (Release notes)
```

#### Phase 1 — CLASSIFY & WAYFIND (CouchGraph Skill ➔ /wayfinder)
1. **Load CouchGraph Skill FIRST**: Read [`skills/couchgraph/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/SKILL.md) to classify the request and extract hard invariants (UUIDv7, schema-first gqlgen, thin resolvers, DataLoader batching).
2. **Execute `/wayfinder`**: Load [`skills/couchgraph/workflows/mattpocock/wayfinder/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/mattpocock/wayfinder/SKILL.md) to map route handlers, schema definitions, Kivik client methods, and repository layers. Output `wayfinder.md`.

#### Phase 2 — GRILLING with /mattpocock/grilling
1. Load [`skills/couchgraph/workflows/mattpocock/grilling/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/mattpocock/grilling/SKILL.md).
2. Formulate 3–5 strategic questions covering scope, data contracts (UUIDv7 keys), and performance.
3. **Wait for user response** and record into `decision-log.md`.

#### Phase 3 — DOMAIN MODELING with /mattpocock/domain-modeling
1. Load [`skills/couchgraph/workflows/mattpocock/domain-modeling/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/mattpocock/domain-modeling/SKILL.md).
2. Define GraphQL types, CouchDB document structures, UUIDv7 keys, and state transitions using Mermaid diagrams. Output `domain-model.md`.

#### Phase 4 — TECHNICAL RESEARCH with /mattpocock/research
1. Load [`skills/couchgraph/workflows/mattpocock/research/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/mattpocock/research/SKILL.md).
2. Test Kivik API endpoints or CouchDB capabilities in `scratch/`. Output `research-spike.md`.

#### Phase 5 — PROTOTYPE / SPIKE with /mattpocock/prototype (Conditional)
1. Run if `needsPrototype` is true. Validate high-risk algorithmic or transport logic. Output `prototype-report.md`.

#### Phase 6 — UX DESIGN (Conditional)
1. Run if `uiInvolved` is true.

#### Phase 7 — TO-SPEC with /mattpocock/to-spec
1. Consolidate evidence into `docs/tasks/task.<reference>.md`.
2. **Gate on explicit user approval** before proceeding to execution.

#### Phase 8 — BLUEPRINT CONTRACT with /workflow-blueprint
1. Transform approved spec into an executable contract at `skills/couchgraph/workflows/<feature-slug>/SKILL.md`.
2. Register the workflow in [`skills/couchgraph/reference/routing-matrix.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/reference/routing-matrix.md).

#### Phase 9 — EXECUTE
1. Execute tasks sequentially: schema edits (`internal/graph/schema/`), code generation (`gqlgen generate`), repository logic (`internal/couch/`), resolver wiring (`internal/graph/resolver/`).

#### Phase 10 — TESTS
1. Run `go test -v -race ./...`, `go vet ./...`, and `go build ./...`. Ensure 100% pass rate.

#### Phase 11 — REVIEW with Code Quality Engine
1. Load [`skills/couchgraph/workflows/thermo-nuclear-code-quality-review/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/thermo-nuclear-code-quality-review/SKILL.md) or [`skills/couchgraph/workflows/review/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/review/SKILL.md). Output severity findings.

#### Phase 12 — FIX LOOP (up to 3x)
1. Load [`skills/couchgraph/workflows/thermo-fix/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/thermo-fix/SKILL.md) to eliminate all 🔴 Blocker and 🟠 High findings.

#### Phase 13 — BLUEPRINTS UPDATE
1. Update `skills/couchgraph/` with new domain patterns or constraints learned during the task.

#### Phase 14 — CHANGELOG Generation
1. Load [`skills/couchgraph/workflows/changelog/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/changelog/SKILL.md) to generate release notes.

---

### Review Gate

- [ ] CouchGraph project skill loaded FIRST in Phase 1 before running `/wayfinder`.
- [ ] Phase 1 Wayfinder brief (`wayfinder.md`) created.
- [ ] Phase 2 Socratic grilling questions answered and logged in `decision-log.md`.
- [ ] Phase 3 Domain model defined with UUIDv7 primary keys.
- [ ] Phase 7 Task spec (`docs/tasks/task.<reference>.md`) approved by user.
- [ ] Phase 8 Reusable workflow contract created.
- [ ] Phase 10 `go test ./...` and `go build ./...` pass 100%.
- [ ] Phase 12 Zero 🔴 or 🟠 findings remaining.
- [ ] Phase 13 Project skills updated with institutional memory.
- [ ] Phase 14 Release changelog generated.

---

### References

- [`../../SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/SKILL.md)
- [`../mattpocock/wayfinder/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/mattpocock/wayfinder/SKILL.md)
- [`../mattpocock/grilling/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/mattpocock/grilling/SKILL.md)
- [`../mattpocock/domain-modeling/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/mattpocock/domain-modeling/SKILL.md)
- [`../mattpocock/research/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/mattpocock/research/SKILL.md)
- [`../mattpocock/prototype/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/mattpocock/prototype/SKILL.md)
- [`../mattpocock/to-spec/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/mattpocock/to-spec/SKILL.md)
- [`../review/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/review/SKILL.md)
- [`../changelog/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/changelog/SKILL.md)
