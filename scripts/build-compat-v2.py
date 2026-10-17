#!/usr/bin/env python3
"""Build the pushed Phase 2 fixture in an isolated ignored directory."""
from pathlib import Path
import io
import subprocess
import tarfile
ROOT=Path(__file__).resolve().parents[1]
COMMIT='9cd61d1';target=ROOT/'.local/compat-v2'
if not target.exists():
    target.mkdir(parents=True)
    archive=subprocess.check_output(['git','archive',COMMIT],cwd=ROOT)
    with tarfile.open(fileobj=io.BytesIO(archive)) as tar:tar.extractall(target,filter='data')
    (target/'fixture-commit').write_text(COMMIT+'\n')
if (target/'fixture-commit').read_text().strip()!=COMMIT:raise SystemExit('Compatibility fixture identity mismatch; preserve directory and inspect it')
if not (target/'node_modules').exists():(target/'node_modules').symlink_to(ROOT/'node_modules',target_is_directory=True)
subprocess.run(['go','build','-o',str(ROOT/'.local/bin/runnelix-v2'),'./cmd/runnelix'],cwd=target,check=True)
subprocess.run(['node',str(ROOT/'node_modules/typescript/bin/tsc'),'-p',str(target/'tsconfig.json')],cwd=target,check=True)
print('Built isolated Phase 2 compatibility fixture at '+COMMIT)
