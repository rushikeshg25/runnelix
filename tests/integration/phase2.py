#!/usr/bin/env python3
"""Live graph execution against local fixtures only; no real provider effects."""
import hashlib
import hmac
import json
import os
from pathlib import Path
import secrets
import threading
import time
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from phase1 import Harness, APP, ROOT, source_hashes
from phase0 import free_port, wait_until

class ExecutionHarness(Harness):
    scope='Local PostgreSQL, Temporal development SQLite, Go engine, TypeScript worker, and loopback provider fixtures. No production deployment or external provider effects.'
    def __init__(self):
        super().__init__()
        self.node_service=f'http://127.0.0.1:{free_port()}'
        self.env.update(RUNNELIX_WORKER_TOKEN=secrets.token_hex(32),RUNNELIX_CREDENTIAL_KEY=secrets.token_hex(32),RUNNELIX_NODE_SERVICE_URL=self.node_service,TEMPORAL_ADDRESS=self.address,RUNNELIX_EGRESS_HOSTS='127.0.0.1',RUNNELIX_DEV_LOOPBACK_EGRESS='1')
    def source_hashes(self):
        result=source_hashes()
        for pattern in ['tests/integration/phase2.py','web/src/*','web/*json','web/vite.config.ts']:
            for p in ROOT.glob(pattern):
                if p.is_file():result[str(p.relative_to(ROOT))]=hashlib.sha256(p.read_bytes()).hexdigest()
        return result
    def workflow(self,nodes,edges,name='Execution fixture'):
        graph={'formatVersion':1,'nodes':[{'id':'start','type':'trigger.manual','version':1,'config':{}},*nodes], 'edges':edges}
        return self.request('POST','/v1/workspaces/workspace_a/workflows',{'name':name,'graph':graph},status=201)
    def execute(self,w,key,input=None):
        return self.request('POST',f'/v1/workspaces/workspace_a/workflows/{w["id"]}/execute',{'input':input or {'message':'fixture'}},{'Idempotency-Key':key},202)
    def finished(self,r,state='succeeded'):
        wait_until(lambda:self.run(r)['state']==state,'run '+state,45)
        return self.run(r)
    def events(self,r,after=0):return self.request('GET',f'/v1/workspaces/workspace_a/runs/{r["runId"]}/events?after={after}')

def node(id,type,**config):return {'id':id,'type':type,'version':1,'config':config}
def edge(source,target,port='out'):return {'from':{'node':source,'port':port},'to':{'node':target,'port':'in'}}

class Provider(BaseHTTPRequestHandler):
    calls={};lock=threading.Lock()
    def log_message(self,*args):pass
    def do_GET(self):self.action()
    def do_POST(self):self.action()
    def action(self):
        with self.lock:self.calls[self.path]=self.calls.get(self.path,0)+1;count=self.calls[self.path]
        if self.path=='/retry' and count==1:self.send_response(429);self.end_headers();return
        if self.path=='/unsafe':self.send_response(500);self.end_headers();return
        if self.path=='/delay':time.sleep(8)
        self.send_response(200);self.send_header('Content-Type','application/json');self.end_headers()
        try:self.wfile.write(json.dumps({'count':count,'authorization':self.headers.get('Authorization',''),'message':'fixture'}).encode())
        except (BrokenPipeError,ConnectionResetError):pass

