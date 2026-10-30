#!/usr/bin/env python3
"""Create a versioned local source release from a clean tracked Git tree."""
import hashlib
from pathlib import Path
import subprocess

root = Path(__file__).resolve().parents[1]
if subprocess.check_output(['git', 'status', '--porcelain', '--untracked-files=no'], cwd=root):
    raise SystemExit('Commit tracked changes before packaging')
revision = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=root, text=True).strip()
out = root / 'artifacts/releases'
out.mkdir(parents=True, exist_ok=True)
archive = out / f'runnelix-local-{revision}.tar.gz'
if archive.exists():
    raise SystemExit('Release already exists; existing archive preserved')
subprocess.run(['git', 'archive', '--format=tar.gz', f'--prefix=runnelix-{revision}/', f'--output={archive}', revision], cwd=root, check=True)
checksum = hashlib.sha256(archive.read_bytes()).hexdigest()
archive.with_suffix(archive.suffix + '.sha256').write_text(f'{checksum}  {archive.name}\n')
print(archive.relative_to(root))
