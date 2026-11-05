//go:build windows

package tui

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode = kernel32.NewProc("SetConsoleMode")
)

const (
	enableLineInput      = 0x0002
	enableEchoInput      = 0x0004
	enableProcessedInput = 0x0001
)

// enableRaw puts the Windows console into character-at-a-time, no-echo mode and
// returns a restore closure. A non-console stdin degrades to a no-op.
func enableRaw() (func(), error) {
	handle := syscall.Handle(os.Stdin.Fd())
	var mode uint32
	r, _, err := procGetConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode)))
	if r == 0 {
		return func() {}, err
	}
	next := mode &^ (enableLineInput | enableEchoInput | enableProcessedInput)
	_, _, _ = procSetConsoleMode.Call(uintptr(handle), uintptr(next))
	return func() { _, _, _ = procSetConsoleMode.Call(uintptr(handle), uintptr(mode)) }, nil
}
