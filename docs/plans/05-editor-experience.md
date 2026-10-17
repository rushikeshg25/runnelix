# Editor and run inspection

[Planning guide](README.md)

## Experience objective

A teammate should understand what a workflow will do before publishing it. After execution, the teammate should understand what happened and what action is safe next.

The editor is a view of the workflow contract. It cannot invent execution semantics that the backend does not enforce.

## Main user journey

1. Create a workflow and choose a trigger.
2. Add an action from the searchable node catalog.
3. Configure the action and select an authorized credential.
4. Map fields from sample trigger data or upstream outputs.
5. Add a condition and connect named output ports.
6. Validate the draft and resolve graph/configuration errors.
7. Run a clearly labeled test with mock or real actions.
8. Publish an immutable version and activate its trigger.
9. Inspect a run and use the supported recovery controls if it fails.

Saving, testing, publishing, and activating are distinct actions. Do not combine them into a button whose side effects are unclear.

## Screens and required behavior

| Screen | Essential information | Required states |
| --- | --- | --- |
| Workflow list | Name, owner, active version, trigger state, last run, failure count. | Empty, loading, stale data, permission denied, archived. |
| Workflow editor | Canvas, node list, current draft revision, save state, validation result. | Unsaved changes, save conflict, invalid graph, read-only access. |
| Node configuration | Schema fields, input mapping, credential reference, limits, example data. | Missing field, invalid type, inaccessible credential, unavailable node version. |
| Test panel | Selected version/draft, sample input, mock/real mode, expected side effects. | Pending, running, cancelled, uncertain, completed. |
| Publication review | Changes, version identity, validation errors, credential readiness. | Valid, blocked, concurrent draft update, activation failure. |
| Run list | Run ID, source, version, state, duration, queue age. | Filtered empty, delayed projections, pagination. |
| Run inspector | Exact graph version, invocation states, attempts, branch choices, sanitized data. | Waiting, retry scheduled, needs attention, cancellation pending, terminal. |
| Credential administration | Type, owner/workspace, access grants, expiry/revocation metadata. | Connected, expired, revoked, refresh failure. |

## Canvas and forms

Use React Flow for graph interaction, with a server-owned graph contract. Support selection, keyboard deletion, undo/redo, named ports, and clear invalid-edge feedback.

Generate ordinary forms from node schemas and maintained UI hints. Show required fields and field-level errors near the input. Keep labels consistent across nodes. A provider-specific action should not redesign the whole form system.

The credential selector exposes safe metadata, never secret values. If the user cannot use a credential, explain the permission requirement without revealing its contents.

## Mapping experience

Let the user select an upstream field or enter a bounded expression. Show the source node and field path explicitly. Distinguish an array value from a Map that executes once per item.

Preview mappings against sample data through the same evaluator contract used at runtime. Label samples with their source and age. A successful preview is not proof that every future event will satisfy the schema.

Show missing required fields as errors. Optional defaults must be explicit. Avoid displaying a plausible empty value when the expression failed.

## Safe testing

Default to a controlled mock provider or a clearly selected sample environment. Test execution must show which actions can perform real writes.

Before a real action, show the target credential/environment and effect description. This is product behavior for workflow users, not a request to add extra approval steps to routine development.

Do not call a real provider from graph validation or configuration rendering. A credential connection check should have a bounded, documented read-only behavior.

## Publication and conflicts

Saving a draft includes its expected revision. If another save changed that revision, show a conflict instead of overwriting silently. Collaborative live editing is deferred; optimistic conflict detection is still required.

Publication shows the immutable version to be created. Validation errors identify the node, port, or field. Missing worker support blocks publication of an unexecutable node version.

Activating a new version affects future admissions. The interface should state that active runs keep their original version.

## Run inspector

Render the run against its pinned version, not the user's latest draft. A node's displayed state must refer to an invocation; a Map node may have several item invocations.

Provide an attempt timeline with timing, error classification, and retry delay. Keep result data separate from errors. Show when output capture expired under retention policy.

If run projections are delayed, show their freshness and allow a supported refresh. A missing projection is not a successful run. SSE reconnect uses a persisted cursor; if the cursor expired, fetch current state and resume from a new cursor.

## Recovery controls

For a retry-safe failure, show the next retry time or exhausted policy. For an uncertain external write, explain that the provider may have completed the action.

Offer only supported operator decisions: inspect the provider, record a verified result, make an explicitly authorized new attempt, cancel, or end the run. Audit all such decisions. Do not expose arbitrary state editing.

A rerun creates a new run and can repeat writes. State this at the action point. Preserve a link to the original run and show which workflow version will be used.

## Accessibility and narrow screens

Desktop is the primary editing surface. Narrow screens prioritize run inspection, errors, and configuration. Provide a node-list alternative to the canvas so essential operations do not require precise dragging.

Verify keyboard access to node selection, form fields, validation errors, and recovery controls. Use visible focus, semantic labels, and text status indicators. Do not encode success or failure only by color.

Test at desktop and narrow viewport widths, at browser zoom, and with reduced motion enabled. Keep code/data panels scrollable inside their own region instead of expanding the whole page horizontally.

## Usability gate

**Gate U-01:** a teammate creates the example workflow, publishes it, observes a successful run, and diagnoses an injected failure without developer assistance. Record the confusing steps and fix them before claiming the first-release experience is complete.

Next: [Security](06-security.md).
