# Decisions, risks, and open questions

[Planning guide](README.md)

## Decision register

| ID | Status | Decision and reason | Alternative or consequence |
| --- | --- | --- | --- |
| D-01 | Confirmed | Internal self-hosted product first. | Hosted SaaS billing and customer isolation are deferred. |
| D-02 | Confirmed | Go backend; TypeScript interface and node workers. | Requires explicit cross-language contracts. |
| D-03 | Confirmed | Extensibility means internal node modules shipped with the application. | No public plugin loader or uploaded executable code. |
| D-04 | Selected for this repository | Product name Runnelix; “Small steps. Reliable flows.” | Limited exact-name checks found no matches; domain/trademark clearance is not established. |
| D-05 | Accepted for continued development; production gate open | Temporal supplies durable runtime mechanisms. [ADR 0001](../decisions/0001-temporal-development-runtime.md) records Phase 0 evidence and limits. | PostgreSQL runner is the fallback only after an explicit scope/runtime decision. |
| D-06 | Proposed | Go owns graph/control semantics; TypeScript owns provider actions. | Adding an action stays local; adding a control construct requires engine design. |
| D-07 | Proposed | Immutable publication and pinned execution versions. | Keep compatible code available for active and still-runnable published versions. |
| D-08 | Proposed | Transactional admission plus an outbox. | Two persistence systems require start receipts and reconciliation. |
| D-09 | Proposed | Provider-aware at-least-once execution with explicit uncertainty. | Do not promise exactly-once external effects. |
| D-10 | Proposed | PostgreSQL stores bounded JSON payloads initially. | Add object storage for binary support or measured storage/recovery pressure. |
| D-11 | Proposed | DAG plus explicit bounded Map. | Arbitrary graph cycles and user-defined subworkflows are deferred. |
| D-12 | Proposed | Markdown documents are canonical; HTML is the visual overview. | Review and update both when major scope or architecture changes. |

## Risk register

| Risk | Consequence | Mitigation and decision trigger |
| --- | --- | --- |
| Temporal operating burden exceeds the team's capacity. | Unreliable self-hosting or delayed delivery. | Phase 0 restore/upgrade spike; compare priced operations with the agreed budget. |
| Go/TypeScript payload or error behavior differs. | Wrong retry handling or failed executions. | Shared fixtures and real cross-language activities before UI investment. |
| Unsafe retry repeats a remote write. | Duplicate tickets/messages or other business effects. | Mandatory per-action retry policy, idempotency/reconciliation tests, needs-attention state. |
| A long-lived run loses compatible worker code. | Run cannot resume or changes meaning. | Pinned artifacts, compatible routing tests, retention reports, explicit retirement policy. |
| A late attempt overwrites a result. | Inconsistent downstream data. | Immutable attempt records and conditional canonical-result selection. |
| Schedule overlap is enforced on the wrong execution. | Concurrent main runs despite “no overlap.” | Admission-level active-run slot; test wrapper completion before main-run completion. |
| Sensitive data leaks into logs/history. | Credential or business-data exposure. | Minimal history payloads, scoped credential access, allowlisted logs, sentinel tests. |
| Map expansion or retained payloads grow without bounds. | Memory/disk exhaustion and missed recovery targets. | Aggregate per-run limits, active-data pins, history chunking, retention monitoring. |
| Canvas scope consumes reliability time. | Attractive UI with incomplete execution guarantees. | Deliver one full workflow first; hold release to durability gates. |
| Database restore repeats already-completed remote actions. | External state diverges after recovery. | Hold unsafe dispatch, reconcile restored receipts/runtime state with providers. |
| Generic SDK becomes too broad. | New nodes remain hard to understand and test. | Require the second real integration to prove the boundary; add only demonstrated capabilities. |

## Open questions and deadlines

| Question | Needed by | Default until resolved |
| --- | --- | --- |
| Which five workflows justify the pilot? | Phase 0. | Use the mock webhook-to-HTTP example for technical proof only. |
| Which two service integrations come first? | Before Phase 4. | GitHub and Slack are examples, not commitments. |
| What host/platform, OIDC provider, and operating owner are available? | Phase 0 and deployment design. | Do not infer an existing production environment. |
| What is the monthly cost ceiling? | Before paid provisioning. | Price expected and 10x configurations; no spend is authorized by this plan. |
| Is single-host restore-based recovery acceptable? | Before pilot deployment. | Describe its failure domain; do not claim high availability. |
| Are proposed retention and payload limits sufficient? | First real-workflow review. | 7-day completed payloads, 30-day summaries, active dependency pins. |
| What external side effects require special operator review? | Each node's design review. | Unsafe ambiguous writes do not retry automatically. |
| What license should the repository use? | Before distributing implementation for reuse. | Public visibility alone grants no chosen open-source license. |
| What staffing supports the delivery date? | After Phase 0. | 14–22 engineer-weeks is a rough estimate, not a commitment. |

## How to change a decision

Record the new evidence, affected requirement IDs, affected contracts, and operational migration. Link to the canonical document that changes. Do not rewrite an old assumption as though it was always confirmed.

If a change affects published workflow meaning, explain versioning and migration before implementation. If it affects cost or service operation, update the deployment budget and release gate.

## Implementation references

Use primary documentation during the spike and record the exact supported versions:

- [Temporal documentation](https://docs.temporal.io/): workflow determinism, activities, retries, schedules, worker deployment/versioning, persistence, and self-hosting.
- [Temporal Go SDK](https://github.com/temporalio/sdk-go) and [TypeScript SDK](https://github.com/temporalio/sdk-typescript): actual runtime and serialization contracts.
- [React Flow documentation](https://reactflow.dev/): canvas primitives and accessibility support.
- [JSON Schema](https://json-schema.org/): schema dialect and validation behavior.
- [OpenAPI specification](https://spec.openapis.org/oas/latest.html): HTTP contract design and supported generator dialect.
- [CEL for Go](https://github.com/google/cel-go): bounded expression evaluation and supported value types.
- [PostgreSQL documentation](https://www.postgresql.org/docs/current/): transactions, constraints, indexing, backup, and recovery.
- [OWASP SSRF prevention guidance](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html): destination validation and network controls.

These links identify verification sources. They do not claim that dependency compatibility, current hosting prices, or production behavior were validated during planning.
