# Deployment, operations, and cost

[Planning guide](README.md)

## Deployment levels

**Local development:** Compose starts PostgreSQL, the selected Temporal development deployment, the Go processes, and TypeScript workers. Mock external providers support repeatable tests. Development credentials and data are disposable.

**Internal pilot:** pinned container images, persistent storage, TLS, OIDC, explicit root-key management, backup, and named operational ownership. A single host is acceptable only when its downtime and restore-based recovery fit the agreed service target.

**Higher availability:** replicated application processes and a supported highly available PostgreSQL/Temporal topology. This requires an explicit operational design and budget. A single VM with several containers is still one host-level failure domain.

Prefer the team's existing supported hosting platform. Do not introduce Kubernetes solely because the product has workers.

## Installation deliverables

Before pilot rollout, provide:

- A versioned container release and configuration reference.
- A Compose definition or equivalent for the selected host.
- An example environment file containing placeholders only.
- Database migration and initial-owner procedures.
- OIDC, TLS, encryption-key, and internal-worker authentication setup.
- A preflight check for storage, connectivity, required configuration, and compatible component versions.
- A smoke workflow against a controlled endpoint.
- Upgrade, rollback, backup, restore, and incident runbooks.

These are planned deliverables. This documentation repository does not yet include a runnable deployment.

## Initial operating envelope

| Dimension | Proposed target | Verification signal |
| --- | --- | --- |
| Users/workspaces | 50 users across 10 workspaces. | Representative access and list queries. |
| Daily volume | 10,000 runs/day, average 10 action invocations/run. | Sustained workload with realistic payload sizes. |
| Burst | 20 accepted starts/second for 60 seconds. | Outbox age, dispatch latency, and drain time. |
| Dispatch latency | p95 below 5 seconds at supported load. | Durable acceptance timestamp to first action start. |
| Durable waits | 1,000 concurrent waiting runs. | Resume after worker restarts without one occupied worker per wait. |
| Availability | Proposed 99.5% monthly trigger-ingress availability. | External probes and a documented maintenance policy. |
| RPO | Proposed maximum 15 minutes of disaster-recovery data loss. | Verified backup/transaction-log recovery point. |
| RTO | Proposed restore within 2 hours. | Timed full restore, keys, reconnection, and reconciliation. |

All values are assumptions or acceptance targets. No benchmark or availability history supports them yet.

RPO means recovery point objective: how much recent data a disaster restore may lose. RTO means recovery time objective: how long restoration may take. These differ from ordinary process-restart recovery.

## Backpressure and fairness

Bound the number of pending starts and payload bytes. Alert on oldest outbox age before the storage limit is reached. When admission capacity is exhausted, reject clearly with a retryable response instead of accepting work that cannot be retained.

Use per-workspace concurrency and provider/credential quotas. A busy workflow must not consume every worker slot indefinitely. Track active work and queue age by workspace. Define a simple fair scheduling policy before adding priority classes.

More worker replicas cannot overcome a provider's rate limit. Scale only after identifying whether CPU, database contention, external latency, or provider quota is the limiting factor.

## Observability

Every log, trace, and metric should identify the relevant run and invocation without exposing raw payloads. Keep high-cardinality run IDs in logs/traces rather than unbounded metric labels.

Monitor:

- Trigger rejection rate and durable acceptance latency.
- Oldest pending outbox entry and dispatch p50/p95/p99.
- Runtime task backlog and worker availability.
- Active/waiting/uncertain runs and exhausted retries.
- Projection lag and reconciliation failures.
- Provider rate limits and credential refresh failures.
- PostgreSQL connections, locks, disk, I/O, and slow queries.
- Payload/history growth, retention backlog, and backup age.

Alerts need an owner, a clear symptom, and a linked runbook. Avoid paging on every expected node failure; page on platform failures or unresolved operational impact.

## Backup and restore

Back up application data, Temporal persistence, encryption-key material, and deployment configuration. If application and runtime databases share a cluster, document both their isolation and their shared failure domain.

Choose supported backup and point-in-time recovery procedures for the deployed database/runtime versions. The relationship between application and runtime restore points matters: a run may exist in one restored system but not the other.

Restore procedure:

1. Stop new admissions and unsafe automatic dispatch.
2. Restore the chosen application and runtime data points with required keys.
3. Verify database integrity, version compatibility, and credential decryption.
4. Reconcile accepted requests, start receipts, active-run slots, runtime executions, and terminal projections.
5. Identify external writes that may have happened after the restored point.
6. Reconcile idempotent or queryable provider actions. Hold ambiguous unsafe writes for operator review.
7. Run the smoke workflow and verify monitoring before reopening admission.

