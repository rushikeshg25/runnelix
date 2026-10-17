# Provider contracts for the first two integrations

Checked: 2026-10-18. Implementation target: local fixtures. No provider records or messages were created while checking these documents.

| Action | Provider contract | Credential | Retry decision |
| --- | --- | --- | --- |
| Create GitHub issue | `POST https://api.github.com/repos/{owner}/{repo}/issues`; title required, body optional; successful creation returns issue identifiers. | Fine-grained bearer token restricted to the selected repository with Issues write permission. | Unsafe write. Do not resend automatically after an uncertain response. |
| Post Slack message | `POST https://slack.com/api/chat.postMessage`; channel and text; JSON result includes `ok`, channel, and timestamp. HTTP 200 alone does not mean success. | Bot bearer token with `chat:write` and access to the selected channel. | Unsafe write. Do not infer exactly-once behavior from a client message identifier. |

Sources: [GitHub create issue](https://docs.github.com/en/rest/issues/issues#create-an-issue), [Slack chat.postMessage](https://docs.slack.dev/reference/methods/chat.postMessage/).

## Scope

Use manual token provisioning for the local installation. An OAuth installation/refresh flow is not included in these two adapters. Store tokens as encrypted credential records, grant each record to the intended workflow, and bind it to the provider hostname. Never place tokens in workflow config, input, exports, or examples.

The first schemas should expose only the fields needed for the selected narrow actions. Add labels, attachments, blocks, repository lookup, pagination, or additional operations only when a real workflow requires them. Breaking schema or behavior changes require a new node version.

## Uncertain results

An operator must check the target repository or channel before confirming success. Confirm with the schema-valid output from the actual provider record, or fail the run. A failed network call does not prove the provider did nothing. A local fixture verifies classification and recovery flow; it does not establish live-provider behavior under every outage.
