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
