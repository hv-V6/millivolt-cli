package tui

import (
	"os"
	"strconv"
)

// terminalSize prefers the shell's COLUMNS/LINES, falling back to a sane frame.
// RenderFrame clips every line to the reported width, so an incorrect guess
// never produces a broken screen.
func terminalSize() (int, int) {
	cols, rows := 100, 32
	if v := os.Getenv("COLUMNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 20 && n <= 400 {
			cols = n
		}
	}
	if v := os.Getenv("LINES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 10 && n <= 200 {
			rows = n
		}
	}
	return cols, rows
}
