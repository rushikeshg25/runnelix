#!/usr/bin/env python3
"""Inventory compiled Go module license files and installed npm declarations."""
import hashlib
import json
from pathlib import Path
import subprocess

root = Path(__file__).resolve().parents[1]
raw = subprocess.check_output(['go', 'list', '-deps', '-json', './...'], cwd=root, text=True)
decoder = json.JSONDecoder()
modules = {}
while raw.strip():
    item, end = decoder.raw_decode(raw.lstrip())
    raw = raw.lstrip()[end:]
    module = item.get('Module', {})
    if module and not module.get('Main'):
        modules[module['Path']] = module
records = []
for name, module in sorted(modules.items()):
    directory = Path(module['Dir'])
    files = sorted(f for f in directory.iterdir() if f.is_file() and f.name.upper().startswith(('LICENSE', 'COPYING', 'NOTICE')))
    records.append({'module': name, 'version': module['Version'], 'licenseFiles': [
        {'name': f.name, 'sha256': hashlib.sha256(f.read_bytes()).hexdigest()} for f in files]})
licenses = json.loads(subprocess.check_output(['pnpm', 'licenses', 'list', '--json'], cwd=root, text=True))
npm = [{'name': item['name'], 'versions': item['versions'], 'declaredLicense': license}
       for license, items in licenses.items() for item in items]
# Capture the source-file evidence behind missing metadata without guessing.
exceptions = []
for item in licenses.get('Unknown', []):
    directory = Path(item['paths'][0])
    files = sorted(f for f in directory.iterdir() if f.is_file() and f.name.upper().startswith(('LICENSE', 'COPYING')))
    exceptions.append({'name': item['name'], 'licenseFiles': [{'name': f.name, 'sha256': hashlib.sha256(f.read_bytes()).hexdigest()} for f in files]})
print(json.dumps({'scope': 'Compiled Go modules and installed npm packages on this host; inventory is not legal approval or a binary SBOM',
                  'go': records, 'npm': sorted(npm, key=lambda x: x['name']), 'npmMissingMetadata': exceptions}, indent=2))
