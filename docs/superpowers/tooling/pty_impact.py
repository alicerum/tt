"""Plan 10: an old database (made by the previous tt) migrates on open; its
open top-level tasks wait in the Inbox; one is triaged with a due date first
and ⏎; the backlog shows urgency and the marker; the pinned review got impact
1 and a due date. Usage: pty_impact.py OLD_TT NEW_TT DB"""
import os, pty, sys, time, re, select, sqlite3, subprocess, fcntl, termios, struct
OLD, NEW, DB = sys.argv[1], sys.argv[2], sys.argv[3]
env = dict(os.environ, TT_DB=DB, TT_CONFIG="/nonexistent", TERM="xterm-256color")

for line in ["Old top !1 ~s", "Old low !3 ~m", "Untriaged idea"]:
    subprocess.run([OLD, "add", line], env=env, check=True, capture_output=True)
con = sqlite3.connect(DB)
con.execute("""INSERT INTO source_items (kind, external_id, number, title, url, author, repo, requested_at, first_seen_at)
               VALUES ('github', 'acme/api#901', 901, 'Add idempotency key', 'https://github.com/acme/api/pull/901',
                       'ana', 'acme/api', '2026-10-12T08:00:00.000000000Z', '2026-10-12T08:00:00.000000000Z')""")
con.execute("""INSERT INTO tasks (title, link, priority, size, status, source_kind, source_id, created_at, updated_at)
               VALUES ('Review #901: Add idempotency key', 'https://github.com/acme/api/pull/901', 2, 's', 'open',
                       'github', 'acme/api#901', '2026-10-12T09:00:00.000000000Z', '2026-10-12T09:00:00.000000000Z')""")
con.commit()
print("before:", con.execute("select max(version) from schema_migrations").fetchone())
con.close()

pid, fd = pty.fork()
if pid == 0:
    os.execve(NEW, [NEW], env)
fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack('HHHH', 40, 140, 0, 0))
buf = b""
def read(t=0.6):
    global buf
    end = time.time() + t
    while time.time() < end:
        r, _, _ = select.select([fd], [], [], 0.05)
        if r:
            try: buf += os.read(fd, 65536)
            except OSError: return
def strip(b):
    return re.sub(r'\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b[\]P^_][^\x07\x1b]*(\x07|\x1b\\)|\x1b[=>()][0-9A-Za-z]?', '', b.decode('utf-8', 'replace'))
def send(s, t=0.5):
    global buf
    buf = b""
    os.write(fd, s.encode()); read(t)
    return strip(buf)
def expect(out, *parts):
    for p in parts:
        print(("OK   " if p in out else "MISS ") + repr(p))

read(1.5); out = strip(buf)
expect(out, "tt · Today", "inbox 3")
out = send("\x1b", 0.4)             # a new-day banner, if any
out = send("i"); expect(out, "INBOX · 1 of 3", "Old top", "— · ^ to set", "who notices if it slips?")
out = send("\r"); expect(out, "set impact and size first")
out = send("^"); expect(out, "due ›")
out = send("2026-10-30"); out = send("\r"); expect(out, "Fri 30 Oct")
out = send("3", 0.8); out += send("s", 1.5); expect(out, "next card"); print("   screen:", repr(out[-400:]))
out = send("\r"); expect(out, "2", "low"); print("   screen:", repr(out[-300:]))
out = send("\x1b", 0.4)
out = send("L", 0.8); expect(out, "BACKLOG · by urgency", "Old top →30 oct")
out = send("q", 0.8)
os.waitpid(pid, 0)
con = sqlite3.connect(DB)
print("after:", con.execute("select max(version) from schema_migrations").fetchone())
print("tasks:", con.execute("select id, title, priority, impact, due, size from tasks order by id").fetchall())
