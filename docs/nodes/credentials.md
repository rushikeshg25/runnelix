# Integration credentials

Store bearer tokens through `POST /v1/workspaces/{workspace}/credentials` with `name`, `kind: "bearer"`, `value`, and `destination`. Grant access with `PUT /v1/workspaces/{workspace}/credentials/{credential}/grants/{workflow}`. Tokens remain encrypted in PostgreSQL and resolve only at the Go network boundary.

GitHub issue creation needs a fine-grained token restricted to the target repository with **Issues: write**. Use destination `api.github.com`. Slack posting needs a bot token with **chat:write** and channel membership. Use destination `slack.com`. Provider-side permissions remain authoritative. Interactive OAuth installation is unsupported.

Rotate a token with `PUT /v1/workspaces/{workspace}/credentials/{credential}/value` and body `{ "value": "replacement-token" }`. The ID, destination, kind, and workflow grants stay fixed. Future requests, including requests from existing published versions, resolve the replacement. Rotation is atomic; concurrent rotations use the last committed value. It does not revoke the old token at the provider or rotate the database encryption key.

Use `POST /v1/workspaces/{workspace}/credentials/{credential}/revoke` to block future resolutions. Repeating revocation succeeds. Rotation cannot reactivate a revoked credential; create and grant a new credential instead. Also revoke the token in GitHub or Slack. A request that already resolved a token can finish; local revocation cannot undo an external effect.

Responses and credential lists contain metadata only. Do not paste real tokens into shell history or workflow JSON. These routes are currently local development controls; production identity and workspace roles belong to Phase 5.

List credentials with `GET /v1/workspaces/{workspace}/credentials`. Results sort by ID with at most 100 records. Pass `?after={lastCredentialId}` to read the next page. Revoked records remain visible as metadata.
