# Phase 0 — runtime proof

Status: development proof complete in nine meaningful commits. This is a development spike, not the production application. [ADR 0001](../decisions/0001-temporal-development-runtime.md) accepts continued development with Temporal and keeps the production operations gate open.

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

The CLI reports embedded Server **1.32.0** and UI **2.54.1**. The integration suite runs headless. CLI and server version numbers are different.

## Sources

- [Go SDK](https://github.com/temporalio/sdk-go) and [TypeScript SDK](https://github.com/temporalio/sdk-typescript).
- [CLI v1.9.1 release](https://github.com/temporalio/cli/releases/tag/v1.9.1), including checksums.
- [Temporal server CLI documentation](https://docs.temporal.io/cli/command-reference/server).
- [Phase 0 plan](../plans/08-delivery-plan.md#phase-0-prove-the-runtime-boundary).

## Run it

From the repository root:

```sh
make tools
make deps
make test check
make integration
```

The first command downloads the fixed CLI release into `.tools/`. It verifies the archive SHA-256 before extracting the executable. A slow connection can make the 43 MB macOS ARM64 download take several minutes. The installer has a connection timeout and a maximum download duration.

Go automatically downloads the toolchain required by `go.mod`. `pnpm install --frozen-lockfile` uses the committed dependency graph. Native build scripts are explicitly allowed for SWC, the Temporal bridge, and protobufjs; arbitrary package scripts are not globally permitted.

`make integration` compiles the Go executables and TypeScript worker. The Python harness chooses loopback ports, starts its own services, runs the cases, and terminates only those process groups. It keeps the development database, fake provider ledger, requests, logs, and results under a unique `artifacts/phase0/` directory. Do not use real secrets or production data.

For a shorter check after building:

```sh
python3 tests/integration/phase0.py --suite base
python3 tests/integration/phase0.py --suite recovery
python3 tests/integration/phase0.py --suite full
```

The full suite includes the base and recovery suites. Missing prerequisites and assertion failures produce a nonzero exit. Inspect the preserved logs when a process fails to start. Do not delete the database to make a failing recovery assertion pass.

## Observed results

The recorded full run is [phase-0-full.json](evidence/phase-0-full.json). It records UTC timestamps, runtime/tool versions, elapsed case durations, and SHA-256 values for the tested source files. These are correctness timings, not throughput or latency benchmarks.

| Check | Observed result |
| --- | --- |
| Cross-language round trip | Passed: Unicode, nulls, arrays, numbers, and large-integer strings preserved. |
| Transient retry | Passed: two requests, one external effect. |
| Safe response loss after commit | Passed: retry returned the original effect. |
| Unsafe response loss after commit | Passed: one request and a needs-attention result. |
| Worker death during wait | Passed: both workers restarted; persisted timer resumed. |
| Worker death after safe write | Passed: retry completed with one effect. |
| Worker death after unsafe write | Passed: no automatic second request. |
| Cancellation during wait | Passed: no provider request. |
| Cancellation during active action | Passed: workflow cancelled; already committed effect remained. |
| Temporal server crash | Passed: same SQLite database recovered. |
| Version routing | Passed: v2 queues did not execute the old v1 workflow/action. |
| Cold snapshot restore | Passed: earlier runtime snapshot replayed a safe action; live provider retained one effect. |

Other checks: `go test -race ./...`, `go vet ./...`, TypeScript compilation/type checks, all 21 TypeScript tests, and `git diff --check`. CI is defined in [phase0.yml](../../.github/workflows/phase0.yml); a local pass is not a hosted CI pass.

## Cold restore procedure used

1. Start a safe-action workflow with a durable wait.
2. Stop both workers and the development server.
3. Use SQLite's backup API to copy runtime state into a separate directory; require `PRAGMA integrity_check` to return `ok`.
4. Restart the original runtime and complete the action against the mock provider.
5. Stop the original runtime and workers again.
6. Start the restored runtime copy while keeping the provider's later state.
7. Resume the old run and verify two requests reference one external effect.

This is deliberately a cold development snapshot. It does not prove PostgreSQL point-in-time recovery, runtime/application database alignment, encryption-key restore, or safe recovery for a non-idempotent external write.

## Commit ledger

| Slice | Commit | Change |
| --- | --- | --- |
| 1 | `cfd2a15` | Tooling pins and evidence records. |
| 2 | `da5a1b8` | Shared Go/TypeScript contract and fixtures. |
| 3 | `8eb08f6` | Persistent provider fixture and fault controls. |
| 4 | `b9fa212` | Go workflow, CLI, and retry policies. |
| 5 | `b857451` | TypeScript activity executor and worker. |
| 6 | `dbfed23` | Isolated real-runtime harness. |
| 7 | `2f2f063` | Live crash/cancellation tests and evidence. |
| 8 | `bf513cf` | Version isolation and cold restore evidence. |
| 9 | Commit containing this completed ledger | CI, setup instructions, final checks, and conditional runtime decision. |

The final commit identifies itself through Git history to avoid a self-referential commit hash. Each slice also updates [HISTORY.md](../../HISTORY.md) and the dated decision log.

## Remaining product work

Phase 1 has not started. The spike does not implement a graph engine, web UI, PostgreSQL product records, admission outbox, authentication, credential encryption, payload-reference storage, real integration nodes, or production egress controls. Its `needs_attention` response is terminal; the planned product reconciliation state is not implemented.

Version routing uses explicit application task queues. It does not test Temporal's deployment-version API. Old workers remain necessary, and artifact retirement must be designed for production.
