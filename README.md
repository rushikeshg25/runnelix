# Runnelix

**Small steps. Reliable flows.**

Runnelix is a self-hosted workflow platform under development for internal teams. The planned product connects triggers, data transformations, decisions, and service actions in a visual workflow.

The design uses a **Go backend**, a **TypeScript interface and node workers**, and an internal node architecture that developers can extend. Adding an ordinary integration action should not require changes to the execution engine or canvas.

> **Project status: Phase 5 local security and operations implemented.** Local workflows support conditions, merge, bounded Map, durable waits, schedules, operator reconciliation, GitHub/Slack actions, credential rotation, and portable imports/exports. OIDC identity, workspace roles, and browser login with PKCE, HttpOnly sessions, and CSRF protection are implemented and locally verified. Transactional, append-only audit records are available for privileged workspace changes. Explicit terminal-data retention, owner metrics, schema readiness, local packaging, and isolated backup/restore checks are implemented. Production provider acceptance, representative load, independent teammate acceptance, and rollout remain open.

## Run the development checks

Prerequisites: Go 1.26.10 or automatic toolchain downloads enabled, Node 24, pnpm 11.5.0, Python 3, curl, and make. Supported CLI download targets: macOS/Linux on ARM64 or AMD64. PostgreSQL server/client binaries are required for the database and live integration checks. Docker is not required.

```sh
make tools
make deps
make test check
make integration
make integration-phase1
make integration-phase2
make integration-phase3
make integration-phase4
make integration-phase5
make integration-operations
make integration-phase6
```

`make tools` downloads a checksum-pinned Temporal CLI into this checkout. `make integration` builds the programs, starts isolated loopback services, runs the twelve cases, and stops its own processes. It preserves fake provider/runtime state and reports under `artifacts/phase0/` for inspection.

`make integration-phase5` runs signed OIDC and workspace/session security tests against an isolated PostgreSQL instance. It covers permission checks, login replay, session expiry, CSRF, logout closing authenticated event streams, and audit rollback/isolation. Set `RUNNELIX_PG_BIN` if the PostgreSQL binaries are not on PATH.

Start the local editor with `make dev`, then open <http://127.0.0.1:5173>. The launcher owns a separate PostgreSQL cluster and Temporal development server. It preserves local data and keys in `.local/dev` on shutdown. Use fixture data. It does not touch an existing system database.

Read the [Phase 4 integrations and portability guide](docs/implementation/phase-4.md), [Phase 3 controls and recovery guide](docs/implementation/phase-3.md), [Phase 2 guide](docs/implementation/phase-2.md), [Phase 1 setup and API examples](docs/implementation/phase-1.md), [Phase 0 runtime proof](docs/implementation/phase-0.md), [implementation status](docs/implementation/README.md), [history](HISTORY.md), and [runtime decision](docs/decisions/0001-temporal-development-runtime.md).

## Enable local browser login

The default development mode is unauthenticated and restricted to loopback. To use OIDC login, apply migrations through **013**, register a public OIDC client with authorization-code flow and S256 PKCE, and explicitly provision the first workspace owner. Signing in never grants workspace membership automatically.

Follow the [browser login setup](docs/implementation/phase-5-sessions.md#configure-the-local-installation) and [owner provisioning instructions](docs/implementation/phase-5-memberships.md#provision-the-first-owner). These guides specify the issuer, client ID, browser origin, and exact callback URL. The editor currently uses `workspace_dev`.

Browser sessions expire within one hour or at ID-token expiry, whichever comes first. Logout revokes the current local session. Session expiry preserves the current unsaved draft while disabling editing. Provider tokens are not stored in browser storage.

To try the flow without a real provider, use the [isolated fake-provider browser environment](docs/implementation/phase-5-sessions.md#verification). It creates its own database and test owner, and stops its services on Ctrl-C. This local verification does not approve public deployment.

## Inspect privileged changes

Workspace owners can read `GET /v1/workspaces/{workspace}/audit-records?after=0` in OIDC mode. Records cover publications, credential and membership changes, run admissions/cancellation/reconciliation, schedules, and draft edits. Audit writes commit with the protected change; failures roll back both.

Apply migration **012** before using this slice. See the [audit guide](docs/implementation/phase-5-audit.md) for pagination, actor attribution, excluded sensitive data, append-only protections, and administrative limits. Development mode remains unauthenticated on loopback.

## Read the plan

Start with the [planning guide](docs/plans/README.md). It links the requirements, architecture, execution rules, node contracts, data model, security design, delivery stages, and release checks.

For a visual introduction, download [the standalone HTML overview](docs/plans/product-plan.html) and open it in a browser. It includes a recovery walkthrough and needs no dependencies or network connection. GitHub's file view displays HTML source; downloading the file does not require deploying a website.

## Intended first release

- Manual, authenticated webhook, and scheduled triggers.
- HTTP actions, field mapping, conditions, merges, bounded iteration, and durable waits.
- GitHub issue and Slack message integrations, initially verified with provider fixtures.
- Immutable published versions, execution history, cancellation, and explicit retry policies.
- OIDC login, workspace permissions, protected credentials, audit records, and tested recovery.

“Extensible nodes” means modules developed and shipped with Runnelix. A plugin marketplace and user-uploaded executable code are outside the first release.

## Next engineering work

The local Phase 5 scope is complete. See the [retention commands](docs/implementation/phase-5-retention.md), [operations and restore guide](docs/implementation/phase-5-operations.md), [dependency review](docs/implementation/phase-5-dependencies.md), and [release checklist](docs/implementation/phase-5-completion.md).

`make package-local` creates a versioned source archive from a clean tracked tree. Backups keep keys separate, and restores always create a new state directory. Migration **013** is required. Dependency security updates change the node bundle digest; retain matching artifacts for unfinished older runs and republish new revisions before new admissions.

Phase 6 local preparation includes five [portable training workflows](examples/pilot/README.md), an automated import/publish/run check (`make integration-phase6`), and a [pilot kit](docs/pilot/README.md) with installation, participant, usage, incident, and release records. See the [Phase 6 evidence](docs/implementation/phase-6.md).

A repeatable [local capacity diagnostic](docs/implementation/local-capacity.md) is available with `make integration-capacity`. It measures a 20-starts/second offered burst and worker-outage recovery without changing admission limits.

The real pilot has not started. Production provider/hosting selection, accountable ownership, independent acceptance, representative load, two weeks of teammate usage, external redistribution licensing, and rollout approval remain open. Local fixture results do not satisfy those gates.

## About the name

**Runnelix** is pronounced **RUN-uh-liks**. The name starts with *runnel*, a small flowing channel. It suggests small steps joining into a dependable flow. See the [product brief](docs/plans/00-product-brief.md#name-and-positioning) for naming evidence and its limits.

## License status

The repository is public. A distribution license has not yet been selected. Public visibility does not, by itself, grant an open-source license.
