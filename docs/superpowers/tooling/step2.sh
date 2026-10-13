#!/bin/sh
# For each scratch commit (main..HEAD): parent tree + that commit's test files →
# go test output, i.e. each task's real "Step 2: expect failure" messages.
# Usage: step2.sh SCRATCH_DIR REPO_SUBDIR   (e.g. step2.sh "$SCRATCHPAD" p5b)
S=$1
cd "$S/$2"
for c in $(git rev-list --reverse main..HEAD); do
  rm -rf $S/s2; git worktree add -q --detach $S/s2 $c^ 2>/dev/null
  for f in $(git diff --name-only $c^ $c | grep '_test.go$'); do
    mkdir -p $S/s2/$(dirname $f); git show $c:$f > $S/s2/$f
  done
  echo "=== $(git log -1 --format=%s $c)"
  (cd $S/s2 && go test ./... 2>&1 | grep -v '^ok\|no test files' | head -12)
  git worktree remove --force $S/s2
done
