#!/usr/bin/env python3
"""Regenerate Claude merge goldens from the real, read-only legacy jq filter.

Usage: python3 scripts/claude-goldens.py /path/to/agent-config/bin/agent-config-sync
Requires jq on PATH. Never executes or sources the legacy synchronization script.
"""
from pathlib import Path
import re
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parent.parent / "internal/adapters/claude/testdata/merge"
LEGACY = Path(sys.argv[1]).read_text()
FILTER = re.search(r"^merge_program='\n(.*?)\n'", LEGACY, re.M | re.S).group(1)
REPO = "/fixture/config repo"
MARKER = "# agent-config-managed"
# Bridge the approved migration without changing numeric literals or key order.
CURRENT = '''walk(if type == "string" and endswith("# kitbag-managed")
    then rtrimstr("# kitbag-managed") + "# agent-config-managed" else . end)'''
FRAGMENT = '''walk(if type == "string" then split("{repo}") | join("@REPO@") else . end)'''
OUTPUT = '''if has("hooks") then .hooks |= map_values(map(
    if (.hooks | type) == "array" then .hooks |= map(
        if (.command? | type) == "string" and (.command | endswith("# agent-config-managed"))
        then .command |= (rtrimstr("# agent-config-managed") + "# kitbag-managed")
        else . end) else . end)) else . end'''


def run(*args):
    return subprocess.run(["jq", *args], capture_output=True, check=False)


def checked(*args):
    result = run(*args)
    if result.returncode:
        raise RuntimeError(result.stderr.decode())
    return result.stdout


# The filter is unchanged. This wrapper emulates its shell's file checks.
with tempfile.TemporaryDirectory(prefix="kitbag-claude-goldens-") as scratch:
    current_path = Path(scratch) / "current.json"
    want_path = Path(scratch) / "want.json"
    for current in sorted(ROOT.glob("*.current.json")):
        stem = current.name.removesuffix(".current.json")
        fragment = ROOT / (stem + ".fragment.json")
        failure = ROOT / (stem + ".error.txt")
        if current.stat().st_size and run("-e", 'type == "object"', str(current)).returncode:
            failure.write_text("current is not a JSON object; left alone\n")
            continue
        current_path.write_bytes(checked(CURRENT, str(current)) if current.stat().st_size else b"{}")
        want_path.write_bytes(checked(FRAGMENT, str(fragment)))
        result = run("-n", "--arg", "m", MARKER, "--arg", "repo", REPO,
                     "--slurpfile", "cur", str(current_path),
                     "--slurpfile", "want", str(want_path), FILTER)
        if result.returncode:
            if "only command hooks are supported" not in result.stderr.decode():
                raise RuntimeError(result.stderr.decode())
            failure.write_text("only command hooks are supported\n")
            continue
        legacy = ROOT / (stem + ".legacy.json")
        legacy.write_bytes(result.stdout)
        (ROOT / (stem + ".want.json")).write_bytes(checked(OUTPUT, str(legacy)))
