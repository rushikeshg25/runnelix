import contextlib
import importlib.util
import os
from pathlib import Path
import signal
import sqlite3
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('local_ops', ROOT / 'scripts/local_ops.py')
ops = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ops)


class LocalOperationsTests(unittest.TestCase):
    def test_sigterm_stops_owned_database(self):
        with tempfile.TemporaryDirectory() as folder:
            state = Path(folder)
            script = '''
import sys,time
from pathlib import Path
sys.path.insert(0,sys.argv[1])
import local_ops as ops
state=Path(sys.argv[2])
ops.pg=lambda name:name
ops.subprocess.run=lambda *a,**kw:type('Result',(),{'returncode':0})()
def command(args,**kw):
 with (state/'calls').open('a') as f:f.write(args[-1]+'\\n')
ops.command=command
def backup(*args):
 with ops.database(state,1234):
  (state/'ready').touch()
  time.sleep(60)
ops.backup=backup
sys.argv=['local_ops','backup','--output',str(state/'out'),'--keys-out',str(state/'keys')]
ops.main()
'''
            child = subprocess.Popen([sys.executable, '-c', script, str(ROOT / 'scripts'), str(state)], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            try:
                deadline = time.monotonic() + 5
                while not (state / 'ready').exists() and child.poll() is None and time.monotonic() < deadline:
                    time.sleep(.02)
                self.assertTrue((state / 'ready').exists())
                child.send_signal(signal.SIGTERM)
                child.wait(timeout=5)
                self.assertNotEqual(child.returncode, 0)
                self.assertEqual((state / 'calls').read_text().splitlines(), ['start', 'stop'])
            finally:
                if child.poll() is None:
                    child.kill()
                    child.wait()

    def test_failed_start_stops_server_if_it_did_start(self):
        calls = []
        def command(args, **kwargs):
            calls.append(args[-1])
            if args[-1] == 'start':
                raise RuntimeError('startup wait failed')
        with patch.object(ops, 'pg', side_effect=lambda name: name), patch.object(ops, 'command', side_effect=command), patch.object(ops.subprocess, 'run', return_value=subprocess.CompletedProcess([], 0)):
            with self.assertRaisesRegex(RuntimeError, 'startup wait failed'):
                with ops.database(Path('/unused-fixture'), 1234):
                    self.fail('failed startup yielded')
        self.assertEqual(calls, ['start', 'stop'])

    def test_sqlite_paths_are_not_uri_syntax(self):
        with tempfile.TemporaryDirectory(prefix='runnelix-') as folder:
            root = Path(folder)
            state = root / 'state#?%'
            state.mkdir()
            (state / 'settings.json').write_text('{"credentialKey":"' + 'a'*64 + '", "workerToken":"' + 'b'*64 + '"}')
            with sqlite3.connect(state / 'temporal.sqlite') as db:
                db.execute('CREATE TABLE marker(value text)')
                db.execute("INSERT INTO marker VALUES('preserved')")
            @contextlib.contextmanager
            def fake_database(*args):
                yield 'fixture'
            def command(args, **kwargs):
                if '--file' in args:
                    Path(args[args.index('--file') + 1]).write_bytes(b'fixture dump')
                return b'postgres (PostgreSQL) 17.0'
            with patch.object(ops, 'stopped_state', side_effect=lambda _: contextlib.nullcontext()), patch.object(ops, 'database', side_effect=fake_database), patch.object(ops, 'pg', side_effect=lambda name: name), patch.object(ops, 'command', side_effect=command):
                ops.backup(state, root / 'backup', root / 'keys.json')
                ops.restore(root / 'backup', root / 'keys.json', root / 'restored#?%')
            self.assertFalse((root / 'state').exists())
            self.assertFalse((root / 'restored').exists())
            with sqlite3.connect(root / 'backup/temporal.sqlite') as db:
                self.assertEqual(db.execute('SELECT value FROM marker').fetchone(), ('preserved',))
