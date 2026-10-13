#!/usr/bin/env python3
"""Replay a plan doc on a fresh clone task by task; diff the result against the scratch tree.

Usage: replay.py PLAN_DOC ORIGIN_REPO SCRATCH_REPO WORK_DIR BRANCH
  ORIGIN_REPO  the real repo (cloned fresh into WORK_DIR, from main)
  SCRATCH_REPO the scratch clone holding the verified commits on BRANCH
Each task's Create/Patch blocks are applied, then gofmt/vet/test must pass and the
task's commit runs. At the end the tree must equal SCRATCH_REPO's BRANCH."""
import re, subprocess, sys, os
DOC, ORIGIN, SCRATCH, WORK, BRANCH = sys.argv[1:6]
def sh(cmd, cwd=WORK, check=True):
    r = subprocess.run(cmd, cwd=cwd, shell=isinstance(cmd, str), capture_output=True, text=True)
    if check and r.returncode:
        sys.exit(f"FAILED {cmd}:\n{r.stdout}{r.stderr}")
    return r.stdout
subprocess.run(["rm", "-rf", WORK]); subprocess.run(["git", "clone", "-q", ORIGIN, WORK], check=True)
sh(["git", "checkout", "-q", "-b", BRANCH])
text = open(DOC).read()
tasks = re.split(r"^### Task \d+: ", text, flags=re.M)[1:]
for i, t in enumerate(tasks, 1):
    lines = t.split("\n")
    j = 0
    while j < len(lines):
        m = re.match(r"^(Create|Patch) `([^`]+)`", lines[j])
        if m:
            j += 2
            assert lines[j].startswith("```"), lines[j]
            k = j + 1
            while lines[k] != "```":
                k += 1
            body = "\n".join(lines[j+1:k]) + "\n"
            if m.group(1) == "Create":
                p = os.path.join(WORK, m.group(2)); os.makedirs(os.path.dirname(p), exist_ok=True)
                open(p, "w").write(body)
            else:
                open(os.path.join(WORK, "..", "replay.patch"), "w").write(body)
                sh(["git", "apply", os.path.join(WORK, "..", "replay.patch")])
            j = k
        j += 1
    assert sh("gofmt -l .").strip() == "", "gofmt"
    sh("go vet ./...")
    sh("go test ./...")
    commit = re.search(r"```bash\n(git add [^\n]+)\ngit commit -m '(.*?)'\n```", t, re.S)
    sh(commit.group(1))
    sh(["git", "commit", "-q", "-m", commit.group(2)])
    print(f"task {i}: ok")
st = sh("git status --porcelain")
assert st == "", "untracked/uncommitted after replay:\n" + st
sh(["git", "fetch", "-q", SCRATCH, BRANCH + ":scratch"])
diff = sh(["git", "diff", "--stat", "scratch", "HEAD"])
print("tree diff vs scratch:", diff or "none")
