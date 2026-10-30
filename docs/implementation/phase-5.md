# Phase 5: local security and operations

Status: local scope implemented and verified. Identity, permissions, sessions, audit, terminal-data retention, bounded metrics/readiness, native local packaging, paired backup/restore, dependency review, and release evidence are complete. See the [completion checklist](phase-5-completion.md), [operations guide](phase-5-operations.md), and [retention guide](phase-5-retention.md). Public deployment, a production identity provider, an operational owner, redistribution licensing, and pilot acceptance remain open.

## Implementation boundary

Build security controls that can be tested locally. Keep unauthenticated development mode explicitly restricted to loopback. Add configurable identity verification and workspace roles without treating a workspace path as authentication. Test unauthorized cross-workspace requests, including payload and event access.

Keep secrets at the Go network boundary. Record safe audit metadata, not credential values or request bodies. Retain encryption keys separately from database backups. Do not claim production payload encryption from credential encryption alone.

Use the existing PostgreSQL and Temporal services. No Kubernetes, broker, or cache is needed for this local target. Package reproducible startup, preflight, backup, restore, and upgrade procedures. Never delete existing data during a drill.

## Planned slices

1. Identity configuration and verified principal contract.
2. Workspace memberships and server-side permissions.
3. Browser authentication/session handling and authenticated event access.
4. Append-only safe audit records for privileged actions.
5. Explicit terminal-data retention with active-run protection.
6. Bounded operational metrics and readiness checks.
7. Local deployment packaging, preflight, and migration/upgrade procedures.
8. Timed isolated restore verification and dependency/license review evidence.
9. Release checklist, security regressions, documentation, and push.

The implementation ledger must record any changed scope. Keep incomplete controls marked open. A local fixture issuer is not a deployed production identity provider. A local restore is not evidence of production RPO or availability.
