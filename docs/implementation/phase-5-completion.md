# Phase 5 local completion checklist

The selected local installation is implemented and verified. This is not production or pilot approval.

| Local deliverable | State | Evidence |
| --- | --- | --- |
| Verified identity, memberships, browser sessions | Complete | [Identity](phase-5-identity.md), [memberships](phase-5-memberships.md), [sessions](phase-5-sessions.md); signed PostgreSQL security suite |
| Transactional append-only audit | Complete | [Audit](phase-5-audit.md); rollback/immutability/isolation tests |
| Explicit retention, active pins, holds | Complete | [Retention](phase-5-retention.md); PostgreSQL tests and real CLI/HTTP expiry checks |
| Owner operational aggregates, schema readiness | Complete | [Operations](phase-5-operations.md); fixed metric fields, deadlines, schema drift and authorization tests |
| Local packaging, configuration, upgrade/rollback | Complete | Native launcher, `.env.example`, `make package-local`, operations runbook |
| Paired isolated backup/restore | Complete | Exact restored Temporal history, resumed durable wait, receipt/audit preservation, encrypted credential use, safe new execution |
| Dependency/security review | Complete for local scope | [Dependency review](phase-5-dependencies.md); zero final npm and Go vulnerability findings; license exceptions recorded |
| Independent code review | Complete | [Standards and Spec findings](phase-5-review.md), fixed locally |

## Final local checks (2026-10-30)

- `make test check`: Go race tests/vet, 43 worker tests, five web transport tests, TypeScript, formatting, generated catalog, and diff checks passed.
- `make integration-phase4 integration-phase5 integration-operations`: passed. Phase 4 live report: `artifacts/phase1/20261030T101809Z-79166b/results.json`. Final signed/store suite: `artifacts/phase1/20261030T101844Z-b31933/result.json`.
- Final paired restore report: `artifacts/phase1/20261030T101904Z-6d3326/restored/results.json`. Backup took 0.407 seconds. Restore through history comparison, durable-wait resumption, credential verification, and new workflow execution took 17.861 seconds. These are small local fixture timings, not production RPO/RTO.
- `make integration-phase3`: all eight live groups passed, including schedules/DST, durable waits, operator reconciliation, projection repair, retained V2 workers, and bounded concurrent execution. [Report](evidence/phase-5-compatibility.json).
- [Runtime report](evidence/phase-5-runtime.json), [restore report](evidence/phase-5-restore.json), and [PostgreSQL report](evidence/phase-5-postgres.json) preserve the final evidence.
- Live reports confirm unchanged source hashes during execution and no cleanup errors. Services were stopped and fake-data artifacts retained.
- OpenAPI validation passed. [Browser expiry evidence](evidence/phase-5-retention-browser.json) verifies the actual HTTP 410 message at 1280px and 390px, with no narrow-screen horizontal overflow.
- `pnpm audit --json` and `govulncheck` v1.9.0 report no vulnerabilities after updates. [npm result](evidence/phase-5-npm-audit.json), [Go result](evidence/phase-5-govulncheck.txt), [license inventory](evidence/phase-5-dependencies.json).

The first retention fixture incorrectly attempted to reconcile an already accepted completion; the fixture was corrected to leave the completion unaccepted before reconciliation. The first operations run checked outbox acknowledgment immediately after execution completion; it now waits for the independently committed acknowledgment. Existing product assertions were not weakened.

## Remaining external gates

- Select production hosting, identity provider, TLS/access policy, database roles, and named operational ownership.
- Establish production payload/backup encryption, key custody, monitoring/alert ownership, capacity, and backup/Temporal retention policies.
- Run representative load, independent teammate acceptance, provider acceptance, and the two-week pilot. Local smoke results do not establish those outcomes.
- Resolve the missing upstream module license evidence and choose the repository's own license before external redistribution. Local source packaging is not legal approval to distribute.
- Approve rollout only after production gates pass. No production service or account was changed by this phase.

Phase 6 can proceed as pilot preparation after these decisions. Do not infer permission to provision paid infrastructure, publish a service, or use real provider credentials from this local completion.
