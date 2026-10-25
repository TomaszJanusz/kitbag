#!/usr/bin/env bash
# Linux /proc measurement; build tools are supplied by mise.
set -euo pipefail
python3 - "${1:-10}" <<'PY'
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time

minutes = float(sys.argv[1])
if not sys.platform.startswith("linux") or not 0 < minutes <= 1440:
    raise SystemExit("usage: mise exec -- bash scripts/daemon-bench.sh <minutes> (Linux)")

with tempfile.TemporaryDirectory(prefix="kitbag-daemon-bench-", dir="/tmp") as directory:
    root = Path(directory)
    home = root / "home"
    home.mkdir()
    executable = root / "kitbag"
    subprocess.run(["go", "build", "-trimpath", "-ldflags=-s -w", "-o", str(executable), "./cmd/kitbag"], check=True)
    environment = {"HOME": str(home), "PATH": os.environ["PATH"],
                   "GIT_CONFIG_GLOBAL": str(home / ".gitconfig"), "GIT_CONFIG_NOSYSTEM": "1",
                   "XDG_RUNTIME_DIR": str(root / "run")}
    # GOMAXPROCS/GOGC stay unset: the daemon applies its own runtime defaults.
    remote, seed = root / "remote.git", root / "seed"
    def git(*args):
        subprocess.run(["git", *map(str, args)], env=environment, check=True, stdout=subprocess.DEVNULL)
    git("init", "--quiet", "--bare", "--initial-branch=main", remote)
    git("clone", "--quiet", remote, seed)
    (seed / "kitbag.toml").write_text('schema=1\n[[files]]\nsrc="source"\ndest="~/output"\n')
    (seed / "source").write_text("idle fixture\n")
    def push(message):
        git("-C", seed, "add", ".")
        git("-C", seed, "-c", "user.name=Benchmark", "-c", "user.email=bench@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", message)
        git("-C", seed, "push", "--quiet", "origin", "HEAD:main")
    push("Seed")
    config = home / ".config/kitbag/config.toml"
    config.parent.mkdir(parents=True)
    config.write_text(f'schema=1\n[repo]\nurl="{remote}"\npath="~/repo"\n[git]\nbackend="system"\n[daemon]\nupdate_check=false\n')
    with (root / "stderr").open("w+") as errors:
        process = subprocess.Popen([str(executable), "daemon"], env=environment, stdout=subprocess.DEVNULL, stderr=errors)
        try:
            deadline = time.monotonic() + 15
            while not (home / "output").exists():
                if process.poll() is not None or time.monotonic() > deadline:
                    errors.seek(0)
                    raise RuntimeError("daemon failed to start: " + errors.read())
                time.sleep(0.1)
            time.sleep(2)
            def usage():
                stat = Path(f"/proc/{process.pid}/stat").read_text().split()
                fields = dict(line.split(":", 1) for line in Path(f"/proc/{process.pid}/status").read_text().splitlines())
                memory = {key: int(fields[key].split()[0]) for key in ("VmRSS", "RssAnon", "RssFile")}
                return memory, int(stat[13]) + int(stat[14]), sum(int(stat[i]) for i in (13, 14, 15, 16))
            first, cpu_start, total_start = usage()
            start = time.monotonic()
            samples = []
            # One upstream change mid-run proves polling still applies it.
            push_at, pushed = min(120, minutes * 30), False
            while time.monotonic() - start < minutes * 60:
                if process.poll() is not None:
                    raise RuntimeError("daemon exited during measurement")
                if not pushed and time.monotonic() - start >= push_at:
                    (seed / "source").write_text("upstream change\n")
                    push("Upstream change")
                    pushed = True
                memory, cpu, total = usage()
                samples.append(memory["VmRSS"])
                time.sleep(min(1, max(0, minutes * 60 - (time.monotonic() - start))))
            memory, cpu, total = usage()
            samples.append(memory["VmRSS"])
            elapsed = time.monotonic() - start
            ticks = os.sysconf("SC_CLK_TCK")
            print(json.dumps({"elapsed_seconds": round(elapsed, 3), "samples": len(samples),
                              "start_rss_kib": first["VmRSS"], "peak_rss_kib": max(samples), "end_rss_kib": memory["VmRSS"],
                              "mean_rss_kib": round(sum(samples) / len(samples), 1),
                              "end_rss_anon_kib": memory["RssAnon"], "end_rss_file_kib": memory["RssFile"],
                              "upstream_applied": (home / "output").read_text() == "upstream change\n",
                              "daemon_cpu_seconds": round((cpu - cpu_start) / ticks, 3),
                              "cpu_including_git_children_seconds": round((total - total_start) / ticks, 3),
                              "poll_interval": "60s", "update_check": False}, indent=2), flush=True)
        finally:
            if process.poll() is None:
                process.send_signal(signal.SIGTERM)
            try:
                process.wait(timeout=15)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
                raise RuntimeError("daemon did not shut down within 15 seconds")
            if process.returncode != 0:
                errors.seek(0)
                raise RuntimeError(f"daemon exit {process.returncode}: " + errors.read())
PY
