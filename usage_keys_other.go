//go:build !windows

package main

import (
	"bufio"
	"errors"
	"os"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// fdKeys reads VT key sequences from a terminal in raw mode, waiting with poll so a
// wait can end on a timeout without leaving a read behind.
type fdKeys struct {
	fd int
	r  *bufio.Reader
}

func (k *fdKeys) next(timeout time.Duration) (keyEvent, bool) {
	if k.r.Buffered() == 0 {
		fds := []unix.PollFd{{Fd: int32(k.fd), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, int(timeout.Milliseconds()))
		switch {
		case errors.Is(err, unix.EINTR):
			return keyEvent{}, false
		case err != nil:
			return keyEvent{kind: keyEOF}, true
		case n == 0:
			return keyEvent{}, false
		}
	}
	return readKey(k.r), true
}

// openKeys puts the terminal in raw mode and returns the key source and a function that
// puts the terminal back. ok is false when the input is not a terminal that can do this.
func openKeys(file *os.File) (keys keySource, restore func(), ok bool) {
	fd := int(file.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, nil, false
	}
	return &fdKeys{fd: fd, r: bufio.NewReader(file)}, func() { _ = term.Restore(fd, state) }, true
}
