#!/usr/bin/env python3
"""Import and execute the shipped pilot fixtures against isolated local services."""
import hashlib
import json
from phase0 import wait_until
from phase1 import APP, ROOT
from phase2 import ExecutionHarness


class Harness(ExecutionHarness):
    scope = 'Phase 6 preparation: five shipped portable fixtures on isolated PostgreSQL, Temporal, Go and TypeScript workers. No participant acceptance, real provider effects, or weekly usage evidence.'

    def source_hashes(self):
        result = super().source_hashes()
        for path in [ROOT / 'tests/integration/phase6.py', *sorted((ROOT / 'examples/pilot').glob('*.json'))]:
            result[str(path.relative_to(ROOT))] = hashlib.sha256(path.read_bytes()).hexdigest()
        return result


def main():
    h = Harness()
    base = '/v1/workspaces/workspace_a'
    try:
        h.pg.start()
        h.command('migrate')
        h.command('bootstrap', '--workspace', 'workspace_a')
        h.start('api', [APP, 'api', '--address', h.api.removeprefix('http://')])
        wait_until(lambda: h.request('GET', '/health')['status'] == 'ready', 'API readiness')
        h.start('node-service', [APP, 'node-service', '--address', h.node_service.removeprefix('http://')])
        h.server()
        h.start('engine', [APP, 'engine', '--temporal-address', h.address])
        h.start('nodes', ['node', 'dist/node-worker/product-main.js'])
        h.dispatcher()
        fixtures = ROOT / 'examples/pilot'
        manifest = json.loads((fixtures / 'cases.json').read_text())
        assert len(manifest) == 5
        assert {entry['file'] for entry in manifest} == {p.name for p in fixtures.glob('*.json') if p.name != 'cases.json'}
        for entry in manifest:
            def verify():
                bundle = json.loads((fixtures / entry['file']).read_text())
                w = h.request('POST', base + '/workflows/import', {'bundle': bundle, 'bindings': {}}, status=201)
                assert w['activeVersionId'] is None
                h.request('POST', f'{base}/workflows/{w["id"]}/execute', {'input': {}}, {'Idempotency-Key': 'unpublished'}, 409)
                version = h.publish(w)
                for case in entry['cases']:
                    r = h.execute(w, case['name'], case['input'])
                    finished = h.finished(r, case.get('state', 'succeeded'))
                    assert finished['workflowVersionId'] == version['id']
                    events = h.events(r)
                    assert [e['sequence'] for e in events] == list(range(1, len(events) + 1))
                    if 'output' in case:
                        ref = [e['outputRef'] for e in events if e.get('nodeId') == case['node'] and e.get('nodeState') == 'succeeded'][-1]
                        assert h.request('GET', f'{base}/runs/{r["runId"]}/payloads/{ref}') == case['output']
                    if 'skipped' in case:
                        assert any(e.get('nodeId') == case['skipped'] and e.get('nodeState') == 'skipped' for e in events)
                    if 'attempts' in case:
                        assert len(h.request('GET', f'{base}/runs/{r["runId"]}/attempts')) == case['attempts']
                return f'Imported an unpublished draft, published, and verified {len(entry["cases"])} execution cases against the shipped manifest.'
            h.record(entry['file'], verify)
    finally:
        h.close()


if __name__ == '__main__':
    main()
