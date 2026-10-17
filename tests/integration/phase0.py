#!/usr/bin/env python3
"""Real Go/TypeScript/Temporal boundary checks. Own processes; preserve artifacts."""
import argparse
import datetime
import json
import os
import pathlib
import signal
import socket
import subprocess
import time
import urllib.request
import uuid

ROOT = pathlib.Path(__file__).resolve().parents[2]
CLI = ROOT / ".tools/temporal/1.9.1/temporal"
SPIKE = ROOT / ".local/bin/spike"
PROVIDER = ROOT / ".local/bin/mock-provider"

def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]

def wait_until(check, description, seconds=30):
    deadline = time.monotonic() + seconds
    last = None
    while time.monotonic() < deadline:
        try:
            result = check()
            if result:
                return result
        except (OSError, subprocess.SubprocessError, ValueError) as error:
            last = type(error).__name__
        time.sleep(0.1)
    raise AssertionError(f"Timed out: {description}; last error={last}")

class Harness:
    def __init__(self):
        stamp = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
        self.directory = ROOT / "artifacts/phase0" / f"{stamp}-{uuid.uuid4().hex[:6]}"
        self.directory.mkdir(parents=True)
        self.address = f"127.0.0.1:{free_port()}"
        self.provider_url = f"http://127.0.0.1:{free_port()}"
        self.processes = {}
        self.results = []
        self.counter = 0

    def start(self, name, args, extra_env=None):
        assert name not in self.processes, f"process already running: {name}"
        self.counter += 1
        log = (self.directory / f"{self.counter:02}-{name}.log").open("wb")
        environment = os.environ.copy()
        environment.update(extra_env or {})
        process = subprocess.Popen([str(a) for a in args], cwd=ROOT, env=environment,
                                   stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
        log.close()
        self.processes[name] = process
        return process

    def stop(self, name, crash=False):
        process = self.processes.pop(name, None)
        if process is None:
            return
        if process.poll() is None:
            os.killpg(process.pid, signal.SIGKILL if crash else signal.SIGTERM)
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                os.killpg(process.pid, signal.SIGKILL)
                process.wait(timeout=5)

    def close(self):
        for name in list(reversed(self.processes)):
            self.stop(name)
        report = {"timestampUtc": datetime.datetime.now(datetime.timezone.utc).isoformat(),
                  "scope": "development Temporal SQLite; not production PostgreSQL",
                  "results": self.results}
        (self.directory / "results.json").write_text(json.dumps(report, indent=2) + "\n")
        print(f"Evidence: {self.directory.relative_to(ROOT)}", flush=True)

    def command(self, command, *args, timeout=40):
        completed = subprocess.run([str(SPIKE), command, "--address", self.address, *map(str,args)],
                                   cwd=ROOT, capture_output=True, text=True, timeout=timeout, check=True)
        return json.loads(completed.stdout)

    def stats(self):
        with urllib.request.urlopen(self.provider_url + "/stats", timeout=2) as response:
            return json.load(response)

    def server(self, database=None):
        db = database or self.directory / "temporal.sqlite"
        process = self.start("temporal", [CLI, "server", "start-dev", "--headless", "--ip", "127.0.0.1",
                              "--port", self.address.split(":")[1], "--db-filename", db])
        def healthy():
            if process.poll() is not None:
                raise AssertionError("Temporal exited; inspect its artifact log")
            result = subprocess.run([str(CLI), "operator", "cluster", "health", "--address", self.address],
                                    capture_output=True, timeout=3)
            return result.returncode == 0
        wait_until(healthy, "Temporal readiness", 45)

    def workers(self, version="v1"):
        self.start("go-" + version, [SPIKE, "worker", "--address", self.address, "--version", version])
        self.node_worker(version)

    def node_worker(self, version="v1"):
        self.start("node-" + version, ["node", "dist/node-worker/main.js"], {
            "TEMPORAL_ADDRESS": self.address, "RUNNELIX_PROVIDER_URL": self.provider_url,
            "RUNNELIX_NODE_VERSION": version})

    def provider(self):
        self.start("provider", [PROVIDER, "--address", self.provider_url.removeprefix("http://"),
                                "--state", self.directory / "provider.json"])
        wait_until(lambda: urllib.request.urlopen(self.provider_url + "/health", timeout=2).close() is None,
                   "mock provider readiness")

    def request(self, name, **changes):
        request = json.loads((ROOT / "contracts/spike/fixtures.json").read_text())["base"]
        request.update(runId=name + "-" + uuid.uuid4().hex[:8])
        request.update(changes)
        path = self.directory / (request["runId"] + ".json")
        path.write_text(json.dumps(request))
        self.command("start", "--input", path)
        return request

    def result(self, request):
        return self.command("result", "--id", request["runId"], "--timeout", "35s")

    def record(self, name, function):
        started = time.monotonic()
        try:
            detail = function()
        except Exception:
            self.results.append({"name":name,"status":"failed"})
            raise
        self.results.append({"name":name,"status":"passed","seconds":round(time.monotonic()-started,3),"detail":detail})
        print(f"PASS {name}", flush=True)

def base_cases(h):
    def happy():
        request = h.request("roundtrip")
        result = h.result(request)
        assert result["status"] == "succeeded", result
        assert result["value"] == request["value"], result
        assert result["workerVersion"] == "v1" and result["attempts"] == 1, result
        assert len(h.stats()[request["runId"]]["actions"]) == 1
        return "Unicode, null, nested arrays, and large integer strings survived Go → TS → Go."
    h.record("cross-language roundtrip", happy)

    def transient():
        request = h.request("transient", fault="transient-once")
        result = h.result(request)
        entry = h.stats()[request["runId"]]
        assert result["status"] == "succeeded" and result["attempts"] == 2, result
        assert entry["requests"] == 2 and len(entry["actions"]) == 1, entry
        return "A pre-commit 503 retried once and created one action."
    h.record("safe transient retry", transient)

    def lost_response():
        request = h.request("lost-safe", fault="disconnect")
        result = h.result(request)
        entry = h.stats()[request["runId"]]
        assert result["status"] == "succeeded" and result["attempts"] == 2, result
        assert entry["requests"] == 2 and len(entry["actions"]) == 1, entry
        return "Provider committed before losing the response; retry returned the original action."
    h.record("safe post-commit response loss", lost_response)

    def unsafe():
        request = h.request("lost-unsafe", mode="unsafe", fault="disconnect")
        result = h.result(request)
        entry = h.stats()[request["runId"]]
        assert result["status"] == "needs_attention", result
        assert entry["requests"] == 1 and len(entry["actions"]) == 1, entry
        return "One remote effect, one request, and an explicit uncertain result."
    h.record("unsafe post-commit response loss", unsafe)

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.parse_args()
    for path in [CLI, SPIKE, PROVIDER, ROOT / "dist/node-worker/main.js"]:
        if not path.is_file():
            raise SystemExit(f"Missing prerequisite {path.relative_to(ROOT)}; run make tools and make build")
    h = Harness()
    try:
        h.server()
        h.provider()
        h.workers()
        base_cases(h)
    finally:
        h.close()

if __name__ == "__main__":
    main()
