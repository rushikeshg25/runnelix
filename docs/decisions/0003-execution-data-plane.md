# ADR 0003: pinned execution and a protected node data plane

Date: 2026-10-17. Status: accepted for local development. Production security and operations gates remain open.

## Context

Phase 1 proved durable admission. Phase 2 needs actual graph execution, a visual editor, and action modules that do not own orchestration or database credentials. Existing admission-only runs must retain their meaning.

## Decision

Add `RunnelixExecuteV2` on a separate engine queue. New execution requests persist their execution mode. Go interprets the pinned plan. Go activities evaluate bounded CEL and manage references. TypeScript activities execute trusted internal action modules on a source-digest queue.

Give workers a protected internal HTTP capability. The Go node service authenticates the worker token, checks the complete persisted invocation, resolves scoped payloads, applies egress policy, and records immutable attempt evidence. Only the engine selects the canonical attempt after Temporal accepts completion. A late completion cannot overwrite the selected result.

Store provider credentials encrypted with workspace/credential identity as authenticated data. Require workflow grants and exact destination-host binding. Match each HTTP request to the published URL and method. Separate read-only GET from unsafe POST to make retry behavior explicit.

Keep cancellation intent in PostgreSQL and reconcile it to Temporal. Keep event history in persisted sequence order and resume SSE with that sequence. The UI renders a run's published graph, not the current draft.

## Alternatives

- Giving workers the owner DSN or provider credentials would enlarge their authority and expose secrets to node code.
- Reusing the admission workflow would silently change old run behavior and complicate replay compatibility.
- Selecting the first worker-side completion would race runtime timeouts and retries.
- Canvas-owned validation would create a second interpreter. Publication stays authoritative.

## Consequences and limits

The local installation uses more than one process but adds no new persistence service. Source digests identify a worker bundle; they do not prove signed artifact availability. Keep old artifacts while their published versions or active runs still need them. General retirement and upgrade operations follow later.

Payload storage remains plaintext and bounded to 8 KiB. The local API does not authenticate people. OIDC, roles, audit policy, retention, complete projection recovery, schedules, and other control semantics remain future phases. Do not expose this development installation publicly.

## Evidence

[Live execution report](../implementation/evidence/phase-2-full.json), [browser record](../implementation/evidence/phase-2-browser.json), [implementation guide](../implementation/phase-2.md), and [source tests](../../tests/integration/phase2.py).
