#!/usr/bin/env python3
"""Generate or check a source-bundle identity; not a container signing mechanism."""
import argparse
import hashlib
import json
from pathlib import Path
root = Path(__file__).resolve().parents[1]
paths = ['contracts/product/catalog.source.json', 'contracts/product/activity.schema.json',
         'node-worker/product-host.ts', 'node-worker/nodes/mock-echo.ts', 'package.json', 'pnpm-lock.yaml']
hash_state = hashlib.sha256()
for name in sorted(paths):
    content = (root / name).read_bytes()
    hash_state.update(name.encode() + b'\0' + str(len(content)).encode() + b'\0' + content)
catalog = json.loads((root / paths[0]).read_text())
for node in catalog:
    node['implementationDigest'] = 'sha256:' + hash_state.hexdigest()
output = json.dumps(catalog, indent=2) + '\n'
target = root / 'contracts/product/catalog.json'
p = argparse.ArgumentParser(); p.add_argument('--check', action='store_true'); args = p.parse_args()
if args.check:
    if not target.exists() or target.read_text() != output:
        raise SystemExit('Catalog is stale. Run python3 scripts/build-catalog.py')
else:
    target.write_text(output)
