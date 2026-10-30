# Phase 5: local browser login and sessions

The browser editor can now use the configured OIDC provider. Login uses authorization code with S256 PKCE, a random state bound to an HttpOnly cookie, and a nonce checked against the verified ID token. The Go API exchanges the code. Provider access, ID, and refresh tokens are not persisted or returned to the editor.

## Configure the local installation

Apply migrations through `go run ./cmd/runnelix migrate`, including `011_browser_sessions.sql`. Create the workspace and [provision its first owner](phase-5-memberships.md#provision-the-first-owner). Register a **public client** with authorization-code flow, S256 PKCE, and the exact callback below at the provider. This slice does not support a confidential client secret or token refresh.

Example configuration for the existing Vite development proxy:

```sh
export RUNNELIX_AUTH_MODE=oidc
export RUNNELIX_OIDC_ISSUER=https://issuer.example
export RUNNELIX_OIDC_CLIENT_ID=registered-client-id
export RUNNELIX_BROWSER_ORIGIN=http://127.0.0.1:5173
export RUNNELIX_OIDC_REDIRECT_URL=http://127.0.0.1:5173/v1/auth/callback
```

The browser origin must be an HTTP or HTTPS **literal loopback** origin with no path, query, or fragment. The callback must equal that origin plus `/v1/auth/callback`. There is no caller-supplied return destination. The API remains bound to literal loopback and the browser proxy forwards `/v1` to it. Provider discovery, authorization, and token endpoints require HTTPS. Only an explicit `RUNNELIX_DEV_OIDC_LOOPBACK=1` permits HTTP literal-loopback test providers.

Omitting `RUNNELIX_OIDC_REDIRECT_URL` keeps bearer-only OIDC behavior and disables browser login. Supplying it in development mode fails API startup. Worker and node-service processes do not initialize browser authentication.

The local HTTP exception trusts the machine and other processes on it. Cookies are host-scoped, not port-scoped. This is not a public hosting configuration. For a configured HTTPS loopback origin, cookies use `Secure` and the `__Host-` prefix; TLS termination remains the operator's responsibility. The plain HTTP loopback cookie has no Secure flag so the supported local development flow works.

## Session behavior

- Login challenges expire after ten minutes. PostgreSQL stores the state hash, temporary PKCE verifier, nonce, issuer, and client ID. A callback atomically consumes its challenge before token exchange; failed exchange requires a new login. State must also match the initiating browser's cookie.
- A successful login issues a new 256-bit random opaque session and revokes the previous session presented by that browser. Only its SHA-256 hash, principal, client ID, and expiry are stored. Sessions survive API restart and are bound to the configured issuer/client.
- The session expires at the earlier of ID-token expiry and one hour. There is no sliding extension, refresh-token storage, or automatic renewal. Provider-side revocation is not polled: local session expiry, local logout, and current workspace membership define local access.
- Cookies are HttpOnly, host-only, Path `/`, and SameSite=Lax. Cookie and bearer identities cannot be combined on the same authenticated request.
- `GET /v1/auth/session` returns sign-in availability and, for a valid session, the principal, expiry, and CSRF token. Responses are no-store. The browser keeps CSRF state in memory, never localStorage or sessionStorage.
- Cookie-authenticated writes require the exact configured `Origin` and one matching `X-CSRF-Token` header. The CSRF token is SHA-256 of `csrf:` followed by the opaque cookie, separate from its stored hash. Both CSRF and state comparisons use constant-time comparison. Bearer requests retain their existing authentication model.
- `POST /v1/auth/logout` deletes the current local session and clears the cookie. It does not log out of the identity provider or revoke another browser's session. The editor confirms discarding unsaved edits before explicit logout.
- Session status is checked on page load and every 30 seconds. A protected API 401 immediately marks the session expired. The current draft stays mounted but inactive, and the account notice receives focus. Navigating to sign in still warns about unsaved edits. No draft is silently saved to browser storage.
- EventSource uses the same-origin session cookie. Each stream poll rechecks both the session and membership, so logout, expiry, or revoked data access closes the stream. Requests still have the existing ten-second bound; an already fetched event batch can finish sending. The editor stops event reconnection on an observed 401/403.

The store caps pending challenges at 1,000 and sessions at 10,000 across the installation. New inserts clean up expired rows; successful callbacks and logout remove their rows immediately. Dedicated retention scheduling and rate limiting remain operations work. The caps bound storage but are not a complete abuse-control policy.

Workspace roles and credential-use grants remain independent checks. Login does not create membership or assign ownership. The editor still targets `workspace_dev`; workspace selection and role-specific UI controls are not part of this slice. Server permission checks remain authoritative.

## Verification

`make integration-phase5` runs signed OIDC and PostgreSQL tests. Coverage includes cookie-bound state, PKCE and nonce rejection, expired challenges, concurrent single-use consumption, session hashing/rotation/issuer-client binding, restart persistence, expired session rejection, missing/wrong CSRF and Origin, mixed-authentication rejection, and logout closing a live cookie-authenticated event stream. `make test check` includes browser transport regressions and the existing unit/static suites.

For a reproducible fake-data browser environment:

```sh
go build -o .local/bin/runnelix ./cmd/runnelix
go build -o .local/bin/oidc-fixture ./tests/integration/oidcfixture
python3 tests/integration/browser_auth.py --hold
```

Use the printed browser URL. The fixture provider clearly identifies itself as a local test identity and offers **Continue as fixture owner**. The script owns an isolated PostgreSQL database, provisioned fake owner, API, provider, and Vite server. Ctrl-C stops its processes and retains artifacts. Set `RUNNELIX_PG_BIN` to the PostgreSQL binary directory if needed, and put the repository's pnpm shim on PATH when using the local tooling. Do not use the fixture provider for real users.

Browser verification covers actual redirect login, authenticated workflow creation, no provider tokens in browser storage, CSRF rejection, unsaved draft preservation after revocation, and desktop/narrow layouts. See [the browser record](evidence/phase-5-sessions-browser.json).

Production provider acceptance, confidential-client policy, public deployment, provider-wide logout, append-only audit, dedicated retention, and production security acceptance remain open. The next planned slice is safe append-only audit records.

Local verification on 2026-10-30:

- `make test check`: passed Go race tests/vet, 43 worker tests, TypeScript checks, formatting, catalog checks, and existing web tests. After the transport tests and final UI changes, `node --experimental-strip-types --test tests/web/*.test.ts` passed all five tests and `make check` passed.
- `make integration-phase4`: all four live groups passed, with unchanged source hashes and no cleanup errors. Evidence: `artifacts/phase1/20261030T084741Z-b192cd/results.json`.
- Final `make integration-phase5`: passed against isolated PostgreSQL after the session-status edge-case fix. Evidence: `artifacts/phase1/20261030T085826Z-a5abb7/result.json`.
- `go test -race ./internal/identity ./internal/api` and `pnpm check:web`: passed after the final status/heading changes.
- `.tools/openapi/bin/python -m openapi_spec_validator contracts/product/openapi.json`: passed.

The live Phase 4 run preceded the final origin-query guard and stale-cookie status fix; targeted Go/PostgreSQL tests verify those changes. The [browser record](evidence/phase-5-sessions-browser.json) distinguishes observed browser behavior from configuration/static checks. No production provider or deployment was tested.
