#!/usr/bin/env python3
"""Built-in adapters through real local workers and an operator-bound fixture server."""
import hashlib
import json
import socket
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from phase1 import APP, ROOT
from phase2 import ExecutionHarness, node, edge
from phase0 import wait_until

class Harness(ExecutionHarness):
    scope='Phase 4 local PostgreSQL, Temporal, Go and TypeScript workers with loopback GitHub/Slack fixtures. No real provider effects or production evidence.'
    def source_hashes(self):
        result=super().source_hashes()
        for path in ['tests/integration/phase4.py']:
            result[path]=hashlib.sha256((ROOT/path).read_bytes()).hexdigest()
        return result

class Provider(BaseHTTPRequestHandler):
    calls={}; failures=[]; token='fixture-token-v1';lock=threading.Lock()
    def log_message(self,*args):pass
    def do_POST(self):
        body=json.loads(self.rfile.read(int(self.headers.get('Content-Length','0'))))
        key=body.get('title',body.get('text','missing'))
        with self.lock:
            self.calls[key]=self.calls.get(key,0)+1
            if self.headers.get('Authorization')!='Bearer '+self.token:self.failures.append('authorization mismatch')
        if self.path=='/repos/fixture-team/example.repo/issues':
            if self.headers.get('X-GitHub-Api-Version')!='2022-11-28':self.failures.append('missing pinned API version')
            result={'id':123,'number':7,'html_url':'https://github.com/fixture-team/example.repo/issues/7'};status=201
        elif self.path=='/api/chat.postMessage':
            if body.get('channel')!='C123ABC':self.failures.append('channel mismatch')
            result={'ok':True,'channel':'C123ABC','ts':'123456.000001'};status=200
        else:self.failures.append('unexpected route');self.send_error(404);return
        if key=='drop-after-effect':
            self.connection.shutdown(socket.SHUT_RDWR);self.connection.close();return
        if key=='slack-reject':result={'ok':False,'error':'fixture-sensitive-detail'}
        if key=='malformed-success':result={'ok':True,'ts':123}
        self.send_response(status);self.send_header('Content-Type','application/json');self.end_headers()
        self.wfile.write(json.dumps(result).encode())

