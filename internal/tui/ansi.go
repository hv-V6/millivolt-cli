// Package tui renders a terminal dashboard for millivolt using its own palette
// tokens. Standard library only; no terminal widget library.

package tui

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;?]*[A-Za-z]")

var enabled = os.Getenv("NO_COLOR") == "" && os.Getenv("MVCTL_NO_COLOR") == "" && isTTY(os.Stdout)

func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func SetColor(on bool)   { enabled = on }
func ColorEnabled() bool { return enabled }

var styleCodes = map[string]string{
	"bold": "1", "dim": "2", "italic": "3", "underline": "4", "inverse": "7",
	"red": "31", "green": "32", "yellow": "33", "blue": "34",
	"magenta": "35", "cyan": "36", "white": "37", "gray": "90",
	"brightRed": "91", "brightGreen": "92", "brightYellow": "93",
	"brightBlue": "94", "brightMagenta": "95", "brightCyan": "96",
}

// Paint applies named SGR styles. Unknown style names are ignored.
func Paint(text string, styles ...string) string {
	if !enabled || len(styles) == 0 {
		return text
	}
	var codes []string
	for _, s := range styles {
		if code, ok := styleCodes[s]; ok {
			codes = append(codes, code)
		}
	}
	if len(codes) == 0 {
		return text
	}
	return "\x1b[" + strings.Join(codes, ";") + "m" + text + "\x1b[0m"
}

func parseHex(hex string) (int, int, int) {
	h := strings.TrimPrefix(hex, "#")
	if len(h) != 6 {
		return 136, 136, 136
	}
	r, errR := strconv.ParseUint(h[0:2], 16, 8)
	g, errG := strconv.ParseUint(h[2:4], 16, 8)
	b, errB := strconv.ParseUint(h[4:6], 16, 8)
	if errR != nil || errG != nil || errB != nil {
		return 136, 136, 136
	}
	return int(r), int(g), int(b)
}

// RGB paints text with a 24-bit foreground colour.
func RGB(text string, hex string) string {
	if !enabled {
		return text
	}
	r, g, b := parseHex(hex)
	return "\x1b[38;2;" + strconv.Itoa(r) + ";" + strconv.Itoa(g) + ";" + strconv.Itoa(b) + "m" + text + "\x1b[0m"
}

// BGRGB paints text with a 24-bit background colour.
func BGRGB(text string, hex string) string {
	if !enabled {
		return text
	}
	r, g, b := parseHex(hex)
	return "\x1b[48;2;" + strconv.Itoa(r) + ";" + strconv.Itoa(g) + ";" + strconv.Itoa(b) + "m" + text + "\x1b[0m"
}

func Strip(s string) string { return ansiPattern.ReplaceAllString(s, "") }

func Width(s string) int { return utf8.RuneCountInString(Strip(s)) }

// Clip truncates a coloured string to a visible width without breaking escape
// sequences. It appends a reset so a truncated line never leaks colour.
func Clip(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if Width(s) <= max {
		return s
	}
	var b strings.Builder
	visible := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && !((s[j] >= 'A' && s[j] <= 'Z') || (s[j] >= 'a' && s[j] <= 'z')) {
				j++
			}
			if j < len(s) {
				j++
			}
			b.WriteString(s[i:j])
			i = j
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if visible+1 > max {
			break
		}
		b.WriteRune(r)
		visible++
		i += size
	}
	if enabled {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

var Screen = struct {
	Enter, Leave, HideCursor, ShowCursor, Clear, Home, EraseLine string
	Move                                                         func(row, col int) string
	Line                                                         func(row, col int, text string) string
}{
	Enter:      "\x1b[?1049h\x1b[?25l\x1b[2J\x1b[H",
	Leave:      "\x1b[?25h\x1b[?1049l",
	HideCursor: "\x1b[?25l",
	ShowCursor: "\x1b[?25h",
	Clear:      "\x1b[2J\x1b[H",
	Home:       "\x1b[H",
	EraseLine:  "\x1b[2K",
	Move: func(row, col int) string {
		return "\x1b[" + strconv.Itoa(row) + ";" + strconv.Itoa(col) + "H"
	},
	Line: func(row, col int, text string) string {
		return "\x1b[" + strconv.Itoa(row) + ";" + strconv.Itoa(col) + "H\x1b[2K" + text
	},
}
