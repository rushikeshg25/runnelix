#!/usr/bin/env python3
"""Owned local development services. Keep data and keys across shutdowns."""
import fcntl
import json
import os
from pathlib import Path
import secrets
import shutil
import signal
import socket
import subprocess
import time
import urllib.request
ROOT=Path(__file__).resolve().parents[1]
STATE=ROOT/'.local/dev';APP=ROOT/'.local/bin/runnelix';CLI=ROOT/'.tools/temporal/1.9.1/temporal'

def pg(name):
    folder=os.environ.get('RUNNELIX_PG_BIN');result=str(Path(folder)/name) if folder else shutil.which(name)
    if not result or not Path(result).exists():raise RuntimeError(f'{name} missing; set RUNNELIX_PG_BIN')
    return result

def wait(check,processes,label):
    deadline=time.monotonic()+45
    while time.monotonic()<deadline:
        for name,p in processes:
            if p.poll() is not None:raise RuntimeError(f'{name} exited; inspect .local/dev/{name}.log')
        try:
            if check():return
        except (OSError,ValueError,subprocess.SubprocessError):pass
        time.sleep(.2)
    raise RuntimeError(f'{label} did not become ready')

def main():
    os.umask(0o077);STATE.mkdir(parents=True,exist_ok=True);os.chmod(STATE,0o700)
    lock=(STATE/'owner.lock').open('w')
    try:fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
    except BlockingIOError:raise RuntimeError('A local development launcher already owns this directory')
    for binary in [APP,CLI]:
        if not binary.exists():raise RuntimeError('Run make deps tools build-phase2 first')
    for name in ['initdb','pg_ctl']:pg(name)
    processes=[];started_pg=False;stopping=False
    def stop_signal(*_):
        nonlocal stopping;stopping=True
    signal.signal(signal.SIGINT,stop_signal);signal.signal(signal.SIGTERM,stop_signal)
    settings_path=STATE/'settings.json'
    if settings_path.exists():settings=json.loads(settings_path.read_text())
    else:
        with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
        settings={'postgresPort':port,'workerToken':secrets.token_hex(32),'credentialKey':secrets.token_hex(32)}
        settings_path.write_text(json.dumps(settings)+'\n');os.chmod(settings_path,0o600)
    for port in [5173,7233,8787,8788,settings['postgresPort']]:
        with socket.socket() as sock:
            try:sock.bind(('127.0.0.1',port))
            except OSError:raise RuntimeError(f'Loopback port {port} is occupied; existing service was left untouched')
    env={**os.environ,'RUNNELIX_DATABASE_URL':f'postgresql://runnelix_dev@127.0.0.1:{settings["postgresPort"]}/postgres?sslmode=disable',
         'RUNNELIX_WORKER_TOKEN':settings['workerToken'],'RUNNELIX_CREDENTIAL_KEY':settings['credentialKey'],'RUNNELIX_NODE_SERVICE_URL':'http://127.0.0.1:8788','RUNNELIX_BROWSER_ORIGIN':'http://127.0.0.1:5173','RUNNELIX_API_URL':'http://127.0.0.1:8787','TEMPORAL_ADDRESS':'127.0.0.1:7233'}
    def start(name,args,worker=False):
        child_env=env.copy()
        if worker:
            child_env.pop('RUNNELIX_DATABASE_URL',None);child_env.pop('RUNNELIX_CREDENTIAL_KEY',None)
        with (STATE/f'{name}.log').open('ab') as log:
            p=subprocess.Popen([str(a) for a in args],cwd=ROOT,env=child_env,stdout=log,stderr=subprocess.STDOUT,start_new_session=True)
        processes.append((name,p));return p
    try:
        data=STATE/'postgres'
        if not (data/'PG_VERSION').exists():
            with (STATE/'initdb.log').open('ab') as log:subprocess.run([pg('initdb'),'-D',str(data),'-U','runnelix_dev','--auth=trust','--encoding=UTF8','--no-locale'],stdout=log,stderr=subprocess.STDOUT,check=True)
        subprocess.run([pg('pg_ctl'),'-D',str(data),'-l',str(STATE/'postgres.log'),'-o',f"-h 127.0.0.1 -p {settings['postgresPort']} -k ''",'-w','start'],check=True,stdout=subprocess.DEVNULL);started_pg=True
        for command in ['migrate','bootstrap']:subprocess.run([str(APP),command],cwd=ROOT,env=env,check=True,stdout=subprocess.DEVNULL)
        start('temporal',[CLI,'server','start-dev','--headless','--ip','127.0.0.1','--port','7233','--db-filename',STATE/'temporal.sqlite'])
        wait(lambda:subprocess.run([str(CLI),'operator','cluster','health','--address','127.0.0.1:7233'],capture_output=True,timeout=3).returncode==0,processes,'Temporal')
        for name,args in [('api',['api']),('node-service',['node-service','--address','127.0.0.1:8788']),('engine',['engine']),('admission',['worker']),('dispatcher',['dispatcher'])]:start(name,[APP,*args])
        start('nodes',['node','dist/node-worker/product-main.js'],worker=True);start('editor',['pnpm','dev:web'],worker=True)
        wait(lambda:urllib.request.urlopen('http://127.0.0.1:5173',timeout=2).status==200,processes,'Editor')
        print('Runnelix ready: http://127.0.0.1:5173\nLocal fixture-data mode. Ctrl+C stops owned services and preserves .local/dev.',flush=True)
        while not stopping:
            for name,p in processes:
                if p.poll() is not None:raise RuntimeError(f'{name} exited; inspect its local log')
            time.sleep(.5)
    finally:
        for name,p in reversed(processes):
            if p.poll() is None:
                os.killpg(p.pid,signal.SIGTERM)
                try:p.wait(timeout=8)
                except subprocess.TimeoutExpired:os.killpg(p.pid,signal.SIGKILL);p.wait(timeout=5)
        if started_pg:subprocess.run([pg('pg_ctl'),'-D',str(STATE/'postgres'),'-m','fast','-w','stop'],check=True,stdout=subprocess.DEVNULL)
        print('Owned services stopped. Local data and keys preserved.',flush=True)
if __name__=='__main__':
    try:main()
    except Exception as error:raise SystemExit(str(error))
