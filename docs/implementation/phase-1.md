# Phase 1: contracts and persistence

Status: development deliverables complete in nine meaningful commits. Baseline: `a2bfeba` (Phase 0). Phase 1 exit checks have local evidence; production gates remain open.

## Outcome and boundary

A developer can validate a workflow, save a revision, publish an immutable plan, and durably admit one run per trigger key. PostgreSQL transactions own admission. Temporal receives a stable run ID through a recoverable outbox.

Phase 1 supports one manual trigger and pure mock actions. It compiles a bounded DAG but does not execute the graph. Its runtime admission workflow records a durable start receipt; execution and run projections arrive in Phase 2. No user can mistake admission for action completion.

The API is a loopback-only development interface. Workspace routing enforces data boundaries, but it does not authenticate a user. OIDC, roles, real credentials, public webhooks, production deployment, and full graph interpretation remain later work.

## Nine slices

1. Workflow/plan schemas, catalog interfaces, graph validation, deterministic compilation, and fixtures.
2. Internal TypeScript node host, mock module, activity envelope, and shared contract tests.
3. PostgreSQL migrations and isolated database verification harness.
4. Workspace-scoped drafts, optimistic revisions, and immutable publication.
5. Transactional inbox/outbox admission and stable run identity.
6. Leased dispatch, fenced acknowledgement, and durable runtime receipts.
7. Development HTTP API, OpenAPI baseline, and process entry points.
8. Real PostgreSQL/Temporal failure-window integration evidence.
9. CI, operational documentation, final checks, and phase ledger.

Every slice updates HISTORY.md and the dated decision log. Tests must establish the relevant behavior before completion; no empty commits to reach the count.

## Infrastructure check

Intent: product development. Build the simple version first. PostgreSQL supplies transactions and constraints; Temporal remains the selected runtime. Use a PostgreSQL outbox, not another message broker. Use one Go module and one TypeScript package, not independently managed services.

Measurable outcome for this phase: concurrent duplicate admission and dispatch recovery produce one product run and one durable runtime receipt, with pinned published content. Verify locally during this phase; no production traffic target is claimed.

Local cost: existing machine, no purchased service. Future monthly cost remains compute + PostgreSQL/runtime storage + backups + operations time; traffic, 10x load, and provider prices are unknown. No broker/cache is justified until measured database contention prevents the agreed service target. Stop promotion if admission atomicity, scope checks, or dispatch recovery fail. Preserve test state and evidence instead of deleting failed databases.

## Implemented flow

```text
Workflow JSON → schema + graph validation → pinned compiled plan
       ↓
Draft revision → immutable publication → active version pointer
       ↓
Manual admission transaction → inbox + accepted run + outbox
       ↓
Dispatcher lease → stable Temporal admission workflow ID
       ↓
Receipt activity → PostgreSQL runtime receipt → fenced outbox acknowledgement
```

The final receipt confirms the runtime admission handshake. It does not prove graph or node execution. The separate mock-node fixture proves the internal action-host contract.

## Where the code lives

| Area | Canonical implementation |
| --- | --- |
| Workflow, plan, activity, HTTP contracts | [contracts/product](../../contracts/product) |
| Graph rules and deterministic compilation | [internal/definition](../../internal/definition) |
| Transaction boundaries and database queries | [internal/store](../../internal/store) |
| Forward migration | [001_initial.sql](../../internal/store/migrations/001_initial.sql) |
| Leased runtime handoff and receipt workflow | [internal/dispatch](../../internal/dispatch) |
| Development HTTP handlers | [internal/api](../../internal/api) |
| Process commands | [cmd/runnelix](../../cmd/runnelix) |
| Generic TypeScript node host | [product-host.ts](../../node-worker/product-host.ts) |
| Internal module registration | [nodes/index.ts](../../node-worker/nodes/index.ts) |
| Live failure tests | [phase1.py](../../tests/integration/phase1.py) |

Read [ADR 0002](../decisions/0002-postgresql-admission-and-receipts.md) for the alternatives, state model, and recovery boundaries.

