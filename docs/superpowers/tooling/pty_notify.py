"""Plan 9 pty check: a fake osascript on PATH logs its arguments.
Run 1 places a block (keys), run 2 starts with [notify] blocks = true and
alerts timed to land a minute or two after launch: the first one while
focused (dropped), the second after a blur (sent). Run 3 checks that with
every kind off no focus reports are asked for.
Usage: pty_notify.py TT WORKDIR"""
import os, pty, sys, time, re, select, sqlite3, fcntl, termios, struct, datetime, math

TT, WORK = sys.argv[1], sys.argv[2]
os.makedirs(WORK, exist_ok=True)
DB, LOG, CFG = os.path.join(WORK, "tt.db"), os.path.join(WORK, "osascript.log"), os.path.join(WORK, "config.toml")
fake = os.path.join(WORK, "bin")
os.makedirs(fake, exist_ok=True)
with open(os.path.join(fake, "osascript"), "w") as f:
    f.write('#!/bin/sh\nfor a in "$@"; do printf "%%s\\n" "$a"; done >> "%s"\necho --- >> "%s"\n' % (LOG, LOG))
os.chmod(os.path.join(fake, "osascript"), 0o755)
if os.path.exists(LOG):
    os.remove(LOG)


def run(config):
    with open(CFG, "w") as f:
        f.write(config)
    env = dict(os.environ, TT_DB=DB, TT_CONFIG=CFG, TERM="xterm-256color", PATH=fake + ":" + os.environ["PATH"])
    pid, fd = pty.fork()
    if pid == 0:
        os.execve(TT, [TT], env)
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 140, 0, 0))
    return pid, fd


raw = b""


def read(fd, t):
    global raw
    end = time.time() + t
    while time.time() < end:
        r, _, _ = select.select([fd], [], [], 0.05)
        if r:
            try:
                raw += os.read(fd, 65536)
            except OSError:
                return


def strip(b):
    return re.sub(r'\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b[\]P^_][^\x07\x1b]*(\x07|\x1b\\)|\x1b[=>()][0-9A-Za-z]?', '', b.decode("utf-8", "replace"))


def send(fd, s, t=0.5):
    global raw
    raw = b""
    os.write(fd, s.encode())
    read(fd, t)
    return strip(raw)


def check(ok, what):
    print(("OK   " if ok else "MISS ") + what)


hours = '[work_hours]\nstart = "04:00"\nend = "23:59"\n[github]\nenabled = false\n'

# Run 1: place a block two rows below the now row.
pid, fd = run(hours)
read(fd, 1.5)
send(fd, "\x1b", 0.4)
send(fd, "L")
send(fd, "a"); send(fd, "Alpha !1 ~s"); out = send(fd, "\r"); check("added #1" in out, "added #1")
out = send(fd, "p"); check("pinned" in out, "pinned")
send(fd, "B")
out = send(fd, "b"); check("placing" in out, "placing")
send(fd, "j"); send(fd, "j")
out = send(fd, "\r"); check("blocked" in out, "blocked")
send(fd, "q", 0.8)
os.waitpid(pid, 0)
day, start_min = sqlite3.connect(DB).execute("select day, start_min from blocks").fetchone()
start = datetime.datetime.strptime(day, "%Y-%m-%d") + datetime.timedelta(minutes=start_min)
print("block at", start)

# Run 2: alerts at the first whole minute at least 15s after launch, and the minute after.
launch = datetime.datetime.now()
first = (launch + datetime.timedelta(seconds=15)).replace(second=0, microsecond=0) + datetime.timedelta(minutes=1)
lead = int((start - first).total_seconds() // 60)
assert 1 <= lead <= 120, lead
pid, fd = run(hours + "[notify]\nblocks = true\nalerts = [%d, %d]\n" % (lead, lead - 1))
raw = b""
read(fd, 1.5)
check(b"\x1b[?1004h" in raw, "focus reports asked for")
send(fd, "\x1b", 0.4)
send(fd, "\x1b[I", 0.3)  # focus in
read(fd, max(0, (first - datetime.datetime.now()).total_seconds() + 3))
check(not os.path.exists(LOG), "nothing sent while focused (%s)" % first.strftime("%H:%M"))
send(fd, "\x1b[O", 0.3)  # focus out
second = first + datetime.timedelta(minutes=1)
read(fd, max(0, (second - datetime.datetime.now()).total_seconds() + 3))
log = open(LOG).read() if os.path.exists(LOG) else ""
print(log)
check(("tt · in %d min\nAlpha\n" % (lead - 1)) in log, "the second alert sent with title and body as arguments")
check(log.count("---") == 1, "exactly one notification")
send(fd, "q", 0.8)
os.waitpid(pid, 0)

# Run 3: every kind off: no focus reports.
pid, fd = run(hours)
raw = b""
read(fd, 1.5)
check(b"\x1b[?1004h" not in raw, "no focus reports with notifications off")
send(fd, "\x1b", 0.4)
send(fd, "q", 0.8)
os.waitpid(pid, 0)
