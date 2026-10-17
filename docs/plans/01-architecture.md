# Architecture

[Planning guide](README.md)

## Design objective

Separate workflow control from integration actions. Keep the execution model in Go. Keep integration code in TypeScript. Use one documented contract at their boundary.

The proposed implementation is a modular backend with a small number of process roles. It is not a service-per-node architecture. Separate processes provide independent worker scaling without requiring independently owned microservices.

## Components and ownership

| Component | Owns | Must not own |
| --- | --- | --- |
| React interface | Graph editing, configuration forms, mapping previews, run inspection. | Authoritative graph validation or authorization. |
| Go API | Authentication, workspaces, drafts, publication, manual admission, credential management. | Running external actions inside an HTTP request. |
| Trigger admission | Validated event receipt, deduplication, quotas, pinned run request, transactional outbox. | A second workflow scheduler. |
| Go compiler | Graph checks and immutable execution plans. | Calling providers to discover execution results. |
| Go orchestrator | Run state, branch/merge/Map behavior, durable wait, action dispatch, cancellation. | Provider-specific HTTP code or direct nondeterministic calls in replayed logic. |
| Go activity helpers | Mapping evaluation, payload access, admission, projection persistence. | Hidden changes to graph semantics. |
| TypeScript workers | Versioned integration executors and structured action outcomes. | Direct application SQL, arbitrary next-node scheduling, or public request authorization. |
| PostgreSQL | Product definitions, accepted requests, payloads, audit, query projections. | Competing with Temporal as the live execution state machine. |
| Temporal | Durable orchestration history, timers, task delivery, retries, recovery. | Product permission policy, node definitions, or external exactly-once guarantees. |

## Proposed stack

- Go with `net/http` or chi for the API.
- PostgreSQL with pgx and sqlc; explicit SQL migrations.
- React, TypeScript, Vite, and React Flow for the UI.
- Temporal Go SDK for orchestration and Go helpers.
- Temporal TypeScript SDK for integration activities.
- OpenAPI for HTTP contracts and JSON Schema for node/task contracts.
- A restricted CEL evaluator in Go helpers for mappings and conditions.
- Container images and Compose for local development and a single-host pilot.

Choose supported versions during Phase 0. Check dependency licenses before release. No exact version or cross-language compatibility has been verified by this planning work.

## Why Temporal is proposed

Durable waits and recovery are core requirements. Temporal can supply these mechanisms while Runnelix implements graph behavior. This avoids building a general durable scheduler as part of the first release.

The cost is another system to operate. Workflow logic must also obey replay rules, and deployments must preserve compatible behavior for active runs.

A PostgreSQL job table is the alternative. It uses fewer deployed components, but Runnelix would need leases, fencing, retry scheduling, timer recovery, cancellation handling, and durable execution history. Those are substantial product responsibilities.

**Gate A-01:** keep Temporal only after the runtime spike proves cross-language tasks, worker upgrades, failure recovery, and acceptable self-hosted operations. If that gate fails, revise scope and choose one engine. Do not build both engines or a generic engine adapter framework.

## Normal execution path

1. The API authenticates a manual request, or ingress validates a trigger event.
2. Admission checks workspace permission, trigger state, request size, deduplication, and capacity.
3. One PostgreSQL transaction records the event, the chosen workflow version, the run request, and the start-outbox row.
4. Ingress acknowledges acceptance only after commit. A repeated event returns the existing receipt when its deduplication key matches.
5. The dispatcher starts the main Temporal workflow using the stable product run ID.
6. The Go orchestrator interprets the pinned plan and dispatches versioned node activities.
7. An activity reads authorized payloads and credentials, performs its bounded work, and stores its result.
8. The orchestrator accepts the activity outcome and advances control flow. Separate idempotent work updates the run view.
9. The interface reads run projections and follows Server-Sent Events (SSE). It can refresh from a persisted cursor after disconnecting.

The dispatcher can crash after starting execution but before marking the outbox row sent. Retry must use the same run ID and a duplicate-start policy. Keep application-side start receipts beyond runtime history retention. Do not assume the runtime remembers a completed workflow ID forever.

## Schedule admission

PostgreSQL stores the desired schedule configuration. An outbox reconciles that configuration into one Temporal schedule. Track the desired and observed revision separately.

A scheduled occurrence starts a small admission workflow. It calls a Go activity that submits the occurrence through the same product admission transaction as other triggers. Use `(schedule ID, scheduled timestamp)` as its stable occurrence key. The resulting outbox starts the main workflow.

The admission wrapper can finish before the main run finishes. Therefore, its scheduler overlap setting cannot enforce product-level “one active run.” Admission must reserve an active-run slot transactionally when that policy is enabled. Release the slot on a confirmed terminal run. Reconcile stale slots against runtime state; never free them merely because a timeout elapsed.

This separates schedule delivery from run admission without adding a second cron scheduler. Details are in [Workflow semantics](02-workflow-semantics.md#triggers-and-schedules).

## Payload and result boundary

Pass compact references through orchestration history when practical. Do not include credentials in activity arguments, results, or errors. Mapping helpers can return an output reference and a small branch decision instead of a full sensitive document.

Store immutable attempt results. Use separate keys for logical invocation and attempt identity. A late attempt must not overwrite a later accepted result. The orchestrator selects the canonical invocation result through an idempotent, conditional update.

This local rule prevents stale result overwrites. It does not prevent duplicate effects at a provider; see [Failure recovery](02-workflow-semantics.md#external-actions-and-retry-safety).

## Replay and deployment rules

Workflow code uses the runtime's deterministic APIs. It does not read the wall clock, use arbitrary randomness, query PostgreSQL, or call providers directly. Sort graph edges and map keys whenever their iteration order affects scheduling.

Run nondeterministic work in activities. Version the plan interpreter and pin each node implementation. Phase 0 must demonstrate routing to compatible Go and TypeScript worker deployments using the selected SDK/server versions.

Keep old compatible workers available while active runs need them. Published versions also need an executable path for future runs. Replacing an image under the same mutable tag is not a versioning strategy.

## Repository layout

```text
cmd/api/                 Go HTTP entry point
cmd/orchestrator/        Go workflows and activity workers
internal/workflows/      draft, publication, version operations
internal/compiler/       validation and plan construction
internal/execution/      control-flow implementation
internal/triggers/       admission, webhook inbox, outbox, schedules
internal/credentials/    authorization, encryption, refresh
internal/identity/       membership and role checks
internal/runview/        execution events and projections
contracts/               OpenAPI, JSON Schema, contract fixtures
web/                     React interface
node-worker/             TypeScript activity host
nodes/                   built-in action modules
node-sdk/                context, shared errors, test harness
db/                      SQL queries and migrations
deploy/                  container definitions and runbooks
tests/integration/       actual system-boundary tests
docs/plans/              design and delivery plan
```

Create these directories when their slice needs them. Keep APIs narrow. Prefer explicit SQL queries to a generic repository abstraction that hides transaction boundaries.

## Infrastructure to defer

Do not add Kafka, Redis, Kubernetes, or separate services without a measured need. PostgreSQL can initially coordinate modest quotas and hold bounded JSON payloads. [Operations](07-operations.md) defines switch triggers and the cost model.

Next: [Workflow semantics](02-workflow-semantics.md).
