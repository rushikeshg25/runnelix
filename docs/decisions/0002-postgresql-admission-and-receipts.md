# ADR 0002: transactional admission and durable runtime receipts

Date: 2026-10-17. Status: implemented for Phase 1 development; production approval remains open.

## Problem

PostgreSQL and Temporal cannot commit one shared transaction. A process can die between accepting a run, starting its runtime, and recording the acknowledgement. A timeout cannot distinguish an unreceived request from a lost response.

## Decision

PostgreSQL owns workflow definitions and the admission decision. One transaction locks the workflow, checks the trigger key and fingerprint, pins the published version, and inserts the inbox, run, and outbox rows.

The dispatcher claims pending rows with `FOR UPDATE SKIP LOCKED`. Each claim has a random token and an expiry. A stale token cannot acknowledge or reschedule the claim. The dispatcher releases database locks before making a network call.

Each product run uses the stable runtime ID `runnelix/admission/{runId}`. Temporal rejects reuse of a completed workflow ID and returns the existing execution for a running duplicate. An ambiguous response leaves the outbox retryable.

The receipt-only runtime invokes an idempotent activity. That activity verifies the product run's pinned version and plan digest, then inserts a durable PostgreSQL receipt. Only that receipt permits outbox acknowledgement. The dispatcher checks the receipt before calling Temporal again.

This durable record matters because Temporal's retained execution history is finite. The offline-runtime integration case proves that a saved receipt avoids another start call. It does not simulate the passage of the actual retention interval.

## State and scope

`accepted` means that the product run exists durably. `dispatchState` separately reports `pending`, `receipt_recorded`, or `acknowledged`. The Phase 1 admission workflow does not execute actions. Completing that workflow leaves the product run accepted.

The TypeScript mock-node host is independently tested. It resolves payload references through an injected workspace/run-scoped interface. It is not wired to a production payload service or the admission workflow.

Workspace IDs appear in routes and composite database keys. This prevents accidental cross-workspace data references. It is not user authentication or row-level security. The development API binds only to literal loopback, requires a literal loopback Host header, and rejects browser Origin headers.

## Alternatives

- **Start Temporal directly from the HTTP handler:** a crash can lose accepted work or return an ambiguous response without a durable recovery record.
- **Add another broker:** adds another delivery boundary without removing the PostgreSQL-to-runtime transaction gap.
- **Acknowledge on a successful start RPC:** does not prove the runtime recorded the durable product receipt.
- **Rely only on Temporal workflow IDs:** history retention eventually removes that deduplication evidence.
- **Run the full graph during Phase 1:** would combine execution semantics with admission evidence before the product contracts stabilize.

## Consequences and limits

- Trigger keys and runtime receipts have no automatic expiry. A future retention policy must preserve retry and reconciliation guarantees.
- Same key plus same canonical JSON input returns the original run, even after later publication. Changed input returns a conflict.
- Admission and publication serialize on one workflow row. This is a correctness choice; throughput is unmeasured.
- Current admission requires the pinned plan to match the available catalog. Worker liveness and production artifact retention remain deployment concerns.
- Dispatch retries transient failures with a bounded linear delay. A failed or 24-hour-expired runtime admission workflow is not automatically replaced. Operators must inspect it; automated reconciliation is future work.
- Receipt records are not proof of external effects. They cannot make unsafe provider writes exactly once.
- API rollback after a newer migration requires compatibility review. Migration application rejects changed checksums and unknown migration versions.

## Evidence and next phase

See the [live report](../implementation/evidence/phase-1-full.json), [PostgreSQL transaction tests](../../internal/store), and [phase record](../implementation/phase-1.md).

Phase 2 needs a new execution contract and interpreter route. Do not silently change `RunnelixAdmissionV1` into an executing workflow: replay and existing accepted development runs need an explicit transition policy. Use fake development data while that policy is designed.
