# Phase 5: workspace memberships

The local API now checks workspace membership after OIDC verification. Membership keys are the exact issuer and subject, scoped to a workspace. Email and workspace paths never establish identity. Every request reads current membership from PostgreSQL. Missing membership returns 403 without revealing whether a workspace exists. Database failures fail closed with 500.

## Provision the first owner

Run migrations and create the workspace with the existing `bootstrap` command. With `RUNNELIX_DATABASE_URL` set for that installation, run:

```sh
go run ./cmd/runnelix provision-owner --workspace workspace_dev --issuer https://issuer.example --subject provider-subject
```

Use the exact issuer and subject from the intended owner's verified identity. This is a trusted installation operation with database access, not an HTTP endpoint. It succeeds only when the workspace has no memberships. It never grants ownership to the first user who logs in. Provisioning does not validate a provider account or configure the OIDC issuer.

## Roles and grants

| Capability | Viewer | Editor | Operator | Owner |
| --- | --- | --- | --- | --- |
| Workflow definitions, exports, versions, run summaries, required artifacts | Yes | Yes | Yes | Yes |
| Create, edit, validate, import an unbound draft | No | Yes | No | Yes |
| Publish | No | `publish` grant | `publish` grant | Yes |
| Admit, execute, cancel, reconcile, invoke a webhook | No | `execute` grant | Yes | Yes |
| Run attempts, events, payloads, schedule inputs | `dataRead` grant | `dataRead` grant | `dataRead` grant | Yes |
| Credentials, credential grants, bound imports, memberships | No | No | No | Yes |

Schedule writes require execution permission **and** data-read permission because their responses contain schedule inputs. Publishing and execution are separate. Workflow definitions and exports include configured literals: do not use them to store secrets or runtime payloads. Viewer publish/execute flags have no effect. An owner has all current permissions. A future finer payload policy remains open.

Credential-use grants still independently constrain worker resolution. Editing a credential reference does not create a credential-use grant. Imports with any explicit binding require owner permission because import creates grants. Webhooks in OIDC mode require both execution membership and the existing HMAC signature; HMAC alone cannot bypass membership. Worker-service authentication stays on its separate worker token boundary.

`GET /v1/node-types`, `POST /v1/mapping-preview`, and `GET /v1/identity` require a signed identity but no workspace membership. `/health` remains public and reports readiness and mode. Development mode retains loopback-only access without role enforcement; membership mutations require OIDC even in development mode.

## Manage members

Owners use `GET`, `PUT`, or `DELETE /v1/workspaces/{workspace}/memberships`. PUT replaces the role and all flags for the supplied issuer/subject; omitted flags become false. DELETE accepts an issuer/subject JSON body. All mutations use the normal strict JSON and body-size limits. Example PUT body:

```json
{"issuer":"https://issuer.example","subject":"provider-subject","role":"editor","publish":false,"execute":true,"dataRead":false}
```

The list is ordered by issuer/subject. A workspace supports at most 100 members. The last owner cannot be removed or demoted. A workspace row lock serializes membership mutations and provisioning. The transaction rechecks the acting owner's role before writing, so a stale request cannot retain member-management authority after demotion. Concurrent owner demotions cannot leave the workspace without an owner. Concurrent valid PUT requests use last-writer-wins semantics.

Revocation affects requests admitted after the permission check. Already accepted runs and schedules remain workspace resources; revocation does not cancel them. SSE streams check membership every 300 ms before fetching another batch and close on denial or database failure. An already fetched batch can finish sending. Streams retain the existing 10-second request bound; token validation occurs at connection time and again on reconnect.

Reconciliation records `oidc:` followed by the SHA-256 hex digest of the JSON-encoded verified principal (`issuer`, then `subject`). This binds actions to the principal within the existing actor length limit. It is not a substitute for the planned audit log.

## Verification and remaining scope

`make integration-phase5` owns an isolated PostgreSQL instance and runs the store and signed-token suites sequentially. It verifies every workspace route rejects a signed non-member, issuer/subject isolation, role restrictions, explicit grants, valid create/publish/execute, revocation, live SSE closure, and concurrent last-owner protection. The store suite also checks existing credential grants and workspace isolation. Unit-only runs skip the database tests explicitly.

The subsequent [browser session slice](phase-5-sessions.md) implements local login and CSRF protection. Production provider selection, production session policy, append-only audit, data retention, production payload protection, and operational acceptance remain open. This work does not approve public deployment.

Local verification on 2026-10-30:

- `make test check`: passed Go race tests/vet, 43 worker tests, three web transport tests, TypeScript checks, catalog consistency, and formatting.
- `make integration-phase4`: passed all four live integration groups with unchanged source hashes and no cleanup errors. Evidence: `artifacts/phase1/20261030T081608Z-66fb20/results.json`.
- `make integration-phase5 check`: passed after the final membership DELETE contract adjustment. Isolated database evidence: `artifacts/phase1/20261030T081741Z-074fa6/result.json`.
- `.tools/openapi/bin/python -m openapi_spec_validator contracts/product/openapi.json`: passed.

The live Phase 4 run preceded the final DELETE-body adjustment; the signed API suite verifies that adjustment. No browser-login test or production deployment was performed. Apply migration `010_memberships.sql` through the migration command before using these API changes. Existing installations receive no automatic memberships or privileges.
