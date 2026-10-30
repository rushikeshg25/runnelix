#!/usr/bin/env python3
"""Owned fake-data browser login environment; --hold keeps it up for UI checks."""
import json
import os
import signal
import sys
import urllib.request
from phase1 import Harness, APP, ROOT, free_port, wait_until

h=Harness()
try:
    h.pg.start()
    h.command('migrate')
    h.command('bootstrap')
    issuer=f'http://127.0.0.1:{free_port()}'
    browser=f'http://127.0.0.1:{free_port()}'
    h.env.update({
        'RUNNELIX_AUTH_MODE':'oidc',
        'RUNNELIX_OIDC_ISSUER':issuer,
        'RUNNELIX_OIDC_CLIENT_ID':'fixture-client',
        'RUNNELIX_DEV_OIDC_LOOPBACK':'1',
        'RUNNELIX_BROWSER_ORIGIN':browser,
        'RUNNELIX_OIDC_REDIRECT_URL':browser+'/v1/auth/callback',
        'RUNNELIX_API_URL':h.api,
    })
    h.command('provision-owner','--issuer',issuer,'--subject','fixture-owner')
    h.start('issuer',[ROOT/'.local/bin/oidc-fixture','--address',issuer.removeprefix('http://'),'--redirect',browser+'/v1/auth/callback'])
    def ready(url):
        try:
            with urllib.request.urlopen(url,timeout=1) as response:return response.status==200
        except Exception:return False
    wait_until(lambda:ready(issuer+'/.well-known/openid-configuration'),'fixture issuer',20)
    h.start('api',[APP,'api','--address',h.api.removeprefix('http://')])
    wait_until(lambda:ready(h.api+'/health'),'API',20)
    h.start('browser',[ROOT/'.local/bin/pnpm','dev:web','--port',browser.rsplit(':',1)[1]])
    wait_until(lambda:ready(browser+'/v1/auth/session'),'browser proxy',20)
    state={'browser':browser,'api':h.api,'issuer':issuer,'artifacts':str(h.directory.relative_to(ROOT))}
    (h.directory/'browser.json').write_text(json.dumps(state,indent=2)+'\n')
    print(json.dumps(state),flush=True)
    if '--hold' in sys.argv:
        signal.signal(signal.SIGTERM,lambda *_:sys.exit(0))
        signal.pause()
finally:
    for name in list(h.processes):h.stop(name)
    h.pg.close()
