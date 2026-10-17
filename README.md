# Runnelix

**Small steps. Reliable flows.**

Runnelix is a planned, self-hosted workflow platform for internal teams. It connects triggers, data transformations, decisions, and service actions in a visual workflow.

The design uses a **Go backend**, a **TypeScript interface and node workers**, and an internal node architecture that developers can extend. Adding an ordinary integration action should not require changes to the execution engine or canvas.

> **Project status: planning.** This repository contains the product and engineering plan. It does not contain a working workflow engine, application, or production deployment. Proposed reliability and capacity targets have not been demonstrated.

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

## The first engineering task

Prove a Go orchestration worker can execute a TypeScript action, survive a worker restart, wait durably, and resume on a compatible deployment. [Phase 0](docs/plans/08-delivery-plan.md#phase-0-prove-the-runtime-boundary) defines the exit checks. Temporal is the proposed runtime, subject to that result and the operating budget.

## About the name

**Runnelix** is pronounced **RUN-uh-liks**. The name starts with *runnel*, a small flowing channel. It suggests small steps joining into a dependable flow. See the [product brief](docs/plans/00-product-brief.md#name-and-positioning) for naming evidence and its limits.

## License status

The repository is public. A distribution license has not yet been selected. Public visibility does not, by itself, grant an open-source license.
