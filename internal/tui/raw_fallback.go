//go:build !windows && !linux

package tui

// enableRaw is a no-op on platforms without a portable stdlib termios path.
// The TUI still works; keys are delivered after Enter instead of immediately.
func enableRaw() (func(), error) { return func() {}, nil }
