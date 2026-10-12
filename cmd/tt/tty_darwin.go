package main

import (
	"os"
	"syscall"
	"time"
	"unsafe"
)

// fionread is FIONREAD (bytes waiting to be read); package syscall omits it on darwin.
const fionread = 0x4004667f

// ttyPending drains what a paste left on f's input queue (D33): it waits
// for the paste to settle, returns the complete lines, and flushes the rest
// (a partial line included) so the shell never runs it. A failed ioctl or
// read degrades to "nothing pending"; the flush is still attempted.
func ttyPending(f *os.File) func() (string, error) {
	fd := int(f.Fd())
	return func() (string, error) {
		defer func() {
			flushRead := 1 // FREAD
			syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.TIOCFLUSH, uintptr(unsafe.Pointer(&flushRead)))
		}()
		waiting := func() int {
			var n int32
			if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), fionread, uintptr(unsafe.Pointer(&n))); e != 0 {
				return -1
			}
			return int(n)
		}
		last := -1
		for range 10 { // settle: until the count stops growing
			n := waiting()
			if n < 0 || n == last {
				break
			}
			last = n
			time.Sleep(20 * time.Millisecond)
		}
		if last <= 0 {
			return "", nil
		}
		// A partial line may not be readable in canonical mode; don't block on it.
		if err := syscall.SetNonblock(fd, true); err != nil {
			return "", nil
		}
		defer syscall.SetNonblock(fd, false)
		var data []byte
		buf := make([]byte, 4096)
		for {
			n, err := syscall.Read(fd, buf)
			if n <= 0 || err != nil {
				break
			}
			data = append(data, buf[:n]...)
		}
		complete := 0
		for i, b := range data {
			if b == '\n' {
				complete = i + 1
			}
		}
		return string(data[:complete]), nil
	}
}
