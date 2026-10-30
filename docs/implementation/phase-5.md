# Phase 5: local security and operations

Status: started. Identity verification, workspace permissions, local browser sessions, and transactional audit records are implemented and locally tested. Terminal-data retention is next. See [audit records](phase-5-audit.md). See [browser sessions](phase-5-sessions.md). See [workspace memberships](phase-5-memberships.md). See [identity configuration and limits](phase-5-identity.md). The user selected a local installation. Public deployment, a production identity provider, and an operational owner remain unselected.

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
