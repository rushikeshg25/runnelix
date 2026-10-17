#!/usr/bin/env python3
"""Actual HTTP/PostgreSQL/Temporal admission and crash-window checks."""
import concurrent.futures
import datetime
import hashlib
import json
import os
from pathlib import Path
import signal
import subprocess
import time
import urllib.error
import urllib.request

from phase0 import free_port, wait_until, CLI, ROOT
from postgres import Postgres, executable

APP=ROOT/'.local/bin/runnelix'
PROBE=ROOT/'.local/bin/phase1-probe'
GRAPH={'formatVersion':1,'nodes':[{'id':'start','type':'trigger.manual','version':1,'config':{}},{'id':'echo','type':'mock.echo','version':1,'config':{}}],'edges':[{'from':{'node':'start','port':'out'},'to':{'node':'echo','port':'in'}}]}

def source_hashes():
    paths=sorted(set(p for pattern in ['internal/**/*.go','internal/store/migrations/*.sql','cmd/runnelix/*.go','contracts/product/*.json','node-worker/product*.ts','node-worker/nodes/*.ts','scripts/build-catalog.py','tests/integration/phase1*.py','tests/integration/postgres.py','tests/integration/phase1probe/*.go','go.mod','go.sum','package.json','pnpm-lock.yaml'] for p in ROOT.glob(pattern)))
    return {str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for p in paths}

