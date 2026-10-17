#!/usr/bin/env python3
"""Control, schedule, projection-outage, upgrade, and bounded-load evidence."""
import concurrent.futures
import hashlib
import json
import subprocess
import threading
import time
from pathlib import Path
from http.server import ThreadingHTTPServer
from phase2 import ExecutionHarness, Provider, node, edge, ROOT, APP
from phase0 import free_port, wait_until, CLI
class Harness(ExecutionHarness):
    scope='Local Phase 3 control/recovery/schedule/load checks using PostgreSQL, Temporal development SQLite, and fixture providers. Not production load or pilot evidence.'
    def source_hashes(self):
        result=super().source_hashes()
        for name in ['tests/integration/phase3.py','scripts/build-compat-v2.py']:
            result[name]=hashlib.sha256((ROOT/name).read_bytes()).hexdigest()
        return result

def main():
    h=Harness();provider=ThreadingHTTPServer(('127.0.0.1',0),Provider);provider.daemon_threads=True;threading.Thread(target=provider.serve_forever,daemon=True).start();url=f'http://127.0.0.1:{provider.server_port}'
    try:
        h.pg.start();h.command('migrate');h.command('bootstrap','--workspace','workspace_a');h.command('bootstrap','--workspace','workspace_b')
        h.start('api',[APP,'api','--address',h.api.removeprefix('http://')]);wait_until(lambda:h.request('GET','/health')['status']=='ready','API')
        h.start('node-service',[APP,'node-service','--address',h.node_service.removeprefix('http://')]);h.server();h.start('engine',[APP,'engine','--temporal-address',h.address]);h.start('nodes',['node','dist/node-worker/product-main.js']);h.dispatcher()
        def maps():
            w=h.workflow([node('map','control.map',collectionExpression='input.items',bodyType='mock.echo',bodyVersion=1,bodyConfig={},maxItems=10,concurrency=3)],[edge('start','map')]);h.publish(w)
            for key,items in [('five',[{'index':i} for i in range(5)]),('empty',[])]:
                r=h.execute(w,key,{'items':items});h.finished(r);events=h.events(r)
                output=[e['outputRef'] for e in events if e.get('nodeId')=='map' and e.get('nodeState')=='succeeded'][-1]
                assert h.request('GET',f'/v1/workspaces/workspace_a/runs/{r["runId"]}/payloads/{output}')=={'items':items}
                attempts=h.request('GET',f'/v1/workspaces/workspace_a/runs/{r["runId"]}/attempts');assert len(attempts)==len(items);assert len({a['invocationId'] for a in attempts})==len(items)
            r=h.execute(w,'excess',{'items':[{}]*11});h.finished(r,'failed');assert not h.request('GET',f'/v1/workspaces/workspace_a/runs/{r["runId"]}/attempts')
            return 'Five ordered item outputs have distinct invocation IDs; empty collection runs no action; expansion beyond maxItems fails before action dispatch.'
        h.record('Bounded Map ordering identity and empty/oversized collections',maps)
        def merge_wait():
            w=h.workflow([node('condition','control.condition',expression='true'),node('merge','control.merge',policy='named'),node('wait','control.wait',seconds=3),node('echo','mock.echo')],[edge('start','condition'),{'from':{'node':'condition','port':'true'},'to':{'node':'merge','port':'a'}},{'from':{'node':'condition','port':'false'},'to':{'node':'merge','port':'b'}},edge('merge','wait'),edge('wait','echo')]);h.publish(w);r=h.execute(w,'wait')
            h.finished(r,'waiting');h.stop('engine',crash=True);h.start('engine',[APP,'engine','--temporal-address',h.address]);h.finished(r)
            output=next(e['outputRef'] for e in h.events(r) if e.get('nodeId')=='echo' and 'outputRef' in e)
            assert h.request('GET',f'/v1/workspaces/workspace_a/runs/{r["runId"]}/payloads/{output}')=={'a':{'message':'fixture'}}
            return 'Merge ignored the skipped false input; a durable timer and output survived engine SIGKILL and restart.'
        h.record('Merge skip behavior and durable-wait restart',merge_wait)
        def resolution():
            w=h.workflow([node('post','http.post',url=url+'/unsafe')],[edge('start','post')]);h.publish(w);r=h.execute(w,'uncertain');h.finished(r,'needs_attention');h.stop('dispatcher')
            endpoint=f'/v1/workspaces/workspace_a/runs/{r["runId"]}/invocations/invocation_post/resolve'
            h.request('POST',endpoint,{'action':'confirm','output':{'status':201,'body':{'id':'verified-fixture'}},'note':'Checked local fixture; confirmed result'},status=202)
            assert h.run(r)['state']=='needs_attention';h.dispatcher();h.finished(r);assert Provider.calls['/unsafe']==1
            return 'Operator result persisted with dispatcher stopped, then resumed after restart without another unsafe provider call.'
        h.record('Durable operator reconciliation without resend',resolution)
        def projection():
            h.sql("CREATE FUNCTION fixture_projection_outage() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture projection outage'; END $$; CREATE TRIGGER fixture_projection_outage BEFORE INSERT ON run_events FOR EACH ROW EXECUTE FUNCTION fixture_projection_outage();")
            w=h.workflow([node('echo','mock.echo')],[edge('start','echo')]);h.publish(w);r=h.execute(w,'projection')
            def closed():
                result=subprocess.run([str(CLI),'workflow','describe','--workflow-id','runnelix/execute/'+r['runId'],'--address',h.address,'--output','json'],capture_output=True,text=True,timeout=5)
                return result.returncode==0 and 'COMPLETED' in result.stdout
            wait_until(closed,'closed workflow with projection outage',30);h.stop('dispatcher');h.stop('engine')
            assert h.run(r)['state']=='accepted';h.sql('DROP TRIGGER fixture_projection_outage ON run_events; DROP FUNCTION fixture_projection_outage();')
            h.dispatcher();h.finished(r);assert [e['sequence'] for e in h.events(r)]==[1,2,3,4,5]
            assert len(h.request('GET',f'/v1/workspaces/workspace_a/runs/{r["runId"]}/attempts'))==1
            h.start('engine',[APP,'engine','--temporal-address',h.address])
            return 'Every projection insert failed while execution completed. With engine offline, dispatcher rebuilt all five events from the closed runtime result, with no repeated action.'
        h.record('Projection outage recovers with engine offline',projection)
        def schedule():
            w=h.workflow([node('wait','control.wait',seconds=8),node('echo','mock.echo')],[edge('start','wait'),edge('wait','echo')]);h.publish(w)
            spec={'kind':'interval','intervalSeconds':86400,'hour':0,'minute':0,'timezone':'UTC','enabled':True,'input':{'scheduled':True}}
            schedule=h.request('POST','/v1/workspaces/workspace_a/schedules',{'workflowId':w['id'],'spec':spec})
            wait_until(lambda:any(s['id']==schedule['id'] and s['observedRevision']==1 for s in h.request('GET','/v1/workspaces/workspace_a/schedules')),'schedule sync')
            runtime='runnelix-schedule-workspace_a-'+schedule['id']
            def trigger():subprocess.run([str(CLI),'schedule','trigger','--schedule-id',runtime,'--address',h.address],check=True,capture_output=True,timeout=10)
            trigger()
            wait_until(lambda:h.sql(f"SELECT count(*) FROM schedule_occurrences WHERE schedule_id='{schedule['id']}' AND outcome='accepted'")=='1','actual schedule occurrence',20)
            runid=h.sql(f"SELECT run_id FROM schedule_occurrences WHERE schedule_id='{schedule['id']}' AND outcome='accepted'")
            r={'runId':runid};h.finished(r,'waiting')
            # TemporalScheduledStartTime has second precision. Use a distinct occurrence.
            first_second=int(time.time())
            wait_until(lambda:int(time.time())>first_second,'distinct scheduled second',3)
            trigger()
            wait_until(lambda:h.sql(f"SELECT count(*) FROM schedule_occurrences WHERE schedule_id='{schedule['id']}' AND outcome='overlap'")=='1','main-run overlap skip',20)
            h.finished(r);spec['enabled']=False;h.request('PUT',f'/v1/workspaces/workspace_a/schedules/{schedule["id"]}',{'workflowId':w['id'],'spec':spec},{'If-Match':'"1"'})
            wait_until(lambda:any(s['id']==schedule['id'] and s['observedRevision']==2 for s in h.request('GET','/v1/workspaces/workspace_a/schedules')),'schedule disable sync')
            return 'Temporal Schedule triggered the admission wrapper; second occurrence was skipped while the main run waited. Desired disable revision reached the runtime.'
        h.record('Actual Temporal schedule delivery overlap and disable',schedule)
        def daylight_saving():
            w=h.workflow([node('echo','mock.echo')],[edge('start','echo')]);h.publish(w)
            def matches(hour,start,end):
                spec={'kind':'daily','intervalSeconds':0,'hour':hour,'minute':30,'timezone':'America/New_York','enabled':False,'input':{}}
                schedule=h.request('POST','/v1/workspaces/workspace_a/schedules',{'workflowId':w['id'],'spec':spec})
                wait_until(lambda:any(s['id']==schedule['id'] and s['observedRevision']==1 for s in h.request('GET','/v1/workspaces/workspace_a/schedules')),'daily schedule sync')
                result=subprocess.check_output([str(CLI),'schedule','list-matching-times','--schedule-id','runnelix-schedule-workspace_a-'+schedule['id'],'--start-time',start,'--end-time',end,'--address',h.address,'--output','json'],text=True,timeout=10)
                data=json.loads(result);times=data.get('startTime',data.get('startTimes'))
                assert isinstance(times,list),data
                return times
            spring=matches(2,'2026-03-07T00:00:00Z','2026-03-10T00:00:00Z');assert len(spring)==2 and not any('2026-03-08' in t for t in spring),spring
            fall=matches(1,'2026-10-31T00:00:00Z','2026-11-03T00:00:00Z');assert len(fall)==4 and sum('2026-11-01' in t for t in fall)==2,fall
            return {'springMatches':spring,'fallMatches':fall,'policy':'Temporal skips nonexistent daily wall time; repeated times are deduplicated by the tested PostgreSQL admission key.'}
        h.record('Actual Temporal calendar DST gap and repeated hour',daylight_saving)
        def compatibility():
            oldapi=f'http://127.0.0.1:{free_port()}';h.start('old-api',[ROOT/'.local/bin/runnelix-v2','api','--address',oldapi.removeprefix('http://')]);h.start('old-nodes',['node',ROOT/'.local/compat-v2/dist/node-worker/product-main.js'])
            saved=h.api;h.api=oldapi
            try:
                wait_until(lambda:h.request('GET','/health')['status']=='ready','V2 API')
                w=h.workflow([node('echo','mock.echo')],[edge('start','echo')]);version=h.publish(w);assert version['plan']['interpreterVersion']==2
                r=h.execute(w,'version2');h.finished(r)
            finally:h.api=saved
            assert h.run(r)['workflowVersionId']==version['id'];h.stop('old-api');h.stop('old-nodes')
            return 'Pushed Phase 2 API/worker fixture at 9cd61d1 published V2; current dispatcher/engine executed it on the retained V2 route and old digest queue.'
        h.record('V2 workflow and action artifacts coexist with V3',compatibility)
        def load():
            w=h.workflow([node('echo','mock.echo')],[edge('start','echo')]);h.publish(w);start=time.monotonic()
            with concurrent.futures.ThreadPoolExecutor(max_workers=10) as pool:runs=list(pool.map(lambda i:h.execute(w,f'load_{i}'),range(30)))
            for r in runs:h.finished(r)
            seconds=time.monotonic()-start;assert len({r['runId'] for r in runs})==30
            return {'runs':30,'concurrentHTTPClients':10,'nodeWorkerConcurrency':4,'inputBytes':len(json.dumps({'message':'fixture'}).encode()),'seconds':round(seconds,3),'completedRunsPerSecond':round(30/seconds,3),'workload':'single pure echo action; no provider latency; local development services','claim':'bounded smoke load, not production operating-envelope acceptance'}
        h.record('Bounded concurrent admission and execution load',load)
    finally:provider.shutdown();provider.server_close();h.close()
if __name__=='__main__':main()
