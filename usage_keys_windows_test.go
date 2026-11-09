//go:build windows

package main

import (
	"encoding/binary"
	"testing"
)

func down(vk uint16, char uint16, control uint32, repeat uint16) keyRecord {
	return keyRecord{Down: true, Repeat: repeat, VK: vk, Char: char, Control: control}
}

func kinds(events []keyEvent) []keyKind {
	out := make([]keyKind, len(events))
	for i, e := range events {
		out[i] = e.kind
	}
	return out
}

func TestConsoleKeysTranslateNavigationKeys(t *testing.T) {
	k := &consoleKeys{}
	cases := map[uint16]keyKind{vkUp: keyUp, vkDown: keyDown, vkLeft: keyLeft, vkRight: keyRight, vkPrior: keyPgUp, vkNext: keyPgDn, vkHome: keyHome, vkEnd: keyEnd}
	for vk, want := range cases {
		got := k.translate(down(vk, 0, 0, 1))
		if len(got) != 1 || got[0].kind != want {
			t.Errorf("vk %#x gives %v, want %v", vk, kinds(got), want)
		}
	}
}

func TestConsoleKeysTranslateTextAndControlKeys(t *testing.T) {
	k := &consoleKeys{}
	check := func(name string, r keyRecord, want ...keyKind) {
		t.Helper()
		got := kinds(k.translate(r))
		if len(got) != len(want) {
			t.Errorf("%s gives %v, want %v", name, got, want)
			return
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s gives %v, want %v", name, got, want)
			}
		}
	}
	check("a", down('A', 'a', 0, 1), keyRune)
	check("shift+A", down('A', 'A', 0x10, 1), keyRune)
	check("enter", down(vkReturn, '\r', 0, 1), keyEnter)
	check("tab", down(vkTab, '\t', 0, 1), keyTab)
	check("backspace", down(vkBack, 0x08, 0, 1), keyBackspace)
	check("escape", down(vkEscape, 0x1b, 0, 1), keyEsc)
	check("ctrl+c", down('C', 0x03, ctrlLeftCtrl, 1), keyCtrlC)
	check("ctrl+u", down('U', 0x15, ctrlLeftCtrl, 1), keyCtrlU)
	check("ctrl+d", down('D', 0x04, ctrlLeftCtrl, 1), keyCtrlD)
	check("ctrl+x is something else", down('X', 0x18, ctrlLeftCtrl, 1), keyOther)
	check("shift alone", down(0x10, 0, 0x10, 1))
	check("f5 has no character", down(0x74, 0, 0, 1))
	check("alt+a is not typed", down('A', 'a', ctrlLeftAlt, 1))
	check("altgr (ctrl+alt) still types", down('Q', '@', ctrlLeftCtrl|ctrlRightAlt, 1), keyRune)
	check("key up is ignored", keyRecord{Down: false, VK: vkUp})
	if got := k.translate(down('é', 'é', 0, 1)); len(got) != 1 || got[0].r != 'é' {
		t.Errorf("accented letter = %+v", got)
	}
}

func TestConsoleKeysRepeatCountBecomesRepeatedKeys(t *testing.T) {
	k := &consoleKeys{}
	got := k.translate(down(vkDown, 0, 0, 3))
	if len(got) != 3 || got[2].kind != keyDown {
		t.Errorf("held Down key = %v", kinds(got))
	}
	if got := k.translate(down('A', 'a', 0, 0)); len(got) != 1 {
		t.Errorf("a repeat count of 0 still counts once: %v", kinds(got))
	}
}

func TestConsoleKeysJoinSurrogatePairs(t *testing.T) {
	k := &consoleKeys{}
	// U+1F600 is the pair D83D DE00.
	if got := k.translate(down(0, 0xD83D, 0, 1)); len(got) != 0 {
		t.Fatalf("the high half waits for its low half: %v", got)
	}
	got := k.translate(down(0, 0xDE00, 0, 1))
	if len(got) != 1 || got[0].r != 0x1F600 {
		t.Errorf("pair = %+v", got)
	}
	if got := k.translate(down(0, 0xDE00, 0, 1)); len(got) != 0 {
		t.Errorf("a lone low half is dropped: %v", got)
	}
}

func TestInputRecordKeyParsing(t *testing.T) {
	var r inputRecord
	r.EventType = eventTypeKey
	binary.LittleEndian.PutUint32(r.Event[0:4], 1)      // key down
	binary.LittleEndian.PutUint16(r.Event[4:6], 2)      // repeat count
	binary.LittleEndian.PutUint16(r.Event[6:8], vkDown) // virtual key
	binary.LittleEndian.PutUint16(r.Event[8:10], 0x50)  // scan code (unused)
	binary.LittleEndian.PutUint16(r.Event[10:12], 'x')  // character
	binary.LittleEndian.PutUint32(r.Event[12:16], ctrlLeftCtrl)
	kr, ok := r.key()
	if !ok || !kr.Down || kr.Repeat != 2 || kr.VK != vkDown || kr.Char != 'x' || kr.Control != ctrlLeftCtrl {
		t.Errorf("key record = %+v, %v", kr, ok)
	}
	r.EventType = 0x0002 // a mouse event
	if _, ok := r.key(); ok {
		t.Error("only key events are keys")
	}
}

func TestOpenKeysRefusesWhatIsNotAConsole(t *testing.T) {
	if _, _, ok := openKeys(nil); ok {
		// A nil file has no handle; asking must not panic or succeed.
		t.Error("a nil file is not a console")
	}
}