## Exact contract boundaries

- Workflow format 1: at most 100 nodes, 200 edges, 256 KiB JSON, and 32 nesting levels. Node IDs are stable ASCII identifiers.
- One manual trigger; current action catalog contains `mock.echo@1`. Control nodes, expressions, credentials, and real providers are absent.
- Reject duplicate JSON keys, unknown structural fields, missing fields, unsafe numeric magnitudes, unknown types/versions/ports, duplicate node IDs, disconnected required inputs, multiple edges to one input, cycles, and unreachable nodes.
- Drafts can be semantically incomplete but must match the structural format. Publication performs complete supported graph validation.
- Compiler output has deterministic topological ordering and exact implementation references. Editor node ordering does not change its digest.
- `If-Match: "1"` supplies the expected draft revision. Stale saves or publication return `409`. Missing or malformed preconditions return `400`.
- Repeating publication of the current revision returns its existing version. A later draft revision creates a distinct version, even when its plan digest is unchanged.
- A run pins the published version inside the admission transaction. Dispatch never reads the current active pointer to choose another version.
- Manual idempotency keys are ASCII identifiers, 1–128 characters. Scope is workspace + workflow + source + key. Equivalent JSON object order replays the original run; changed input returns `409`.
- Admission input is one JSON object, at most 8 KiB after canonical encoding. This development ceiling is deliberately smaller than the planned production payload limit.
- Activity envelopes contain workspace/run/node/invocation identity, exact behavior version and source-bundle digest, scoped references, and a logical action key. Resolved mock inputs/config/output are each limited to 8 KiB.
- Catalog generation hashes the trusted node sources, host, contract, package manifest, and dependency lockfile. This detects source drift; it is not container signing or proof that a worker is deployed.
- Admission rechecks the available catalog against the published plan. Old source bundles require an explicit compatibility and artifact-retention policy before real deployment.
- PostgreSQL owns admission; Temporal owns its admission handshake. The product state stays `accepted`. `dispatchState` separately exposes `pending`, `receipt_recorded`, and `acknowledged`.

## Reproduce the checks

Prerequisites: the Phase 0 Go/Node/pnpm/Python toolchain, Temporal CLI installed by `make tools`, and PostgreSQL server/client binaries. Local evidence used PostgreSQL **17.6**. CI is configured for PostgreSQL **16** on Ubuntu 24.04; hosted CI has not run yet.

```sh
make deps tools
make test check
make integration-phase1
make integration
```

`make test` runs Go race tests and the 33 TypeScript tests. PostgreSQL tests explicitly skip when `RUNNELIX_TEST_DATABASE_URL` is absent. `make integration-phase1` supplies an isolated test database and runs those tests, then runs the actual HTTP/runtime suite. Do not count the skipped checks as database evidence.

If PostgreSQL binaries are not on PATH, set `RUNNELIX_PG_BIN` to their directory. For this observed macOS installation:

```sh
RUNNELIX_PG_BIN=/opt/homebrew/opt/postgresql@17/bin make integration-phase1
```

The harness creates a unique cluster under `artifacts/phase1/`, chooses a loopback port, disables Unix sockets, and uses trust authentication only for that fake-data fixture. It never connects to an existing database or deletes its files. It stops its own services and retains logs, databases, and reports. Do not run `initdb` as root.

The live report captures source hashes before startup and checks that they remain unchanged at shutdown. It records cleanup failures and fails the command when cleanup or source consistency fails.

## Run the development API manually

Use fake data and an unused directory/port. `initdb` refuses an existing data directory; do not remove one to make these commands succeed. The following example creates a separate local cluster:

```sh
mkdir -p .local
initdb -D .local/phase1-postgres -U runnelix_dev --auth=trust --encoding=UTF8 --no-locale
pg_ctl -D .local/phase1-postgres -l .local/phase1-postgres.log \
  -o "-h 127.0.0.1 -p 55432 -k ''" -w start
export RUNNELIX_DATABASE_URL='postgresql://runnelix_dev@127.0.0.1:55432/postgres?sslmode=disable'
make build-phase1
.local/bin/runnelix migrate
.local/bin/runnelix bootstrap --workspace workspace_dev
.local/bin/runnelix api --address 127.0.0.1:8787
```

