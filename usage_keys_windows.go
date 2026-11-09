//go:build windows

package main

import (
	"encoding/binary"
	"os"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// consoleKeys reads key events straight from the console input buffer with
// ReadConsoleInput. With virtual-terminal input off, an arrow or PgUp key is one
// event with a virtual-key code, so nothing depends on how the terminal spells it.
// WaitForSingleObject on the input handle is the timed wait: it is signalled while the
// buffer holds an event, and every event is read, so a mouse or focus event never
// leaves it signalled with no key behind it.
type consoleKeys struct {
	handle  windows.Handle
	pending []keyEvent
	high    uint16 // a high surrogate waiting for its low half
}

var procReadConsoleInput = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReadConsoleInputW")

const (
	eventTypeKey = 0x0001

	vkBack, vkTab, vkReturn = 0x08, 0x09, 0x0D
	vkEscape                = 0x1B
	vkPrior, vkNext         = 0x21, 0x22 // PageUp, PageDown
	vkEnd, vkHome           = 0x23, 0x24
	vkLeft, vkUp            = 0x25, 0x26
	vkRight, vkDown         = 0x27, 0x28

	ctrlRightAlt, ctrlLeftAlt   = 0x0001, 0x0002
	ctrlRightCtrl, ctrlLeftCtrl = 0x0004, 0x0008
)

// inputRecord is INPUT_RECORD: a 2-byte type, 2 bytes of padding and a 16-byte union.
type inputRecord struct {
	EventType uint16
	_         uint16
	Event     [16]byte
}

// keyRecord is the KEY_EVENT_RECORD part of the union.
type keyRecord struct {
	Down    bool
	Repeat  uint16
	VK      uint16
	Char    uint16
	Control uint32
}

func (r inputRecord) key() (keyRecord, bool) {
	if r.EventType != eventTypeKey {
		return keyRecord{}, false
	}
	return keyRecord{
		Down:    binary.LittleEndian.Uint32(r.Event[0:4]) != 0,
		Repeat:  binary.LittleEndian.Uint16(r.Event[4:6]),
		VK:      binary.LittleEndian.Uint16(r.Event[6:8]),
		Char:    binary.LittleEndian.Uint16(r.Event[10:12]),
		Control: binary.LittleEndian.Uint32(r.Event[12:16]),
	}, true
}

// translate turns one key event into the key presses the screen understands. A pending
// high surrogate is joined to the low half that follows it.
func (k *consoleKeys) translate(r keyRecord) []keyEvent {
	if !r.Down {
		return nil
	}
	repeat := int(max(r.Repeat, 1))
	one := func(e keyEvent) []keyEvent {
		out := make([]keyEvent, repeat)
		for i := range out {
			out[i] = e
		}
		return out
	}
	switch r.VK {
	case vkUp:
		return one(keyEvent{kind: keyUp})
	case vkDown:
		return one(keyEvent{kind: keyDown})
	case vkLeft:
		return one(keyEvent{kind: keyLeft})
	case vkRight:
		return one(keyEvent{kind: keyRight})
	case vkPrior:
		return one(keyEvent{kind: keyPgUp})
	case vkNext:
		return one(keyEvent{kind: keyPgDn})
	case vkHome:
		return one(keyEvent{kind: keyHome})
	case vkEnd:
		return one(keyEvent{kind: keyEnd})
	}
	ch := r.Char
	if ch == 0 {
		return nil // a modifier, a function key or Delete: no character to give
	}
	altOnly := r.Control&(ctrlLeftAlt|ctrlRightAlt) != 0 && r.Control&(ctrlLeftCtrl|ctrlRightCtrl) == 0 // AltGr counts as Ctrl+Alt
	if altOnly {
		return nil
	}
	switch ch {
	case '\r', '\n':
		return one(keyEvent{kind: keyEnter})
	case '\t':
		return one(keyEvent{kind: keyTab})
	case vkBack, 0x7f:
		return one(keyEvent{kind: keyBackspace})
	case vkEscape:
		return one(keyEvent{kind: keyEsc})
	case 0x03:
		return one(keyEvent{kind: keyCtrlC})
	case 0x04:
		return one(keyEvent{kind: keyCtrlD})
	case 0x15:
		return one(keyEvent{kind: keyCtrlU})
	case 0x0c:
		return one(keyEvent{kind: keyCtrlL})
	}
	if ch < 0x20 {
		return one(keyEvent{kind: keyOther})
	}
	if utf16.IsSurrogate(rune(ch)) {
		if ch < 0xDC00 { // high half: wait for the low one
			k.high = ch
			return nil
		}
		high := k.high
		k.high = 0
		if high == 0 {
			return nil
		}
		return one(keyEvent{kind: keyRune, r: utf16.DecodeRune(rune(high), rune(ch))})
	}
	k.high = 0
	return one(keyEvent{kind: keyRune, r: rune(ch)})
}

func (k *consoleKeys) next(timeout time.Duration) (keyEvent, bool) {
	deadline := time.Now().Add(timeout)
	for {
		if len(k.pending) > 0 {
			key := k.pending[0]
			k.pending = k.pending[1:]
			return key, true
		}
		wait := time.Until(deadline)
		if wait <= 0 {
			return keyEvent{}, false
		}
		ms := uint32(wait.Milliseconds())
		if ms == 0 {
			ms = 1
		}
		event, err := windows.WaitForSingleObject(k.handle, ms)
		if err != nil {
			return keyEvent{kind: keyEOF}, true
		}
		if event == uint32(windows.WAIT_TIMEOUT) {
			return keyEvent{}, false
		}
		var records [64]inputRecord
		var count uint32
		ok, _, _ := procReadConsoleInput.Call(uintptr(k.handle), uintptr(unsafe.Pointer(&records[0])), uintptr(len(records)), uintptr(unsafe.Pointer(&count)))
		if ok == 0 {
			return keyEvent{kind: keyEOF}, true
		}
		for _, rec := range records[:count] {
			if kr, isKey := rec.key(); isKey {
				k.pending = append(k.pending, k.translate(kr)...)
			}
		}
	}
}

// openKeys switches the console input to raw key events and returns the key source and
// a function that puts the console mode back. ok is false when the input is not a
// console.
func openKeys(file *os.File) (keys keySource, restore func(), ok bool) {
	handle := windows.Handle(file.Fd())
	var mode uint32
	if windows.GetConsoleMode(handle, &mode) != nil {
		return nil, nil, false
	}
	raw := mode &^ (windows.ENABLE_ECHO_INPUT | windows.ENABLE_LINE_INPUT | windows.ENABLE_PROCESSED_INPUT |
		windows.ENABLE_VIRTUAL_TERMINAL_INPUT | windows.ENABLE_MOUSE_INPUT | windows.ENABLE_WINDOW_INPUT)
	if windows.SetConsoleMode(handle, raw) != nil {
		return nil, nil, false
	}
	return &consoleKeys{handle: handle}, func() { _ = windows.SetConsoleMode(handle, mode) }, true
}
