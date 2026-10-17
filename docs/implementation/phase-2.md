# Phase 2: first executing workflow

Status: in progress. Baseline: `f4ded52` (pushed Phase 1).

## Scope

Build a usable local author → publish → execute → inspect path. Keep `RunnelixAdmissionV1` unchanged for existing admission-only records. New explicit execution requests use a separate runtime workflow and a pinned execution mode. Existing development runs are never silently converted into actions.

Nine slices:

1. Execution-mode routing and PostgreSQL payload/invocation/event contracts.
2. Deterministic Go graph interpretation, bounded mapping, and conditions.
3. Authenticated internal node data plane, HTTP action, and credential references.
4. Execution API, signed webhooks, cancellation, SSE, and inspection.
5. TypeScript editor foundation, workflow list, schema forms, and typed requests.
6. Graph canvas, mappings, publication, and pinned-version run inspector.
7. Actual runtime and security failure tests.
8. Browser/keyboard/narrow-layout fixes and repeatable local startup/CI.
9. Complete docs, evidence, checks, and push.

The local mock demonstration is agent-verifiable. A teammate's unassisted acceptance test is a separate human gate; record it as pending until someone performs it. Do not call this a production deployment.

## Product decisions

Keep the existing PostgreSQL and Temporal services. Use a versioned engine workflow, a Go activity boundary for database/control work, and TypeScript activities for internal action modules. Use a protected internal HTTP service for scoped data and approved provider requests; workers do not need database-owner credentials.

The editor follows the existing plan: React, TypeScript, Vite, and React Flow. Add no extra state-management framework. Store graph meaning on the server and editor selection/positions locally. Expose a keyboard-accessible node list alongside the canvas.

Production OIDC/roles, generalized merge/Map/wait/schedules, real integrations, and operational release evidence follow in their planned phases. Do not reduce those gates merely to mark the project finished.
