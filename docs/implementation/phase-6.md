# Phase 6: controlled pilot preparation

Status: local preparation implemented. A real pilot has not started.

The user selected local deployment. No operational owner, production identity provider, or participant list has been confirmed. Do not fabricate participants, acceptance signatures, provider actions, or two weeks of usage.

The [pilot kit](../pilot/README.md) includes an installation checklist, participant instructions, incident and weekly usage records, node-authoring feedback, and a release decision template. The [five portable fixtures](../../examples/pilot/README.md) cover Echo, CEL transformation, conditional routing, durable wait, and bounded Map. They require no credentials or real provider effects.

`make integration-phase6` imports the actual files as unpublished drafts, verifies execution is blocked before publication, publishes, and checks eight cases through the real local PostgreSQL/Temporal/Go/TypeScript stack. It verifies pinned version IDs, event ordering, expected node output, skipped branches, Map attempt counts, and oversized rejection. The [live report](evidence/phase-6-fixtures.json) records source hashes and cleanup results. The existing CI runtime job runs this target.

These fixtures are training examples. Participants must select five useful workflows and record actual activity before claiming pilot completion. No shared hosting or production topology was added.

## Gates that require external evidence

- A teammate completes the author/publish/run/diagnose path without assistance.
- Another developer adds an action through the documented node path.
- An owner accepts the measured installation limits and recovery procedures.
- Three teammates use five useful workflows weekly for two consecutive weeks.
- Incidents and recovery effort are reviewed with actual records.

Prepared documents do not close these gates. Implementation can proceed locally while the external decisions remain open.
