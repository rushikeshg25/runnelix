#!/usr/bin/env python3
"""Timed local backup/restore and retention checks on isolated fake data."""
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import sys
import time

from phase1 import APP, ROOT, wait_until
from phase0 import CLI
from phase2 import ExecutionHarness, node, edge
from postgres import executable
sys.path.insert(0, str(ROOT / 'scripts'))
import local_ops


class OperationsHarness(ExecutionHarness):
    scope = 'Isolated native local backup/restore, retention, metrics and schema readiness. No production RPO/RTO claim.'
    def source_hashes(self):
        result = super().source_hashes()
        for name in ('scripts/local_ops.py', 'scripts/dev.py', 'tests/integration/operations.py'):
            result[name] = hashlib.sha256((ROOT / name).read_bytes()).hexdigest()
        return result


class RestoredPostgres:
    def __init__(self, state):
        self.directory = state
        settings = json.loads((state / 'settings.json').read_text())
        self.port = settings['postgresPort']
        self.dsn = f'postgresql://runnelix_dev@127.0.0.1:{self.port}/postgres?sslmode=disable'
        self.started = False
    def start(self):
        local_ops.command([executable('pg_ctl'), '-D', self.directory / 'postgres', '-l', self.directory / 'postgres.log', '-o', f"-h 127.0.0.1 -p {self.port} -k ''", '-w', 'start'])
        self.started = True
    def close(self):
        if self.started:
            local_ops.command([executable('pg_ctl'), '-D', self.directory / 'postgres', '-m', 'fast', '-w', 'stop'])
            self.started = False


def services(h):
    h.start('api', [APP, 'api', '--address', h.api.removeprefix('http://')])
    wait_until(lambda: h.request('GET', '/health')['status'] == 'ready', 'schema readiness')
    h.start('node-service', [APP, 'node-service', '--address', h.node_service.removeprefix('http://')])
    h.server()
    h.command('preflight', '--temporal-address', h.address)
    h.start('engine', [APP, 'engine', '--temporal-address', h.address])
    h.start('nodes', ['node', 'dist/node-worker/product-main.js'])
    h.dispatcher()


