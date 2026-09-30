# Routing Matrix

> Task classification and workflow resolution guide for **CouchGraph**.

---

## 1. Task Classification Table

| Task Category | Intent Triggers | Slash Command | Canonical Workflow |
|---|---|---|---|
| **Full Feature / Architecture** | "Add new feature", "Build subscription support", "Add auth middleware", "Complex domain change" | `/couchgraph radioactive` or `/radioactive` | [`workflows/radioactive/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/radioactive/SKILL.md) |
| **Codebase Orientation** | "Where is X implemented?", "Map codebase", "Explore architecture", "Find entrypoint" | `/couchgraph wayfinder` or `/wayfinder` | [`workflows/mattpocock/wayfinder/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/mattpocock/wayfinder/SKILL.md) |
| **Requirements Clarification** | "Interview me", "Clarify requirements", "Grill me on design", "Ask questions about scope" | `/couchgraph grilling` or `/grilling` | [`workflows/mattpocock/grilling/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/mattpocock/grilling/SKILL.md) |
| **Domain & Schema Modeling** | "Design schema", "Model documents", "Create entities", "State machine design" | `/couchgraph domain-modeling` | [`workflows/mattpocock/domain-modeling/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/mattpocock/domain-modeling/SKILL.md) |
| **Technical Investigation** | "Research dependency", "Spike performance", "Test Kivik API", "Explore CouchDB endpoint" | `/couchgraph research` | [`workflows/mattpocock/research/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/mattpocock/research/SKILL.md) |
| **POC / Prototype** | "Build proof of concept", "Validate prototype", "Quick spike" | `/couchgraph prototype` | [`workflows/mattpocock/prototype/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/mattpocock/prototype/SKILL.md) |
| **Specification Synthesis** | "Write task spec", "Synthesize findings into plan", "Create formal spec" | `/couchgraph to-spec` | [`workflows/mattpocock/to-spec/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/mattpocock/to-spec/SKILL.md) |
| **Architecture Diagramming** | "Generate C4 diagram", "Document system architecture", "Create Mermaid diagrams" | `/couchgraph c4-architecture` | [`workflows/c4-architecture/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/c4-architecture/SKILL.md) |
| **Documentation Update** | "Update README", "Document endpoints", "Update API guide", "Fix doc links" | `/couchgraph document` or `/document` | [`workflows/document/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/document/SKILL.md) |
| **Code Review & Audit** | "Review code", "Audit PR", "Check code quality", "Find bugs / vulnerabilities" | `/couchgraph review` or `/review` | [`workflows/review/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/review/SKILL.md) |
| **Deep Nuclear Quality Audit** | "Thermonuclear review", "Deep quality review", "Strict code audit" | `/couchgraph thermo-review` | [`workflows/thermo-nuclear-code-quality-review/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/thermo-nuclear-code-quality-review/SKILL.md) |
| **Review Fix Loop** | "Fix review findings", "Resolve blocker findings", "Apply quality fixes" | `/couchgraph thermo-fix` | [`workflows/thermo-fix/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/thermo-fix/SKILL.md) |
| **Release Changelog** | "Generate changelog", "Write release notes", "Summarize commits" | `/couchgraph changelog` or `/changelog` | [`workflows/changelog/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/changelog/SKILL.md) |
| **Workflow Blueprint Creation** | "Create skill from plan", "Scaffold new workflow", "Save reusable workflow" | `/couchgraph plan-to-blueprint` | [`workflows/plan-to-blueprint/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/plan-to-blueprint/SKILL.md) |
| **Idea Brainstorming** | "Brainstorm features", "Explore options", "Discuss architecture alternatives" | `/couchgraph brainstorming` | [`workflows/brainstorming/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/brainstorming/SKILL.md) |
| **Task Plan Writing** | "Write implementation plan", "Break down tasks", "Create task list" | `/couchgraph plan-writing` | [`workflows/plan-writing/SKILL.md`](file:///Users/ton/work/couchgraph/skills/couchgraph/workflows/plan-writing/SKILL.md) |

---

## 2. Disambiguation Rules

1. **If the task touches multiple layers** (e.g. GraphQL schema + CouchDB repository + tests): Route to `/couchgraph radioactive`.
2. **If the task is purely documentation**: Route to `/couchgraph document`.
3. **If the task is verifying code quality without changing business logic**: Route to `/couchgraph review`.
