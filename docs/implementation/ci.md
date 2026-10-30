# Continuous integration

The [CI workflow](../../.github/workflows/ci.yml) runs on pushes to `main`, all pull requests, manual dispatch, and Mondays at 06:23 UTC. Feature-branch pushes without a pull request no longer start automatic runs; use a pull request or manual dispatch. This avoids running both push and PR workflows for the same feature commit.

## Coverage and failure behavior

| Job | Coverage |
| --- | --- |
| Unit, static, security, and PostgreSQL checks | Workflow lint, Python regressions, Go race tests/vet, worker/web tests, catalog consistency, TypeScript/formatting, OpenAPI, live store/signed identity tests, source archive/checksum, npm/Go advisory checks. |
| Runtime phase0 | Cross-language runtime proof and failure injection. |
| Runtime phase1 | Contracts, admission, pinning, dispatch crash windows, and receipts. |
| Runtime phase2 | Execution, retry/unsafe actions, cancellation, credentials, webhooks/SSE, and worker restart. |
| Runtime phase3 | Controls, schedules/DST, projection recovery, reconciliation, retained V2 compatibility, and bounded smoke load. |
| Runtime phase4 | Provider fixtures, portability, credential lifecycle, paired backup/restore, and Phase 6 portable workflows. |
| CI passed | Requires the checks job and every runtime matrix entry to succeed. Failed, cancelled, or skipped dependencies cannot pass this gate. |

Runtime jobs wait for the shared checks. Matrix fail-fast is disabled so one runtime failure does not cancel evidence from other suites. A new run cancels older runs for the same event and branch/PR; scheduled and push runs use different groups. All jobs have explicit timeouts. The stable aggregate check is named **CI passed**. No branch-protection settings were changed; existing repository protection/ruleset queries showed no required old contexts when this workflow replaced them.

Unit/static checks now run once rather than five times; OpenAPI validation runs once rather than four times. The complete isolated store/signed-identity suite replaces repeated store-only runs. Each runtime runner still builds its own binaries against its own checkout. The [shared setup action](../../.github/actions/setup/action.yml) centralizes dependency installation and caches Go and pnpm dependencies. It derives Go from `go.mod` and pnpm from `package.json`, uses Node 24, installs PostgreSQL 16 tools when needed, and verifies the pinned Temporal archive on each runtime installation. Only Phase 3 fetches full Git history, which its historical interpreter build requires.

The longer capacity diagnostic remains opt-in with `make integration-capacity`. CI success is not browser, real-provider, capacity, production, licensing, or pilot acceptance.

## Workflow and dependency maintenance

Run `make check-ci` to lint workflow syntax, expressions, action inputs, and available shell checks with pinned actionlint v1.7.12. CI runs this before the product checks. External actions are pinned to full commit SHAs; Dependabot checks those action references weekly and proposes grouped updates. It does not auto-merge them. Weekly CI also reruns advisory checks, so a newly disclosed dependency issue can fail even without a source push.

## Artifacts

Successful and failed runs upload only `result.json` and `results.json` reports under `artifacts/`, with suite-specific artifact names and seven-day retention. Fixture databases, settings, credentials/keys, raw payload logs, and source release archives are not uploaded. Reports include outcomes, version information, and source hashes where the harness provides them. If a run fails before writing a report, inspect the job console and reproduce the named suite locally; missing reports are not fabricated.

Source packaging is checked on the runner, but no release is published or redistributed. Untracked local decision logs remain outside commits and release archives.

## Review

### Standards

Independent read-only review found no standards violations. The shared setup removes repeated configuration, least-privilege permissions and action pins are preserved, and the aggregate gate rejects incomplete runs.

### Spec

Independent read-only review found no missing prior coverage. The all-branch push policy changed intentionally as documented above. Hosted execution is required to validate the composite action, cache initialization, and matrix; local lint alone cannot establish these results.
