# Runnelix history

Scope: product planning and implementation. Dates use Asia/Kolkata unless stated otherwise. This record links evidence and distinguishes completed work from proposals.

## 2026-10-17 — product plan published

- **What:** named the product Runnelix and published the planning documents in public repository `rushikeshg25/runnelix`.
- **Why:** establish readable product requirements, execution rules, and release gates before implementation.
- **Evidence:** commit [`f3f6864`](https://github.com/rushikeshg25/runnelix/commit/f3f6864264c08aa2f0b4c611c7082b346eee111b), [planning guide](docs/plans/README.md).
- **Confidence:** confirmed by the committed documents and matching remote file hashes.

## 2026-10-17 — Phase 0 implementation begins

- **What:** adopted nine meaningful commits for the runtime proof, with per-slice decisions and evidence.
- **Why:** the user requested phased implementation, 8–10 commits per phase, and a durable record of what changed and why.
- **Choice:** use a project-local Temporal development server because Docker is not running. Keep production PostgreSQL recovery unverified rather than equating it with a development restore.
- **Evidence:** [Phase 0 implementation record](docs/implementation/phase-0.md), [tool installer](scripts/install-temporal.py), [decision log](logs/2026-10-17.md).

## Open questions

- Which production topology and budget should the Temporal operations gate use?
- Which real integrations and pilot workflows come first?

Per-step reasoning lives in `logs/YYYY-MM-DD.md`; this history records milestones and links to their evidence.

## 2026-10-17 — shared invocation contract

- **What:** added a bounded Phase 0 request contract, JSON Schema, and shared Go/TypeScript fixtures.
- **Why:** cross-language workers must agree on required fields, unknown fields, version IDs, numeric limits, Unicode, and payload limits before executing actions.
- **Evidence:** [contract fixtures](contracts/spike/fixtures.json), [Go validation/tests](internal/spike/contract_test.go), [TypeScript tests](node-worker/contract.test.ts). Go tests and all 15 TypeScript tests passed.
- **Prior milestone:** tooling commit `cfd2a15`.

## 2026-10-17 — durable mock provider

- **What:** added a loopback-only provider fixture with a persistent fake-action ledger, safe/unsafe writes, pre-write transient failures, lost responses, and held responses.
- **Why:** recovery tests need an observable external effect independent of Temporal's activity result.
- **Evidence:** [provider tests](internal/mockprovider/server_test.go), [server](internal/mockprovider/server.go). `go test -race ./internal/mockprovider` passed, including concurrent deduplication and post-commit disconnect/reload.
- **Prior milestone:** shared-contract commit `da5a1b8`.

## 2026-10-17 — Go runtime orchestration

- **What:** implemented one durable wait followed by a version-routed TypeScript activity, plus start/query/result/cancel/describe commands.
- **Why:** prove the runtime boundary before building a graph engine. Unsafe actions receive one attempt; ambiguous failures return a distinct needs-attention result.
- **Evidence:** [workflow tests](internal/spike/workflow_test.go), [CLI](cmd/spike/main.go). Go tests prove safe retry, unsafe non-retry, pre-send failure handling, and cancellation during a durable wait.
- **Limit:** needs-attention is a completed spike result, not the product's eventual paused reconciliation state.
- **Prior milestone:** provider commit `8eb08f6`.

## 2026-10-17 — TypeScript action worker

- **What:** added the Temporal activity worker, fixed loopback destination, request/result validation, response-size cap, cancellation, heartbeats, and sanitized typed failures.
- **Why:** provider-specific work belongs outside replayed Go workflow code; raw network errors must not leak into execution history.
- **Evidence:** [activity tests](node-worker/activity.test.ts), [worker](node-worker/main.ts). All 21 TypeScript tests pass; cross-process runtime checks follow.
- **Prior milestone:** Go orchestration commit `b9fa212`.

## 2026-10-17 — real-runtime test harness

- **What:** added an integration harness that starts isolated Temporal/provider/Go/TypeScript processes, preserves per-run artifacts, and stops only its own process groups.
- **Why:** unit tests do not prove cross-language transport or remote-effect behavior. The baseline cases assert payload round trips, safe retries, and unsafe uncertainty against a durable provider ledger.
- **Evidence:** [harness](tests/integration/phase0.py). `make build` and Python compilation passed. Live execution is pending the development-server download at this milestone.
- **Blocker:** the CLI release download was slow; its partial archive was preserved and the remaining ranges were downloaded concurrently. Full-archive SHA-256 verification is required before execution.
- **Prior milestone:** TypeScript worker commit `b857451`.

## 2026-10-17 — live crash and cancellation evidence

- **What:** ran ten integration cases against Temporal CLI 1.9.1 with embedded Server 1.32.0, real Go and TypeScript workers, and the persistent mock provider.
- **Why:** validate process failure boundaries rather than infer recovery from unit tests.
- **Observed:** all ten cases passed: cross-language data, safe transient retry, safe/unsafe response loss, worker restart across a wait, safe/unsafe worker death after commit, waiting/active cancellation, and server crash recovery.
- **Evidence:** [machine-readable results](docs/implementation/evidence/phase-0-recovery.json), [failure harness](tests/integration/phase0.py). The CLI archive matched its pinned upstream SHA-256.
- **Limits:** localhost development server and SQLite persistence; no production PostgreSQL restore, throughput target, or production isolation claim.
- **Prior milestone:** harness commit `dbfed23`.

## 2026-10-17 — version routing and cold restore

- **What:** all twelve live integration cases passed. Added v1/v2 workflow/action queue isolation and a cold SQLite backup restored into a separate directory.
- **Why:** old runs must retain compatible workers; restoring runtime state can replay a remote action that happened after the snapshot.
- **Observed:** v2 workers did not consume the old v1 run/action. Restoring an earlier runtime snapshot caused a second provider request, but provider idempotency retained the same single external effect.
- **Evidence:** [full report with versions and source hashes](docs/implementation/evidence/phase-0-full.json), [harness](tests/integration/phase0.py).
- **Limits:** application-level versioned queues, not Temporal deployment-version APIs; SQLite cold backup, not production PostgreSQL PITR or unsafe-write reconciliation.
- **Prior milestone:** crash/cancellation commit `2f2f063`.

## 2026-10-17 — Phase 0 development proof completed

- **What:** completed the ninth implementation slice: CI configuration, reproducible setup instructions, a commit ledger, and a runtime decision. Updated the HTML proposal to link current implementation evidence.
- **Why:** future phases need a repeatable check and a precise boundary between observed behavior and production promises.
- **Decision:** continue development with Temporal. Technical feasibility passed for this narrow spike; production topology, PostgreSQL recovery, operating budget, capacity, and ownership remain open. This explicitly refines the original all-or-nothing Phase 0 gate.
- **Evidence:** [ADR 0001](docs/decisions/0001-temporal-development-runtime.md), [phase record](docs/implementation/phase-0.md), [CI workflow](.github/workflows/phase0.yml), and [full integration report](docs/implementation/evidence/phase-0-full.json).
- **Verification:** `make test check` passed: Go race tests, all 21 TypeScript tests, Go vet, type checks, and whitespace checks. All 20 source hashes still match the twelve-case integration report. Checked 98 local document links/anchors. Desktop and narrow HTML previews rendered without console errors.
- **Limits:** implementation commits are local; hosted CI has not run. Phase 1 has not started. This is not a production release.
- **Prior milestone:** version/restore commit `bf513cf`.

## 2026-10-17 — Phase 1 contracts begin

- **What:** established nine Phase 1 slices and implemented bounded workflow parsing and deterministic plan compilation.
- **Why:** publication needs a stable, validated execution definition before persistence or dispatch can be safe.
- **Evidence:** [phase scope](docs/implementation/phase-1.md), [compiler tests](internal/definition/definition_test.go). `go test ./internal/definition` passed.
- **Boundary:** manual trigger and mock actions only; the phase's runtime records admission, not graph completion. PostgreSQL 17.6 is installed locally; integration will use a separate cluster.

## 2026-10-17 — internal node host and activity contract

- **What:** added the mock.echo module, shared activity fixtures, scoped resolver boundary, and generated catalog with a source-bundle digest.
- **Why:** internal actions should be added through their module and catalog, while the host enforces validation and pinned identity.
- **Evidence:** [host tests](node-worker/product-host.test.ts), [Go fixture tests](internal/definition/catalog_test.go), [catalog generator](scripts/build-catalog.py). TypeScript and Go contract checks passed.
- **Limit:** the fixture host does not provide a production payload service, credentials, or a product graph interpreter.

## 2026-10-17 — PostgreSQL foundation

- **What:** added schema constraints, immutable publication storage, checksum-verified migrations, and an isolated PostgreSQL test harness.
- **Why:** test actual transaction/locking behavior without changing existing databases or relying on an in-memory substitute.
- **Evidence:** [migration](internal/store/migrations/001_initial.sql), [migration test](internal/store/store_test.go), [harness](tests/integration/postgres.py). `python3 tests/integration/postgres.py go test -race ./internal/store` passed on PostgreSQL 17.6.
- **Limit:** schema creation is not a production migration or recovery drill.

## 2026-10-17 — draft and publication transactions

- **What:** implemented optimistic draft revisions and immutable, validated publication with pinned plans.
- **Why:** concurrent editors must see conflicts, and later edits must not alter published execution definitions.
- **Evidence:** [store methods](internal/store/workflows.go), [PostgreSQL tests](internal/store/workflows_test.go). Race-enabled database tests passed, including direct SQL immutability and cross-workspace FK rejection.

## 2026-10-17 — atomic run admission

- **What:** implemented durable manual admission, canonical input fingerprints, stable run IDs, and atomic inbox/outbox writes.
- **Why:** a successful acceptance response must survive dispatcher downtime without creating duplicate intended runs.
- **Evidence:** [admission tests](internal/store/admission_test.go). Twelve concurrent duplicate requests returned the same run; a forced final-insert failure left no partial run or receipt. Race-enabled PostgreSQL tests passed.
- **Boundary:** keys have no automatic expiry yet; input is bounded fake development data. Retention, quotas, and authenticated triggers remain later phases.

## 2026-10-17 — recoverable runtime admission bridge

- **What:** implemented leased outbox dispatch and a receipt-only Temporal workflow with stable identity.
- **Why:** handle uncertain start responses and dispatcher death without trusting an RPC response as proof of recorded admission.
- **Evidence:** [dispatcher](internal/dispatch/dispatch.go), [lease tests](internal/store/dispatch_test.go), [workflow identity tests](internal/dispatch/dispatch_test.go). Database fencing and workflow unit tests passed; live process failure tests remain the next evidence step.
- **Limit:** a runtime receipt does not change the product run from accepted to succeeded.

## 2026-10-17 — development HTTP contract

- **What:** added a loopback-only API and separate operational commands, plus an OpenAPI baseline and compiled-plan schema.
- **Why:** make the persistence slice usable without a canvas while preserving explicit revisions, scope, and durable acceptance semantics.
- **Evidence:** [API](internal/api/api.go), [boundary tests](internal/api/api_test.go), [OpenAPI](contracts/product/openapi.json), [commands](cmd/runnelix/main.go). Go API/contract checks and command build passed.
- **Limit:** workspace routes do not authenticate callers. This API supports local fake-data development only; frontend generation and authenticated access remain future work.

## 2026-10-17 — Phase 1 live failure-window evidence

- **What:** seven live HTTP/PostgreSQL/Temporal integration cases passed. Dispatcher SIGKILL tests cover lease acquisition, accepted runtime start, and durable receipt before acknowledgement.
- **Why:** unit tests cannot establish that database admission and runtime handoff survive process boundaries and lost acknowledgements.
- **Observed:** twelve duplicate HTTP starts returned one run; delayed dispatch retained the published version; two dispatchers produced one receipt per run. Receipt reconciliation completed with Temporal offline and no new start call.
- **Evidence:** [machine-readable report](docs/implementation/evidence/phase-1-full.json), [harness](tests/integration/phase1.py), [crash probe](tests/integration/phase1probe/main.go). The report records source hashes, unchanged-source verification, runtime versions, and clean shutdown.
- **Correction during testing:** the first restart fixture omitted its isolated port and failed to bind default port 5432. Shared startup options fixed the harness; the existing system database remained untouched. The failed artifact directory is recorded in the decision log.
- **Review fixes:** added future-migration rejection and field-level schema paths; moved node registration outside the generic host. Database race tests and all 33 TypeScript tests passed.
- **Limits:** actual PostgreSQL restart, but no production backup/PITR or full graph execution. Temporal still uses development SQLite.

## 2026-10-17 — Phase 1 development deliverables completed

- **What:** completed nine meaningful implementation commits for contracts, persistence, publication, durable admission, runtime receipts, and a local API. Added CI and reproducible setup/verification instructions.
- **Why:** the next phase can build actual workflow execution on tested admission and version boundaries instead of combining every failure mode at once.
- **Evidence:** [phase record and commit ledger](docs/implementation/phase-1.md), [ADR 0002](docs/decisions/0002-postgresql-admission-and-receipts.md), [PostgreSQL test record](docs/implementation/evidence/phase-1-postgres.json), [Phase 1 live report](docs/implementation/evidence/phase-1-full.json), and [Phase 0 regression report](docs/implementation/evidence/phase-0-after-phase-1.json).
- **Verification:** `make test check integration` and `make integration-phase1` passed. All 33 TypeScript tests, seven Phase 1 live cases, twelve Phase 0 regression cases, and actual PostgreSQL race tests passed. OpenAPI 3.1 validation passed. Document links, evidence hashes, CI YAML, and desktop/narrow HTML previews were checked.
- **Remaining:** implementation commits are local; hosted CI has not run. Phase 2 has not started. This phase proves admission, not product graph execution or production readiness.
- **Prior milestone:** failure-window evidence commit `d228d6a`.

## 2026-10-17 — hosted baseline and Phase 2 execution records

- **What:** GitHub Actions passed both Phase 0 and Phase 1 at pushed commit `f4ded52`. Began Phase 2 with an explicit execution mode and PostgreSQL payload/invocation/attempt/event contracts.
- **Why:** existing admission-only records must retain their meaning, and late attempts must not overwrite the result selected by the runtime.
- **Evidence:** [Phase 0 CI](https://github.com/rushikeshg25/runnelix/actions/runs/39055529924), [Phase 1 CI](https://github.com/rushikeshg25/runnelix/actions/runs/39055529911), [execution persistence tests](internal/store/execution_test.go), [phase scope](docs/implementation/phase-2.md). Isolated PostgreSQL race tests passed.

## 2026-10-17 — first pinned graph interpreter

- **What:** added a separate execution workflow with branch/skip behavior, structured events, cancellation, and reference-based node invocation. Added bounded CEL mapping and condition evaluation.
- **Why:** execute the published plan deterministically while keeping database/network work in activities and payload values outside workflow history.
- **Evidence:** [engine tests](internal/engine/engine_test.go), [mapping tests](internal/mapping/mapping_test.go), and [compiler](internal/definition/definition.go). Go and PostgreSQL checks passed. CEL is pinned at `cel.dev/cel-go v0.32.0`.
- **Boundary:** the real TypeScript data-plane connection and UI are still being built; this milestone is not an end-to-end execution claim.

## 2026-10-17 — protected node execution data plane

- **What:** added a token-protected node service, digest-routed TypeScript worker, and HTTP GET/POST modules. Added encrypted credentials with workflow grants and destination binding.
- **Why:** node modules need scoped input, output recording, and approved network calls without database credentials or plaintext provider secrets.
- **Decisions:** GET may retry; POST is unsafe and cannot retry automatically. Requests must match the published URL and node method. DNS results are checked before dialing; redirects and proxy environment settings are disabled. Decoded credential echoes are redacted before storage.
- **Evidence:** [egress tests](internal/nodeservice/egress_test.go), [credential tests](internal/store/credentials_test.go), [worker tests](node-worker/product-activity.test.ts). Focused Go tests and isolated PostgreSQL race tests passed. Full runtime wiring follows in the next slice.

## 2026-10-17 — execution and inspection API

- **What:** routed explicit execution runs to the graph engine; retained the receipt-only workflow. Added runtime commands, bounded list pages, event streaming, payload/attempt inspection, credential metadata/grants, and mapping preview.
- **Why:** the editor needs a complete author-to-inspection path, while cancellation and webhook delivery must survive lost responses.
- **Decisions:** persist cancellation intent before the dispatcher calls Temporal. Repeat cancellation until a terminal projection exists. Sign timestamp, delivery key, and raw webhook body; reject old/future signatures and isolate webhook deduplication from manual requests. EventSource reconnects from persisted sequence IDs.
- **Evidence:** [API implementation](internal/api/execution.go), [signature tests](internal/api/execution_test.go), [trigger-source tests](internal/store/execution_test.go), [OpenAPI](contracts/product/openapi.json). OpenAPI validation passed; live runtime checks follow in slice 7.

## 2026-10-17 — local editor foundation

- **What:** added React/TypeScript/Vite, a workflow sidebar, bounded list paging, schema-driven configuration fields, draft saving, validation, and publication. The editor preserves local edits when a draft revision conflicts.
- **Why:** expose server-owned workflow contracts through simple forms without adding a second graph interpreter or state-management framework.
- **Evidence:** [editor source](web/src/App.tsx), [typed client](web/src/api.ts), [schema forms](web/src/SchemaForm.tsx). Browser verification follows after canvas and inspection wiring.

## 2026-10-17 — canvas and pinned run inspection

- **What:** added the graph canvas, keyboard-accessible connection controls, CEL preview, published-version execution, and a persisted-event run inspector with outputs and attempt records.
- **Why:** graph layout is an editor preference, while graph meaning and run versions remain server-owned. Inspection must show the version that ran, even after the draft changes.
- **Decisions:** keep positions in local browser storage. Keep connection editing available through ordinary form controls. Retain the same submission key after an uncertain HTTP response, so a retry does not create another intended run.
- **Evidence:** [canvas](web/src/Canvas.tsx), [inspector](web/src/RunInspector.tsx), [mapping preview](web/src/MappingPreview.tsx). Production build passed; live browser verification follows.

## 2026-10-17 — live Phase 2 execution evidence

- **What:** seven live cases passed across PostgreSQL, Temporal, the Go engine, the TypeScript worker, and a local HTTP provider fixture.
- **Observed:** branch/skip execution produced the expected mapped output; GET retried once after 429; unsafe POST paused after one call; pre-dispatch cancellation made no provider call; credential echoes were redacted; signed delivery deduplicated; SSE resumed; engine SIGKILL recovered a queued node.
- **Fix:** the first run exposed JSON whole-number arithmetic failing in CEL. A focused regression failed before recursive numeric normalization and passed afterward. The full live run then passed with unchanged sources and clean process shutdown.
- **Evidence:** [live report](docs/implementation/evidence/phase-2-full.json), [harness](tests/integration/phase2.py), [numeric regression](internal/mapping/mapping_test.go). No real provider records were created.

## 2026-10-17 — local startup and browser verification

- **What:** added an owned local launcher with persistent development data, loopback bindings, retained encryption keys, process cleanup, port preflight, and a single-owner lock. Added Phase 2 CI and reproducible editor formatting.
- **Observed:** browser creation, publication, execution, pinned inspection, connection controls, keyboard focus/activation, and desktop/narrow layouts work. A duplicate React key caused sibling identity warnings during the first browser check; unique component keys fixed it. A fresh reload showed one canvas and one inspector with no new console errors.
- **Evidence:** [launcher](scripts/dev.py), [CI](.github/workflows/phase2.yml), [browser record](docs/implementation/evidence/phase-2-browser.json). Existing system PostgreSQL was not stopped or modified. Local state survived a launcher shutdown/restart.
- **Boundary:** browser checks used fixture data. Independent teammate acceptance remains pending.

## 2026-10-17 — saved graph normalization fix

- **Observed:** after editing a connection, the API saved the draft but validation stayed disabled. PostgreSQL JSONB reordered object keys, while the editor compared serialized graphs and retained the pre-save representation.
- **Fix:** replace the local graph with the canonical saved graph returned by the API. Browser verification now shows Draft saved after save; an unconnected node produces the expected field error, and removing it restores successful validation. The old run still displays published draft revision 1 after draft revision 5.
- **Evidence:** [save handler](web/src/App.tsx), [updated browser record](docs/implementation/evidence/phase-2-browser.json).

## 2026-10-17 — Phase 2 local development deliverables

- **What:** completed nine meaningful Phase 2 slices with a visual editor, pinned graph engine, protected TypeScript node worker, HTTP actions, credential grants, signed webhook admission, cancellation, and run inspection. Added ADR 0003, local setup, CI, evidence, and updated the HTML overview.
- **Verification:** `make test check integration-phase2` passed, including 36 TypeScript tests, isolated PostgreSQL race tests, seven live execution cases, editor type/format checks, and production editor build. Phase 1's seven live regression cases and Phase 0's twelve runtime cases passed. OpenAPI validation and local documentation links passed. Final source hashes match the execution report. HTML previews at 1100px and 390px reported no console errors.
- **Evidence:** [phase ledger](docs/implementation/phase-2.md), [execution](docs/implementation/evidence/phase-2-full.json), [browser](docs/implementation/evidence/phase-2-browser.json), [admission regression](docs/implementation/evidence/phase-1-after-phase-2.json), [runtime regression](docs/implementation/evidence/phase-0-after-phase-2.json), [ADR 0003](docs/decisions/0003-execution-data-plane.md).
- **Limits:** teammate acceptance remains pending. GitHub issues and Slack messages are selected for Phase 4; deployment remains local. Phase 3 semantics, production controls, actual rollout, and a two-week pilot remain unfinished. Hosted Phase 2 CI has not yet run.

## 2026-10-17 — versioned merge and durable wait

- **What:** Phase 2 is pushed at `9cd61d1`. Added interpreter version 3 with explicit two-input merge policies and durable waits; retained the V2 workflow and queue for old histories.
- **Why:** new control semantics must not silently alter replay of existing executions. Merge waits for deterministic upstream outcomes and ignores skipped branches without waiting for nonexistent work.
- **Evidence:** [V3 workflow test](internal/engine/v3_test.go), [control activity](internal/engine/controls.go), [Phase 3 plan](docs/implementation/phase-3.md). Compiler/engine/dispatcher checks passed. The durable-clock test covers a one-hour timer and a merge with one skipped input.

## 2026-10-18 — bounded per-item action execution

- **What:** added explicit Map bodies containing one pinned trusted action, per-item invocation identity, bounded batches, and ordered output assembly. Empty Maps schedule no body action; nested controls are rejected as bodies.
- **Why:** arrays must expand only through an explicit construct, and retries must retain the same logical action identity for each item.
- **Decisions:** maximum 1,000 items and 10 concurrent bodies, with the existing 8 KiB payload boundary. Serialize projection calls so concurrently completed items cannot create event-sequence gaps. Map output is `{items:[...]}` in input order.
- **Evidence:** [Map execution](internal/engine/map.go), [ordering/concurrency tests](internal/engine/v3_test.go), [identity test](internal/store/execution_test.go). Runtime and PostgreSQL verification accompany this slice.

## 2026-10-18 — durable uncertain-outcome reconciliation

- **What:** added immutable operator decisions for an uncertain invocation. An operator can confirm a schema-valid provider result or end the run; the runtime consumes the decision through a retried signal.
- **Why:** an uncertain write must not be retried merely because a response was lost. Decision persistence must survive API or dispatcher failure.
- **Decisions:** no automatic resend option. Confirmed results fence late worker results. Concurrent Map completions retain the needs-attention state while any item remains unresolved. The local operator identity is explicit; production role enforcement follows in hardening.
- **Evidence:** [resolution persistence tests](internal/store/resolutions_test.go), [runtime reconciliation test](internal/engine/resolutions_test.go). Tests cover immutable choices, durable delivery, late-attempt fencing, and resumption without another provider request.

## 2026-10-18 — transactional execution and provider limits

- **What:** added a 100-active-execution workspace limit, pinned per-run limits of 1,000 action invocations and 8 MiB of payload storage, and shared host/credential request buckets.
- **Why:** per-process counters cannot enforce limits across replicas or concurrent Map items. Existing references and invocations must remain idempotent without consuming budget twice.
- **Decisions:** use PostgreSQL row locks and transactional counters. Provider admission allows 10 requests/second per workspace/host and 5 per workspace/credential, with no burst. A rejected unsafe request fails before send; it is not reported as an uncertain provider effect.
- **Evidence:** [limit tests](internal/store/limits_test.go), [provider gates](internal/store/limits.go). Concurrent checks verify exact invocation capacity, payload deduplication, shared request admission, and refill.

## 2026-10-18 — Temporal-owned schedules and transactional overlap protection

- **What:** added versioned daily/interval schedules, desired/observed synchronization, a Temporal admission wrapper, and immutable occurrence records. The active run itself acts as the durable workflow slot.
- **Why:** the scheduling wrapper can finish while the product run is still active. Admission must enforce no overlap under the same database locks as manual starts.
- **Policy:** daily schedules use an IANA timezone; nonexistent local times are skipped by Temporal, repeated daily wall times share one occurrence key. Interval schedules use UTC and a minimum 60-second interval. Overlap is skipped, catch-up is limited to Temporal's one-minute window, and no pending-occurrence coalescing is claimed.
- **Evidence:** [schedule store tests](internal/store/schedules_test.go), [runtime adapter](internal/scheduling/scheduling.go). Tests cover repeated DST-hour identity, active-run exclusion, terminal release, disabled/stale revisions, and desired/observed replay. Actual schedule delivery is verified in the live Phase 3 suite.

## 2026-10-18 — independent projection recovery and artifact references

- **What:** V3 retains an ordered execution journal and returns it with terminal product results. Projection writes are best-effort; a separate reconciler copies missing runtime-owned events back to PostgreSQL. Added a report of worker digests still referenced by active versions or runs.
- **Why:** a database status-write outage must not erase known execution outcomes or create a second scheduler. Closed-run repair must work without an available workflow worker.
- **Decisions:** inspect runtime closure before querying a worker; completed executions expose their journal directly through the stored result. Rotate recovery candidates by a separate checked timestamp so long waits do not starve other repairs.
- **Evidence:** [journal regression](internal/engine/v3_test.go), [reconciler](internal/dispatch/recovery.go), [artifact report](internal/store/recovery.go). Engine and PostgreSQL checks passed. Phase 2 hosted CI passed for Phase 0, Phase 1, and Phase 2 workflows at `9cd61d1`.
- **Limit:** forced Temporal termination/history deletion is not normal product completion and still needs operator recovery; no external-effect reconciliation is inferred from runtime status alone.

## 2026-10-18 — live Phase 3 failure and compatibility evidence

- **What:** eight live cases passed with isolated PostgreSQL, Temporal, and fixture providers. Checks cover bounded Map, engine-crash wait recovery, immutable operator reconciliation, offline-engine projection repair, schedules, actual DST calendar matching, retained V2 workers, and concurrent admission.
- **Why:** unit tests alone cannot prove behavior across process crashes or runtime/database boundaries. A bounded dispatcher batch removes the one-start-per-tick bottleneck while yielding to reconciliation on each tick.
- **Evidence:** [generated report](docs/implementation/evidence/phase-3-full.json), [harness](tests/integration/phase3.py), [V2 fixture builder](scripts/build-compat-v2.py). Thirty single-action runs completed in 2.390 seconds (12.554 runs/second) with ten HTTP clients. This is local smoke-load evidence, not production capacity acceptance.

## 2026-10-18 — expose Phase 3 controls in the editor

- **What:** added JSON object configuration, defaults for merge/wait/Map, schedule creation and enable/pause controls, and permanent operator decisions with invocation identities. Added Phase 3 OpenAPI routes and a CI job that builds the pinned V2 compatibility fixture from full Git history.
- **Why:** authors need the same supported contracts in the editor and API. Invalid JSON remains in the draft so publication cannot silently use an older valid value. Schedules start paused so creation does not immediately start actions.
- **Evidence:** [schema editor](web/src/SchemaForm.tsx), [schedule editor](web/src/Schedules.tsx), [resolution form](web/src/ResolutionForm.tsx), [CI](.github/workflows/phase3.yml). Browser verification is recorded with the completed phase evidence.
- **Browser findings:** permanent confirmation resumed a real local uncertain run to success. Fixed mobile grid overflow with three saved workflows and a restart preflight that mistook TCP TIME_WAIT for a listening service. Updated run-picker states during inspection.

## 2026-10-18 — distinguish schedule duplicates from overlapping occurrences

- **What:** a repeat of the live suite exposed two manual triggers within the same Temporal scheduled second. The database correctly stored one occurrence; the test expected two distinct occurrences.
- **Why:** duplicate delivery and overlap are separate contracts. The overlap fixture now crosses a second boundary before its second trigger and avoids automatic minute ticks during the test.
- **Evidence:** failed evidence remains at `artifacts/phase1/20261017T185721Z-5a4f94`; retained PostgreSQL and Temporal recentActions confirmed the shared second identity. The test still requires one accepted run and one overlap outcome.

## 2026-10-18 — close Phase 3 regressions and document local limits

- **What:** fixed cancellation while waiting for an operator decision by selecting the resolution signal or context cancellation. Added a focused test that failed with ScheduleToClose before the fix. The live Phase 2 cancellation regression now passes.
- **Why:** an indefinite signal receive does not wake on cancellation automatically. A cancelled uncertain action must stop waiting without sending another provider request.
- **Evidence:** [cancellation regression](internal/engine/resolutions_test.go), [Phase 2 regression report](docs/implementation/evidence/phase-2-after-phase-3.json), [Phase 3 report](docs/implementation/evidence/phase-3-full.json), [Phase 1 regression report](docs/implementation/evidence/phase-1-after-phase-3.json). The first nine Phase 3 commits were pushed through `e316d2a`; this final slice includes the fix and documentation.
- **Limits:** production load, independent teammate acceptance, a different developer's node-authoring trial, and the two-week pilot remain open. Local schedule policy uses skip overlap, with no coalesced pending occurrence.

## 2026-10-18 — declarative action request policies

- **What:** moved HTTP request authorization from provider-name switches into trusted catalog descriptors. Policies pin method, fixed URL or a generic URL field, escaped path fields, fixed headers, and required credential presence.
- **Why:** new provider actions need the same network boundary without changes to the interpreter or ordinary editor. Workflow authors cannot set authorization headers or replace a fixed provider host.
- **Evidence:** [policy tests](internal/definition/http_policy_test.go), [network boundary tests](internal/nodeservice/egress_test.go), [Phase 4 plan](docs/implementation/phase-4.md). Generic HTTP remains compatible; the existing worker envelope is unchanged.

## 2026-10-19 — GitHub issue action through the built-in node path

- **What:** added `github.createIssue@1`, narrow config/input/output schemas, fixed request metadata, and adapter fixtures. The worker returns only the issue ID, number, and URL.
- **Why:** the first integration proves that an ordinary action can use existing execution and editor contracts. Creating an issue is an unsafe write; missing responses must not trigger automatic resends.
- **Evidence:** [adapter fixtures](node-worker/product-integrations.test.ts), [author instructions](docs/nodes/github-create-issue.md). All 39 TypeScript checks and Go catalog checks passed. No real issue was created.

## 2026-10-30 — finish Phase 4 integrations and portable workflows

- **What:** completed the started phase in six continuation commits: Slack adapter, credential rotation/revocation, portable workflow API, editor controls, live fixture verification, and node-authoring documentation with catalog completeness checks. The two earlier Phase 4 commits remain the HTTP policy and GitHub foundation.
- **Why:** integrations should use the existing interpreter/editor seam, while workflow transfer must not copy credentials, runtime state, or foreign publication artifacts.
- **Decisions:** exact destination-matched credential slots include Map bodies. Import atomically creates a new unpublished draft and explicit grants. Rotation preserves credential IDs; revocation blocks future resolutions and cannot be undone by rotation. Unsafe outcomes retain conservative operator reconciliation without automatic resend.
- **Evidence:** [live Phase 4 report](docs/implementation/evidence/phase-4-full.json), [PostgreSQL report](docs/implementation/evidence/phase-4-postgres.json), [Phase 2 regression](docs/implementation/evidence/phase-2-after-phase-4.json), and [browser verification](docs/implementation/evidence/phase-4-browser.json). Four live groups and all seven Phase 2 cases passed. Real local workers exercised provider success, dropped responses, Slack rejection, malformed output, restart recovery, import execution, rotation, and revocation. Unit/static checks and 43 TypeScript tests passed.
- **Limits:** only local fixture providers were used. Independent developer node-author acceptance, teammate acceptance, real-provider permissions, production security/load, and pilot gates remain open. The browser file picker was not exercised. No push or deployment was performed.

## 2026-10-30 — review fixes and first Phase 5 identity slice

- **What:** fixed asynchronous import data loss, preserved raw JSON for strict duplicate-key validation, reduced export file inflation, added credential cursor pagination, and wrapped long imported node IDs on narrow screens. The [review record](docs/implementation/phase-4-review.md) separates Standards and Spec findings and records their local fixes.
- **Evidence:** full unit/static checks, 43 worker tests, three transport tests, isolated PostgreSQL, four live Phase 4 groups, seven Phase 2 regressions, OpenAPI validation, and browser reproductions passed. The large export reopened through the file input, a credential behind 100 revoked records became selectable, and a delayed import preserved unsaved edits.
- **Next phase:** added configured OIDC discovery/JWKS verification using pinned `coreos/go-oidc`, with issuer/subject identity inspection and safe failure responses. Real generated-RSA fixture tests cover signature, issuer, audience, time, authorized party, discovery bounds, and API access denial. The identity review's missing issued-at and query-bearing JWKS findings were fixed before commit `dfa4db6`.
- **Boundary:** OIDC mode grants no workspace privileges; all application routes except identity inspection remain closed until membership authorization ships. Development mode stays loopback-only. Browser sessions, production provider selection, roles, audit, retention, and operations remain open. No pending MR existed to merge, and no changes were pushed.

## 2026-10-30 — Phase 5 workspace authorization

- **What:** added issuer/subject memberships, four roles, separate publish/execute/data-read grants, explicit initial-owner provisioning, and owner-managed membership APIs. This replaces the preceding identity-only closed boundary.
- **Why:** a verified identity must not grant access to another workspace. Bound imports require owner access because they create credential grants. Membership transactions preserve the last owner, and event streams recheck permissions while connected.
- **Evidence:** persistence/provisioning commit `6019d9f` and API enforcement commit `318e864`; [membership implementation and verification](docs/implementation/phase-5-memberships.md). Unit/static checks, signed-token PostgreSQL integration, live SSE revocation, concurrent owner demotions, four existing live integration groups, and OpenAPI validation passed locally.
- **Limits:** browser sessions and the remaining security/operations slices are still open. Migration 010 is required; no existing workspace gains an owner automatically. This continuation was committed locally without a push or deployment.

## 2026-10-30 — Phase 5 local browser sessions

- **What:** pushed the three workspace-membership commits through `6e6f769` to `origin/main`, then added public-client OIDC authorization-code login with S256 PKCE, nonce/state validation, PostgreSQL sessions, HttpOnly cookies, CSRF checks, and local logout.
- **Why:** the browser editor and EventSource need authenticated access without putting provider tokens in browser storage. Revocation must affect live streams, and session expiry must not silently discard unsaved drafts.
- **Evidence:** server commit `1c63b37`, editor commit `9e16cf8`, [session design and checks](docs/implementation/phase-5-sessions.md), and [browser verification](docs/implementation/evidence/phase-5-sessions-browser.json). Signed PostgreSQL tests cover replay, expiry, CSRF, session binding/rotation, and logout closing a live stream. Actual desktop/narrow browser checks cover login, workflow creation, draft preservation, and logout. Four existing live Phase 4 groups passed.
- **Limits:** the new slice is committed locally, without an additional push or deployment. Migration 011 and a registered public OIDC client are required. Only a local fixture provider was exercised; production provider acceptance and the audit/operations slices remain open. Owned verification services were stopped and their artifacts retained.

## 2026-10-30 — Phase 5 transactional workspace audit

- **What:** added append-only audit records for committed workflow, credential, membership, admission, cancellation, reconciliation, and schedule changes. Owners can page through workspace records through a new API. Updated the README and OpenAPI contract.
- **Why:** audit insertion must succeed in the same transaction as the business change. A workspace counter preserves committed pagination order under concurrency. Typed fields exclude sensitive payloads; actors and request IDs come from trusted server boundaries.
- **Evidence:** [design and exact verification](docs/implementation/phase-5-audit.md), [rollback and concurrency tests](internal/store/audit_test.go), and [signed API checks](internal/identity/memberships_test.go). Unit/static checks, isolated PostgreSQL, four live Phase 4 groups, seven live Phase 2 regressions, and OpenAPI validation passed.
- **Limits:** migration 012 is required, with no historical backfill. Database administrators can bypass append-only protections. Production throughput and security acceptance remain open. The next slice is terminal-data retention; audit history is excluded. No push or deployment was performed for this slice.

## 2026-10-30 — finish the local Phase 5 operations scope

- **What:** added opt-in terminal-data retention with holds and audit outcomes, owner-only operational aggregates, schema-aware readiness/preflight, a versioned native source package, and offline PostgreSQL/Temporal backup/restore into a new directory. Updated README, configuration examples, runbooks, API contracts, and CI.
- **Why:** local operations must preserve active dependencies and prevent duplicate effects after old execution history expires. Minimal run receipts remain while payloads/history expire. Backup keys stay separate; a restore never overwrites existing state.
- **Review fixes:** recheck unresolved children after acquiring retention locks; verify exact restored Temporal history and resume a pre-backup durable wait. The [two-axis review](docs/implementation/phase-5-review.md) records the findings and local fixes.
- **Security:** dependency commit `0055177` updates Ajv, fast-uri, Go 1.26.10, and x/net with its required dependencies. Final Go/npm scans report no vulnerabilities. The [license review](docs/implementation/phase-5-dependencies.md) records source-file evidence and unresolved redistribution requirements.
- **Implementation:** store/retention commit `2c4e70e`, API/CLI commit `a0225b9`, and packaging/restore commit `60ecf7c`. [Completion evidence](docs/implementation/phase-5-completion.md) includes unit/static checks, signed PostgreSQL security tests, four live Phase 4 groups, eight Phase 3 groups, and browser expiry checks at desktop/narrow widths.
- **Recovery evidence:** the final isolated backup took 0.407 seconds; restore through exact runtime-history comparison, durable-wait resumption, credential verification, and new execution took 17.861 seconds. These small fixture timings do not establish production RPO/RTO.
- **Boundary:** migration 013 is required. The dependency update changes node bundle identities; retain matching old artifacts for unfinished work. Production hosting/provider selection, operational ownership, independent acceptance, representative load, redistribution licensing, and pilot approval remain open. No production deployment occurred.

## 2026-10-30 — pre-pilot corrections and Phase 6 preparation

- **What:** corrected elapsed-time retention around daylight-saving changes, backup interruption cleanup and SQLite URI escaping, and expired-run inspection. Replaced up to 51 run-list queries with one and reused maintained payload byte counters.
- **Why:** remove confirmed correctness and operational defects before pilot use; reduce avoidable database work without changing API contracts.
- **Evidence:** commits `3b79e8e`, `b952264`, `3ae9979`, and `2851728`; [follow-up review and regression evidence](docs/implementation/phase-5-followup-review.md).
- **What:** added five portable training workflows, eight live execution cases, CI coverage, and participant/usage/incident/release templates. Updated the README and phase ledger.
- **Evidence:** commits `c2c5d81` and `75d76ab`; [Phase 6 implementation](docs/implementation/phase-6.md), [live fixture report](docs/implementation/evidence/phase-6-fixtures.json), [pilot kit](docs/pilot/README.md).
- **Limit:** local preparation is implemented. No real pilot participants, useful-workflow adoption, independent acceptance, or two-week usage have been recorded. These external gates remain open.