Run each long-lived process in a separate terminal with the same database environment. Start the owned Temporal development server, then the worker and dispatcher:

```sh
.tools/temporal/1.9.1/temporal server start-dev --headless --ip 127.0.0.1 \
  --port 7233 --db-filename .local/phase1-temporal.sqlite
.local/bin/runnelix worker --temporal-address 127.0.0.1:7233
.local/bin/runnelix dispatcher --temporal-address 127.0.0.1:7233
```

Use different ports if those are occupied. Do not stop an existing service to free them. Stop these foreground processes with Ctrl-C, then stop only the example cluster:

```sh
pg_ctl -D .local/phase1-postgres -m fast -w stop
```

When restarting this cluster, repeat the explicit `-o` loopback/port/socket options. The first test failure showed why omitting them can make PostgreSQL try the default port instead.

## HTTP example

The [OpenAPI baseline](../../contracts/product/openapi.json) is the HTTP reference. All routes below use explicit workspace scope. The API has no user authentication and must stay local.

Create a draft:

```sh
curl --fail-with-body http://127.0.0.1:8787/v1/workspaces/workspace_dev/workflows \
  -H 'Content-Type: application/json' \
  --data '{"name":"Echo example","graph":{"formatVersion":1,"nodes":[{"id":"start","type":"trigger.manual","version":1,"config":{}},{"id":"echo","type":"mock.echo","version":1,"config":{}}],"edges":[{"from":{"node":"start","port":"out"},"to":{"node":"echo","port":"in"}}]}}'
```

Copy its returned `id` into `WORKFLOW_ID`. Publish revision 1, then durably admit a run:

```sh
WORKFLOW_ID='workflow_REPLACE_WITH_RETURNED_ID'
curl --fail-with-body -X POST \
  "http://127.0.0.1:8787/v1/workspaces/workspace_dev/workflows/$WORKFLOW_ID/publish" \
  -H 'If-Match: "1"'
curl --fail-with-body \
  "http://127.0.0.1:8787/v1/workspaces/workspace_dev/workflows/$WORKFLOW_ID/runs" \
  -H 'Content-Type: application/json' -H 'Idempotency-Key: example_1' \
  --data '{"input":{"message":"fake data"}}'
```

The second request returns `202` and a stable `runId`. Repeat it with the same key/input to obtain the same run. Use `GET /v1/workspaces/workspace_dev/runs/{runId}` to inspect admission and dispatch. The graph has not executed, even after `dispatchState` becomes `acknowledged`.

## Add an internal mock action

1. Add a trusted executor under `node-worker/nodes/`.
2. Register it in `node-worker/nodes/index.ts`.
3. Add the serializable descriptor and schemas to `contracts/product/catalog.source.json`.
4. Add fixture tests for valid data, rejected config, bounds, cancellation, and typed errors.
5. Run `make catalog`, then `make test check integration-phase1`.

Do not edit generated `catalog.json` manually. Ordinary action registration does not require edits to the compiler or generic host. New control semantics still require explicit compiler/interpreter work. The independent-developer extensibility gate remains unverified until real integrations are added.

## Recovery and migration notes

- Apply forward migrations explicitly. They use one transaction, an advisory lock, and stored SHA-256 checksums. Concurrent attempts serialize. Changed files or unknown database migration versions stop application.
- There is no destructive down migration. Never recreate a database to bypass a failure. Inspect `schema_migrations` and compare the application revision before rollout or rollback.
- Outbox leases expire after 10 seconds in the application. Start RPCs have a 5-second deadline. Retries release the lease and use a delay capped at 60 seconds.
- Receipt insertion is idempotent and verifies workspace, run, version, plan digest, and runtime identity. Stale lease tokens cannot acknowledge or reschedule a newer claim.
- A permanently failed admission workflow needs operator investigation. This phase does not automatically reset or replace it. Do not manually create a new product run to hide an unknown outcome.
- No retention job removes keys, payloads, receipts, or old artifacts yet. Retention must be designed with replay/recovery guarantees before production use.

