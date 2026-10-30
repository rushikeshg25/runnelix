# Add a built-in action

An ordinary integration is a trusted TypeScript module shipped with Runnelix. It is not downloaded or uploaded at runtime. The GitHub and Slack actions demonstrate the extension path; the interpreter and canvas do not contain provider-specific switches.

1. Add a descriptor to `contracts/product/catalog.source.json`. Declare a stable type and explicit version, config/input/output schemas, ports, and retry safety. Use narrow schemas and bounded strings. A write without a documented provider idempotency contract must use `unsafe`.
2. Declare the HTTP capability in `httpPolicy`: method, fixed HTTPS URL, optional path placeholders, fixed headers, and required credential presence. Placeholders can replace path segments only. Never accept arbitrary provider hosts, authorization headers, or credential values from workflow config.
3. Add an `Executor` in `node-worker/nodes/` and register `type@version` in `nodes/index.ts`. Follow `github.ts` or `slack.ts`. Check cancellation and call the supplied request capability. Do not use `fetch` directly. The module receives a credential reference, never its decrypted value.
4. Convert the response into bounded output fields. Throw `NodeFailure` with an existing stable code and a safe message. Do not copy raw provider errors. Slack demonstrates why HTTP status alone is insufficient.
5. Add fixtures for success, input/config rejection before send, malformed responses, cancellation, and provider rejection. Check that output drops unrelated provider fields. Add a live case when transport or effect semantics differ.
6. Run `make catalog`, then `make test check integration-phase4`. The catalog test checks that every action descriptor has a registered executor and vice versa. Commit the generated catalog with its source.
7. Document permissions, destination, supported fields, output identity, rate limits, and uncertain-result reconciliation. Use fake tokens and loopback fixtures only for development checks.

## Error and retry contract

`InvalidInput` means schema or pre-send validation failed. `ProviderRejected` is a sanitized provider rejection; `InvalidOutput` means a response violated the declared contract. The Go boundary owns destination enforcement, credential resolution, HTTP status classification, redaction, timeouts, and response size limits. Provider I/O never happens inside a Temporal workflow.

The current interpreter treats unsafe post-send failures conservatively, including explicit provider rejections: the run needs operator attention and does not resend. Pre-send errors such as unavailable credentials fail without implying an external effect. Reconciliation records a permanent operator decision. A fresh manual run is a new effect, not a safe retry mechanism.

## Versions and retained artifacts

`make catalog` hashes the worker source bundle and dependencies. Any change to that bundle changes the digest shared by the catalog. Published plans pin the digest and node version; workers listen on digest-specific queues. Keep compatible workers for every digest still reported by the required-artifacts endpoint. A source hash identifies a bundle; it is not a signed container or supply-chain attestation.

Breaking config, input, output, or behavior changes require a new node version. Keep the old descriptor and implementation, then migrate a draft explicitly as described in [portable workflows](portable-workflows.md). Existing versions and running executions must not be rewritten.

## Fixture transport

Set `RUNNELIX_DEV_LOOPBACK_EGRESS=1`, approve the provider and literal loopback hosts in `RUNNELIX_EGRESS_HOSTS`, and supply a JSON object in `RUNNELIX_DEV_PROVIDER_FIXTURES`, for example `{"slack.com":"http://127.0.0.1:9000"}`. These are startup-loaded node-service settings. Use fixture tokens only.

The service first checks the published provider URL and resolves a credential bound to the original provider host. The transport then replaces only the origin with the configured literal loopback origin. Path, method, body, and trusted headers remain unchanged. Redirects remain disabled. Invalid fixture settings stop startup. Workflow data cannot enable or change a fixture mapping. Without the setting, normal fixed HTTPS provider destinations apply.

Independent acceptance still requires another developer to add an action without editing the interpreter or canvas. Automated catalog and fixture checks do not close that gate.
