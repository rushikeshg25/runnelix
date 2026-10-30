#!/usr/bin/env python3
"""Bounded local burst and worker-outage measurements; not production acceptance."""
import concurrent.futures
import hashlib
import json
import math
import platform
import subprocess
import threading
import time
import urllib.error
import urllib.request
from phase0 import wait_until
from phase1 import APP, ROOT
from phase2 import ExecutionHarness, node, edge


def percentiles(values):
    ordered = sorted(values)
    return {f'p{p}': round(ordered[max(0, math.ceil(len(ordered) * p / 100) - 1)], 4)
            for p in (50, 95, 99)} if ordered else None


class Harness(ExecutionHarness):
    scope = 'Local capacity diagnostic: one workspace, ten sequential pure echoes, 20 offered starts/second for 60 seconds, then worker outage/recovery. No production capacity acceptance.'

    def source_hashes(self):
        result = super().source_hashes()
        path = ROOT / 'tests/integration/capacity.py'
        result[str(path.relative_to(ROOT))] = hashlib.sha256(path.read_bytes()).hexdigest()
        return result


def main():
    h = Harness()
    stop = threading.Event()
    sampler = None
    samples, sample_errors = [], []
    base = '/v1/workspaces/workspace_a'
    payload = {'message': 'x' * 1000}
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
        w = h.workflow([node(f'echo{i}', 'mock.echo') for i in range(10)],
                       [edge('start' if i == 0 else f'echo{i-1}', f'echo{i}') for i in range(10)], 'Capacity diagnostic')
        h.publish(w)
        warm = h.execute(w, 'warmup', payload)
        h.finished(warm)

        def sample():
            while not stop.is_set():
                try:
                    metrics = h.request('GET', base + '/operations')
                    pids = ','.join(str(p.pid) for p in h.processes.values())
                    rows = subprocess.check_output(['ps', '-p', pids, '-o', 'rss=', '-o', '%cpu='], text=True).splitlines()
                    samples.append({'monotonicSeconds': time.monotonic(), 'pendingStarts': metrics['pendingStarts'],
                                    'oldestPendingSeconds': metrics['oldestPendingSeconds'],
                                    'rssKiB': sum(int(row.split()[0]) for row in rows),
                                    'cpuPercent': sum(float(row.split()[1]) for row in rows)})
                except Exception as error:
                    sample_errors.append(type(error).__name__)
                stop.wait(1)

        sampler = threading.Thread(target=sample, daemon=True)
        sampler.start()
        body = json.dumps({'input': payload}).encode()

        def submit(index, scheduled):
            started = time.monotonic()
            request = urllib.request.Request(h.api + f'{base}/workflows/{w["id"]}/execute', data=body,
                       headers={'Content-Type': 'application/json', 'Idempotency-Key': f'burst_{index}'}, method='POST')
            try:
                response = urllib.request.urlopen(request, timeout=30)
            except urllib.error.HTTPError as error:
                response = error
            with response:
                result = json.load(response)
                return {'status': response.status, 'latencySeconds': time.monotonic() - started,
                        'sendLagSeconds': started - scheduled, 'runId': result.get('runId')}

        def burst():
            started = time.monotonic()
            with concurrent.futures.ThreadPoolExecutor(max_workers=40) as pool:
                futures = []
                for index in range(1200):
                    scheduled = started + index / 20
                    time.sleep(max(0, scheduled - time.monotonic()))
                    futures.append(pool.submit(submit, index, scheduled))
                responses = [future.result() for future in futures]
            admitted_at = time.monotonic()
            statuses = {str(status): sum(r['status'] == status for r in responses) for status in sorted({r['status'] for r in responses})}
            accepted = [r for r in responses if r['status'] == 202]
            (h.directory / 'burst-responses.json').write_text(json.dumps(responses, indent=2) + '\n')
            assert all(r['status'] in (202, 429) for r in responses), statuses
            assert accepted and len({r['runId'] for r in accepted}) == len(accepted)
            wait_until(lambda: int(h.sql("SELECT count(*) FROM runs WHERE state NOT IN ('succeeded','failed','cancelled')")) == 0, 'burst drain', 180)
            assert int(h.sql("SELECT count(*) FROM runs WHERE state <> 'succeeded'")) == 0
            assert int(h.sql('SELECT count(*) FROM attempts')) == (len(accepted) + 1) * 10
            # Recorded scheduling precedes worker start. First completion includes action duration.
            timing = json.loads(h.sql("""SELECT json_agg(t) FROM (
                SELECT extract(epoch FROM min(e.recorded_at)-r.accepted_at)::float8 AS scheduled,
                  extract(epoch FROM (SELECT min(a.completed_at) FROM attempts a WHERE a.run_id=r.id)-r.accepted_at)::float8 AS completed
                FROM runs r JOIN run_events e ON e.run_id=r.id
                WHERE r.id <> '%s' AND e.event->>'nodeState'='executing'
                GROUP BY r.id,r.accepted_at) t""" % warm['runId']))
            assert len(timing) == len(accepted)
            return {'offeredStarts': 1200, 'offeredPerSecond': 20, 'offeredDurationSeconds': 60,
                    'httpStatuses': statuses, 'admissionWallSeconds': round(admitted_at - started, 3),
                    'drainAfterResponsesSeconds': round(time.monotonic() - admitted_at, 3),
                    'httpLatencySeconds': percentiles([r['latencySeconds'] for r in responses]),
                    'clientSendLagSeconds': percentiles([r['sendLagSeconds'] for r in responses]),
                    'acceptanceToFirstSchedulingSeconds': percentiles([r['scheduled'] for r in timing]),
                    'acceptanceToFirstCompletionSeconds': percentiles([r['completed'] for r in timing]),
                    'targetStatus': 'not established: actual first-action start timestamps and representative provider workload are not measured'}
        h.record('Twenty offered starts per second for sixty seconds', burst)
        stop.set()
        sampler.join(timeout=35)
        assert not sampler.is_alive()
        assert samples and not sample_errors, sample_errors

        def outage():
            h.stop('nodes', crash=True)
            runs = [h.execute(w, f'outage_{i}', payload) for i in range(20)]
            wait_until(lambda: int(h.sql("SELECT count(*) FROM runs WHERE state='running'")) == 20, 'queued outage runs', 30)
            before = int(h.sql('SELECT count(*) FROM attempts'))
            time.sleep(2)
            assert int(h.sql('SELECT count(*) FROM attempts')) == before
            restarted = time.monotonic()
            h.start('nodes', ['node', 'dist/node-worker/product-main.js'])
            wait_until(lambda: int(h.sql("SELECT count(*) FROM runs WHERE state <> 'succeeded'")) == 0, 'worker recovery drain', 120)
            assert int(h.sql('SELECT count(*) FROM attempts')) == before + 200
            assert all(h.run(r)['state'] == 'succeeded' for r in runs)
            return {'queuedRuns': 20, 'actionsPerRun': 10, 'noWorkerObservationSeconds': 2,
                    'restartToDrainSeconds': round(time.monotonic() - restarted, 3), 'completedAttempts': 200}
        h.record('Worker outage preserves queued work', outage)
        h.results.append({'name': 'Environment and sampled resources', 'status': 'measured', 'detail': {
            'os': platform.platform(), 'machine': platform.machine(), 'workerProcesses': 1, 'workerActivityConcurrency': 4,
            'inputJSONBytes': len(json.dumps(payload).encode()), 'actionsPerRun': 10, 'workspaces': 1,
            'provider': 'pure mock.echo; no network delay, branches, Map or intentional retries',
            'sampleCount': len(samples), 'sampleIntervalSeconds': 1,
            'peakSampledRSSKiB': max(s['rssKiB'] for s in samples),
            'peakSampledCPUPercent': max(s['cpuPercent'] for s in samples),
            'peakSampledPendingStarts': max(s['pendingStarts'] for s in samples),
            'peakSampledOldestPendingSeconds': max(s['oldestPendingSeconds'] for s in samples),
            'postgresDatabaseBytes': int(h.sql('SELECT pg_database_size(current_database())')),
            'eventRows': int(h.sql('SELECT count(*) FROM run_events')),
            'resourceScope': 'owned API, node service, Temporal, engine, node worker and dispatcher PIDs; excludes PostgreSQL, children, client and OS. ps CPU is process-lifetime average, not instantaneous utilization.'}})
        (h.directory / 'samples.json').write_text(json.dumps(samples, indent=2) + '\n')
    finally:
        stop.set()
        if sampler:
            sampler.join(timeout=35)
        h.close()


if __name__ == '__main__':
    main()
