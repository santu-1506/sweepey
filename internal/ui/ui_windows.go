//go:build windows

package ui

import (
	"os"
	"syscall"
	"unsafe"
)

// Older Windows consoles need virtual-terminal processing switched on before
// ANSI color codes render instead of printing as garbage.
func init() {
	const enableVirtualTerminalProcessing = 0x0004
	k32 := syscall.NewLazyDLL("kernel32.dll")
	getMode := k32.NewProc("GetConsoleMode")
	setMode := k32.NewProc("SetConsoleMode")
	h := os.Stdout.Fd()
	var mode uint32
	if r, _, _ := getMode.Call(h, uintptr(unsafe.Pointer(&mode))); r == 0 {
		colorOn = false
		return
	}
	if r, _, _ := setMode.Call(h, uintptr(mode|enableVirtualTerminalProcessing)); r == 0 {
		colorOn = false
	}
}
