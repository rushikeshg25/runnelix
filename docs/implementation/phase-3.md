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