## Evidence and limits

The [full Phase 1 report](evidence/phase-1-full.json) records seven passing live cases:

| Case | What it demonstrates |
| --- | --- |
| HTTP contracts/scope | Useful invalid-graph errors, stale revision conflict, workspace isolation, Origin/Host rejection. |
| Concurrent admission/pinning | Twelve duplicate starts return one run; changed input conflicts; old admissions keep their version. |
| API/database restart | Accepted work and deduplication survive process death and PostgreSQL restart before dispatch. |
| Concurrent dispatchers | Two dispatchers acknowledge one durable receipt per intended run. |
| Death before runtime start | An expired claim can be reclaimed after dispatcher SIGKILL. |
| Death after runtime start | A lost dispatcher can recover the same stable runtime identity. |
| Death after receipt | Reconciliation finishes with Temporal offline and no new start request. |

PostgreSQL tests separately cover atomic rollback after a forced final-insert failure, concurrent draft saves, SQL immutability, cross-workspace foreign keys, migration drift, receipt identity, and stale lease fencing. Unit tests and shared fixtures cover graph rules and Go/TypeScript activity contracts.

The first full run failed because the PostgreSQL restart fixture omitted startup options and tried occupied default port 5432. The fixture now shares startup code and tracks stop/start state. The successful restart case is the regression evidence. Existing system databases were untouched.

This phase does not establish production throughput, HA, PostgreSQL PITR, authenticated access, payload encryption/retention, real provider retry safety, full graph execution, or worker deployment availability. The API and SQL queries are handwritten; frontend client generation and sqlc remain unimplemented proposals, not hidden prerequisites.

## Commit ledger

| Slice | Commit | Result |
| --- | --- | --- |
| 1 | `a8fb831` | Workflow contracts and deterministic compiler. |
| 2 | `4d8ead7` | Internal node host, catalog, and shared activity fixtures. |
| 3 | `550db11` | PostgreSQL schema, migration runner, and isolated harness. |
| 4 | `dc2e10d` | Scoped drafts and immutable publication. |
| 5 | `2a96979` | Atomic inbox/run/outbox admission. |
| 6 | `6f7ea4c` | Leased dispatch and durable runtime receipts. |
| 7 | `5c2c148` | Development API, OpenAPI, and commands. |
| 8 | `d228d6a` | Live recovery evidence and contract-review fixes. |
| 9 | Commit containing the completed phase record | CI, reproducible operating guide, final checks, and phase completion. |

The ninth commit identifies itself through Git history to avoid a self-referential hash. Each slice updates [history](../../HISTORY.md) and the dated [decision log](../../logs/2026-10-17.md).

## Final verification record

- `make test check integration` passed. The 12-case [Phase 0 regression report](evidence/phase-0-after-phase-1.json) preserves the original runtime guarantees after Phase 1 changes.
- `make integration-phase1` passed from the documented target. [PostgreSQL command evidence](evidence/phase-1-postgres.json) records a successful race-enabled store test run on PostgreSQL 17.6. The live suite again passed all seven cases.
- All 33 TypeScript tests passed. Go vet, type checks, formatting, catalog drift detection, and whitespace checks passed.
- OpenAPI 3.1 validation passed with `openapi-spec-validator==0.7.2`. CI includes the same validator. To reproduce without changing the host Python environment:

```sh
python3 -m venv .tools/openapi
.tools/openapi/bin/pip install openapi-spec-validator==0.7.2
.tools/openapi/bin/python -m openapi_spec_validator contracts/product/openapi.json
```

The implementation commits remain local. GitHub Actions has been configured and its YAML parsed locally; hosted CI and its PostgreSQL 16 lane have not run. The production release gate remains open.
