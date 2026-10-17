# ADR 0001: continue development with Temporal

Date: 2026-10-17. Status: accepted for continued development; production operations approval remains open.

## Context

Runnelix needs durable waits and provider-aware retries across Go and TypeScript. The product plan proposed Temporal but required runtime evidence before deeper implementation.

The current host has a stopped Docker daemon. The production topology, budget, and operator are not selected. A local development proof can establish transport and failure behavior, but cannot validate those operational choices.

## Decision

Use Temporal for the next development phase. Keep the Go workflow boundary, TypeScript activity host, and explicit task contracts. Do not build a competing PostgreSQL execution engine.

Treat A-01 as two recorded parts:

- **Technical feasibility: passed for the narrow development spike.** Go/TypeScript transport, retry behavior, waits, cancellation, worker/server failure, application-level version routing, and cold development-state restore have evidence.
- **Production operations: open.** PostgreSQL recovery, keys, topology, capacity, budget, and operational ownership need separate evidence before a production rollout.

This refines the earlier all-or-nothing Phase 0 gate. Continued reversible development does not depend on selecting paid infrastructure now. It does not waive production release checks or claim that the development server is production-ready.

## Evidence

[The full report](../implementation/evidence/phase-0-full.json) records twelve passed cases, exact versions, and source-file hashes. The runtime is Temporal CLI 1.9.1 with embedded Server 1.32.0. Workers use Go SDK 1.50.0 and TypeScript SDK 1.24.0.

The cold restore deliberately preserves the provider's later state. A replayed safe action made a second request but kept one external effect. This supports the documented distinction between runtime recovery and provider idempotency.

## Alternatives considered

- **Build a PostgreSQL runner now:** fewer runtime services, but more scheduling/recovery responsibilities before the product has a working end-to-end slice.
- **Pause all development until production hosting is selected:** blocks independent contract and application work without resolving it faster.
- **Claim A-01 is entirely passed:** unsupported; SQLite cold backup is not PostgreSQL PITR or a production operating test.

## Limits and consequences

- The spike runs one action, not a DAG.
- `needs_attention` is a completed spike result, not the product's paused reconciliation state.
- Version-specific queues prove application routing. Temporal worker deployment-version APIs and artifact retirement remain untested.
- Payloads are fake and appear in runtime history. The future production payload-reference and credential boundaries are not implemented.
- No authentication, UI, admission outbox, product PostgreSQL schema, production connector, or production egress policy exists yet.
- The CLI development server and mock provider must remain loopback-only.
- Non-idempotent writes after disaster rollback require an admission hold and operator reconciliation design. This spike does not make them safe automatically.

## Revisit when

Revisit this decision if the intended self-hosted PostgreSQL deployment cannot meet agreed recovery, capacity, cost, or maintenance limits. Before the pilot, demonstrate restore of application/runtime state and keys, then reconcile provider effects. Do not bypass that evidence with a successful development test.
