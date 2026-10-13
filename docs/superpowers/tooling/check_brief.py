#!/usr/bin/env python3
"""git apply --check every patch block of a task brief against the current tree."""
import re, subprocess, sys, os

brief, repo, tmp = sys.argv[1], sys.argv[2], sys.argv[3]
lines = open(brief).read().split("\n")
i, n = 0, 0
while i < len(lines):
    m = re.match(r"^Patch `([^`]+)`", lines[i])
    if m:
        j = i + 2
        k = j + 1
        while lines[k] != "```":
            k += 1
        path = os.path.join(tmp, f"check-{n}.patch")
        open(path, "w").write("\n".join(lines[j + 1:k]) + "\n")
        r = subprocess.run(["git", "apply", "--check", path], cwd=repo, capture_output=True, text=True)
        print(("ok   " if r.returncode == 0 else "FAIL ") + m.group(1) + (" " + r.stderr.strip() if r.returncode else ""))
        n += 1
        i = k
    i += 1
