# Dispatch throughput correction

## Confirmed cause

The [initial capacity diagnostic](local-capacity.md) exposed a growing start backlog. A repeat with temporary boundary timers reproduced the failure: 261 of 1,200 offered starts were accepted and 939 were rate-limited. The dispatcher ran projection repair, schedule reconciliation, resolution delivery, cancellation, and a batch of starts serially on one 250 ms ticker. A slow projection query prevented every later job from running.

Across the instrumented run, projection repair consumed 78.658 seconds (p95 2.009 seconds per call). Dispatch batches consumed 22.752 seconds; individual claims had p95 3.244 ms and Temporal start calls p95 36.446 ms. Other reconciliation consumed 0.201 seconds. This isolated head-of-line blocking in the periodic loop as a throughput constraint. Timings include warm-up, burst, drain, and recovery; start counts include repeated attempts, not distinct runs. Raw fixture logs remain in `artifacts/phase1/20261030T192924Z-74aa6a`. Temporary timers were removed from source.

## Change and safety boundaries

Each of the five periodic jobs now runs independently at the existing 250 ms cadence. Each job is serial: a slow invocation never overlaps another invocation of that same job. There is still one dispatch job and at most sixteen claim/start attempts in a batch. The runner cancels and joins every job before the caller closes shared database and Temporal clients.

Admission quotas, activity execution concurrency, leases, receipt-based acknowledgment, stable runtime IDs, and retry delays are unchanged. No migration or configuration change is required. Schedule/cancellation/resolution jobs can now progress while projection repair waits. Their existing idempotency and database fencing remain authoritative.

`TestSlowMaintenanceDoesNotBlockDispatch` uses Go's virtual-time test support to hold maintenance open while dispatch continues. The extracted old serial loop failed with zero dispatch cycles; the corrected runner passes. It also verifies no overlapping maintenance invocation. `TestPeriodicJobsStopAndJoinOnCancellation` verifies shutdown waits for an in-flight job and stops further ticks.

The runner requires jobs to honor context cancellation. A future job that ignores context can delay shutdown; adding a goroutine does not make an unbounded operation safe.

## Second constraint: engine workflow-task pickup

Independent periodic jobs alone improved accepted requests from 267 to 322 in the same offered workload, but scheduling p95 remained 42.829 seconds. This ruled out the shared loop as the only constraint. PostgreSQL timestamps placed p95 acceptance-to-receipt delay at 42.438 seconds, while receipt-to-first-scheduling was 0.421 seconds.

Inspecting one slow fixture's persisted Temporal history located the delay more precisely: its first workflow task was scheduled at `2026-10-30T19:32:51.354788Z` and started at `19:33:37.519322Z`. Bootstrap then completed in milliseconds. The V3 Go worker used the SDK's default two workflow-task pollers. A controlled follow-up changed only that worker's workflow-task poller count to eight. Activity execution limits, the TypeScript worker's four activity slots, the database, and admission quotas remained unchanged.

Eight fixed pollers did **not** improve the result (287 accepted, 913 rejected), so that setting was removed. Inspection of the pinned Go SDK v1.50.0 showed that fixed-count `Mixed` polling prefers the sticky queue whenever it reports backlog. More mixed pollers do not guarantee coverage of the normal queue. The supported adaptive behavior creates separate normal and sticky pollers and preserves normal coverage.

V3 now uses `WorkflowTaskPollerBehavior` with `NewPollerBehaviorAutoscaling`: initial/minimum one and maximum two pollers per queue. This is a polling setting inside the existing process, not worker-replica autoscaling. The SDK may exceed the configured maximum when the server requires coverage of additional routing locations. It uses the existing SDK/server compatibility matrix; it does not claim compatibility with arbitrary older Temporal installations. V2 worker registration and workflow definitions are unchanged.

## Comparative result

The [final source-hashed report](evidence/dispatch-capacity.json) used the same 1,200-request/sixty-second burst, ten sequential Echo actions per run, one workspace, and four TypeScript activity slots. No other test suite ran during these measurements.

| Measurement | Original local run | Independent jobs + separate queue polling |
| --- | --- | --- |
| Accepted / offered | 267 / 1,200 | 300 / 1,200 |
| HTTP 429 | 933 | 900 |
| Scheduling p50 / p95 / p99 | 17.525 / 41.512 / 50.730 s | 1.456 / 3.119 / 3.329 s |
| First completion p95 | 41.797 s | 4.365 s |
| Peak sampled pending starts | 98 | 27 |
| Peak sampled oldest pending age | 53.644 s | 1.567 s |
| Drain after admission responses | 27.060 s | 14.370 s |
| Worker restart to drain twenty queued runs | 5.497 s | 5.389 s |

All accepted runs succeeded, attempt totals matched, and queued work survived worker restart. Sources stayed unchanged during the measurement and cleanup succeeded. These are comparative local observations, not statistical performance guarantees. Sampling exclusions and percentile definitions remain in the capacity guide.

The dispatch-delay symptom is substantially reduced; the complete capacity target is still unmet. Nine hundred requests were rejected by admission limits. This work does not establish that increasing those limits is safe, or that this installation supports twenty accepted starts per second with representative provider work. Exact first-action-start timing and production workload acceptance remain unverified. Further capacity work must measure execution occupancy and sustained throughput before changing concurrency or quotas.

## Validation

- `go test -race ./internal/dispatch -count=1`: passed after the deterministic blocked-dispatch regression failed against the extracted serial loop.
- `make test check`: Python regressions, Go race tests, worker/web tests, catalog checks, vet, TypeScript, formatting, and diff checks passed.
- `make integration-phase1`: all seven live admission groups passed, including dispatcher deaths before/after start, receipt survival, concurrent dispatch, and publication pinning.
- `make integration-phase3`: all eight live control/recovery groups passed, including independent projection repair, schedules/DST, operator resolution, and V2/V3 coexistence. [Report](evidence/dispatch-recovery.json).
- `python3 tests/integration/phase2.py`: all seven live execution groups passed, including pre-dispatch cancellation, unsafe outcomes without resend, SSE reconnect, and engine restart.
- `make integration-capacity`: final comparative burst and worker-outage checks passed with the capacity limitations above.

No browser behavior changed; no browser test or production deployment is claimed. The changed process startup and dispatch scheduling paths were exercised by real isolated local services.