class Harness:
    def __init__(self):
        self.pg=Postgres();self.directory=self.pg.directory
        self.env={**os.environ,'RUNNELIX_DATABASE_URL':self.pg.dsn}
        self.address=f'127.0.0.1:{free_port()}';self.api=f'http://127.0.0.1:{free_port()}'
        self.processes={};self.results=[];self.counter=0
        self.hashes=source_hashes()
    def command(self,*args):
        return subprocess.run([str(APP),*args],env=self.env,cwd=ROOT,capture_output=True,text=True,check=True,timeout=20)
    def start(self,name,args):
        assert name not in self.processes
        self.counter+=1
        with (self.directory/f'{self.counter:02}-{name}.log').open('wb') as log:
            self.processes[name]=subprocess.Popen([str(a) for a in args],env=self.env,cwd=ROOT,stdout=log,stderr=subprocess.STDOUT,start_new_session=True)
    def stop(self,name,crash=False):
        process=self.processes.pop(name,None)
        if process and process.poll() is None:
            os.killpg(process.pid,signal.SIGKILL if crash else signal.SIGTERM)
            try:process.wait(timeout=10)
            except subprocess.TimeoutExpired:os.killpg(process.pid,signal.SIGKILL);process.wait(timeout=5)
    def server(self):
        self.start('temporal',[CLI,'server','start-dev','--headless','--ip','127.0.0.1','--port',self.address.split(':')[1],'--db-filename',self.directory/'temporal.sqlite'])
        wait_until(lambda:subprocess.run([str(CLI),'operator','cluster','health','--address',self.address],capture_output=True,timeout=3).returncode==0,'Temporal readiness',45)
    def worker(self):self.start('worker',[APP,'worker','--temporal-address',self.address])
    def dispatcher(self,name='dispatcher'):self.start(name,[APP,'dispatcher','--temporal-address',self.address])
    def request(self,method,path,body=None,headers=None,status=200):
        hdr={'Content-Type':'application/json',**(headers or {})}
        request=urllib.request.Request(self.api+path,data=json.dumps(body).encode() if body is not None else None,headers=hdr,method=method)
        try:response=urllib.request.urlopen(request,timeout=12)
        except urllib.error.HTTPError as error:response=error
        with response:
            result=json.load(response)
            assert response.status==status,(method,path,response.status,result)
            assert response.headers.get('X-Request-ID')
            return result
    def sql(self,query):
        # Only fake-data fixture queries; never accept an arbitrary external database URL.
        return subprocess.check_output([executable('psql'),self.pg.dsn,'-X','-A','-t','-v','ON_ERROR_STOP=1','-c',query],text=True).strip()
    def new_workflow(self):
        return self.request('POST','/v1/workspaces/workspace_a/workflows',{'name':'Fixture','graph':GRAPH},status=201)
    def publish(self,w,revision=1):return self.request('POST',f'/v1/workspaces/workspace_a/workflows/{w["id"]}/publish',headers={'If-Match':f'"{revision}"'})
    def admit(self,w,key):return self.request('POST',f'/v1/workspaces/workspace_a/workflows/{w["id"]}/runs',{'input':{'message':'fake data'}},{'Idempotency-Key':key},202)
    def run(self,r):return self.request('GET',f'/v1/workspaces/workspace_a/runs/{r["runId"]}')
    def acknowledged(self,r):
        wait_until(lambda:self.run(r)['dispatchState']=='acknowledged','durable dispatch acknowledgement',25)
        assert self.run(r)['state']=='accepted','Admission must not imply graph execution'
    def probe(self,mode,r):
        checkpoint=self.directory/f'{mode}.json'
        self.start('probe',[PROBE,'--mode',mode,'--checkpoint',checkpoint,'--run',r['runId'],'--address',self.address])
        wait_until(checkpoint.exists,mode+' checkpoint',20)
        self.stop('probe',crash=True)
    def record(self,name,callback):
        start=time.monotonic()
        try:detail=callback()
        except Exception:
            self.results.append({'name':name,'status':'failed'});raise
        self.results.append({'name':name,'status':'passed','seconds':round(time.monotonic()-start,3),'detail':detail});print('PASS',name,flush=True)
    def close(self):
        cleanup_errors=[]
        for name in list(reversed(self.processes)):
            try:self.stop(name)
            except Exception as error:cleanup_errors.append(name+': '+type(error).__name__)
        try:self.pg.close()
        except Exception as error:cleanup_errors.append('postgres: '+type(error).__name__)
        unchanged=self.hashes==source_hashes()
        report={'timestampUtc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'scope':'Local PostgreSQL and Temporal development SQLite. Admission only; no graph execution or production deployment.','versions':{'postgres':subprocess.check_output([executable('postgres'),'--version'],text=True).strip(),'temporal':subprocess.check_output([str(CLI),'--version'],text=True).strip(),'go':subprocess.check_output(['go','version'],text=True,cwd=ROOT).strip()},'sourceSha256':self.hashes,'sourcesUnchanged':unchanged,'cleanupErrors':cleanup_errors,'results':self.results}
        (self.directory/'results.json').write_text(json.dumps(report,indent=2)+'\n')
        print('Evidence:',self.directory.relative_to(ROOT),flush=True)
        if cleanup_errors or not unchanged:raise RuntimeError('Harness cleanup or source consistency failed; inspect results.json')

def main():
    for p in [APP,PROBE,CLI]:
        if not p.exists():raise SystemExit(f'Missing {p.name}; build phase 1 and install Temporal first')
    h=Harness()
    try:
        h.pg.start();h.command('migrate');h.command('migrate');h.command('bootstrap','--workspace','workspace_a');h.command('bootstrap','--workspace','workspace_b')
        h.start('api',[APP,'api','--address',h.api.removeprefix('http://')]);wait_until(lambda:h.request('GET','/health')['status']=='ready','API readiness')
        w=h.new_workflow();version=h.publish(w)
        base=f'/v1/workspaces/workspace_a/workflows/{w["id"]}'
        def contracts():
            h.request('GET','/v1/node-types',headers={'Origin':'https://example.test'},status=403)
            h.request('GET','/v1/node-types',headers={'Host':'rebind.example.test'},status=403)
            h.request('GET',f'/v1/workspaces/workspace_b/workflows/{w["id"]}',status=404)
            assert h.request('POST',base+'/validate')['digest']==version['digest']
            invalid={**GRAPH,'edges':[]};draft=h.request('POST','/v1/workspaces/workspace_a/workflows',{'name':'Incomplete','graph':invalid},status=201)
            result=h.request('POST',f'/v1/workspaces/workspace_a/workflows/{draft["id"]}/publish',headers={'If-Match':'"1"'},status=422)
            assert result['issues'][0]['code']=='missing_input'
            h.request('PUT',base+'/draft',{'graph':GRAPH},{'If-Match':'"1"'})
            h.request('PUT',base+'/draft',{'graph':GRAPH},{'If-Match':'"1"'},409)
            return 'HTTP validation, stale revision conflict, workspace isolation, Origin and Host rejection passed.'
        h.record('HTTP contracts and scope boundaries',contracts)
        runs=[]
        def admission():
            with concurrent.futures.ThreadPoolExecutor(max_workers=12) as pool:duplicates=list(pool.map(lambda _:h.admit(w,'event_duplicate'),range(12)))
            assert len({r['runId'] for r in duplicates})==1
            first=duplicates[0];runs.append(first)
            h.request('POST',base+'/runs',{'input':{'different':True}},{'Idempotency-Key':'event_duplicate'},409)
            newer=h.publish(w,2)
            assert h.admit(w,'event_duplicate')['workflowVersionId']==version['id']
            second=h.admit(w,'event_new');runs.append(second);assert second['workflowVersionId']==newer['id']
            h.request('GET',f'/v1/workspaces/workspace_b/runs/{first["runId"]}',status=404)
            assert h.sql('SELECT count(*) FROM runs')=='2'
            return 'Twelve duplicates share one run; changed input conflicts; later publication does not move the original pinned version.'
        h.record('Concurrent admission and publication pinning',admission)
        def restart():
            h.stop('api',crash=True)
            h.pg.stop_server()
            h.pg.start_server()
            h.start('api',[APP,'api','--address',h.api.removeprefix('http://')]);wait_until(lambda:h.request('GET','/health')['status']=='ready','API restart')
            assert h.admit(w,'event_duplicate')['runId']==runs[0]['runId']
            assert all(h.run(r)['dispatchState']=='pending' for r in runs)
            return 'Accepted rows and deduplication survive API death and a PostgreSQL restart before any runtime is started.'
        h.record('Admission survives process/database restart',restart)
        h.server();h.worker()
        def initial_dispatch():
            h.dispatcher();h.dispatcher('dispatcher2')
            for r in runs:h.acknowledged(r)
            assert h.sql('SELECT count(*) FROM runtime_receipts')=='2'
            h.stop('dispatcher');h.stop('dispatcher2')
            return 'Two dispatchers acknowledged two durable receipts; product runs remain accepted, not succeeded.'
        h.record('Concurrent real-runtime dispatch',initial_dispatch)
        def before_start():
            r=h.admit(w,'crash_before');h.probe('before-start',r);assert h.run(r)['runtimeId'] is None
            h.dispatcher();h.acknowledged(r);h.stop('dispatcher')
            return 'SIGKILL after lease acquisition before RPC; another dispatcher reclaimed the lease and recorded one receipt.'
        h.record('Dispatcher death before runtime start',before_start)
        def after_start():
            h.stop('worker');r=h.admit(w,'crash_after');h.probe('after-start',r);assert h.run(r)['runtimeId'] is None
            h.worker();h.dispatcher();h.acknowledged(r);h.stop('dispatcher')
            return 'SIGKILL after Temporal accepted start but before receipt; compatible worker and dispatcher recovered the same stable run.'
        h.record('Dispatcher death after runtime start',after_start)
        def after_receipt():
            r=h.admit(w,'crash_receipt');h.probe('after-receipt',r);assert h.run(r)['dispatchState']=='receipt_recorded'
            h.stop('worker');h.stop('temporal');time.sleep(2.1)
            subprocess.run([str(PROBE),'--mode','reconcile'],env=h.env,cwd=ROOT,check=True,timeout=10)
            h.acknowledged(r)
            assert h.sql(f"SELECT count(*) FROM runtime_receipts WHERE run_id='{r['runId']}'")=='1'
            return 'SIGKILL after durable receipt before acknowledgement; reconciliation succeeded with Temporal offline and a starter that fails if called.'
        h.record('Receipt survives acknowledgement loss',after_receipt)
    finally:h.close()

if __name__=='__main__':main()
