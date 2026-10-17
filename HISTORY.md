# Runnelix history

Scope: product planning and implementation. Dates use Asia/Kolkata unless stated otherwise. This record links evidence and distinguishes completed work from proposals.

## 2026-10-17 — product plan published

- **What:** named the product Runnelix and published the planning documents in public repository `rushikeshg25/runnelix`.
- **Why:** establish readable product requirements, execution rules, and release gates before implementation.
- **Evidence:** commit [`f3f6864`](https://github.com/rushikeshg25/runnelix/commit/f3f6864264c08aa2f0b4c611c7082b346eee111b), [planning guide](docs/plans/README.md).
- **Confidence:** confirmed by the committed documents and matching remote file hashes.

## 2026-10-17 — Phase 0 implementation begins

- **What:** adopted nine meaningful commits for the runtime proof, with per-slice decisions and evidence.
- **Why:** the user requested phased implementation, 8–10 commits per phase, and a durable record of what changed and why.
- **Choice:** use a project-local Temporal development server because Docker is not running. Keep production PostgreSQL recovery unverified rather than equating it with a development restore.
- **Evidence:** [Phase 0 implementation record](docs/implementation/phase-0.md), [tool installer](scripts/install-temporal.py), [decision log](logs/2026-10-17.md).

## Open questions

- Which production topology and budget should the Temporal operations gate use?
- Which real integrations and pilot workflows come first?

Per-step reasoning lives in `logs/YYYY-MM-DD.md`; this history records milestones and links to their evidence.

## 2026-10-17 — shared invocation contract

- **What:** added a bounded Phase 0 request contract, JSON Schema, and shared Go/TypeScript fixtures.
- **Why:** cross-language workers must agree on required fields, unknown fields, version IDs, numeric limits, Unicode, and payload limits before executing actions.
- **Evidence:** [contract fixtures](contracts/spike/fixtures.json), [Go validation/tests](internal/spike/contract_test.go), [TypeScript tests](node-worker/contract.test.ts). Go tests and all 15 TypeScript tests passed.
- **Prior milestone:** tooling commit `cfd2a15`.

## 2026-10-17 — durable mock provider

- **What:** added a loopback-only provider fixture with a persistent fake-action ledger, safe/unsafe writes, pre-write transient failures, lost responses, and held responses.
- **Why:** recovery tests need an observable external effect independent of Temporal's activity result.
- **Evidence:** [provider tests](internal/mockprovider/server_test.go), [server](internal/mockprovider/server.go). `go test -race ./internal/mockprovider` passed, including concurrent deduplication and post-commit disconnect/reload.
- **Prior milestone:** shared-contract commit `da5a1b8`.

## 2026-10-17 — Go runtime orchestration

- **What:** implemented one durable wait followed by a version-routed TypeScript activity, plus start/query/result/cancel/describe commands.
- **Why:** prove the runtime boundary before building a graph engine. Unsafe actions receive one attempt; ambiguous failures return a distinct needs-attention result.
- **Evidence:** [workflow tests](internal/spike/workflow_test.go), [CLI](cmd/spike/main.go). Go tests prove safe retry, unsafe non-retry, pre-send failure handling, and cancellation during a durable wait.
- **Limit:** needs-attention is a completed spike result, not the product's eventual paused reconciliation state.
- **Prior milestone:** provider commit `8eb08f6`.

## 2026-10-17 — TypeScript action worker

- **What:** added the Temporal activity worker, fixed loopback destination, request/result validation, response-size cap, cancellation, heartbeats, and sanitized typed failures.
- **Why:** provider-specific work belongs outside replayed Go workflow code; raw network errors must not leak into execution history.
- **Evidence:** [activity tests](node-worker/activity.test.ts), [worker](node-worker/main.ts). All 21 TypeScript tests pass; cross-process runtime checks follow.
- **Prior milestone:** Go orchestration commit `b9fa212`.

## 2026-10-17 — real-runtime test harness

- **What:** added an integration harness that starts isolated Temporal/provider/Go/TypeScript processes, preserves per-run artifacts, and stops only its own process groups.
- **Why:** unit tests do not prove cross-language transport or remote-effect behavior. The baseline cases assert payload round trips, safe retries, and unsafe uncertainty against a durable provider ledger.
- **Evidence:** [harness](tests/integration/phase0.py). `make build` and Python compilation passed. Live execution is pending the development-server download at this milestone.
- **Blocker:** the CLI release download was slow; its partial archive was preserved and the remaining ranges were downloaded concurrently. Full-archive SHA-256 verification is required before execution.
- **Prior milestone:** TypeScript worker commit `b857451`.

## 2026-10-17 — live crash and cancellation evidence

- **What:** ran ten integration cases against Temporal CLI 1.9.1 with embedded Server 1.32.0, real Go and TypeScript workers, and the persistent mock provider.
- **Why:** validate process failure boundaries rather than infer recovery from unit tests.
- **Observed:** all ten cases passed: cross-language data, safe transient retry, safe/unsafe response loss, worker restart across a wait, safe/unsafe worker death after commit, waiting/active cancellation, and server crash recovery.
- **Evidence:** [machine-readable results](docs/implementation/evidence/phase-0-recovery.json), [failure harness](tests/integration/phase0.py). The CLI archive matched its pinned upstream SHA-256.
- **Limits:** localhost development server and SQLite persistence; no production PostgreSQL restore, throughput target, or production isolation claim.
- **Prior milestone:** harness commit `dbfed23`.
