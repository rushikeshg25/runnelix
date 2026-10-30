# Phase 5 first slice: verified identity

This is an identity foundation for the local installation. It does not enable production use. The subsequent [browser session slice](phase-5-sessions.md) adds local login. The subsequent [membership slice](phase-5-memberships.md) adds workspace permissions.

## Configuration

The API reads these settings at startup:

- `RUNNELIX_AUTH_MODE`: `development` (default) or `oidc`. Unknown modes fail startup.
- `RUNNELIX_OIDC_ISSUER`: exact OIDC issuer URL, without query, fragment, or URL credentials.
- `RUNNELIX_OIDC_CLIENT_ID`: expected ID-token audience.
- `RUNNELIX_DEV_OIDC_LOOPBACK=1`: permits literal loopback HTTP issuer and signing-key endpoints for local fixtures. It never permits an HTTP hostname such as `localhost` or a non-loopback HTTP destination.

OIDC settings in development mode fail startup instead of being silently ignored. Both modes retain the current literal-loopback listening and Host restrictions and the configured browser-Origin check. Node-service authentication remains the separate worker-token protocol.

In OIDC mode the API discovers provider metadata and verifies RS256 ID tokens with pinned `github.com/coreos/go-oidc/v3`. Discovery and key requests have five-second timeouts, a 1 MiB response limit, and no redirects. HTTPS is required unless the explicit local fixture option applies. Key URL queries are supported. Failed discovery stops startup; failed key retrieval or validation rejects the request without falling back to development mode.

## Verified principal

Send an ID token for the configured client in a single `Authorization: Bearer ...` header to `GET /v1/identity`. Query-string tokens and duplicate Authorization headers are rejected. The token must have a valid signature, exact issuer, expected audience, unexpired expiration, nonempty subject, and an integer issued-at value no more than 30 seconds in the future. A future `nbf` is rejected. Multiple audiences require the configured client as `azp`; any supplied `azp` must match.

The response contains `mode: "oidc"` and a `principal` with only `issuer` and `subject`. It never returns the token or provider claims such as email. Identity uses the issuer/subject pair; neither an email nor a workspace path grants access.

Without authentication, `/v1/identity` returns 401 with a safe error and `WWW-Authenticate`. Invalid-token details and token values do not appear in responses. In development mode it returns only `mode: "development"`, without claiming an authenticated user.

## Workspace authorization boundary

OIDC mode now checks [workspace memberships and permissions](phase-5-memberships.md) after verifying identity. A valid token alone grants no workspace access. `/health` remains public; identity inspection, catalog, and mapping preview require authentication without membership. The local editor can now use the [configured browser session flow](phase-5-sessions.md).

Local browser login now has PKCE and cookie/CSRF protection. Privileged workspace mutations now have [audit records](phase-5-audit.md). Production provider selection, production token policy, and operational rollout remain open. This endpoint is a local verifier probe, not a completed login/session protocol.

## Verification

`go test -race ./internal/identity ./internal/api ./cmd/runnelix` runs a real loopback discovery/JWKS issuer with generated fixture RSA keys. Tests cover signature, issuer, audience, expiry, issued-at/not-before, authorized-party checks, unsafe discovery, bounded responses, missing/duplicate header authentication, and safe authentication failures. `make integration-phase5` additionally tests signed workspace permissions against isolated PostgreSQL. No external identity provider or real user account is used.
