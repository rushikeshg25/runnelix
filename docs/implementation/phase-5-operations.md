# Phase 5: local operations

This slice targets the existing native, loopback-only macOS/Linux installation. It does not approve public hosting or a pilot. PostgreSQL and Temporal development SQLite remain the only durable services. A versioned source archive and the owned launcher provide local packaging; container images, TLS ingress, production database roles, and production Temporal topology remain separate deployment work.

## Build, preflight, and readiness

Use the versions pinned in `go.mod`, `package.json`, `pnpm-lock.yaml`, and `scripts/install-temporal.py`. Install PostgreSQL 17 binaries locally (CI also exercises PostgreSQL 16), then:

```sh
make deps tools build-phase2
make dev
```

The launcher checks executable availability and loopback ports before startup. It owns its PostgreSQL cluster, Temporal server, API, engine, dispatcher, node service, TypeScript worker, and editor. It preserves `.local/dev` on shutdown. `python3 scripts/dev.py --state /absolute/new-state` selects another state directory; fixed service ports mean only one launcher can run at a time. Never start independent processes against a launcher-owned data directory.

For a running installation with its protected `RUNNELIX_DATABASE_URL` set:

```sh
.local/bin/runnelix preflight --temporal-address 127.0.0.1:7233
```

Preflight checks exact migration checksums and Temporal health with a five-second deadline. It never migrates data. `GET /health` checks PostgreSQL and the exact schema with a one-second deadline, returning 503 on incompatibility. Neither check proves that workers can execute a workflow. A controlled smoke execution is required after startup or restore.

`make package-local` creates a Git-revision-named source archive and SHA-256 file under `artifacts/releases`. It requires a clean tracked tree, includes only committed source, and never includes `.local`, credentials, databases, artifacts, or untracked logs. Extract into a new directory and use the pinned build commands above. This is a native source release, not a prebuilt container or cross-platform binary.

## Metrics and response procedures

Owners can read `GET /v1/workspaces/{workspace}/operations`. Fixed aggregate fields report run counts by state, pending starts, oldest pending age, unresolved invocations, retention holds, stored payload bytes, and expired-history receipts. No run IDs, subjects, destinations, or payloads appear. Queries share a two-second deadline; failure returns 503. Values are observational snapshots, not a transactional monitoring ledger. Table scans have bounded time rather than a fixed row budget.

| Signal | Local response |
| --- | --- |
| `/health` returns 503 | Check database connectivity and migration compatibility. Stop rollout; never recreate data to clear a migration error. |
| Pending starts age keeps increasing | Check Temporal, dispatcher and worker logs. Preserve start IDs and receipts; do not bulk-submit replacements. |
| Unresolved invocations increase | Inspect uncertain actions and provider outcomes. Reconcile explicitly; do not resend unsafe actions. |
| Payload bytes or disk use increases | Preview retention and inspect holds/active runs. Add capacity or stop excess admissions; never delete live dependencies. |
| Restore/key verification fails | Keep admissions and dispatch stopped. Preserve the source and failed restore directory for investigation. |

No production paging owner or threshold is configured. Percentile latency, runtime backlog/poller health, host disk/I/O, external probe history, and backup-age alerts remain production observability work. Avoid treating these local aggregates as proof of a service target.

## Paired offline backup

Stop the launcher with Ctrl-C and wait for its owned services to stop. Use protected storage with sufficient free space. Backup files contain payloads and sessions; this tool does not encrypt them. Keep the separate key file under independent access control and back up external OIDC/environment configuration separately.

```sh
python3 scripts/local_ops.py backup \
  --state /absolute/project/.local/dev \
  --output /protected/backups/runnelix-YYYYMMDD \
  --keys-out /separate/protected/keys/runnelix-YYYYMMDD.json
```

The command acquires the launcher lock and requires PostgreSQL to be stopped. It briefly starts only that cluster on a fresh loopback port for `pg_dump`, stops it again, and snapshots Temporal SQLite with an integrity check. The manifest records hashes, time, and PostgreSQL version. Output paths must be new; keys cannot be inside the database backup or source state. Store the manifest with the backup. Both files and keys are required for recovery.

The supported ownership boundary is the launcher lock. Independently started processes are unsupported: stop them before backup. A consistent local snapshot does not provide point-in-time recovery, production RPO, or knowledge of external writes after the snapshot.

## Restore into a new directory

```sh
python3 scripts/local_ops.py restore \
  --backup /protected/backups/runnelix-YYYYMMDD \
  --keys /separate/protected/keys/runnelix-YYYYMMDD.json \
  --target /absolute/new-runnelix-state
```

Restore rejects an existing target, mismatched checksums/keys, and another PostgreSQL major version. It initializes a new cluster, restores in one transaction, validates Temporal SQLite, and leaves all services stopped. Source data and partial failed restores are retained. It never restores over the current installation.

Before starting the restored launcher, compare the backup time with known external effects. Keep ambiguous unsafe actions held for review. A rewind can omit later provider writes; neither a receipt in the restored database nor a runtime restart proves that no external effect occurred after the backup. After reconciliation, start the restored state, run preflight, verify credential use with a controlled provider, and execute a safe smoke workflow. Do not connect an isolated drill to real providers.

`make integration-operations` performs a timed drill against fresh fake-data services. It backs up PostgreSQL and Temporal, restores into a new directory, verifies audit/receipt counts and old-run deduplication, decrypts a restored HMAC credential through signed webhook admission, and executes new Go/TypeScript work. It also exercises retention through the CLI and HTTP 410 behavior. All services are stopped after the drill; artifacts are retained.

## Upgrade and rollback

1. Record the current Git revision, dependency locks, schema migration versions, configuration, and required worker versions. Keep the old checkout and binaries.
2. Stop admissions and the launcher. Take and verify a paired backup with separately stored keys.
3. Build the new revision in a separate checkout. Read migration notes before changing the database. Migration 013 requires the updated readers; old readers do not understand expired payloads.
4. Apply migrations using the new binary and the protected database environment. A failure stops rollout. The launcher also applies migrations on startup, so use it only after accepting the upgrade.
5. Run preflight and a safe smoke workflow. Verify old waiting runs against the retained compatible worker versions before retiring any worker.
6. If rollback is needed, stop services. Do not run an older binary against a newer incompatible schema. Restore the paired backup into a new directory with its matching old binaries, then reconcile external effects before reopening admission.

Migration checksums are immutable. There is no automatic down migration and no destructive reset command. Audit history, pinned versions, and active-run data remain protected by their existing boundaries.
