import os, pty, sys, time, re, select, sqlite3
TT, DB = sys.argv[1], sys.argv[2]
env = dict(os.environ, TT_DB=DB, TT_CONFIG="/nonexistent", TERM="xterm-256color")
pid, fd = pty.fork()
if pid == 0:
    os.execve(TT, [TT], env)
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
import fcntl, termios, struct
fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack('HHHH', 40, 140, 0, 0))
read(1.5); out = strip(buf)
expect(out, "tt · Today", "TIMELINE", "CANDIDATES", "outside of working hours")
out = send("\x1b", 0.4)          # dismiss the new-day banner
out = send("L"); expect(out, "Log")
out = send("a"); out = send("Alpha !1 ~s"); out = send("\r"); expect(out, "added #1")
out = send("p"); expect(out, "pinned")
out = send("B"); expect(out, "Today")
out = send("b"); expect(out, "placing")
out = send("\r"); expect(out, "blocked")
out = send("c"); expect(out, "committed")
out = send("q", 0.8)
os.waitpid(pid, 0)
con = sqlite3.connect(DB)
print("blocks:", con.execute("select task_id, day, start_min, duration_min from blocks").fetchall())
print("committed:", con.execute("select committed_at is not null from days").fetchall())
