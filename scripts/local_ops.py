#!/usr/bin/env python3
"""Offline backup/restore for the native local launcher; never overwrite state."""
import argparse
from contextlib import contextmanager
import datetime
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shutil
import socket
import sqlite3
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]


def pg(name):
    value = str(Path(os.environ['RUNNELIX_PG_BIN']) / name) if os.environ.get('RUNNELIX_PG_BIN') else shutil.which(name)
    if not value or not Path(value).exists():
        raise RuntimeError(f'{name} unavailable; set RUNNELIX_PG_BIN')
    return value


def command(args, **kwargs):
    # Child errors can contain connection settings or data. Preserve no raw output.
    result = subprocess.run([str(a) for a in args], stdout=subprocess.PIPE, stderr=subprocess.PIPE, **kwargs)
    if result.returncode:
        raise RuntimeError(f'{Path(str(args[0])).name} failed (exit {result.returncode}); retained files were not removed')
    return result.stdout


def free_port():
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        return sock.getsockname()[1]


def digest(path):
    result = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b''):
            result.update(block)
    return result.hexdigest()


@contextmanager
def stopped_state(state):
    if not (state / 'settings.json').is_file() or not (state / 'postgres/PG_VERSION').is_file():
        raise RuntimeError('Not an initialized local launcher state directory')
    with (state / 'owner.lock').open('a') as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise RuntimeError('Stop the local launcher before backup') from None
        status = subprocess.run([pg('pg_ctl'), '-D', str(state / 'postgres'), 'status'], capture_output=True)
        if status.returncode != 3:
            raise RuntimeError('PostgreSQL is running or its state is unknown; stop it before backup')
        # The launcher lock is the supported process ownership boundary. The
        # caller must not run independent processes against this state directory.
        yield


@contextmanager
def database(state, port):
    data = state / 'postgres'
    command([pg('pg_ctl'), '-D', data, '-l', state / 'operations-postgres.log', '-o', f"-h 127.0.0.1 -p {port} -k ''", '-w', 'start'])
    try:
        yield f'postgresql://runnelix_dev@127.0.0.1:{port}/postgres?sslmode=disable'
    finally:
        command([pg('pg_ctl'), '-D', data, '-m', 'fast', '-w', 'stop'])


def backup(state, target, keys):
    state, target, keys = state.resolve(), target.resolve(), keys.resolve()
    if keys.is_relative_to(target) or target.is_relative_to(state) or keys.is_relative_to(state):
        raise RuntimeError('Backup and separate keys must be outside source state; keys must be outside backup')
    if target.exists() or keys.exists():
        raise RuntimeError('Backup directory and keys file must not already exist')
    with stopped_state(state):
        if not (state / 'temporal.sqlite').is_file():
            raise RuntimeError('Temporal database missing; cannot make a complete local backup')
        settings = json.loads((state / 'settings.json').read_text())
        target.mkdir(mode=0o700, parents=True, exist_ok=False)
        keys.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        with keys.open('x') as stream:
            json.dump({k: settings[k] for k in ('credentialKey', 'workerToken')}, stream)
        os.chmod(keys, 0o600)
        with database(state, free_port()) as dsn:
            command([pg('pg_dump'), '--format=custom', '--no-owner', '--no-acl', '--file', target / 'application.dump', dsn])
        with sqlite3.connect(f'file:{state / "temporal.sqlite"}?mode=ro', uri=True) as source:
            with sqlite3.connect(target / 'temporal.sqlite') as destination:
                source.backup(destination)
                if destination.execute('PRAGMA integrity_check').fetchone()[0] != 'ok':
                    raise RuntimeError('Temporal integrity check failed')
        manifest = {
            'formatVersion': 1,
            'createdAt': datetime.datetime.now(datetime.timezone.utc).isoformat(),
            'postgresVersion': command([pg('postgres'), '--version']).decode().strip(),
            'files': {name: digest(target / name) for name in ('application.dump', 'temporal.sqlite')},
            'keysSha256': digest(keys),
            'scope': 'Stopped native local launcher; external OIDC configuration and keys require separate custody',
        }
        (target / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
        return manifest


def restore(backup_dir, keys, target):
    backup_dir, keys, target = backup_dir.resolve(), keys.resolve(), target.resolve()
    if target.exists():
        raise RuntimeError('Restore target must be a new directory; existing data is never replaced')
    manifest = json.loads((backup_dir / 'manifest.json').read_text())
    if manifest.get('formatVersion') != 1 or set(manifest.get('files', {})) != {'application.dump', 'temporal.sqlite'}:
        raise RuntimeError('Unsupported backup manifest')
    for name, checksum in manifest['files'].items():
        if digest(backup_dir / name) != checksum:
            raise RuntimeError('Backup checksum mismatch')
    if digest(keys) != manifest['keysSha256']:
        raise RuntimeError('Separate keys do not match this backup')
    if command([pg('postgres'), '--version']).decode().split()[2].split('.')[0] != manifest['postgresVersion'].split()[2].split('.')[0]:
        raise RuntimeError('Use the same PostgreSQL major version for this local restore')
    settings = json.loads(keys.read_text())
    if set(settings) != {'credentialKey', 'workerToken'} or any(len(settings[k]) != 64 for k in settings):
        raise RuntimeError('Invalid local key file')
    for value in settings.values():
        try:
            bytes.fromhex(value)
        except ValueError:
            raise RuntimeError('Invalid local key encoding') from None
    settings['postgresPort'] = free_port()
    target.mkdir(mode=0o700, parents=True, exist_ok=False)
    command([pg('initdb'), '-D', target / 'postgres', '-U', 'runnelix_dev', '--auth=trust', '--encoding=UTF8', '--no-locale'])
    with database(target, settings['postgresPort']) as dsn:
        command([pg('pg_restore'), '--exit-on-error', '--single-transaction', '--no-owner', '--no-acl', '--dbname', dsn, backup_dir / 'application.dump'])
    shutil.copyfile(backup_dir / 'temporal.sqlite', target / 'temporal.sqlite')
    with sqlite3.connect(f'file:{target / "temporal.sqlite"}?mode=ro', uri=True) as db:
        if db.execute('PRAGMA integrity_check').fetchone()[0] != 'ok':
            raise RuntimeError('Restored Temporal integrity check failed')
    (target / 'settings.json').write_text(json.dumps(settings) + '\n')
    os.chmod(target / 'settings.json', 0o600)
    return target


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='action', required=True)
    save = sub.add_parser('backup')
    save.add_argument('--state', type=Path, default=ROOT / '.local/dev')
    save.add_argument('--output', type=Path, required=True)
    save.add_argument('--keys-out', type=Path, required=True)
    load = sub.add_parser('restore')
    load.add_argument('--backup', type=Path, required=True)
    load.add_argument('--keys', type=Path, required=True)
    load.add_argument('--target', type=Path, required=True)
    args = parser.parse_args()
    if args.action == 'backup':
        backup(args.state, args.output, args.keys_out)
        print('Backup complete. Store the separate key file securely; files are not encrypted by this tool.')
    else:
        restore(args.backup, args.keys, args.target)
        print('Restore created in a new directory. Services remain stopped; reconcile external effects before startup.')


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        # Parser errors aside, never print arbitrary subprocess/database contents.
        print(str(error) if isinstance(error, RuntimeError) else f'Local operation failed ({type(error).__name__}); files retained', file=sys.stderr)
        sys.exit(1)
