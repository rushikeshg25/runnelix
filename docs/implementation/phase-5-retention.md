# Phase 5: terminal-data retention

Migration 013 adds explicit, opt-in retention. The installation operator runs a bounded command; API startup never removes data. Defaults are seven elapsed days after the terminal projection for payloads and thirty for history. The clock uses the run's terminal `updated_at`, so long waits do not shorten retention.

## Preview and apply

Set `RUNNELIX_DATABASE_URL` through the installation's protected environment, then run:

```sh
runnelix retention -workspace workspace_dev
runnelix retention -workspace workspace_dev -apply
```

The first command previews at most 100 candidates. `-payload-days 7 -history-days 30 -limit 100` makes defaults explicit. Days must be positive, history must be at least payload retention, and limits must be 1–100. Each applied batch is one transaction. A transaction failure rolls back its changes and audit records. If a connection fails during commit, inspect the audit trail before retrying; the client may not know whether commit completed. Repeat the command to process more batches. A preview is a current snapshot, not a reservation; locked rows are skipped and can appear in a later batch.

Only succeeded, failed, or cancelled runs with acknowledged dispatch qualify. Active runs, unresolved reconciliation, and operator holds pin data. A terminal run with unresolved uncertainty remains pinned for operator investigation; this command cannot discard that uncertainty.

```sh
runnelix retention-hold -workspace workspace_dev -run run_EXAMPLE
runnelix retention-hold -workspace workspace_dev -run run_EXAMPLE -hold=false
```

Holds apply to remaining data and cannot restore expired data. Hold changes and each expiry stage append safe audit records in the same transaction. Run locks serialize holds and cleanup. Late inserts and payload writes cannot recreate expired data.

## What expires and what remains

At payload expiry, the run's original trigger input is cleared and payload bodies become NULL with explicit expiry timestamps. Payload identifiers and references remain valid until history expiry. Reading a retained payload tombstone returns HTTP 410 (`data_expired`). The inspector explains that output expired. Workflow definitions/configuration and reconciliation notes remain until their respective history boundaries; this is not a universal sensitive-data erasure guarantee.

At history expiry, attempts, events, invocations, resolution notes, uncertainty records, and payload tombstones are deleted in foreign-key order. Ordinary updates to immutable history remain prohibited. Database guards permit deletion only after the terminal parent has been marked expired. Public run reads return 410 and run lists omit expired history. The scoped payload endpoint returns 404 when the history and payload identifier have both been removed.

Minimal run identity, terminal state, version identity, timestamps, counters, trigger fingerprints/keys, start outbox, runtime receipts, and schedule occurrence identities remain. An identical admission retry returns its original receipt with `historyExpiredAt`; a changed fingerprint still conflicts. This deliberate exception to the original thirty-day summary proposal prevents duplicate external execution after Temporal forgets a workflow ID. It means database growth is reduced, not fully bounded.

Audit records/counters, workflow versions, credentials, sessions, backups, and Temporal history are outside this cleanup. Retention does not securely erase old database pages or backup copies. Database owners can bypass application controls. Keep backup and Temporal retention policies separate; no production encryption or erasure claim is made.

## Verification

Isolated PostgreSQL tests exercise preview/apply, both expiry stages, reference-safe deletion, retry deduplication, immutable evidence, audit-failure rollback, active/uncertain/dispatch pins, holds, skipped locks, invalid policies, and workspace isolation. Signed identity/session regressions also pass. Final Phase 5 evidence is recorded in the operations completion guide.
