# Data model and API contracts

[Planning guide](README.md)

## Sources of truth

PostgreSQL owns workflow definitions, accepted trigger requests, credentials, audit records, and stored payloads. Temporal owns live orchestration state. PostgreSQL run views are derived projections used for browsing and inspection.

Never schedule new workflow steps from a run-projection row. Reconciliation repairs projections; it does not create a second execution authority.

## Proposed records

| Record | Essential fields | Invariants |
| --- | --- | --- |
| `workspaces` | ID, name, policy revision, limits. | Stable identity and explicit retention/access policy. |
| `memberships` | Workspace ID, user ID, role. | Unique membership; audited role changes. |
| `workflows` | ID, workspace ID, name, active version, archived flag. | Active version belongs to this workflow. |
| `workflow_drafts` | Workflow ID, revision, graph, editor metadata. | Optimistic revision prevents silent lost updates. |
| `workflow_versions` | ID, sequence, graph, plan, schema/interpreter versions, digest, publisher. | Immutable after publication. |
| `trigger_inbox` | Workspace/source/trigger key, event key, fingerprint, payload reference, receipt. | Unique deduplication key within the documented retention window. |
| `start_outbox` | Run ID, workflow version, dispatch state, attempts, next try. | Created atomically with admission; stable run identity. |
| `schedules` | Spec, timezone, desired/observed revision, overlap/catch-up policy. | Reconcile desired state; no competing cron owner. |
| `active_run_slots` | Workflow/policy key, run ID, reservation state. | Transactional no-overlap admission; release only on verified terminal state. |
| `runs` | ID, pinned version, source, runtime ID, projected state, projection checkpoint. | Stable product ID and explicit projection freshness. |
| `node_invocations` | Run/node/scope identity, state, accepted result reference. | One logical invocation per stable identity. |
| `attempts` | Invocation ID, attempt identity, worker build, timing, result/error reference. | Preserve retry history and immutable results. |
| `run_events` | Run ID, event ID, ordering/checkpoint, sanitized payload. | Idempotent insertion and monotonic projection application. |
| `credentials` | Workspace ID, type, access grants, active version, revoked state. | Secret values never returned by general read APIs. |
| `credential_versions` | Ciphertext, wrapped key, key version, rotation metadata. | Encryption and rotation metadata stored separately from usable plaintext. |
| `payloads` | Workspace/run scope, checksum, size, encrypted or protected data, retention pin. | Authorized access and no expiry of live dependencies. |
| `node_catalog` | Type/version, schema, supported artifact references. | No duplicate behavior identity or executable browser content. |
| `audit_events` | Actor, workspace, action, target, timestamp, safe metadata. | Append-only application behavior; no secret payload copies. |

Use foreign keys and uniqueness constraints for enforceable relationships. Composite workspace references should prevent accidentally linking records across workspaces. Add indexes for pending outbox work, workspace/time run browsing, invocation lookup, and expiry scans.

## Transactions that matter

**Publication:** check draft revision, validate graph, insert immutable version, and optionally update the active-version pointer in one transaction. A version digest covers the execution-relevant definition; editor-only positions need not change execution identity.

**Admission:** validate deduplication key/fingerprint, reserve capacity if required, insert event receipt, pin the active version, create the run request, and add the outbox row atomically.

**Result selection:** store immutable attempt output, then select the invocation's canonical result with an idempotent conditional update. Late results cannot replace a chosen result.

**Projection:** insert a uniquely identified event and apply its projection/checkpoint together. If events arrive out of order, buffer or reconcile them under a documented ordering rule. Do not order events by wall-clock timestamps alone.

**Credential refresh:** serialize refresh for a logical credential and use version checks so two workers cannot overwrite a newer refresh token with an older one.

## Payload policy

Proposed initial limits are 256 KiB per node input/output and 1 MiB per webhook envelope. Set total bytes, depth, and invocation limits per run. Trigger parsing must explicitly reduce or reject envelopes that cannot fit the node contract.

Keep bounded JSON payloads in PostgreSQL initially. Protect sensitive payloads through encryption at rest, access checks, and explicit capture policy. Capture only data needed for execution and useful diagnosis. Redaction of display values does not erase stored raw data.

Retain completed-run payloads for 7 days and run summaries for 30 days by default. Active runs pin every required dependency. Long waits, open reconciliation, and operator retention holds override ordinary expiry. Retention deletion must preserve referential integrity and an auditable deletion outcome.

