# Phase 1: contracts and persistence

Status: in progress. Baseline: `a2bfeba` (Phase 0). Nine meaningful commits are planned.

## Outcome and boundary

A developer can validate a workflow, save a revision, publish an immutable plan, and durably admit one run per trigger key. PostgreSQL transactions own admission. Temporal receives a stable run ID through a recoverable outbox.

Phase 1 supports one manual trigger and pure mock actions. It compiles a bounded DAG but does not execute the graph. Its runtime admission workflow records a durable start receipt; execution and run projections arrive in Phase 2. No user can mistake admission for action completion.

The API is a loopback-only development interface. Workspace routing enforces data boundaries, but it does not authenticate a user. OIDC, roles, real credentials, public webhooks, production deployment, and full graph interpretation remain later work.

## Nine slices

1. Workflow/plan schemas, catalog interfaces, graph validation, deterministic compilation, and fixtures.
2. Internal TypeScript node host, mock module, activity envelope, and shared contract tests.
3. PostgreSQL migrations and isolated database verification harness.
4. Workspace-scoped drafts, optimistic revisions, and immutable publication.
5. Transactional inbox/outbox admission and stable run identity.
6. Leased dispatch, fenced acknowledgement, and durable runtime receipts.
7. Development HTTP API, OpenAPI baseline, and process entry points.
8. Real PostgreSQL/Temporal failure-window integration evidence.
9. CI, operational documentation, final checks, and phase ledger.

Every slice updates HISTORY.md and the dated decision log. Tests must establish the relevant behavior before completion; no empty commits to reach the count.

## Infrastructure check

Intent: product development. Build the simple version first. PostgreSQL supplies transactions and constraints; Temporal remains the selected runtime. Use a PostgreSQL outbox, not another message broker. Use one Go module and one TypeScript package, not independently managed services.

Measurable outcome for this phase: concurrent duplicate admission and dispatch recovery produce one product run and one durable runtime receipt, with pinned published content. Verify locally during this phase; no production traffic target is claimed.

Local cost: existing machine, no purchased service. Future monthly cost remains compute + PostgreSQL/runtime storage + backups + operations time; traffic, 10x load, and provider prices are unknown. No broker/cache is justified until measured database contention prevents the agreed service target. Stop promotion if admission atomicity, scope checks, or dispatch recovery fail. Preserve test state and evidence instead of deleting failed databases.