def main():
    h=Harness();provider=ThreadingHTTPServer(('127.0.0.1',0),Provider);provider.daemon_threads=True
    threading.Thread(target=provider.serve_forever,daemon=True).start()
    origin=f'http://127.0.0.1:{provider.server_port}'
    h.env.update(RUNNELIX_EGRESS_HOSTS='api.github.com,slack.com,127.0.0.1',RUNNELIX_DEV_PROVIDER_FIXTURES=json.dumps({'api.github.com':origin,'slack.com':origin}))
    base='/v1/workspaces/workspace_a'
    try:
        h.pg.start();h.command('migrate');h.command('bootstrap','--workspace','workspace_a');h.command('bootstrap','--workspace','workspace_b')
        h.start('api',[APP,'api','--address',h.api.removeprefix('http://')]);wait_until(lambda:h.request('GET','/health')['status']=='ready','API readiness')
        h.start('node-service',[APP,'node-service','--address',h.node_service.removeprefix('http://')]);h.server()
        h.start('engine',[APP,'engine','--temporal-address',h.address]);h.start('nodes',['node','dist/node-worker/product-main.js']);h.dispatcher()
        def action(typ):
            host='api.github.com' if typ=='github.createIssue' else 'slack.com'
            credential=h.request('POST',base+'/credentials',{'name':'Phase 4 fixture','kind':'bearer','destination':host,'value':Provider.token},status=201)
            config={'owner':'fixture-team','repository':'example.repo'} if typ=='github.createIssue' else {'channel':'C123ABC'}
            w=h.workflow([node('action',typ,credentialRef=credential['id'],**config)],[edge('start','action')])
            h.request('PUT',f'{base}/credentials/{credential["id"]}/grants/{w["id"]}');h.publish(w)
            return w,credential
        def output(r):
            ref=next(e['outputRef'] for e in h.events(r) if e.get('nodeId')=='action' and e.get('nodeState')=='succeeded')
            return h.request('GET',f'{base}/runs/{r["runId"]}/payloads/{ref}')
        github,gc=action('github.createIssue');slack,sc=action('slack.postMessage')
        def success():
            r=h.execute(github,'github',{'title':'github-success'});h.finished(r)
            assert output(r)=={'id':123,'number':7,'url':'https://github.com/fixture-team/example.repo/issues/7'}
            r=h.execute(slack,'slack',{'text':'slack-success'});h.finished(r)
            assert output(r)=={'channel':'C123ABC','ts':'123456.000001'}
            assert Provider.calls['github-success']==Provider.calls['slack-success']==1 and not Provider.failures
            return 'Both fixed provider routes executed with destination-scoped bearer injection and bounded outputs.'
        h.record('GitHub and Slack worker execution',success)
        def uncertain():
            for w,field,key,code in [(github,'title','drop-after-effect','ProviderNetwork'),(slack,'text','slack-reject','ProviderRejected'),(slack,'text','malformed-success','InvalidOutput')]:
                r=h.execute(w,key,{field:key});h.finished(r,'needs_attention')
                attempts=h.request('GET',f'{base}/runs/{r["runId"]}/attempts')
                assert len(attempts)==1 and attempts[0]['errorCode']==code
                h.stop('engine',crash=True);h.start('engine',[APP,'engine','--temporal-address',h.address])
                time.sleep(1);assert Provider.calls[key]==1
                invocation=attempts[0]['invocationId']
                h.request('POST',f'{base}/runs/{r["runId"]}/invocations/{invocation}/resolve',{'action':'fail','note':'Fixture operator checked outcome'},status=202)
                h.finished(r,'failed');assert Provider.calls[key]==1
                assert 'fixture-sensitive-detail' not in json.dumps(h.events(r))+json.dumps(attempts)
            return 'Dropped response, Slack HTTP-200 rejection, and malformed success each kept one attempt across engine restart; operator ended each run without resend.'
        h.record('Unsafe outcomes and recovery never resend',uncertain)
        def portable():
            bundle=h.request('GET',f'{base}/workflows/{slack["id"]}/export')
            assert sc['id'] not in json.dumps(bundle) and Provider.token not in json.dumps(bundle)
            bindings={bundle['credentialSlots'][0]['slot']:sc['id']}
            h.request('POST',base+'/workflows/import',{'bundle':bundle,'bindings':{}},status=422)
            h.request('POST','/v1/workspaces/workspace_b/workflows/import',{'bundle':bundle,'bindings':bindings},status=404)
            bad={**bundle,'runtimeState':{}};h.request('POST',base+'/workflows/import',{'bundle':bad,'bindings':bindings},status=400)
            w=h.request('POST',base+'/workflows/import',{'bundle':bundle,'bindings':bindings},status=201)
            assert w['activeVersionId'] is None and w['id']!=slack['id']
            h.request('POST',f'{base}/workflows/{w["id"]}/execute',{'input':{'text':'must-not-run'}},{'Idempotency-Key':'before_publish'},409)
            h.publish(w);r=h.execute(w,'imported',{'text':'imported-success'});h.finished(r);assert output(r)['channel']=='C123ABC'
            return 'Export removed credential IDs; strict import rejected missing/foreign bindings and unknown fields, then a newly published imported draft executed.'
        h.record('Portable workflow API and execution',portable)
        def lifecycle():
            Provider.token='fixture-token-v2'
            h.request('PUT',f'{base}/credentials/{sc["id"]}/value',{'value':Provider.token})
            r=h.execute(slack,'rotated',{'text':'rotation-success'});h.finished(r)
            assert not Provider.failures
            h.request('POST',f'{base}/credentials/{sc["id"]}/revoke')
            r=h.execute(slack,'revoked',{'text':'must-not-send'});h.finished(r,'failed')
            assert 'must-not-send' not in Provider.calls
            h.request('PUT',f'{base}/credentials/{sc["id"]}/value',{'value':'fixture-revive'},status=404)
            return 'Existing publication resolved the rotated token; revocation prevented its next request and could not be undone by rotation.'
        h.record('Rotation and revocation on published workflow',lifecycle)
        assert not Provider.failures
    finally:
        provider.shutdown();provider.server_close();h.close()
if __name__=='__main__':main()