Add an object store when binary support or measured storage pressure requires it. Keep a payload-reference boundary so this change does not alter node semantics.

## API conventions

- Version public APIs under `/v1`.
- Derive workspace scope from an authorized route/context; do not trust an arbitrary body field.
- Use OpenAPI as the HTTP contract source and generate the frontend client.
- Use cursor pagination for lists. Define stable ordering and maximum page size.
- Use UTC timestamps with explicit offsets; preserve IANA timezone separately for schedules.
- Represent integers beyond JavaScript's safe range as strings.
- Validate request bodies and return field-specific errors.
- Use an idempotency key for run-start mutations and a revision/ETag for draft updates.
- Return a stable request ID for support without exposing stack traces or secrets.

## Proposed endpoints

| Method and path | Purpose | Important behavior |
| --- | --- | --- |
| `GET /v1/node-types` | Browse allowed node types. | Return serializable schemas and supported versions. |
| `POST /v1/workflows` | Create a workflow. | Workspace permission required. |
| `GET /v1/workflows/{id}` | Read metadata and draft revision. | Enforce workspace access. |
| `PUT /v1/workflows/{id}/draft` | Save a draft. | Reject stale revisions with a conflict response. |
| `POST /v1/workflows/{id}/validate` | Validate a draft. | Return node/port/field errors without executing actions. |
| `POST /v1/workflows/{id}/publish` | Create an immutable version. | Validate node availability and credential references. |
| `POST /v1/workflows/{id}/runs` | Start a manual run. | Idempotency key; acknowledge durable acceptance. |
| `GET /v1/runs/{id}` | Inspect a run. | Include pinned version and projection freshness. |
| `GET /v1/runs/{id}/events` | Follow run events through SSE. | Authorized, cursor-based reconnect; full refresh on cursor expiry. |
| `POST /v1/runs/{id}/cancel` | Request cancellation. | Idempotent request with a separate final state. |
| `POST /v1/runs/{id}/rerun` | Create a linked new run. | Explicit repeated-side-effect warning and permission. |
| `POST /v1/runs/{id}/invocations/{invocationId}/resolve` | Reconcile a supported uncertain outcome. | Validate state, evidence, schema, and operator role; audit the decision. |
| `POST /v1/credentials` | Create a credential. | Accept secrets; return only safe metadata. |
| `POST /v1/credentials/{id}/rotate` | Rotate access material. | Do not expose previous values. |
| `POST /hooks/{triggerId}` | Admit an external event. | Signature/authentication, replay checks, quotas, durable receipt. |

Schedule management, membership, and import/export endpoints should follow these conventions when their slice is implemented. Do not freeze speculative endpoint names before the first contract review.

## Example accepted response

```json
{
  "runId": "run_example_42",
  "workflowVersionId": "version_example_3",
  "state": "accepted",
  "requestId": "request_example_7"
}
```

This response promises durable acceptance, not a completed action. If dispatch is delayed, the UI shows the accepted state and queue age.

## Example action-task envelope

```json
{
  "contractVersion": 1,
  "workspaceId": "workspace_example",
  "runId": "run_example_42",
  "nodeId": "notify",
  "invocationId": "invocation_example_9",
  "nodeType": "http.request",
  "nodeVersion": 1,
  "implementationDigest": "example-immutable-build-id",
  "inputRef": "payload_example_input",
  "configRef": "config_example_notify",
  "logicalActionKey": "run_example_42/notify/item_0"
}
```

The worker obtains attempt identity, cancellation, and deadline from the runtime/host contract. Any credential reference in resolved config is an identifier, not a bearer capability. The credential service must authorize it independently.

## Import and export

Export a format version, workflow metadata, graph, node versions, and nonsecret configuration. Replace credential bindings with logical placeholders. Exclude execution payloads by default.

Import into a draft. Validate format, node versions, graph limits, and unsafe URLs. Require a local credential mapping before publication. Reject unknown executable fields; imported JSON must never become evaluated source code.

## Migration and retention safety

Use forward migrations and expand/contract changes. Deploy readers that accept both representations before changing writers. Remove old columns only after active versions and rollback requirements permit it.

An application rollback is safe only while the database contract remains compatible. A migration failure must stop rollout with a recoverable state; it must not trigger destructive database recreation.

Next: [Editor experience](05-editor-experience.md).
