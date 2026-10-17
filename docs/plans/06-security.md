# Security and access control

[Planning guide](README.md)

## Trust model

Runnelix initially serves a trusted internal organization. Workflow authors can still make mistakes, imported data can be hostile, webhooks can be forged, and provider responses can contain secrets.

Node modules are trusted application code reviewed and shipped with the product. A TypeScript worker process is not an isolation boundary for arbitrary untrusted code. User-uploaded scripts require a separate design and are outside this release.

The main protected assets are credentials, workflow payloads, workflow definitions, execution privileges, and audit records.

## Identity and roles

Integrate the team's OpenID Connect (OIDC) identity provider. Avoid building a password system for the first release. Use secure session cookies or a reviewed token-based deployment model. Enforce CSRF protections where cookie authentication is used.

Proposed roles:

| Capability | Viewer | Editor | Operator | Owner |
| --- | --- | --- | --- | --- |
| Read permitted workflow metadata | Yes | Yes | Yes | Yes |
| Read permitted run/payload data | Yes, subject to data grants | Yes, subject to data grants | Yes, subject to data grants | Yes, subject to policy |
| Edit drafts | No | Yes | No | Yes |
| Publish/activate production versions | No | Separate explicit grant | Separate explicit grant | Yes |
| Start/cancel/rerun production work | No | Separate explicit grant | Yes | Yes |
| Reconcile uncertain outcomes | No | No by default | Yes, with audit | Yes |
| Manage credentials/memberships | No | No by default | No by default | Yes |

Separate permission to edit a workflow from permission to execute it using a production credential. Credential-use grants are additional to workspace roles.

Enforce authorization on every server path, including SSE, dynamic option lookups, payload retrieval, exports, and internal worker APIs. Never rely on hidden UI buttons as access control.

## Workspace isolation

Scope every request and stored object to a workspace. Verify ownership when resolving nested references, not just the top-level workflow.

Use composite foreign keys or equivalent database constraints where they prevent cross-workspace references. Test against a user who can access workspace A but knows valid IDs from workspace B.

Row-level security can provide defense in depth if the team can operate it correctly. It does not replace application checks or authenticated worker scopes.

## Credential lifecycle

1. An authorized owner creates a logical credential and supplies its secret material.
2. The service encrypts the material before storing it.
3. A workflow references the credential ID, never its plaintext value.
4. At execution time, the worker authenticates and requests only the credential authorized for that invocation.
5. The service checks workspace, node type, run identity, credential grant, and revocation status.
6. Rotation creates a new protected version. Future accesses use the approved active version.
7. Revocation blocks future accesses and attempts. Already-sent external requests may still complete.

Use envelope encryption: encrypt values with a data key and protect that key with a versioned root key. Keep the root key outside the application database. A self-hosted installation may use an operator-mounted secret or an existing KMS.

Back up root-key material separately under the installation's access policy. A database backup without the required keys is not a usable recovery plan. Key rotation must retain access to old ciphertext until re-encryption or retention expiry completes.

Do not put decrypted values in Temporal arguments, results, errors, or search metadata. Credential access should return values only inside the authorized activity process for the required request.

## OAuth and token refresh

Use state and PKCE where supported and appropriate. Validate callback destinations, request minimal scopes, and bind the flow to the initiating user/workspace.

Serialize refresh per credential. Use version checks so a stale refresh result cannot replace a newer token. Handle rotated refresh tokens and revoked scopes explicitly. An authentication failure should not become a generic endless transient retry.

Record access and rotation metadata without recording token values. Define whether a connector can continue using an already-resolved short-lived credential during revocation; prefer short lifetimes and checks before new attempts.

## Network access and SSRF

Server-side request forgery (SSRF) occurs when an attacker makes the server access an unintended destination. A generic HTTP node makes this a core design concern.

Block cloud metadata, loopback, link-local, and private networks by default. Internal integrations need explicit approved host/network rules because internal services are a valid use case.

Validate the scheme, host, port, resolved addresses, and each redirect. Bound redirects and response size. Account for DNS rebinding rather than validating a hostname once and connecting elsewhere later. Apply deployment-level egress controls where possible.

The approved HTTP client is the supported path for node traffic. Trusted node review must reject bypasses. Document any provider that requires broader access before enabling it.

## Input and execution limits

Validate webhook signatures and timestamp windows before admission. Use a stable provider event ID for deduplication when available. Enforce request body limits before buffering unbounded data.

Bound graph size, expression cost, object depth, action duration, Map expansion, output bytes, total run bytes, and total invocations. Coordinate workspace concurrency and credential/provider rate limits across worker replicas.

Imported workflows are data. Reject executable fields and unknown formats. Do not evaluate imported strings as source code.

## Logs, payloads, and audit

Default logs to structured safe fields: IDs, duration, status, error code, and retry count. Do not log whole request, response, credential, or exception objects.

Redact known sensitive paths and sanitize provider errors. Secret-redaction tests should inject a distinctive sentinel value and search every output surface. Redaction is a second defense; avoiding raw capture is the first.

Workflow payloads may contain sensitive business data even when they contain no credentials. Define capture and retention rules, encrypt storage/backups, and apply separate read permissions where required.

Audit publication, activation, credential changes, membership changes, run starts, cancellations, reruns, reconciliation decisions, and policy changes. Store actor, target, time, request ID, and safe change metadata. Restrict audit deletion through the documented retention process.

## Build and deployment controls

Build from locked dependencies and reviewed source. Run services as non-root where feasible. Use minimal images, resource limits, TLS, internal service authentication, and restricted database users.

Scan dependencies and images before release. Review licenses and known vulnerabilities. Do not grant every worker the application owner's database credentials. Internal APIs need authentication even when deployed on a private network.

## Security release gate

**Gate S-01:** cross-workspace access tests, credential-grant tests, secret-leak tests, SSRF cases, webhook replay cases, OAuth/refresh cases, and resource-limit tests pass. Record remaining risks and the accountable owner before rollout.

Next: [Operations](07-operations.md).
