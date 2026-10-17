# Phase 4: built-in integrations and portable workflows

Status: planned implementation. Target: local installation with fixture providers only.

The selected actions are GitHub issue creation and Slack message posting. Both create external effects without a documented general exactly-once guarantee. Keep their retries conservative. Use scoped, encrypted bearer credentials: a GitHub fine-grained token and a Slack bot token. Interactive OAuth installation is unnecessary for the selected local token-based path and remains unsupported.

## Nine implementation slices

1. Add a declarative HTTP request policy to node descriptors. Bind methods, destinations, path fields, and fixed headers at the Go boundary. Preserve generic HTTP behavior.
2. Implement GitHub issue creation with strict schemas and fixture-based adapter checks.
3. Implement Slack message posting, including Slack's HTTP-200 error body, with fixture-based adapter checks.
4. Support explicit credential rotation and document provider scope/revocation behavior.
5. Add versioned workflow export/import with explicit credential-slot bindings and strict validation.
6. Add portable workflow controls in the editor and an explicit draft-version migration example.
7. Exercise adapters through real local PostgreSQL/Temporal workers and a loopback provider fixture. Verify no provider resend after uncertainty.
8. Document and check the node-authoring path, catalog completeness, compatible artifacts, and error contracts.
9. Record verification, limitations, phase history, and push.

## Extension boundary

A normal action adds a TypeScript module, a descriptor, fixtures, and documentation. The interpreter remains unchanged. Request authorization is declarative catalog data, not a new provider switch in the engine or ordinary editor. Runtime package loading and arbitrary third-party code remain outside scope.

Provider fixtures must be explicitly configured by the local operator and restricted to literal loopback. Production destinations stay fixed in descriptors. A workflow cannot supply a fixture override or redirect a credential to a different host.

## Acceptance limits

Automated checks can prove the extension seam and local behavior. The original N-01 gate requires a different developer to add a node. That independent acceptance remains open. Real provider credentials, external records, public deployment, and a production pilot are not part of this phase's local evidence.
