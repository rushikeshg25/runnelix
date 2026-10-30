# Pre-pilot review and fixes

Reviewed Phase 5 at `5366e1d` before starting local Phase 6 preparation. Independent read-only reviewers checked Standards and Spec. Findings below are fixed in the continuation commits; no remote review comments or approvals were posted.

| Finding | Standards | Spec | Fix and verification |
| --- | --- | --- | --- |
| Retention used calendar days across daylight-saving transitions | — | Fixed | `3b79e8e`: use elapsed 24-hour intervals. Deterministic spring/fall PostgreSQL tests reproduced early and late expiry before the fix and passed afterward. |
| Backup could leave its owned database running after interruption or startup failure | Fixed | — | `b952264`: cleanup covers startup and ordinary SIGTERM/SIGINT. Regression tests use a real signal subprocess plus mocked database lifecycle. Forced termination cannot execute cleanup. |
| SQLite paths with URI syntax could select the wrong database | — | Fixed | `b952264`: escaped file URIs. A real SQLite marker survives backup and restore under paths containing `#?%`; old code failed. |
| Expired run history remained visible while inspection retried | Fixed | Fixed | `2851728`: HTTP 410 clears selection/history and closes polling/SSE. Live authenticated browser observed the expiry message, no stale timeline, and a request count stable at 37 across separate observations. |
| Run listing issued one query per returned run | — | Fixed | `3ae9979`: shared joined projection. Query tracer verified 51 queries became one for 50 runs, with pagination and workspace isolation preserved. |
| Payload metrics repeatedly serialized retained JSON | Fixed | Fixed | `3ae9979`: sum transactional byte counters for unexpired payloads. Regression compares counters with actual retained bodies before/after deduplication and both retention stages. |

Standards: three findings fixed, zero open. Spec: five findings fixed, zero open. Some findings overlap between axes. These totals describe this bounded review, not proof that no other defects exist.

## Verification

- `make test check`: Python regressions, Go race tests, 43 TypeScript worker tests, five web tests, catalog consistency, Go vet, TypeScript and formatting checks passed.
- `make integration-phase5`: live isolated PostgreSQL store and signed-identity tests passed. [Report](evidence/pre-pilot-postgres.json).
- `make integration-operations`: paired backup/restore and runtime checks; see [report](evidence/pre-pilot-restore.json).
- `make integration-phase6`: five actual portable files passed eight execution cases. [Report](evidence/phase-6-fixtures.json).

The browser expiry reproduction used a fixture run seeded as terminal and aged beyond retention, then the real retention CLI. It verifies UI behavior, not actual execution of that seeded run. The separate Phase 6 harness supplies execution evidence. Browser services and isolated databases were stopped after checks. No production data or provider effects were used.

Local Phase 6 preparation is in the [pilot kit](../pilot/README.md). Independent acceptance, operational ownership, representative load, and real two-week usage remain open.
