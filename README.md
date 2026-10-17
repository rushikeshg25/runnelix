# Runnelix

**Small steps. Reliable flows.**

Runnelix is a self-hosted workflow platform under development for internal teams. The planned product connects triggers, data transformations, decisions, and service actions in a visual workflow.

The design uses a **Go backend**, a **TypeScript interface and node workers**, and an internal node architecture that developers can extend. Adding an ordinary integration action should not require changes to the execution engine or canvas.

> **Project status: Phase 1 development slice.** Workflow JSON can be validated, saved with revisions, published immutably, and admitted durably through PostgreSQL and Temporal. Seven live admission/recovery cases pass. The runtime records admission only; the product graph engine, UI, authentication, and production deployment remain unfinished.

## Run the development checks

Prerequisites: Go with automatic toolchain downloads enabled, Node 24, pnpm 11.5.0, Python 3, curl, and make. Supported CLI download targets: macOS/Linux on ARM64 or AMD64. PostgreSQL server/client binaries are also required for Phase 1. Docker is not required.

```sh
make tools
make deps
make test check
make integration
make integration-phase1
```

`make tools` downloads a checksum-pinned Temporal CLI into this checkout. `make integration` builds the programs, starts isolated loopback services, runs the twelve cases, and stops its own processes. It preserves fake provider/runtime state and reports under `artifacts/phase0/` for inspection.

Read the [Phase 1 setup and API examples](docs/implementation/phase-1.md), [Phase 0 runtime proof](docs/implementation/phase-0.md), [implementation status](docs/implementation/README.md), [history](HISTORY.md), and [runtime decision](docs/decisions/0001-temporal-development-runtime.md).

## Read the plan

Start with the [planning guide](docs/plans/README.md). It links the requirements, architecture, execution rules, node contracts, data model, security design, delivery stages, and release checks.

For a visual introduction, download [the standalone HTML overview](docs/plans/product-plan.html) and open it in a browser. It includes a recovery walkthrough and needs no dependencies or network connection. GitHub's file view displays HTML source; downloading the file does not require deploying a website.

## Intended first release

- Manual, authenticated webhook, and scheduled triggers.
- HTTP actions, field mapping, conditions, merges, bounded iteration, and durable waits.
- Two service integrations selected from real team workflows.
- Immutable published versions, execution history, cancellation, and explicit retry policies.
- OIDC login, workspace permissions, protected credentials, audit records, and tested recovery.

“Extensible nodes” means modules developed and shipped with Runnelix. A plugin marketplace and user-uploaded executable code are outside the first release.

## Next engineering phase

[Phase 2](docs/plans/08-delivery-plan.md#phase-2-first-complete-workflow) adds the first actual execution path, minimal editor, HTTP action, condition, and run inspector. [ADR 0002](docs/decisions/0002-postgresql-admission-and-receipts.md) explains why Phase 1 admission is separate from execution. Production operations approval remains open.

## About the name

**Runnelix** is pronounced **RUN-uh-liks**. The name starts with *runnel*, a small flowing channel. It suggests small steps joining into a dependable flow. See the [product brief](docs/plans/00-product-brief.md#name-and-positioning) for naming evidence and its limits.

## License status

The repository is public. A distribution license has not yet been selected. Public visibility does not, by itself, grant an open-source license.
