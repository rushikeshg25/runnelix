# Phase 3: execution semantics and recovery

Status: in progress. Phase 2 is pushed at `9cd61d1`.

Nine implementation slices:

1. Interpreter version 3, deterministic named/ordered merge, durable wait, and compatibility routing.
2. Bounded Map with explicit single-action item bodies, stable item identities, ordering, and concurrency limits. Nested Map is rejected.
3. Durable operator reconciliation for uncertain outcomes, with state checks and immutable evidence.
4. Run/payload/invocation quotas and shared provider request limits.
5. Temporal-owned schedules, desired-state reconciliation, stable occurrence admission, no-overlap protection, and explicit catch-up/timezone policy.
6. Independent run-projection recovery and artifact-retention reporting.
7. Live control-flow, process-failure, and bounded-load tests.
8. Editor controls and operator inspection for the added semantics.
9. Complete documentation, evidence, verification, and push.

The installation remains local. Existing version 2 workflow code and routing must remain available. New control constructs compile to a new interpreter version. Do not silently change replay behavior of an old execution.

A Map body initially consists of one trusted action descriptor and configuration. This provides explicit per-item external actions without introducing recursive user subworkflows. Each item has its own logical action identity and immutable attempt records. Limits and unsupported forms must fail before dispatch where possible.

Schedules use Temporal as the timer owner. Do not add a competing application cron loop. If the exact proposed coalescing/DST policy cannot be represented safely, expose a narrower supported policy and document the difference before claiming the gate passed.

## Supported control contracts

- `control.merge`: two named input ports, `a` and `b`. `policy=named` returns active values under those names. `policy=ordered` returns `{items:[...]}` in port-name order. If both inputs are skipped, the node is skipped.
- `control.wait`: integer `seconds` from 1 to 2,592,000. Temporal owns the timer. The V3 run has a 31-day deadline. Waiting does not occupy an action worker.
- `control.map`: `collectionExpression` must return an array of objects. Set `bodyType`, `bodyVersion`, `bodyConfig`, `maxItems` (1–1,000), and `concurrency` (1–10). The compiler pins one action body. Controls and nested Maps are rejected as bodies. Results retain item order under `{items:[...]}`.

Every action gets a logical identity. A Map item includes its stable item index. Retries keep that identity; a deliberate new run gets new identities. Payloads remain 8 KiB each. A run has a pinned limit of 1,000 action invocations and 8 MiB of unique persisted payload data. A workspace has at most 100 active executing runs; admission-only development receipts do not consume execution capacity.

## Operator decisions

An uncertain invocation persists a pending record before the runtime pauses. Send `POST /v1/workspaces/{workspace}/runs/{run}/invocations/{invocation}/resolve` with an `action`, `note`, and optional `output`.

`confirm` requires a schema-valid output that the operator verified against the provider. `fail` ends the run and must not include an output. The decision is immutable. A dispatcher retries delivery until the runtime consumes the decision. No option automatically resends the provider request. A late worker result cannot replace an operator-confirmed result.

The development API records the local operator identity. This is not production authentication or role enforcement. Do not expose it publicly.

## Schedule policy

Temporal owns all clocks. PostgreSQL stores desired and observed revisions. Each Temporal occurrence starts a short admission wrapper; the wrapper atomically accepts or skips a main product run.

The initial schedule forms are:

- `interval`: 60–86,400 seconds, in UTC.
- `daily`: an hour/minute in a valid IANA timezone.

The supported overlap policy is **skip**. The active product run is the durable slot; completion of the admission wrapper does not release it. Manual starts of a scheduled workflow also respect the slot. A terminal projection releases capacity; a missing projection conservatively retains it until repair.

Catch-up uses a one-minute Temporal window. Older missed occurrences are not backfilled. There is no coalesced pending occurrence in this implementation. This narrows the original proposal and remains an explicit limitation.

Temporal skips a daily wall time that does not exist during the spring DST transition. Temporal emits two occurrences for a repeated fall wall time; the application deduplicates them by schedule revision and local date/time. Disabling a desired schedule prevents new admission even before external synchronization finishes. Old revision deliveries are recorded as stale.

## Recovery authority

V3 stores an ordered event journal in workflow state and includes it in terminal results. Product failure is a terminal result with `state=failed`; the runtime result still carries the journal. A database projection failure does not create a second scheduler. Recovery reads missing facts from a running workflow query or a completed runtime result and applies them idempotently.

Forced runtime termination, timeout outside normal product completion, or deleted Temporal history requires operator investigation. Do not infer that a remote effect failed merely because the runtime is unavailable. Retain history and compatible artifacts while runs or active versions need them. `GET /v1/workspaces/{workspace}/required-artifacts` reports those digest references; it does not delete or sign artifacts.

## Verification boundary

The live harness verifies Map ordering/empty/bounds, merge with a skipped branch, wait recovery after SIGKILL, durable operator resolution, complete projection loss with offline-engine repair, actual Temporal schedule delivery/overlap/disable, and V2/V3 coexistence. A bounded smoke load uses 30 single-action runs and ten concurrent HTTP clients.

The smoke load does not establish the proposed 20-starts/second burst, ten-action average, 1,000 simultaneous waits, 50-user/10-workspace access profile, or p95 dispatch target. Keep those release gates open until the representative workload is measured on the chosen installation.
