# Local pilot fixtures

These five portable bundles exercise the local runtime without credentials or external provider effects. They are training fixtures, not evidence of five useful production workflows.

| Bundle | Exercise |
| --- | --- |
| `echo.json` | Import, publish, execute, and inspect an unchanged object. |
| `transform.json` | Increment a priority with CEL. |
| `branch.json` | Route values above five; inspect the skipped branch. |
| `wait.json` | Resume after a one-second durable timer. |
| `map.json` | Preserve item order, accept empty input, and reject more than ten items. |

Use [cases.json](cases.json) for exact execution inputs and expected node outputs. It is a test manifest, not an importable workflow. Import one of the five bundles, publish the new draft, then execute each case with a fresh idempotency key. The oversized Map case must fail before any action attempt.

Run `make integration-phase6` from the repository root to verify the actual files through import, publication, execution, events, and payload inspection. This starts and stops isolated local services. See the [pilot kit](../../docs/pilot/README.md) for participant instructions and open acceptance gates.
