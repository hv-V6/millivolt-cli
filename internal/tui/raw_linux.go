//go:build linux

package tui

import (
	"os"
	"syscall"
	"unsafe"
)

// enableRaw uses TCGETS/TCSETS on Linux. A non-terminal stdin degrades to a
// no-op so tests and piped input never break.
func enableRaw() (func(), error) {
	fd := os.Stdin.Fd()
	var termios syscall.Termios
	if _, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, fd, syscall.TCGETS,
		uintptr(unsafe.Pointer(&termios)), 0, 0, 0); errno != 0 {
		return func() {}, nil
	}
	old := termios
	termios.Lflag &^= syscall.ICANON | syscall.ECHO
	termios.Cc[syscall.VMIN] = 1
	termios.Cc[syscall.VTIME] = 0
	_, _, _ = syscall.Syscall6(syscall.SYS_IOCTL, fd, syscall.TCSETS,
		uintptr(unsafe.Pointer(&termios)), 0, 0, 0)
	return func() {
		_, _, _ = syscall.Syscall6(syscall.SYS_IOCTL, fd, syscall.TCSETS,
			uintptr(unsafe.Pointer(&old)), 0, 0, 0)
	}, nil
}
