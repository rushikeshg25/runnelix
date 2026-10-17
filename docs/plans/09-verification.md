# Verification and release gates

[Planning guide](README.md)

## Evidence rule

All checks below are planned. No platform test has passed merely because this document describes it.

For each release check, record the build/commit, dependency versions, environment, test input, expected result, actual result, and evidence location. Classify the result as passed, failed, skipped, or blocked. Explain skipped and blocked checks.

## Requirement coverage

| Check | Requirements | Evidence required |
| --- | --- | --- |
| V-01: graph and mapping | P-01, P-02, P-04 | Invalid graph fixtures, port checks, bounded expressions, useful errors. |
| V-02: admission | P-03 | Duplicate event keys, conflicting fingerprints, crash-safe acceptance, queue limits. |
| V-03: control flow | P-04 | Branch skip propagation, joins, Map ordering/limits, empty collections, wait behavior. |
| V-04: cross-language tasks | C-03, C-04, P-05 | Go and TypeScript pass identical serialization/error fixtures. |
| V-05: failure recovery | P-07 | Real process/network failure injection at every identified boundary. |
| V-06: version compatibility | P-02, P-07 | Old waiting run resumes with compatible code after upgrade. |
| V-07: run inspection | P-06 | Correct pinned version, attempts, sanitization, SSE recovery, stale projection display. |
| V-08: authorization | P-08 | Workspace and credential checks across public and internal APIs. |
| V-09: secret and egress safety | P-08 | Sentinel-secret search, SSRF/redirect/rebinding cases, OAuth/refresh behavior. |
| V-10: restore and rollout | C-02, P-09 | Timed full restore and compatible rollback on the target installation. |
| V-11: capacity | C-06, P-09 | Measured dispatch latency, backlog, memory, contention, and queue drain. |
| V-12: import/export | P-10 | Round trip preserves definition meaning; no secrets; invalid imports rejected. |
| V-13: node authoring | C-05, P-11 | Second integration added without engine/editor changes. |
| V-14: user journey | P-01, P-06, P-07 | Teammate creates, publishes, runs, and diagnoses the example unaided. |

## Test layers

**Go unit and race tests:** compiler rules, deterministic ordering, state transitions, error classification, concurrent admission, and refresh coordination. Use the race detector where concurrency is exercised.

**TypeScript tests:** node host validation, executor fixtures, cancellation, HTTP limits, output validation, and sanitization. Do not test only helper implementation details.

**Shared contract fixtures:** null versus missing values, numeric limits, timestamp formats, arrays/objects, schema mismatches, payload references, and structured errors. Both runtimes consume the same fixture files.

**Integration tests:** use real PostgreSQL and the selected Temporal deployment. Mock external providers so tests can control commit timing, delays, errors, idempotency, and lookup behavior.

**Browser tests:** author/publish/inspect flows, stale draft conflict, node forms, keyboard access, narrow layouts, cancellation, and uncertainty messaging.

**Provider smoke tests:** opt-in tests against isolated provider accounts. They must state their side effects and avoid production credentials.

Choose exact commands when the repository gains its toolchain. Do not document a command as available before its configuration exists.

## Failure-injection matrix

| Injection point | Expected result |
| --- | --- |
| Before admission commit | No accepted receipt; retry can submit again safely. |
| After commit, before response | Repeated idempotency key returns the same accepted run. |
| After runtime start, before outbox acknowledgement | Retry discovers the same run; no second intended execution. |
| After run completion and runtime-history expiry | Durable application receipts still prevent stale-outbox re-creation within the promised deduplication policy. |
| After cancel request, before dispatch | Cancelled accepted work is not started by a stale dispatcher. |
| Before provider request | Retry only when pre-send evidence or idempotency policy makes it safe. |
| After provider commit, before local result | Provider-idempotent action returns its original result; unsafe action becomes uncertain. |
| After local result, before activity completion | Result reporting is idempotent; late attempts cannot overwrite accepted output. |
| During a durable wait | Restarted workers do not lose the wait or consume a worker per waiting run. |
| During projection persistence | Projection repair does not repeat the provider action. |
| During credential refresh | One approved current token version survives concurrent refresh results. |
| During schedule reconciliation | Desired/observed revisions recover without duplicate occurrence admission. |
| During main-run overlap reservation | Concurrent occurrences do not bypass no-overlap policy. |
| During application/runtime database restore | Reconciliation identifies missing runs, stale slots, and potentially repeated remote effects. |

## Capacity test

Use the workload from [Operations](07-operations.md#initial-operating-envelope). Report payload sizes, action durations, branch/Map distribution, retry rates, worker count, database size, and provider mock behavior.

Run sustained load, the proposed 20-starts/second burst, 1,000 durable waits, and recovery after a worker outage. Measure p50/p95/p99 dispatch latency, oldest outbox age, queue drain, memory, CPU, database contention, and history growth.

Repeat at 10x only to identify the next limit and price a realistic configuration. Do not turn an extrapolation into a supported-capacity claim.

## Security test details

Attempt cross-workspace access with valid foreign IDs for workflows, runs, payloads, credentials, SSE, imports, and internal APIs. Include access that changes while a run waits.

Inject a distinctive fake secret into credentials and provider responses. Search logs, traces, runtime history, errors, exports, and UI data for that value. Explain any permitted encrypted storage locations.

Test metadata/loopback/private destinations, redirects, DNS changes, oversized bodies, deep objects, expression cost, pagination limits, and nested fan-out. Verify approved internal destinations still work.

## Release checklist

- [ ] A-01 runtime decision has evidence and exact supported versions.
- [ ] V-01 through V-14 have recorded outcomes.
- [ ] No required security or durability check is skipped without an explicit release decision.
- [ ] Old active runs survive the proposed upgrade path.
- [ ] Backup restore meets the agreed RPO/RTO on the intended environment.
- [ ] Operator owner, budget, retention, and service target are recorded.
- [ ] Dependency licenses and known vulnerabilities are reviewed.
- [ ] Runbooks include uncertain remote outcomes and restore reconciliation.
- [ ] UI behavior has desktop, narrow-screen, and keyboard evidence.
- [ ] Known limitations are visible in release notes and product behavior.
- [ ] A controlled pilot demonstrates real weekly use.

## Release evidence template

```text
Check ID:
Build / commit:
Component versions:
Environment:
Input / failure injected:
Expected result:
Observed result:
Status: passed | failed | skipped | blocked
Evidence location:
Limitations:
Owner and date:
```

Next: [Decisions and risks](10-decisions-and-risks.md).
