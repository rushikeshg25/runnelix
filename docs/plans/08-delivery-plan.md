# Delivery plan

[Planning guide](README.md)

## Estimation boundary

Plan for **14–22 engineer-weeks** for the defined first release. This is a rough effort estimate, not a delivery commitment. Runtime operations, editor polish, and provider behavior are the largest uncertainties.

Two experienced developers can overlap some UI and backend work after contracts stabilize. Effort does not divide directly into calendar time. The pilot also needs at least two calendar weeks of observed use.

The proposed 2027-01-31 adoption date assumes work starts by early November 2026 with sufficient staffing. Re-estimate after Phase 0 and after the first complete workflow.

## Phase 0: prove the runtime boundary

**Effort:** 1–2 engineer-weeks. **Depends on:** access to the intended development environment and a provisional operating budget.

Build one Go workflow that executes one TypeScript action against a mock HTTP endpoint. Demonstrate a durable wait, retry-safe action, uncertain unsafe action, cancellation, and worker restart.

Verify the selected Go SDK, TypeScript SDK, Temporal server, and JSON converter work together. Demonstrate compatible worker routing across an upgrade. Attempt a basic self-hosted backup/restore.

**Deliverables:** runnable spike, recorded failure demonstration, exact version matrix, benchmark/restore notes, and a Temporal decision.

**Exit gate:** A-01 passes. If it fails, revise the runtime or reduce scope before investing in the editor. Do not conceal a failed runtime decision behind an adapter framework.

## Phase 1: contracts and persistence

**Effort:** 2–3 engineer-weeks. **Depends on:** Phase 0 runtime decision.

Define the workflow/plan format, node catalog, activity envelope, OpenAPI baseline, and shared contract fixtures. Implement draft revisions, immutable publication, graph validation, workspace scoping, inbox/outbox admission, and stable run identity.

Set up migrations, formatting, static checks, test commands, and CI appropriate to the chosen tools. Build the internal node host and its fixture harness with one mock action.

**Deliverables:** validated workflow JSON can be published and accepted durably without a polished canvas.

**Exit gate:** invalid graphs fail with useful errors; repeated trigger keys do not create another intended run; admission/dispatch crash windows are covered.

## Phase 2: first complete workflow

**Effort:** 2–3 engineer-weeks. **Depends on:** stable initial contracts.

Build a minimal canvas, schema forms, mapping UI, manual/webhook trigger, HTTP action, condition, and run inspector. Add credential references, payload access, structured errors, SSE, and cancellation.

Use the product brief's mock-endpoint example. Keep the slice end to end: the user can author, publish, execute, inspect, and diagnose it.

**Exit gate:** a teammate completes the happy path and understands an injected failure. The interface shows the exact executed version and branch.

## Phase 3: complete execution semantics

**Effort:** 2–3 engineer-weeks. **Depends on:** the first full workflow.

Add deterministic merge/skip behavior, bounded Map, durable waits, schedule admission, no-overlap slots, catch-up policy, quotas, provider limits, uncertainty handling, and independent projection recovery.

Run failure injection around each transaction and network boundary. Test old runs during an upgrade and bounded history growth under iteration.

**Exit gate:** execution semantics, schedule/DST cases, crash recovery, and the initial load envelope pass.

## Phase 4: prove extensibility with real integrations

**Effort:** 2–3 engineer-weeks. **Depends on:** stable executor and credential contracts.

Choose two integrations from pilot requirements. Prefer different authentication and retry behavior so the contract receives a meaningful test.

Add modules, schemas, fixtures, docs, and catalog entries through the ordinary node-authoring path. Complete needed OAuth/API-key handling, node-version migration examples, and workflow import/export.

**Exit gate:** N-01 passes. A developer outside the engine implementation adds a normal action without changing core execution or ordinary editor form logic.

## Phase 5: production hardening

**Effort:** 3–5 engineer-weeks. **Depends on:** complete first-release behavior.

Finish OIDC and role enforcement, credential audits, egress controls, payload retention, metrics/alerts, deployment packaging, migration safety, upgrade/rollback instructions, and timed restore drills.

Complete dependency/license review and desktop/narrow-screen/keyboard QA. Run the release suite against the intended installation, not just a developer machine.

**Exit gate:** all release requirements in [Verification](09-verification.md) have evidence. Document remaining limitations and the operational owner.

## Phase 6: controlled pilot

**Effort:** 2–3 engineer-weeks, with at least two calendar weeks of observed use. **Depends on:** production gates.

Move five useful workflows into service. Record active users, incident causes, recovery effort, unclear errors, and node-authoring friction. Fix the failures exposed by real usage.

**Exit gate:** three teammates use the workflows weekly for two consecutive weeks, incidents are reviewed, and the owner explicitly accepts the deployment's operating limits.

## Work that can overlap

After Phase 1 contracts stabilize, the interface and backend can progress in parallel. Node fixture development can begin while run inspection is being completed. Runbook drafting can begin before hardening, but restore evidence must come from an actual deployment.

Do not parallelize work across an unresolved runtime contract simply to keep everyone busy. The first spike removes that uncertainty cheaply.

## First implementation tickets

These are proposed local work items, not GitHub issues created by this plan:

1. Record the runtime/server/SDK version matrix and local startup path.
2. Execute a TypeScript mock action from Go using a versioned JSON envelope.
3. Add controlled provider idempotency and an unsafe-write endpoint to the mock.
4. Inject crashes before sending, after remote success, and before reporting completion.
5. Demonstrate a durable wait across worker restarts.
6. Route an old waiting run to a compatible worker after deploying a new version.
7. Restore the spike data and explain the external-effect reconciliation result.
8. Write the runtime decision and re-estimate the next slice.

## Completion discipline

Each slice ends with a demonstration, relevant checks, and a written list of remaining gaps. A successful build is not proof of browser behavior, deployment safety, or production recovery.

Update the plan when evidence changes a decision. Keep requirement and verification IDs stable. Do not mark a phase complete while its exit gate remains unverified.

Next: [Verification](09-verification.md).
