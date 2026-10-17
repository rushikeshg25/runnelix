# ADR 0004: versioned controls and recovery

Date: 2026-10-18. Status: accepted for local development.

## Context

Merge, waits, and per-item actions change replay behavior. The database run projection can fail independently of successful provider actions. A short schedule-admission workflow can close while its product run is still active.

## Decision

Compile new publications to interpreter V3. Keep V2 workflow code, task queue, and compatible action artifacts available. Test a publication from the pushed Phase 2 binary against the current dispatcher and engine.

Use explicit two-port merge policies, Temporal timers, and a Map body containing one pinned action. Bound Map to 1,000 objects and ten concurrent actions. Preserve item order and stable invocation identity. Do not infer iteration from arrays.

Persist immutable operator decisions before signaling uncertain executions. Confirm only a verified schema-valid result, or fail the run. Neither choice repeats the external write. Retain evidence and fence late worker results.

Keep Temporal as the clock owner. Persist desired schedule revisions and reconcile them. A product run holds the no-overlap slot. Support interval and daily schedules with skip overlap and a one-minute catch-up window. Deduplicate repeated local daily times; skip nonexistent times. Pending-occurrence coalescing is deferred.

Keep the V3 event journal in workflow state and the terminal result. Rebuild missing database projections from that journal, including when the engine is offline. PostgreSQL does not infer which graph step should run next.

## Consequences

Old workers and runtime histories remain operational dependencies. The artifact-reference report helps retention decisions but does not delete artifacts. Forced termination, runtime timeout, or history deletion needs operator investigation. Projection loss conservatively holds run capacity until repaired.

The local smoke load is not the proposed production operating envelope. Teammate acceptance and representative load verification remain open.

## Evidence

[Phase 3 implementation](../implementation/phase-3.md), [live report](../implementation/evidence/phase-3-full.json), [browser checks](../implementation/evidence/phase-3-browser.json).
