#!/usr/bin/env python3
"""Install a checksum-pinned development CLI inside this checkout."""
import hashlib
import pathlib
import platform
import subprocess
import tarfile
import tempfile

VERSION = "1.9.1"
CHECKSUMS = {
    ("darwin", "arm64"): "41e0425378fcb4fb5766340b97435e20fe47bbff2d7bf644ec2d51f7662b7c56",
    ("darwin", "amd64"): "48cdd6c84c56e27ae8e4d9a47052b89289a9c552dee029b49dd312c6fb72873f",
    ("linux", "arm64"): "6c57c352d52fc3df34412376fd9ba6f74b7e3ace8e426e6cba8600156d36a145",
    ("linux", "amd64"): "09a0326a51db84d02735e53542b9ebd8c4758daf47482a9ab0abce15844e60d5",
}
ROOT = pathlib.Path(__file__).resolve().parents[1]

def main():
    system = platform.system().lower()
    arch = {"x86_64": "amd64", "aarch64": "arm64"}.get(platform.machine(), platform.machine())
    expected = CHECKSUMS.get((system, arch))
    if not expected:
        raise SystemExit(f"Unsupported platform: {system}/{arch}")
    destination = ROOT / ".tools" / "temporal" / VERSION
    name = f"temporal_cli_{VERSION}_{system}_{arch}.tar.gz"
    with tempfile.TemporaryDirectory(prefix="runnelix-cli-") as temp:
        archive = pathlib.Path(temp) / name
        subprocess.run(["curl", "--fail", "--silent", "--show-error", "--location",
                        f"https://github.com/temporalio/cli/releases/download/v{VERSION}/{name}",
                        "--output", str(archive)], check=True)
        if hashlib.sha256(archive.read_bytes()).hexdigest() != expected:
            raise SystemExit("Temporal CLI archive checksum mismatch")
        with tarfile.open(archive) as bundle:
            executable = bundle.extractfile("temporal")
            if executable is None:
                raise SystemExit("CLI archive has no temporal executable")
            destination.mkdir(parents=True, exist_ok=True)
            binary = destination / "temporal"
            binary.write_bytes(executable.read())
            binary.chmod(0o755)
    subprocess.run([str(binary), "--version"], check=True)

if __name__ == "__main__":
    main()
