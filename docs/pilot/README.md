# Controlled pilot kit

Status: local preparation. No participants, operational owner, start date, or release approval have been recorded. The five [training workflows](../../examples/pilot/README.md) prove basic runtime paths; participants must select five useful workflows before counting pilot usage.

## Installation checklist

Use one local installation per participant for training. The current launcher binds to loopback; this kit does not introduce a shared hosted installation. Confirm a shared deployment separately before any team rollout.

- [ ] Record the Git revision, OS, Go/Node/pnpm/PostgreSQL versions, and catalog digest. Use the pinned versions in the repository manifests.
- [ ] Follow the [native installation guide](../implementation/phase-5-operations.md#build-preflight-and-readiness). Run `make deps tools build-phase2`, then `make dev`. Keep `.local/dev` and its keys protected.
- [ ] Open `http://127.0.0.1:5173`. Confirm the API health check and execute the Echo fixture. Health alone does not prove worker execution.
- [ ] Use only fake data in unauthenticated development mode. For identity testing, follow [OIDC setup](../implementation/phase-5-sessions.md#configure-the-local-installation) and explicitly provision workspace membership.
- [ ] Run `make integration-phase6` to verify all shipped cases on separate fixture services. This does not test the participant's existing installation.
- [ ] Run `make integration-operations` for an isolated restore drill. Before adopting real data, perform and record recovery on the intended installation under the [operations procedure](../implementation/phase-5-operations.md).
- [ ] Review [retention](../implementation/phase-5-retention.md), unresolved unsafe actions, retained worker versions, and measured resource limits with the intended operator.
- [ ] Record who can stop admissions, investigate incidents, restore data, and approve reopening. Leave unassigned roles explicitly open.

## Participant exercise

1. Open **Import or export a workflow**. Choose `examples/pilot/echo.json`, select **Review import**, then **Import as new draft**.
2. Select the imported workflow from the list. Review its graph, validate, and publish. Import alone does not make a workflow executable.
3. Under **Run & inspect**, paste the case's `input` object from [cases.json](../../examples/pilot/cases.json) into **Trigger input (JSON)**. Select **Run published version**.
4. Inspect the selected published version, timeline, attempts, and node output. Compare the output with the case's `output` field.
5. Repeat for transformation, both branch inputs, the durable wait, and all three Map inputs. For the oversized Map, expect `failed` and zero action attempts. Explain the cause from inspection before correcting the input.
6. Author a new workflow yourself, publish it, run it, and diagnose a deliberate failure. Record time, assistance, and obstacles in [acceptance](acceptance.md). The imported examples do not prove unaided authoring.

If an action has an uncertain external outcome, stop and inspect the provider before choosing a reconciliation action. Do not create another run to test whether an unsafe write already happened. Report sanitized evidence through the [incident record](incidents.md).

## Running the actual pilot

Assign pseudonymous participant IDs and an accountable operator. Agree on five useful workflows, expected outputs, permitted data, and measurable limits. Record workflow and published version IDs in [weekly usage](weekly-usage.md). Do not put credentials, payload bodies, personal data, or protected logs in committed records.

Collect two consecutive weeks from three teammates. Record actual uses and incidents; do not backfill missing activity from fixture tests. An independent developer must also follow the [node authoring guide](../nodes/authoring.md) and record feedback. Finish with the [release decision](release-decision.md). Any unresolved gate remains open even when automated checks pass.