A backup file is not proof of recovery. Time the complete procedure on the intended deployment. Never delete existing durable data merely to make the drill easier.

## Upgrade and rollback

Record immutable image digests and the supported component-version matrix. Run preflight checks and verify recent usable backups.

Use expand/contract database migrations. Deploy compatible readers before new writers. Preserve old interpreter/node worker versions needed by active or still-runnable published workflows.

Drain workers gracefully. Do not remove a compatible deployment until its remaining references are understood. Test a waiting old run across the upgrade.

Rollback stops new admissions if necessary, restores compatible application/worker images, and verifies schema compatibility. Do not assume a destructive migration can be reversed by changing an image tag. A restore-based rollback requires the external-effect reconciliation procedure above.

## Incident responses

| Symptom | First checks | Safe response |
| --- | --- | --- |
| Starts remain accepted | Outbox age, runtime connectivity, worker readiness. | Restore dispatch; keep stable IDs; do not bulk-create replacement runs. |
| Provider outage | Typed errors, rate limits, retry safety, deadline pressure. | Pause affected admissions or actions; preserve bounded retry state. |
| Credential compromised | Affected grants, access audit, active attempts. | Revoke/rotate, pause affected work, review already-sent requests. |
| Database disk pressure | Retention lag, payload sizes, runtime history, backups. | Stop excess admission and restore capacity; preserve required active-run data. |
| Run inspector stale | Projection lag and runtime state. | Repair projections; do not rerun actions to refresh the UI. |
| Unknown external outcome | Intent record, provider lookup, attempt timeline. | Reconcile or hold for review; do not assume the write failed. |

## Cost model

No provider, machine inventory, or current price list is selected. Use prices from the actual deployment proposal rather than invented dollar figures.

```text
Monthly cost = API/UI compute + Go worker compute
             + TypeScript worker compute + Temporal compute
             + PostgreSQL compute, storage, and I/O
             + backups + monitoring/log storage + network
             + load balancer / secret manager minimums, if used

Idle cost    = always-on services + retained storage + backups
10x case     = resized compute + measured 10x execution storage
             + database/history overhead + network
```

At 10,000 runs/day and 10 actions/run, a 30-day month has 3 million action invocations. If total captured input/output averages 8 KiB per action, raw payload growth is about 0.82 GB/day. Seven days retain about 5.7 GB. At 10x volume, these become 8.2 GB/day and 57 GB.

Those figures exclude indexes, metadata, runtime history, replicas, backups, and long-running payload pins. Measure those factors during the load test. Do not assume a 10x volume increase requires exactly 10x compute.

Estimate worker slots from peak action arrival rate multiplied by observed action duration. Then apply branch fan-out and external quota constraints. Price the measured configuration at expected and 10x load.

Use 2–4 operator hours/week after stabilization as an initial planning allowance. Measure actual effort during the pilot.

## Infrastructure switch criteria

| Component | Initial choice | Trigger to reconsider |
| --- | --- | --- |
| Durable runtime | Temporal, if Phase 0 passes. | Self-hosted recovery or cost exceeds the agreed limits. |
| Distributed quotas | PostgreSQL coordination. | Contention prevents the dispatch target after query/index fixes. |
| Payload storage | Bounded JSON in PostgreSQL. | Binary workflows are required or storage prevents the restore target. |
| Hosting | Compose or an existing supported container platform. | Multiple hosts and independent scaling need scheduling the team can operate. |
| Broker | No separate broker. | Measured throughput/replay requirements exceed the chosen runtime/admission model. |
| Service decomposition | Modular Go backend and worker roles. | Clear independent ownership, rollout, or scaling needs justify a split. |

## Cost and adoption stop conditions

Before provisioning paid resources, select a monthly budget and operator owner. A spend ceiling remains unresolved; publication of this plan does not authorize paid provisioning.

Pause further expansion if the proposed adoption outcome is missed, operating effort exceeds 4 hours/week for four consecutive weeks, or deployment cost exceeds the agreed budget. Reassess scope before adding infrastructure.

On pause, preserve workflow exports, encrypted backups and keys, architecture notes, benchmark results, a recorded demonstration, and a local run path. Stop compute only after recording how to restart. Do not delete durable state as routine cleanup.

Next: [Delivery plan](08-delivery-plan.md).
