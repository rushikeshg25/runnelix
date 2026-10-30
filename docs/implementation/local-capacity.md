# Local burst and recovery diagnostic

Run `make integration-capacity` with the repository's pinned toolchain and `RUNNELIX_PG_BIN` set when PostgreSQL is not on PATH. The target builds the current code, creates isolated loopback PostgreSQL/Temporal services, runs the workload, and stops its processes. It preserves fake-data artifacts under `artifacts/phase1/`. It does not use `.local/dev` or an external database.

## Workload and measurements

After one warm-up run, offer 20 starts per second for 60 seconds: 1,200 requests to one workspace. Each workflow contains ten sequential pure Echo actions with a roughly 1 KiB input. One TypeScript worker allows four concurrent activities. Forty HTTP client threads keep the offered schedule separate from request completion; client send lag is recorded so a slow generator is visible.

Record HTTP status counts, admission latency, acceptance-to-first-scheduling latency, acceptance-to-first-completion latency, and drain time after all admission responses. Percentiles use the nearest-rank method. HTTP 429 is recorded as backpressure, never counted as an accepted start. Other unexpected HTTP responses fail the diagnostic. Every accepted run must succeed, and the total attempt count must equal ten per run.

Scheduling events precede actual worker execution. First completion includes action duration and persistence. Neither metric is labeled actual first-action-start latency; the proposed p95 dispatch target remains unverified by this diagnostic.

Sample pending outbox count, oldest pending age, RSS, and CPU once per second during the burst and drain. RSS/CPU cover only the harness-owned API, node service, Temporal, engine, TypeScript worker, and dispatcher PIDs. PostgreSQL, subprocesses, the load client, and operating-system costs are excluded. `ps` CPU values are process-lifetime averages, not instantaneous utilization. Short peaks can fall between samples. The final report includes PostgreSQL database size and event count, not a complete storage-growth profile.

After the burst drains, kill the TypeScript worker, admit twenty workflows, verify no new action completion during a two-second observation, restart the worker, and measure recovery drain. All 200 actions must complete. This tests queued work surviving a worker outage; it does not inject interruption during a provider effect.

## Interpretation

A passing diagnostic establishes correctness under this specific offered workload. It does not mean all 1,200 starts were accepted, that the p95 service target passed, or that this machine supports production traffic. Read status counts and timing results together.

The full V-11 capacity gate remains open: representative provider delays/errors, branch/Map mix, sustained daily load, fifty users across ten workspaces, 1,000 simultaneous waits, database contention, complete host resource measurements, and actual first-action-start timing require further evidence. Current admission limits permit only 100 active executing runs per workspace; do not raise limits solely to make a benchmark pass.

This relatively expensive diagnostic is opt-in rather than part of every push. Existing CI still runs the shorter correctness and recovery suites.

## Measured result — 2026-10-31 (Asia/Kolkata)

The [source-hashed report](evidence/local-capacity.json) records a macOS arm64 run with PostgreSQL 17.6, Temporal CLI 1.9.1/server 1.32.0, and Go 1.26.10. Sources stayed unchanged during execution and cleanup completed without errors.

| Measurement | Observed |
| --- | --- |
| Accepted / offered requests | 267 / 1,200 (22.25%) |
| Rejected with HTTP 429 | 933 |
| Admission window | 59.955 seconds |
| Client send lag p95 | 9.4 ms |
| HTTP response latency p95, including rejections | 7.4 ms |
| Acceptance to first scheduling p50 / p95 / p99 | 17.525 / 41.512 / 50.730 seconds |
| Acceptance to first completion p50 / p95 / p99 | 18.176 / 41.797 / 50.956 seconds |
| Drain after admission responses | 27.060 seconds |
| Peak sampled pending starts / oldest age | 98 / 53.644 seconds |
| Worker restart to drain twenty queued runs | 5.497 seconds |
| Peak sampled owned-process RSS | 612,736 KiB (about 598 MiB; exclusions above apply) |
| Final PostgreSQL size / event rows | 18,050,195 bytes / 6,624 |

Correctness passed: all accepted workflows succeeded, expected attempt counts matched, and queued work survived the worker outage. The proposed burst target **did not pass** on this configuration: it did not accept 20 starts/second. Scheduling alone exceeded five seconds at p95, so this run cannot satisfy the proposed first-action-start latency target either. The report's `passed` fields describe correctness assertions, not capacity acceptance.

The backlog establishes a local limit, not its root cause. Investigate dispatch throughput and database/runtime contention before selecting a scaling or tuning change. Do not extrapolate the accepted average into sustained supported capacity, increase quotas to hide rejection, or attribute the limit to worker concurrency without a distinguishing measurement.