def main():
    h=ExecutionHarness();provider=ThreadingHTTPServer(('127.0.0.1',0),Provider);provider.daemon_threads=True
    threading.Thread(target=provider.serve_forever,daemon=True).start();url=f'http://127.0.0.1:{provider.server_port}'
    try:
        h.pg.start();h.command('migrate');h.command('bootstrap','--workspace','workspace_a');h.command('bootstrap','--workspace','workspace_b')
        h.start('api',[APP,'api','--address',h.api.removeprefix('http://')]);wait_until(lambda:h.request('GET','/health')['status']=='ready','API readiness')
        h.start('node-service',[APP,'node-service','--address',h.node_service.removeprefix('http://')]);h.server()
        h.start('engine',[APP,'engine','--temporal-address',h.address]);h.start('nodes',['node','dist/node-worker/product-main.js']);h.dispatcher()
        def graph_execution():
            w=h.workflow([node('map','transform.map',expression='{"value": input.priority + 1}'),node('condition','control.condition',expression='input.value > 5'),node('yes','mock.echo'),node('no','mock.echo')],[edge('start','map'),edge('map','condition'),edge('condition','yes','true'),edge('condition','no','false')]);version=h.publish(w)
            r=h.execute(w,'graph',{'priority':5});h.finished(r);events=h.events(r)
            assert any(e.get('nodeId')=='yes' and e.get('nodeState')=='succeeded' for e in events)
            assert any(e.get('nodeId')=='no' and e.get('nodeState')=='skipped' for e in events)
            output=next(e['outputRef'] for e in events if e.get('nodeId')=='yes' and 'outputRef' in e)
            assert h.request('GET',f'/v1/workspaces/workspace_a/runs/{r["runId"]}/payloads/{output}')=={'value':6}
            h.request('GET',f'/v1/workspaces/workspace_b/runs/{r["runId"]}/payloads/{output}',status=404)
            assert h.run(r)['workflowVersionId']==version['id']
            return 'Pinned graph completed across Go and TypeScript; CEL mapping, true/false skip propagation, output references, and workspace isolation passed.'
        h.record('Cross-language graph execution and branches',graph_execution)
        def retry():
            w=h.workflow([node('get','http.get',url=url+'/retry')],[edge('start','get')]);h.publish(w);r=h.execute(w,'retry');h.finished(r)
            assert Provider.calls['/retry']==2
            attempts=h.request('GET',f'/v1/workspaces/workspace_a/runs/{r["runId"]}/attempts');assert [a['status'] for a in attempts]==['failed','succeeded']
            return '429 caused exactly one retry of a read-only node; both attempts remain visible and only the successful attempt is selected.'
        h.record('Read-only provider retry and attempt evidence',retry)
        def unsafe():
            w=h.workflow([node('post','http.post',url=url+'/unsafe')],[edge('start','post')]);h.publish(w);r=h.execute(w,'unsafe');h.finished(r,'needs_attention');time.sleep(1)
            assert Provider.calls['/unsafe']==1
            h.request('POST',f'/v1/workspaces/workspace_a/runs/{r["runId"]}/cancel',status=202);h.finished(r,'cancelled')
            return 'An unsafe POST with a 500 response paused as needs_attention with one call. Durable cancellation ended the paused run.'
        h.record('Unsafe outcome pauses without retry',unsafe)
        def cancellation():
            h.stop('dispatcher');w=h.workflow([node('get','http.get',url=url+'/cancelled')],[edge('start','get')]);h.publish(w);r=h.execute(w,'cancel_before_dispatch')
            h.request('POST',f'/v1/workspaces/workspace_a/runs/{r["runId"]}/cancel',status=202)
            h.dispatcher();h.finished(r,'cancelled');assert '/cancelled' not in Provider.calls
            return 'Cancellation persisted before dispatch and prevented provider calls after dispatcher restart.'
        h.record('Cancellation before execution survives dispatcher restart',cancellation)
        def credentials():
            secret='fixture-bearer-never-real';w=h.workflow([node('get','http.get',url=url+'/credential')],[edge('start','get')])
            c=h.request('POST','/v1/workspaces/workspace_a/credentials',{'name':'Fixture','kind':'bearer','value':secret,'destination':'127.0.0.1'},status=201)
            w['graph']['nodes'][1]['config']['credentialRef']=c['id']
            h.request('PUT',f'/v1/workspaces/workspace_a/workflows/{w["id"]}/draft',{'graph':w['graph']},{'If-Match':'"1"'})
            h.request('POST',f'/v1/workspaces/workspace_a/workflows/{w["id"]}/publish',headers={'If-Match':'"2"'},status=400)
            h.request('PUT',f'/v1/workspaces/workspace_a/credentials/{c["id"]}/grants/{w["id"]}')
            h.publish(w,2);r=h.execute(w,'credential');h.finished(r)
            assert secret not in json.dumps(h.request('GET','/v1/workspaces/workspace_a/credentials'))
            assert secret not in h.sql('SELECT data FROM payloads')
            output=next(e['outputRef'] for e in h.events(r) if e.get('nodeId')=='get' and 'outputRef' in e)
            value=h.request('GET',f'/v1/workspaces/workspace_a/runs/{r["runId"]}/payloads/{output}');assert value['body']['authorization']=='Bearer [redacted]'
            return 'Ungrantable publication was rejected; granted secret was injected only at the network boundary and its provider echo was redacted before payload persistence.'
        h.record('Encrypted credential grant and output redaction',credentials)
        def webhook():
            w=h.workflow([node('echo','mock.echo')],[edge('start','echo')]);h.publish(w);secret='fixture-signing-secret'
            c=h.request('POST','/v1/workspaces/workspace_a/credentials',{'name':'Hook','kind':'webhook-hmac','value':secret,'destination':''},status=201)
            h.request('PUT',f'/v1/workspaces/workspace_a/credentials/{c["id"]}/grants/{w["id"]}')
            path=f'/v1/workspaces/workspace_a/webhooks/{w["id"]}/{c["id"]}';value={'from':'fixture'};raw=json.dumps(value).encode();stamp=str(int(time.time()));key='delivery_one'
            signature=hmac.new(secret.encode(),(stamp+'.'+key+'.').encode()+raw,hashlib.sha256).hexdigest()
            headers={'Idempotency-Key':key,'X-Runnelix-Timestamp':stamp,'X-Runnelix-Signature':signature}
            r=h.request('POST',path,value,headers,202);again=h.request('POST',path,value,headers,202);assert r['runId']==again['runId'];h.finished(r)
            h.request('POST',path,{'tampered':True},headers,401)
            events=h.events(r);assert [e['sequence'] for e in events]==list(range(1,len(events)+1));after=events[-2]['sequence'];assert len(h.events(r,after))==1
            req=urllib.request.Request(h.api+f'/v1/workspaces/workspace_a/runs/{r["runId"]}/events',headers={'Accept':'text/event-stream','Last-Event-ID':str(after)})
            with urllib.request.urlopen(req,timeout=12) as response:assert response.readline().decode().strip()==f'id: {after+1}'
            return 'Signed duplicate delivery returned one run; modified body failed authentication. JSON and SSE event cursors resumed at the next durable sequence.'
        h.record('Signed webhook deduplication and event reconnect',webhook)
        def worker_restart():
            h.stop('nodes');w=h.workflow([node('echo','mock.echo')],[edge('start','echo')]);h.publish(w);r=h.execute(w,'worker_restart')
            wait_until(lambda:any(e.get('nodeState')=='executing' for e in h.events(r)),'queued activity')
            h.stop('engine',crash=True);h.start('engine',[APP,'engine','--temporal-address',h.address]);h.start('nodes',['node','dist/node-worker/product-main.js']);h.finished(r)
            assert len(h.request('GET',f'/v1/workspaces/workspace_a/runs/{r["runId"]}/attempts'))==1
            return 'Engine SIGKILL while an activity awaited its worker recovered the same run from Temporal history, with one recorded node attempt.'
        h.record('Engine restart with queued node activity',worker_restart)
    finally:
        provider.shutdown();provider.server_close();h.close()
if __name__=='__main__':main()
