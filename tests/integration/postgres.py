#!/usr/bin/env python3
"""Run a command against an owned, disposable PostgreSQL cluster; retain its files."""
import datetime
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import uuid

ROOT=Path(__file__).resolve().parents[2]

def executable(name):
    folder=os.environ.get('RUNNELIX_PG_BIN')
    value=str(Path(folder)/name) if folder else shutil.which(name)
    if not value or not Path(value).exists():
        raise RuntimeError(f'{name} is missing; set RUNNELIX_PG_BIN to PostgreSQL bin directory')
    return value

class Postgres:
    def __init__(self):
        stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
        self.directory=ROOT/'artifacts/phase1'/f'{stamp}-{uuid.uuid4().hex[:6]}'
        self.directory.mkdir(parents=True)
        with socket.socket() as sock:
            sock.bind(('127.0.0.1',0)); self.port=sock.getsockname()[1]
        self.started=False
        self.dsn=f'postgresql://runnelix_test@127.0.0.1:{self.port}/postgres?sslmode=disable'
    def start(self):
        with (self.directory/'initdb.log').open('w') as log:
            subprocess.run([executable('initdb'),'-D',str(self.directory/'data'),'-U','runnelix_test','--auth=trust','--encoding=UTF8','--no-locale'],stdout=log,stderr=subprocess.STDOUT,check=True)
        # Disable Unix sockets; trust authentication exists only on owned loopback services.
        subprocess.run([executable('pg_ctl'),'-D',str(self.directory/'data'),'-l',str(self.directory/'postgres.log'),'-o',f"-h 127.0.0.1 -p {self.port} -k ''",'-w','start'],check=True,stdout=subprocess.DEVNULL)
        self.started=True
    def close(self):
        if self.started:
            subprocess.run([executable('pg_ctl'),'-D',str(self.directory/'data'),'-m','fast','-w','stop'],check=True,stdout=subprocess.DEVNULL)
            self.started=False
        print('PostgreSQL artifacts:',self.directory.relative_to(ROOT),flush=True)

if __name__=='__main__':
    pg=Postgres()
    code=1
    try:
        pg.start()
        env={**os.environ,'RUNNELIX_TEST_DATABASE_URL':pg.dsn}
        code=subprocess.run(sys.argv[1:] or ['go','test','-race','./internal/store'],cwd=ROOT,env=env).returncode
        (pg.directory/'result.json').write_text(json.dumps({'postgres':subprocess.check_output([executable('postgres'),'--version'],text=True).strip(),'command':sys.argv[1:],'exitCode':code},indent=2)+'\n')
    finally:
        pg.close()
    sys.exit(code)
