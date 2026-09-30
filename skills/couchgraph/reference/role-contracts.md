# Role Contracts

> Specialist agent roles, boundaries, and handoff contracts for **CouchGraph**.

---

## 1. Role Definitions

### 1. Orchestrator (`couchgraph`)
- **Mission**: Single point of entry. Classifies user requests, verifies project invariants, routes to specialized workflows, and gates delivery.
- **Boundaries**: Does not write implementation code directly; delegates to workflow contracts.
- **Handoff Output**: Workflow contract execution context.

### 2. Wayfinder & Discovery Specialist (`mattpocock/wayfinder`, `mattpocock/grilling`)
- **Mission**: Map the codebase, discover relevant symbols/entrypoints, interrogate requirements with Socratic grilling, and produce binding decision logs.
- **Boundaries**: Read-only exploration; does not mutate source code.
- **Handoff Output**: `wayfinder.md`, `decision-log.md`.

### 3. Domain Architect (`mattpocock/domain-modeling`, `c4-architecture`)
- **Mission**: Model domain entities, state machines, and Mermaid architecture diagrams. Enforce **UUIDv7** primary keys across all entities.
- **Boundaries**: Abstract data modeling and API contract definitions.
- **Handoff Output**: `domain-model.md`, C4 diagrams.

### 4. Technical Spike & Prototype Specialist (`mattpocock/research`, `mattpocock/prototype`)
- **Mission**: Execute focused spikes, test external APIs (CouchDB, Kivik driver), and build lightweight POCs.
- **Boundaries**: Uses `scratch/` directory for temporary scripts; cleans up before production implementation.
- **Handoff Output**: `research-spike.md`, `prototype-report.md`.

### 5. Spec Writer (`mattpocock/to-spec`, `plan-writing`)
- **Mission**: Consolidate upstream evidence into an actionable task specification at `docs/tasks/task.<ref>.md`.
- **Boundaries**: Does not execute tasks until explicit user approval is granted.
- **Handoff Output**: `docs/tasks/task.<ref>.md`.

### 6. Core Implementer & Executor
- **Mission**: Implement code changes according to the approved spec. Follow schema-first gqlgen conventions, thin resolvers, and DataLoader batching.
- **Boundaries**: Must maintain 100% build and test pass rate.
- **Handoff Output**: Clean, verified code changes.

### 7. Code Quality Auditor (`review`, `thermo-nuclear-code-quality-review`, `thermo-fix`)
- **Mission**: Perform multi-pass code audits, classify findings by severity (🔴 Blocker, 🟠 High, 🟡 Medium, 🟢 Low), and execute iterative fix loops.
- **Boundaries**: Re-verifies builds after every fix.
- **Handoff Output**: Review table with APPROVED verdict.

### 8. Release Chronicler (`changelog`)
- **Mission**: Generate customer-facing release notes categorized by Features, Improvements, and Fixes.
- **Boundaries**: Accurate reflection of session commits against base branch.
- **Handoff Output**: Release changelog.

---

## 2. Handoff Protocol

```
Orchestrator
    │
    ▼
Wayfinder ➔ Grilling ➔ Domain Modeling ➔ Research ➔ To-Spec (User Gate)
                                                            │
    ┌───────────────────────────────────────────────────────┘
    ▼
Executor ➔ Tests ➔ Review ➔ Fix Loop ➔ Blueprints Update ➔ Changelog
```
