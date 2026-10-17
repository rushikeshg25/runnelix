# Phase 0 — runtime proof

Status: implementation started. This is a development spike, not the production application.

## Scope and evidence boundary

Prove Go-to-TypeScript activity execution, retry safety, durable waits, cancellation, process recovery, version-specific routing, and a cold development-state restore. The production PostgreSQL topology and operating budget are not yet selected. A local SQLite restore cannot satisfy that production gate.

The host initially has Go 1.24.6, Node 24.2.0, pnpm 11.5.0, and a stopped Docker daemon. Go's toolchain manager can fetch the module's Go 1.26.0 requirement. No global tool installation or existing container startup is required.

## Nine-commit sequence

1. Pin tooling and establish history/evidence records.
2. Define and test the Go–TypeScript invocation contract.
3. Build a persistent mock provider with idempotency and failure controls.
4. Implement the Go workflow and command-line host.
5. Implement the TypeScript activity worker and safe error classification.
6. Verify cross-language happy path, retry, and uncertain outcomes against the actual runtime.
7. Inject worker crashes and verify durable waits and cancellation.
8. Verify version-specific task queues and cold development-state restore.
9. Add CI, operating instructions, evidence, and the runtime decision.

Each commit must include a decision entry and a history update. Commit subjects identify the slice. Do not add empty commits to reach the count. Record test failures and limitations as they occur.

## Version choices

| Component | Pin | Reason |
| --- | --- | --- |
| Go language/toolchain | 1.26.0 | Required by the selected Go SDK; downloaded per-module instead of replacing host Go. |
| Temporal Go SDK | 1.50.0 | Current published SDK at the start of the spike. |
| Temporal TypeScript SDK | 1.24.0 | Current published worker package; Node support checked through package metadata. |
| Temporal CLI | 1.9.1 | Current published CLI; installer checks a fixed release SHA-256. |
| Node | 24.2.0 on the observed host | Meets the worker SDK requirement; CI will use maintained Node 24. |
| pnpm | 11.5.0 | Existing host package manager, pinned in package metadata. |
| TypeScript | 5.9.3 | Explicit stable compiler pin; no need to adopt a new compiler major for the spike. |

The CLI's embedded server version must be recorded from a running instance. CLI and server version numbers are different.

## Sources

- [Go SDK](https://github.com/temporalio/sdk-go) and [TypeScript SDK](https://github.com/temporalio/sdk-typescript).
- [CLI v1.9.1 release](https://github.com/temporalio/cli/releases/tag/v1.9.1), including checksums.
- [Temporal server CLI documentation](https://docs.temporal.io/cli/command-reference/server).
- [Phase 0 plan](../plans/08-delivery-plan.md#phase-0-prove-the-runtime-boundary).

## Results

Pending. Commands and observed outcomes will be added as each slice is verified.
