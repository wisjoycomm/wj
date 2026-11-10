package main

import (
	"bufio"
	"time"
)

// keySource gives the interactive `wj usage` its key presses, one at a time, with a
// wait that has a time limit. The limit is what lets the screen redraw by itself while
// nobody is typing. There is no reader thread behind it: a thread left blocked on the
// console would swallow the first key typed after the screen is closed.
type keySource interface {
	// next waits up to timeout for a key press. ok is false when the time ran out.
	next(timeout time.Duration) (key keyEvent, ok bool)
}

// readerKeys reads keys from a stream that never times out. Tests and pipes use it.
type readerKeys struct{ r *bufio.Reader }

func (k readerKeys) next(time.Duration) (keyEvent, bool) { return readKey(k.r), true }
