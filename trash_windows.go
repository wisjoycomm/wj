//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// shFileOp is SHFILEOPSTRUCTW as 64-bit Windows lays it out (wj builds for
// amd64 and arm64 only; 32-bit Windows packs this struct differently).
type shFileOp struct {
	hwnd         uintptr
	function     uint32
	from         *uint16
	to           *uint16
	flags        uint16
	aborted      int32
	nameMappings uintptr
	title        *uint16
}

const (
	foDelete          = 3
	fofSilent         = 0x0004
	fofNoConfirmation = 0x0010
	fofAllowUndo      = 0x0040 // to the Recycle Bin instead of gone
	fofNoErrorUI      = 0x0400
)

var shFileOperation = windows.NewLazySystemDLL("shell32.dll").NewProc("SHFileOperationW")

// systemTrash moves files and folders to the Recycle Bin, where File Explorer
// can restore them to where they were.
func systemTrash(paths []string) error {
	// The shell wants absolute paths, each ended by a NUL, and one more NUL at the end.
	const batch = 200
	for start := 0; start < len(paths); start += batch {
		end := min(start+batch, len(paths))
		var list []uint16
		for _, path := range paths[start:end] {
			abs, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			encoded, err := windows.UTF16FromString(strings.TrimPrefix(abs, `\\?\`))
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			list = append(list, encoded...)
		}
		list = append(list, 0)
		op := shFileOp{function: foDelete, from: &list[0], flags: fofAllowUndo | fofNoConfirmation | fofSilent | fofNoErrorUI}
		code, _, _ := shFileOperation.Call(uintptr(unsafe.Pointer(&op)))
		if code != 0 || op.aborted != 0 {
			return fmt.Errorf("Windows không chuyển được vào Thùng rác (mã %#x); không có gì bị xoá vĩnh viễn", code)
		}
	}
	return nil
}
