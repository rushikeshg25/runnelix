# Phase 4: built-in integrations and portable workflows

Status: local development implementation verified; independent node-author acceptance remains open.

Target: local installation with fixture providers only. GitHub issue creation and Slack message posting use the same built-in action interface. Both are unsafe external writes without a general exactly-once guarantee. Scoped bearer credentials remain encrypted and resolve only at the Go network boundary. Interactive OAuth installation is unsupported.

## Implementation slices

The first two slices were already committed. The remaining work follows the current request for six focused commits, superseding the earlier nine-slice plan.

1. Existing: declarative HTTP policies bind methods, destinations, path fields, and fixed headers at the Go boundary.
2. Existing: GitHub issue creation with strict schemas and adapter fixtures.
3. Slack message posting with bounded outputs, HTTP-200 error-body handling, cancellation, and schema fixtures.
4. Credential rotation and irreversible local revocation while preserving workflow grants and destination scope.
5. Versioned portable workflow API with exact credential-slot binding, Map body support, strict validation, and transactional unpublished import.
6. Editor import/export controls, browser verification, and an explicit future draft-version migration example.
7. Operator-only loopback provider fixtures, real PostgreSQL/Temporal worker checks, and a Phase 4 CI job.
8. Node-authoring instructions, catalog/module completeness checks, phase history, and verification records.

## Use and extend

- [GitHub issue action](../nodes/github-create-issue.md)
- [Slack message action](../nodes/slack-post-message.md)
- [Credential creation, rotation, and revocation](../nodes/credentials.md)
- [Portable workflow API, editor controls, and draft migration](../nodes/portable-workflows.md)
- [Node-authoring path, error contract, and retained artifacts](../nodes/authoring.md)

A normal action adds a TypeScript module, a descriptor, fixtures, and documentation. Adding these integrations required no provider-specific interpreter or canvas changes. HTTP authorization is declarative catalog data. Runtime package loading and arbitrary third-party code remain outside scope.

Fixture remapping is an explicit startup setting restricted to literal loopback. Production destinations stay fixed in descriptors. A workflow cannot supply an override or redirect a credential to a different host. The runtime still treats uncertain unsafe outcomes conservatively and never automatically resends them.

## Verification

Run `make test check`, `make integration-phase4`, and `make integration-phase2` with the documented development prerequisites. OpenAPI is checked with `openapi-spec-validator==0.7.2`. The catalog is generated from source; use `make catalog` after worker changes.

[Live evidence](evidence/phase-4-full.json) records four passing groups: both adapters, uncertain outcomes across restart without resend, portable API execution, and rotation/revocation on existing publications. [PostgreSQL checks](evidence/phase-4-postgres.json) and [all seven Phase 2 regressions](evidence/phase-2-after-phase-4.json) passed. `make test check` passed; the final catalog test brings the TypeScript total to 43.

[Browser evidence](evidence/phase-4-browser.json) covers desktop and 390-pixel layouts, pasted imports, downloads, explicit credential binding, persisted unpublished drafts, and unsaved-edit protection. File-picker upload was not exercised. OpenAPI 3.1 validation passed with `openapi-spec-validator==0.7.2`; an IPv4-only installer process resolved the initial package-download timeout.

## Acceptance limits

Automated checks prove local extension and execution behavior. The N-01 gate still requires a different developer to add a node. Independent teammate acceptance, representative production load, real-provider permissions, production identity, public deployment, and a two-week pilot remain open. No real issue or Slack message was created, and no changes were pushed in this continuation.

## Follow-up review

The [follow-up review](phase-4-review.md) records five corrected issues across import state, strict JSON, file size, credential pagination, and narrow layouts. It includes fresh runtime and browser verification.
