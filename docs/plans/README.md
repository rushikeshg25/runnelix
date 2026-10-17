# Runnelix planning guide

**Status:** proposed implementation plan. **Last updated:** 2026-10-17.

This directory defines what Runnelix should do, how its parts should interact, and what evidence is required before release. It is intended for a developer, reviewer, or operator joining the project without the original conversation.

For completed work and current limits, read the [implementation records](../implementation/README.md). Phase 1 now has local evidence for contracts, persistence, and recoverable admission; [ADR 0002](../decisions/0002-postgresql-admission-and-receipts.md) describes its boundary. The plan below remains the target product design. The runtime gate was refined by [ADR 0001](../decisions/0001-temporal-development-runtime.md): technical development evidence exists; production operations approval is still open.

The Markdown documents are the detailed source of truth. [product-plan.html](product-plan.html) is a self-contained visual overview. Update the relevant Markdown document first, then update the overview when scope or architecture changes.

## Start here

| Question | Document |
| --- | --- |
| What are we building, and for whom? | [Product brief](00-product-brief.md) |
| What owns each responsibility? | [Architecture](01-architecture.md) |
| What does executing a graph mean? | [Workflow semantics](02-workflow-semantics.md) |
| How do we add a node? | [Node development](03-node-development.md) |
| What do we store and expose? | [Data and API contracts](04-data-and-api.md) |
| How does a teammate use the product? | [Editor and run inspection](05-editor-experience.md) |
| How do we protect credentials and access? | [Security](06-security.md) |
| How do we deploy, recover, and control cost? | [Operations](07-operations.md) |
| What do we build first? | [Delivery plan](08-delivery-plan.md) |
| How do we prove it works? | [Verification and release gates](09-verification.md) |
| What is settled, uncertain, or risky? | [Decisions and risks](10-decisions-and-risks.md) |

## Reading paths

- **Product review:** product brief → editor experience → delivery plan → decisions and risks.
- **Backend implementation:** architecture → workflow semantics → data and API contracts → verification.
- **Node implementation:** node development → workflow semantics → security → verification.
- **Operational review:** architecture → security → operations → verification.

## What the status words mean

- **Confirmed:** an explicit user requirement, such as Go for the backend.
- **Proposed:** the recommended design. It can change when implementation evidence warrants a change.
- **Assumption:** a planning input without measured or confirmed support, such as initial traffic.
- **Gate:** a check that must pass before proceeding to the specified phase or release.
- **Deferred:** outside the first release. No implementation should be added merely because this plan mentions it.

All interfaces, schemas, paths, and examples in these documents are proposed contracts. They are not implemented APIs. Performance and recovery figures are acceptance targets, not measured results.

## Shared vocabulary

| Term | Meaning |
| --- | --- |
| Workflow | A named automation with a draft and published versions. |
| Draft | An editable graph. Saving it does not make it active. |
| Workflow version | An immutable published definition and compiled plan. |
| Node type | A reusable capability, such as an HTTP request or condition. |
| Node instance | A configured use of a node type in a workflow. |
| Invocation | One logical execution of one node instance for one item or scope. |
| Attempt | One try to complete an invocation. Retries create new attempts. |
| Run | An execution of one pinned workflow version. |
| Trigger event | An input that requests a run. Examples include a webhook delivery or schedule occurrence. |
| Execution plan | The validated representation interpreted by the runtime. |
| Logical-action key | A stable identifier reused across attempts of the same external action. |
| Payload reference | An identifier for stored data, instead of carrying the full data through execution history. |
| Projection | A query-friendly view derived from execution events. It does not control execution. |
| Outbox | A table of work to send to another system after the database transaction commits. |
| Durable wait | A persisted timer that survives restarts without keeping a worker occupied. |
| Uncertain outcome | An external action may have succeeded, but its result is not known reliably. |

## Documentation maintenance

Keep one canonical definition for each behavior. Link to that definition instead of copying it into every file. Preserve requirement IDs and verification IDs when wording changes. Record a changed decision in the decisions document with its reason and affected contracts.

Examples must use fake identifiers, mock endpoints, and credential references. Never commit credentials, private workflow payloads, local environment files, or actual customer data.

The proposed file layout is not a requirement to create every directory before it is useful. Follow the delivery sequence. Keep the first runnable slice small.

## Evidence boundary

The plan comes from confirmed product preferences and an initially empty project directory. It includes engineering analysis, not a completed implementation audit or vendor comparison. Official documentation links identify sources to verify during the runtime spike. Exact dependency versions, licensing, cross-language compatibility, hosting costs, and capacity remain to be checked.