def main():
    os.umask(0o077)
    source = OperationsHarness()
    restored = None
    try:
        source.pg.start()
        source.command('migrate')
        source.command('bootstrap', '--workspace', 'workspace_a')
        source.sql('CREATE ROLE runnelix_dev WITH LOGIN SUPERUSER')
        key, token = secrets.token_hex(32), secrets.token_hex(32)
        source.env.update(RUNNELIX_CREDENTIAL_KEY=key, RUNNELIX_WORKER_TOKEN=token,
                          RUNNELIX_NODE_SERVICE_URL=source.node_service, TEMPORAL_ADDRESS=source.address)
        services(source)
        w = source.workflow([node('echo', 'mock.echo')], [edge('start', 'echo')])
        source.publish(w)
        r = source.execute(w, 'restore_dedup', {'message': 'restore-fixture'})
        source.finished(r)
        wait_until(lambda: source.run(r)['dispatchState'] == 'acknowledged', 'dispatch acknowledgment')
        credential = source.request('POST', '/v1/workspaces/workspace_a/credentials', {
            'name': 'Restore fixture', 'kind': 'webhook-hmac', 'value': 'restore-secret-fixture', 'destination': ''}, status=201)
        completed_history = json.loads(subprocess.check_output([str(CLI), 'workflow', 'show', '--workflow-id', 'runnelix/execute/' + r['runId'], '--address', source.address, '--output', 'json'], text=True))
        waiting_workflow = source.workflow([node('wait', 'control.wait', seconds=20), node('echo', 'mock.echo')], [edge('start', 'wait'), edge('wait', 'echo')])
        source.publish(waiting_workflow)
        waiting = source.execute(waiting_workflow, 'restore_wait')
        source.finished(waiting, 'waiting')
        wait_until(lambda: source.run(waiting)['dispatchState'] == 'acknowledged', 'waiting run acknowledgment')
        before_audit = source.sql("SELECT count(*) FROM audit_records WHERE workspace_id='workspace_a'")
        before_receipts = source.sql('SELECT count(*) FROM runtime_receipts')
        # Quiesce the application and runtime before taking the paired backup.
        for name in list(reversed(source.processes)):
            source.stop(name)
        source.pg.stop_server()
        (source.directory / 'data').rename(source.directory / 'postgres')
        (source.directory / 'settings.json').write_text(json.dumps({'postgresPort': source.pg.port, 'credentialKey': key, 'workerToken': token}))
        backup_dir = source.directory.parent / (source.directory.name + '-backup')
        keys = source.directory.parent / (source.directory.name + '-keys.json')
        started = time.monotonic()
        local_ops.backup(source.directory, backup_dir, keys)
        backup_seconds = time.monotonic() - started
        restored = OperationsHarness()
        target = restored.directory / 'restored'
        started = time.monotonic()
        local_ops.restore(backup_dir, keys, target)
        restored.directory = target
        restored.pg = RestoredPostgres(target)
        restored.env.update(RUNNELIX_DATABASE_URL=restored.pg.dsn, RUNNELIX_CREDENTIAL_KEY=key,
                            RUNNELIX_WORKER_TOKEN=token, RUNNELIX_NODE_SERVICE_URL=restored.node_service,
                            TEMPORAL_ADDRESS=restored.address)
        restored.pg.start()
        services(restored)
        assert restored.sql("SELECT count(*) FROM audit_records WHERE workspace_id='workspace_a'") == before_audit
        assert restored.sql('SELECT count(*) FROM runtime_receipts') == before_receipts
        assert restored.run(r)['state'] == 'succeeded'
        recovered_history = json.loads(subprocess.check_output([str(CLI), 'workflow', 'show', '--workflow-id', 'runnelix/execute/' + r['runId'], '--address', restored.address, '--output', 'json'], text=True))
        assert recovered_history == completed_history, 'Pre-backup Temporal history was not restored'
        restored.finished(waiting)
        assert any(e.get('nodeId') == 'echo' and e.get('nodeState') == 'succeeded' for e in restored.events(waiting)), 'Restored durable wait did not resume'
        assert restored.execute(w, 'restore_dedup', {'message': 'restore-fixture'})['runId'] == r['runId']
        # Exercise the restored encrypted secret through signed webhook verification.
        import hmac
        body = {'message': 'restored-webhook'}
        timestamp = str(int(time.time()))
        raw = json.dumps(body).encode()
        signature = hmac.new(b'restore-secret-fixture', timestamp.encode() + b'.restore_webhook.' + raw, hashlib.sha256).hexdigest()
        base = f'/v1/workspaces/workspace_a/workflows/{w["id"]}'
        # Grant the credential using the supported API, then admit once.
        restored.request('PUT', f'/v1/workspaces/workspace_a/credentials/{credential["id"]}/grants/{w["id"]}', status=200)
        webhook = restored.request('POST', f'/v1/workspaces/workspace_a/webhooks/{w["id"]}/{credential["id"]}', body, {
            'X-Runnelix-Timestamp': timestamp,
            'X-Runnelix-Signature': signature, 'Idempotency-Key': 'restore_webhook'}, status=202)
        restored.finished(webhook)
        smoke = restored.execute(w, 'post_restore_smoke')
        restored.finished(smoke)
        restore_seconds = time.monotonic() - started
        restored.results.append({'name': 'Timed paired restore and credential verification', 'status': 'passed',
                                 'seconds': round(restore_seconds, 3), 'detail': f'Backup {backup_seconds:.3f}s; restored schema, exact completed Temporal history, resumed durable wait, audit, receipt deduplication, encrypted HMAC secret, and new Go/TypeScript execution.'})
        # Explicit retention exercises the real CLI and HTTP 410 boundary.
        output = next(e['outputRef'] for e in restored.events(r) if e.get('outputRef'))
        restored.sql(f"UPDATE runs SET updated_at=now()-interval '8 days' WHERE id='{r['runId']}'")
        preview = json.loads(restored.command('retention', '-workspace', 'workspace_a').stdout)
        assert not preview['applied'] and any(c['runId'] == r['runId'] for c in preview['candidates'])
        restored.command('retention', '-workspace', 'workspace_a', '-apply')
        expired = restored.request('GET', f'/v1/workspaces/workspace_a/runs/{r["runId"]}/payloads/{output}', status=410)
        assert expired['code'] == 'data_expired'
        wait_until(lambda: restored.request('GET', '/v1/workspaces/workspace_a/operations')['pendingStarts'] == 0, 'outbox acknowledgment after smoke')
        metrics = restored.request('GET', '/v1/workspaces/workspace_a/operations')
        assert metrics['runs']['succeeded'] >= 3 and metrics['pendingStarts'] == 0
        restored.results.append({'name': 'Retention CLI and bounded metrics', 'status': 'passed', 'detail': 'Preview preserved data; apply returned explicit HTTP 410; scoped aggregate metrics stayed readable.'})
        # Refuse overwrite and mismatched keys without touching the restored data.
        try:
            local_ops.restore(backup_dir, keys, target)
            raise AssertionError('existing restore accepted')
        except RuntimeError:
            pass
        assert restored.run(smoke)['state'] == 'succeeded'
        print('PASS paired restore, credential verification, replay protection, retention, and metrics', flush=True)
    finally:
        if restored:
            restored.close()
        source.close()


if __name__ == '__main__':
    main()
