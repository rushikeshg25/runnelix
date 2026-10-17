# Product brief

[Planning guide](README.md)

## Purpose

Runnelix helps an internal team automate work across services. A teammate should be able to build a workflow, publish it, and understand a failed run without reading server logs.

Reliability and maintainability are part of the product. A workflow must retain its meaning after an application upgrade. A developer must be able to add a normal service action without rewriting the engine.

## Name and positioning

- **Name:** Runnelix.
- **Pronunciation:** RUN-uh-liks.
- **Tagline:** Small steps. Reliable flows.
- **Name idea:** a *runnel* is a small flowing channel. The coined name connects small, directed flows with executable work.
- **Repository:** `rushikeshg25/runnelix`.
- **Product description:** a self-hosted workflow platform with durable execution and an extensible internal node architecture.

On 2026-10-17, exact-name web searches returned no results for Runnelix. A GitHub repository-name search also returned zero matches. The target repository did not exist before creation. These checks establish a limited naming baseline; they do not establish trademark or domain availability.

## Confirmed requirements

| ID | Requirement | Practical implication |
| --- | --- | --- |
| C-01 | Serve an internal team first. | Prioritize useful workflows and operator visibility over billing and a marketplace. |
| C-02 | Support self-hosting. | Document installation, upgrades, backups, restore, and secret management. |
| C-03 | Use Go for the backend. | Go owns API behavior, graph validation, control flow, and authorization. |
| C-04 | Use TypeScript for the UI and node workers. | Define a tested cross-language task and data contract. |
| C-05 | Make new internal nodes easy to add. | Use a node definition, executor, schemas, fixtures, and one catalog registration. |
| C-06 | Aim for a production-ready first release. | Treat failure recovery, access control, and operational evidence as release gates. |

The product is inspired by n8n and Zapier. Feature parity, compatibility with their workflow formats, and reuse of their implementations are not requirements.

## Primary users

**Workflow author:** connects services, maps fields, chooses branches, and publishes a version. The author needs useful errors and safe testing controls.

**Operator:** monitors failures, pauses triggers, reconciles uncertain outcomes, and starts deliberate reruns. The operator needs run details and a clear audit trail.

**Node developer:** adds a service integration. The developer needs a small interface, fixtures, and a predictable way to declare retry safety.

**Administrator:** manages installation, workspace membership, credentials, backup, and upgrades. The administrator needs an installation-specific operating agreement.

## First-release requirements

| ID | Capability | Acceptance condition |
| --- | --- | --- |
| P-01 | Build and edit a workflow. | Create a graph with named ports, map values, undo an edit, and save a draft. |
| P-02 | Validate and publish. | Invalid graphs are rejected. Publication creates an immutable version. |
| P-03 | Start workflows. | Manual, authenticated webhook, and scheduled events produce durable run requests. |
| P-04 | Execute control flow. | Conditions, merge, bounded Map, errors, and durable wait have explicit tested semantics. |
| P-05 | Perform service actions. | HTTP and two selected integrations work through the same internal node contract. |
| P-06 | Inspect a run. | Show the pinned version, invocation states, attempts, sanitized data, and failures. |
| P-07 | Recover deliberately. | Safe retries recover automatically. Ambiguous external writes become visible. |
| P-08 | Protect access. | Login, workspace roles, credential grants, egress rules, and audit records are enforced. |
| P-09 | Operate the deployment. | Document and test installation, upgrade, restore, retention, and alerting. |
| P-10 | Export and import definitions. | Versioned JSON excludes secret values and validates references before publication. |
| P-11 | Add an internal action node. | A second integration needs no changes to engine or ordinary form-rendering code. |

## Worked example

An internal service emits a signed event for a new request. Runnelix extracts the request ID, priority, and description. A condition routes high-priority requests to an approved HTTP endpoint. The run inspector shows the selected branch and the endpoint response.

The first demonstration uses a mock endpoint with controllable delays and failures. A real integration follows after the execution contract passes the failure tests.

The two service integrations should come from actual pilot needs. GitHub issue creation and Slack messaging are examples, not a fixed commitment. Choose integrations that expose different authentication and retry behavior.

## Explicit exclusions

The first release does not include public plugin installation, uploaded executable code, arbitrary JavaScript/Python nodes, SaaS billing, or a connector marketplace. Developers ship internal node modules with the application.

Also defer collaborative editing, AI-generated workflows, large binary payloads, streaming ETL, user-defined reusable subworkflows, human approval nodes, and unrestricted graph cycles. Internal runtime child executions may still be used to bound history; that does not imply a public subworkflow feature.

## Success measures

The proposed product outcome is **three teammates using five real workflows each week for two consecutive weeks by 2027-01-31**. This assumes work starts by early November 2026 with sufficient staffing. Revise the date when staffing is known.

Additional success signals:

- A teammate can diagnose an intentional workflow error without developer assistance.
- A developer who did not write the engine can add a simple API-key action in one working day, excluding provider access delays.
- Accepted events survive ordinary process restarts within the defined failure model.
- Uncertain external outcomes are visible and are not silently repeated.
- The installation meets the agreed recovery targets in a timed drill.

## Scope-change rule

Add a feature only when it serves a documented workflow or fixes a failed acceptance criterion. Record the requirement, cost, and effect on delivery. Do not treat a competitor's feature list as an automatic backlog.

Next: [Architecture](01-architecture.md).
