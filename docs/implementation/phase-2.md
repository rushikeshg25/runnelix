# Phase 2: first executing workflow

Status: local development implementation complete; independent teammate acceptance pending. Baseline: `f4ded52` (pushed Phase 1).

## Outcome

Build a local author → publish → execute → inspect path. Existing admission-only records retain their original behavior. New execution requests use `RunnelixExecuteV2`; the earlier receipt-only workflow stays available.

## Commit ledger

| Slice | Change | Commit |
|---|---|---|
| 1 | Execution identity, payloads, invocations, immutable attempts/events | `ba4028e` |
| 2 | Pinned graph engine, CEL mapping, conditions and skip propagation | `63e30ab` |
| 3 | Protected node service, HTTP GET/POST, encrypted credential grants | `11a4094` |
| 4 | Execution API, webhook signatures, inspection, durable cancellation | `38f7096` |
| 5 | Editor foundation, workflow list, schema forms, optimistic drafts | `8d60656` |
| 6 | Canvas, accessible connections, mapping preview, pinned inspector | `684c683` |
| 7 | Live runtime/security tests and numeric-boundary fix | `2bad690` |
| 8 | Browser verification, repeatable local startup, CI | `97b5909` |
| 9 | Documentation, evidence, final checks and push | This documentation commit |

## Boundaries

The Go API and node service bind to literal loopback addresses. Workspace paths scope queries; they do not authenticate people. The browser Origin must match the explicitly configured editor origin. Use fake data until production identity and payload controls are complete.

The TypeScript worker receives a worker-service token. It does not need the database DSN or provider credentials. The service checks the complete persisted invocation envelope before resolving input or accepting output. The orchestrator selects the canonical attempt only after Temporal accepts its completion.

HTTP GET nodes can retry transient errors. HTTP POST nodes have one automatic attempt. An uncertain POST result pauses the run as `needs_attention`; cancellation is supported. Operator resolution is Phase 3 work. A provider success followed by a lost completion response remains uncertain.

Bearer credentials use AES-256-GCM, authenticated with workspace and credential identity. Each credential is restricted to a hostname and granted to a workflow. The service additionally checks the pinned node URL and method. HTTP requests ignore proxy environment variables, reject redirects and private destinations, pin a validated DNS result, and bound response bytes and time. A deliberate loopback exception exists only for local provider fixtures.

Payloads are limited to 8 KiB and remain plaintext development data. Credential encryption does not imply encryption of all workflow inputs and outputs. Production retention, encryption policy, quotas, OIDC, and audits remain later phases.

## Mapping behavior

Preview and runtime use the same CEL evaluator. Expressions have a 2,048-byte limit, a 10,000-operation cost limit, and a one-second evaluation deadline. Input and output are limited to 8 KiB. Whole JSON numbers become CEL integers; fractional numbers remain doubles. For mixed arithmetic, use an explicit conversion such as `double(input.count) + 0.5`.

`transform.map` returns an object. `control.condition` returns a Boolean and selects the `true` or `false` output. Unselected branches produce skip events. General merge, Map, wait, and schedule behavior is not part of Phase 2.

## Signed webhook format

Create a `webhook-hmac` credential with an empty destination and grant it to the workflow. Send the JSON input directly to:

`POST /v1/workspaces/{workspace}/webhooks/{workflow}/{credential}`

Set `Idempotency-Key`, `X-Runnelix-Timestamp` (Unix seconds), and `X-Runnelix-Signature`. The signature is lowercase hexadecimal HMAC-SHA256 over the exact bytes of `timestamp + "." + key + "." + raw_body`. Deliveries older than five minutes or more than 30 seconds in the future fail authentication. Retries keep the delivery key and body; generate a fresh timestamp/signature after the freshness window. A duplicate returns the original run, even after publication changes. Changed input with the same key conflicts.

Manual and webhook keys have separate namespaces. Do not share signing secrets between unrelated workflows. This local webhook endpoint is not an Internet ingress deployment.

## Inspection and cancellation

`POST .../workflows/{workflow}/execute` starts the active published version. `POST .../workflows/{workflow}/runs` still records admission only. `GET .../runs/{run}` returns the pinned version, execution mode, state, cancellation intent, and checkpoint.

`GET .../runs/{run}/events?after=N` returns up to 200 persisted events. `Accept: text/event-stream` enables a short-lived SSE connection. Reconnect with `Last-Event-ID`; no in-memory subscription owns the history. A successful cancellation response means intent was persisted. The dispatcher retries the Temporal cancellation until the run reaches a terminal projection.

Lists use a stable ID cursor (`?after=last_id`) and 50-row pages. Attempt inspection is bounded to 1,000 rows. These bounds are development limits, not a completed production quota system.

## Evidence and remaining gates

Live checks use an isolated PostgreSQL cluster, Temporal development SQLite, the Go engine, the TypeScript worker, and a loopback HTTP fixture. No test creates a GitHub issue or sends a Slack message.

A teammate's unassisted acceptance test remains a human gate. Local development verification is not a production rollout, a backup/PITR guarantee, or a two-week pilot.

## Start locally

1. Install Go, Node 24, pnpm 11.5.0, Python 3, and PostgreSQL server/client binaries.
2. Set `RUNNELIX_PG_BIN` if PostgreSQL binaries are not on `PATH`.
3. Run `make deps tools`.
4. Run `make dev`.
5. Open <http://127.0.0.1:5173>. Create a workflow, publish it, and run the initial echo example.
6. Press Ctrl+C in the launcher terminal to stop its services. The launcher preserves `.local/dev`.

The launcher checks ports 5173, 7233, 8787, and 8788 before starting. It refuses an occupied port and does not stop the existing owner. PostgreSQL uses an owned high port stored in private settings. The `.local/dev/settings.json` file contains local keys; do not share or commit it. Back up keys with development data if you need to retain credential access.

Provider egress is denied by default. To use a fixture server, explicitly set `RUNNELIX_EGRESS_HOSTS=127.0.0.1` and `RUNNELIX_DEV_LOOPBACK_EGRESS=1` before `make dev`. For a trusted HTTPS provider, approve its exact hostname without the loopback exception. The GitHub/Slack adapters follow in Phase 4; no real messages or records are required for these checks.

Before changing the worker bundle, stop local services, rebuild, and restart. Existing published versions keep their digest. Save and publish a new draft for new runs after a bundle change. Keep compatible worker artifacts for active old runs; artifact retention and upgrade procedures are not complete in this development slice.

## Verification commands

- `make test check`: Go race tests, 36 TypeScript tests, formatting, static checks, catalog drift, editor type checks.
- `make integration-phase2`: build, isolated PostgreSQL race tests, seven live execution cases.
- `make integration-phase1`: seven admission/dispatch regression cases.
- `make integration`: twelve runtime-spike regression cases.
- `.tools/openapi/bin/python -m openapi_spec_validator contracts/product/openapi.json`: OpenAPI 3.1 validation after installing validator 0.7.2 in that environment.

See [live evidence](evidence/phase-2-full.json) and [browser evidence](evidence/phase-2-browser.json). Browser verification covered creation/publication/execution, an invalid disconnected node, repaired validation, pinned inspection after draft edits, keyboard connection controls, and desktop/narrow layouts. The browser session was agent-driven; it does not satisfy the independent teammate gate.
