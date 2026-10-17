# Internal node development

[Planning guide](README.md)

## The extension promise

Adding a normal integration action should require a new module, schemas, fixtures, documentation, and one catalog registration. It should not require edits to the Go interpreter or ordinary editor form logic.

Nodes are trusted code developed with the product. There is no runtime package installer, public plugin loader, or user-uploaded code. A node ships through the normal application build and release process.

## Three kinds of nodes

| Kind | Example | Implementation owner |
| --- | --- | --- |
| Action | HTTP request, create issue, send message. | TypeScript executor using the node SDK. |
| Control | Condition, merge, Map, durable wait. | Go compiler and execution layer, with catalog metadata for the UI. |
| Trigger | Manual event, webhook, schedule. | Go admission/lifecycle code, with catalog metadata for the UI. |

A new action usually uses the existing extension seam. A new control construct changes execution semantics. A new polling trigger needs lifecycle and cursor rules. Do not force these differences into one arbitrary callback.

## Node definition contract

| Field | Purpose |
| --- | --- |
| `type` | Stable namespaced identifier, such as `http.request`. |
| `version` | Public behavior/schema version. Published workflows pin it. |
| `title`, `description`, `category` | Human-readable catalog metadata. |
| `ports` | Named inputs/outputs and permitted edge multiplicity. |
| `configSchema` | Configuration fields and validation constraints. |
| `inputSchema`, `outputSchema` | Runtime data contracts. |
| `ui` | Field order, labels, help, and existing widget hints. |
| `credentialType` | Logical credential type and required access. |
| `effects` | Pure, read-only, or external-write classification. |
| `retrySafety` | Retry/reconciliation policy from workflow semantics. |
| `limits` | Attempt timeout ceiling, output limit, and rate-limit scope. |
| `redaction` | Sensitive paths and safe diagnostic fields. |
| `examples` | Fake inputs, expected outputs, and common failure cases. |

Schemas and UI hints are serializable. Go and the interface consume a build-produced catalog manifest. They never import executable TypeScript modules.

## Executor contract

Illustrative TypeScript shape; finalize exact names and generated types during Phase 1:

```ts
type InvocationContext = {
  workspaceId: string;
  runId: string;
  nodeId: string;
  invocationId: string;
  attemptId: string;
  logicalActionKey: string;
  deadline: string;
  signal: AbortSignal;
  http: ApprovedHttpClient;
  credentials: ScopedCredentialAccess;
  logger: RedactedLogger;
};

type Outcome<T> =
  | { status: "succeeded"; output: T }
  | { status: "failed"; error: NodeError }
  | { status: "uncertain"; error: NodeError; reference?: string };

interface ActionNode<C, I, O> {
  definition: NodeDefinition;
  execute(ctx: InvocationContext, input: I, config: C): Promise<Outcome<O>>;
}
```

These types describe a boundary, not an implementation available to import today. A logical-action key is data for a provider's idempotency mechanism. Sending it to an API that ignores it provides no guarantee.

## Worker-host responsibilities

Before calling an executor, the TypeScript host validates the task envelope, node version, input, config, permission grant, and limits. It resolves stored payloads through authenticated internal APIs.

After execution, the host validates output, sanitizes errors, persists an immutable attempt result, and returns a reference plus minimal metadata. Map typed failures to runtime retry behavior deliberately. A raw exception must not accidentally enable unsafe activity retries.

The host supplies cancellation, approved HTTP, timeout enforcement, and logging. Node executors must not open database connections, inspect arbitrary environment secrets, or schedule unrelated workflow nodes.

These are trusted-code boundaries enforced by review and tests. A process boundary is not an untrusted-code sandbox.

## Add a node: exact development path

1. Select one action with a narrow outcome. Prefer “create issue” over a generic “do anything with this provider” operation.
2. Add `nodes/<service>/<action>/definition.ts`, `execute.ts`, fixtures, tests, and a short README.
3. Define config/input/output schemas with examples and required fields. Keep secrets out of config fields; reference a credential type.
4. Declare side effects and retry safety before implementing the provider request.
5. Use SDK HTTP and credential access. Bound pagination, response size, and total request time.
6. Register the node once in the catalog. Generate the manifest and fail the build on duplicate IDs/versions.
7. Run contract and behavior tests. Document rate limits, errors, and any manual reconciliation procedure.
8. Ship the node-worker artifact and catalog together. Verify that publication rejects a node version with no compatible worker deployment.

For a provider with several actions, share the provider client and credential definition. Keep each action's schema and retry policy explicit. Avoid a shared client that silently retries all writes.

## Form generation and dynamic options

Use JSON Schema plus a small set of maintained field widgets. Render text, number, Boolean, enum, object, list, and credential-reference fields consistently.

Dynamic option lookups, such as repository selection, are authenticated read-only server calls. They need workspace/credential checks, pagination limits, and cache rules. They must not leak credentials to the browser or execute an action while rendering a form.

If a new widget is needed, add it to the editor deliberately with accessibility tests. Do not let each node inject arbitrary frontend code.

## Compatibility and migration

A workflow pins a node behavior version. A run additionally pins an immutable compatible implementation artifact. Record the deployment/build identity in run diagnostics.

Breaking input, output, or side-effect changes create a new node version. A draft migration can propose config changes with a readable diff. It must never rewrite an immutable published version or an active run.

For a security fix, decide explicitly whether old behavior remains safe. If not, block affected new admissions and follow an audited active-run intervention policy. Do not quietly execute different code while claiming an old run stayed unchanged.

Retain old artifacts while referenced workflows must remain executable. Provide an operator report showing which versions prevent retirement. Test worker routing with active old runs before deleting an old deployment.

## Node acceptance checklist

- Valid input produces schema-valid output.
- Invalid config fails before making a provider request.
- Authentication, rate-limit, transient, permanent, and uncertain failures are classified.
- Retry policy matches actual provider guarantees.
- Cancellation, deadline expiry, and late completion are covered.
- Pagination and output sizes are bounded.
- Logs and errors omit sentinel secret values.
- Shared credential rate limits hold across worker replicas.
- Duplicate delivery uses the same logical-action key where supported.
- The ordinary editor form works without a node-specific canvas change.

## Maintainability gate

**Gate N-01:** a developer who did not implement the engine adds the second integration using only its module, catalog registration, tests, and documentation. A simple API-key action has a proposed one-day target, excluding provider access delays.

If this requires engine edits, examine the boundary. Fix a demonstrated missing contract once. Do not add speculative hooks for every possible provider.

Next: [Data and API contracts](04-data-and-api.md).
