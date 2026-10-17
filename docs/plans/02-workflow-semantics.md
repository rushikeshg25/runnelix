# Workflow semantics

[Planning guide](README.md)

This document defines execution behavior. The editor must explain these rules, and the engine must enforce them. All rules are proposed until their corresponding verification gates pass.

## Publication and run identity

A draft is mutable. A published version is immutable. Publication validates the graph, resolves node types, and writes a compiled execution plan.

Each run pins:

- Workflow ID and published version ID.
- Compiled-plan format and interpreter compatibility version.
- Node type versions and compatible implementation artifacts.
- Trigger event or manual request receipt.
- Evaluated run policy, including limits, retries, and concurrency.

Pin the published version during admission, not when a delayed dispatcher finally starts the run. Changing the active version affects later admissions. Existing runs keep their original version.

Credential references are pinned as logical references. Secret values are resolved under the current access/rotation policy at attempt time. See [Security](06-security.md#credential-lifecycle).

## Run and invocation states

| State | Meaning | Allowed next states |
| --- | --- | --- |
| `accepted` | The database committed a run request. | `running`, `cancel_requested`, `failed`. |
| `running` | The runtime owns execution. | `waiting`, `needs_attention`, `cancel_requested`, `succeeded`, `failed`. |
| `waiting` | Execution is paused for a durable timer or capacity. | `running`, `cancel_requested`, `failed`. |
| `needs_attention` | An outcome requires operator reconciliation. | `running` after an explicit supported decision, `failed`, `cancel_requested`. |
| `cancel_requested` | Stop has been requested, but in-flight effects may still complete. | `cancelled`, `failed`, or an already-committed terminal result. |
| `succeeded` | All required work reached its success policy. | Terminal. |
| `failed` | A required invocation or run limit failed. | Terminal. |
| `cancelled` | Scheduling stopped and cancellation bookkeeping finished. | Terminal. |

The authoritative state transition belongs to the runtime, except for the initial accepted request. Run projections may lag. Never let a delayed projection event move a run backward.

An invocation can be pending, ready, executing, retry scheduled, waiting, succeeded, failed, skipped, cancelled, or uncertain. An attempt records one execution try. Retrying an invocation creates a new attempt, not a new logical invocation.

## Graph and port rules

The first release accepts a directed acyclic graph (DAG). A DAG has directed edges and no graph cycles. Explicit bounded Map handles iteration.

Publication rejects unknown node types, unknown ports, dangling edges, duplicate stable IDs, missing required inputs, unreachable executable nodes, and cycles. Schema validation checks known types. Dynamic expressions still need runtime validation because their values may not be known at publication.

Each ordinary action invocation receives one JSON value. A JSON array remains one value until a Map node explicitly expands it. Do not implicitly execute an action once per array element.

Inputs and outputs use named ports. The compiler defines allowed edge multiplicity. A port accepts one edge unless its node contract explicitly defines aggregation. Never infer a join from whichever branch finishes first.

## Conditions, skipping, and merge

A condition selects named output ports. Selected edges carry data. Unselected edges carry a skip marker.

A merge waits for an outcome on every expected incoming edge. It receives data from selected branches and skip markers from unselected branches. If every input is skipped, the merge is skipped. Otherwise, it combines active inputs using its declared policy.

The first merge policies should be explicit named-input assembly and ordered append. Reject conflicting object-key merges unless the node defines the conflict rule. Completion order must not change output order.

Example: a condition selects `urgent` and skips `normal`. The merge receives one data result and one skip marker. It proceeds once both outcomes are known. It must not wait forever for an unselected branch to execute.

## Bounded Map

A Map node declares its collection, body scope, item identity, output assembly, and concurrency cap. Item paths become part of invocation identity. An empty collection returns an empty collection and schedules no body actions.

Proposed initial ceilings are 1,000 items per Map and 10 concurrent item bodies. Also impose total invocation and byte limits per run so nested Maps cannot bypass the limit. Reject or defer nested Map behavior until the compiler and tests support it explicitly.

Store results in declared item order. A failed item fails the Map unless the workflow chooses an explicit supported error policy. Do not silently drop failed items.

Before runtime history becomes too large, chunk work or use internal child executions. Preserve product run/invocation identity across those runtime boundaries. This is an engine implementation detail, not a user-facing subworkflow feature.

## Mapping and expressions

Use a restricted, versioned CEL environment in Go activity helpers. Expose only documented input data, upstream results, and approved pure functions. Do not expose credentials, network access, the filesystem, or process APIs.

Set evaluation-cost, input-depth, and output-size limits. Define missing-field behavior: a missing required field is an error; an optional field may use an explicit default. Do not silently treat every missing value as an empty string.

Mapping previews and actual execution must use the same evaluator version and rules. Store evaluated branch decisions or output references in runtime history. Preserve the evaluator compatibility version for old runs.

## External actions and retry safety

Execution delivery is at least once. External side effects are not automatically exactly once.

Consider a worker that creates a record remotely, then dies before reporting success. The provider has changed state. The runtime has no completed result. Retrying the request may create another record.

Every action declares one of these policies:

| Policy | When retry is allowed | Required evidence |
| --- | --- | --- |
| Pure or read-only | Retry transient failures within limits. | The action has no externally visible mutation. |
| Provider-idempotent | Reuse the same logical-action key. | Provider guarantees, key scope, and key retention are documented and tested. |
| Reconcile before retry | Query the provider for the original action first. | A reliable lookup distinguishes absent, completed, and unknown outcomes. |
| Unsafe to repeat | Do not retry an ambiguous attempt automatically. | Surface uncertainty and require a documented operator decision. |

The logical-action key includes run ID, node instance ID, and item/scope identity. It must remain the same across attempts of the same invocation. A deliberate new run gets new action keys.

Classify errors as invalid input, authentication, rate limit, transient, permanent, or uncertain outcome. A timeout or HTTP 5xx does not prove a remote write failed. Retry eligibility requires both a retryable error and a safe action policy.

Use bounded exponential backoff with jitter. Honor valid provider retry instructions within the run deadline. Do not hold a worker while waiting through a long backoff. Enforce provider/credential rate limits across replicas.

Disable automatic worker-loss retries for unsafe writes. Persist an intent record before dispatch when needed for reconciliation. A local completed-action table cannot atomically commit with an unrelated provider, so it cannot eliminate this ambiguity.

## Attempt results and late workers

Persist attempt results immutably. A stable result reference and checksum make repeat reporting idempotent. The orchestrator selects the accepted result for the invocation with a conditional write.

A timed-out worker can continue running briefly. Its late result must not replace the accepted result of another attempt. Side-effect safety still depends on the action policy; fencing local writes cannot fence a third-party API.

## Wait, cancellation, and deadlines

A durable wait persists a timer and releases active worker capacity. Propose a 30-day wait limit for the first release. Retain compatible workers, plan versions, and referenced data for every active run.

Cancellation stops new scheduling and requests cancellation of active activities. Activities must honor the cancellation signal where possible. Do not claim cancellation rolled back a completed external action.

If cancellation races with a completed terminal result, preserve the committed outcome and audit the later request. An accepted run cancelled before dispatch must not be started accidentally by a stale dispatcher. Validate this boundary in the failure tests.

Set separate limits for request timeout, activity attempt timeout, overall invocation retry duration, run deadline, and wait duration. A single timeout setting cannot express all these behaviors.

## Errors, operator decisions, and reruns

An unhandled required failure fails the run. A configured error edge can route a typed failure to explicit recovery work. Errors are not converted into empty successful outputs.

An uncertain invocation can pause as `needs_attention`. The operator may record a verified external result or choose a controlled new attempt only through a defined reconciliation API. That API must validate state, permission, and result schema, then record an audit event. It is not arbitrary editing of completed history.

If reconciliation is not supported for a node, the operator can end the run and start a deliberate rerun. Show that the rerun may repeat external writes. A rerun is a new run linked to its source; it does not rewrite the original.

Defer arbitrary “resume from any node.” Such a feature needs separate rules for reused outputs, changed credentials, skipped dependencies, and repeated side effects.

## Triggers and schedules

Manual requests accept a caller idempotency key. Webhooks validate authentication/signatures before admission and use a stable provider event ID where available. Scope keys by workspace, trigger, and source.

Reject a reused key with a different request fingerprint instead of silently returning an unrelated receipt. Document the deduplication retention window. A matching body is not a reliable substitute for an event ID.

Return `202 Accepted` only after the admission transaction commits. Async webhook response is the only initial mode. A request-size limit and a node-input limit are distinct: the trigger mapper must reject or explicitly reduce a larger accepted envelope before it becomes an oversized node input.

Each schedule specifies:

- IANA timezone and expression.
- Active workflow version binding policy.
- Overlap policy: allow concurrent runs, reject/skip an occurrence, or coalesce one pending occurrence.
- Missed-occurrence policy and catch-up limit.
- Behavior during daylight-saving gaps and repeated local times.

Proposed defaults: no concurrent main runs; at most one coalesced pending occurrence; one catch-up run after an outage. Missing local times are skipped. Repeated local times run once using a documented occurrence identity. Confirm that the selected Temporal schedule configuration can express these rules; otherwise expose the supported policy explicitly.

Bind scheduled requests to the active published version during admission. Keep schedule revision and occurrence identity with the run. See [Architecture](01-architecture.md#schedule-admission) for the admission wrapper and active-run slot.

## Publishing, disabling, and deletion

Publishing creates a new immutable version. Activating it changes the workflow's active-version pointer atomically. A concurrent admission sees either the old or new pointer, never a partial publication.

Disabling a trigger prevents new admissions after the disable transaction commits. It does not cancel existing runs unless the operator explicitly requests cancellation.

Archive workflows that still have versions, runs, or audit references. Physical deletion follows retention and referential-integrity rules. Credential deletion must respect revocation policy rather than leaving dangling unreviewed access.

Next: [Node development](03-node-development.md).
