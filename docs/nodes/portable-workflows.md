# Portable workflows

Export a saved valid draft with `GET /v1/workspaces/{workspace}/workflows/{workflow}/export`. The version-1 bundle contains `formatVersion`, `name`, `graph`, and `credentialSlots`. Node versions stay explicit. Each action credential, including a Map body credential, becomes a slot such as `slot_1` with a required kind and destination.

Import with `POST /v1/workspaces/{workspace}/workflows/import`:

```json
{
  "bundle": { "formatVersion": 1, "name": "Example", "graph": {}, "credentialSlots": [] },
  "bindings": { "slot_1": "credential_destination_workspace_id" }
}
```

Replace the example bundle with the complete exported object. Supply exactly one binding per declared slot. Each credential must exist in the destination workspace, be active, and match the slot's kind and destination. The API validates node versions, config, graph edges, Map bodies, and slot consistency before writing. Unknown fields and duplicate JSON keys are rejected. The full request limit is 256 KiB.

Import creates a new workflow ID and revision 1, plus the explicitly requested credential grants, in one transaction. It never publishes, executes, enables a schedule, or restores runtime state. Validate and publish when ready. Export omits credential IDs and values, workflow identity, grants, versions, runs, and schedules. **Literal configuration values remain in the file.** Review expressions, URLs, and other authored data before sharing it.

The format does not promise identical artifact digests across installations. The destination compiles against its installed catalog on publication. Unsupported node versions fail validation rather than upgrading silently. Existing published versions remain immutable.

## Editor controls

Open **Import or export a workflow**. Export downloads the selected saved draft. For import, select a JSON file or paste the bundle, click **Review import**, and select a matching local credential for every slot. **Import as new draft** performs full server validation. Missing or revoked credentials do not appear in the choices. Create credentials through the API first. Pending editor changes must be saved or discarded before import/export.

## Explicit draft migration example

Suppose a future `slack.postMessage@2` changes input from `text` to `message`. Keep version 1 installed for existing published versions and waiting runs. This is an example, not an available version-2 adapter.

1. Export the current draft and retain its published version ID.
2. Install and verify the version-2 adapter and descriptor alongside version 1.
3. Change only the exported Slack node's `version` to `2`. Update its preceding mapping to produce `message`. For a Map action, update `bodyVersion` and its body configuration instead.
4. Import with explicit credential bindings, validate, and run with fixtures after publication.
5. Compare results before switching a trigger or schedule. Keep the old workflow and compatible worker artifacts until their runs finish.

There is no automatic node-version migration. Never edit an immutable published graph, rewrite a stored execution plan, or remove a worker digest that `GET /v1/workspaces/{workspace}/required-artifacts` still reports.
