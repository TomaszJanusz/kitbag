#!/usr/bin/env python3
"""Record the HOME that the real agent-config-sync produces, for adoption tests.

Usage: python3 scripts/legacy-home.py /path/to/agent-config/bin/agent-config-sync

For each script in SCRIPTS, copies its seed/ repository to a throwaway HOME
under /tmp/kitbag-legacy-home.* as ~/checkout, puts a copy of the real
(read-only) script at its bin/agent-config-sync, and runs it there with
--no-pull, AGENT_CONFIG_HOST=fixture, an isolated GIT_CONFIG_GLOBAL and no
network. The resulting HOME (outside the clone) replaces the script's
legacyhome/ sections; the HOME path is written as @HOME@ for the testscript
homefixture command. Requires bash, git and jq on PATH.
"""
from pathlib import Path
import os
import re
import shutil
import subprocess
import sys
import tempfile

SCRIPTS = [Path(__file__).resolve().parent.parent / "cmd/kitbag/testdata/script" / name
           for name in ("adopt_legacy.txtar", "adopt_relocated.txtar")]
SECTION = re.compile(r"^-- (.+) --$", re.M)
FIXTURE = "legacyhome"


def sections(text):
    marks = list(SECTION.finditer(text))
    comment = text[: marks[0].start()] if marks else text
    files = []
    for i, mark in enumerate(marks):
        end = marks[i + 1].start() if i + 1 < len(marks) else len(text)
        files.append((mark.group(1), text[mark.end() + 1 : end]))
    return comment, files


def record(home, gitconfig):
    files, links = [], []
    for directory, names, filenames in os.walk(home):
        names[:] = sorted(n for n in names if Path(directory, n) != home / "checkout")
        for name in sorted(filenames) + [n for n in names if Path(directory, n).is_symlink()]:
            path = Path(directory, name)
            relative = path.relative_to(home).as_posix()
            if path.is_symlink():
                links.append(f"{relative} -> {os.readlink(path).replace(str(home), '@HOME@')}")
            else:
                files.append((f"{FIXTURE}/{relative}", path.read_text().replace(str(home), "@HOME@")))
    files.append((f"{FIXTURE}.links", "\n".join(sorted(links)) + "\n"))
    files.append((f"{FIXTURE}.gitconfig", gitconfig.read_text().replace(str(home), "@HOME@")))
    return files


def main():
    legacy = Path(sys.argv[1]).resolve()
    for script in SCRIPTS:
        regenerate(script, legacy)


def regenerate(script, legacy):
    comment, files = sections(script.read_text())
    kept = [(name, body) for name, body in files if not name.startswith(FIXTURE)]
    scratch = Path(tempfile.mkdtemp(prefix="kitbag-legacy-home.", dir="/tmp"))
    try:
        home = scratch / "home"
        clone = home / "checkout"
        for name, body in kept:
            if name.startswith("seed/"):
                target = clone / name.removeprefix("seed/")
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_text(body)
        shutil.copyfile(legacy, clone / "bin/agent-config-sync")
        (clone / "bin/agent-config-sync").chmod(0o755)
        gitconfig = scratch / "gitconfig"
        gitconfig.write_text("")
        env = {"HOME": str(home), "PATH": os.environ["PATH"], "LC_ALL": "C", "AGENT_CONFIG_HOST": "fixture",
               "GIT_CONFIG_GLOBAL": str(gitconfig), "GIT_CONFIG_NOSYSTEM": "1"}
        subprocess.run(["bash", str(clone / "bin/agent-config-sync"), "--no-pull"], env=env, check=True)
        fixture = record(home, gitconfig)
    finally:
        shutil.rmtree(scratch)
    body = comment + "".join(f"-- {name} --\n{text}" for name, text in kept + fixture)
    script.write_text(body)


if __name__ == "__main__":
    main()
