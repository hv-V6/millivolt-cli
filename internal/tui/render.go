package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/hv-V6/mvctl/internal/themes"
)

func Pad(s string, n int, right bool) string {
	gap := n - Width(s)
	if gap < 0 {
		gap = 0
	}
	if right {
		return strings.Repeat(" ", gap) + s
	}
	return s + strings.Repeat(" ", gap)
}

func Truncate(s string, n int) string {
	if Width(s) <= n {
		return s
	}
	return Clip(s, maxInt(n-1, 0)) + "…"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func Num(n float64) string {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return "—"
	}
	a := math.Abs(n)
	switch {
	case a >= 1e12:
		return fmt.Sprintf("%.2fT", n/1e12)
	case a >= 1e9:
		return fmt.Sprintf("%.2fB", n/1e9)
	case a >= 1e6:
		return fmt.Sprintf("%.2fM", n/1e6)
	case a >= 1e4:
		return fmt.Sprintf("%.1fk", n/1e3)
	case n == math.Trunc(n):
		return strconv.FormatInt(int64(n), 10)
	default:
		return fmt.Sprintf("%.2f", n)
	}
}

func Cost(n float64) string {
	if n >= 100 {
		return fmt.Sprintf("$%.2f", n)
	}
	if n >= 1 {
		return fmt.Sprintf("$%.3f", n)
	}
	return fmt.Sprintf("$%.4f", n)
}

func Dur(ms float64) string {
	if ms <= 0 {
		return "—"
	}
	if ms < 1000 {
		return fmt.Sprintf("%.0fms", ms)
	}
	if ms < 60_000 {
		return fmt.Sprintf("%.1fs", ms/1000)
	}
	if ms < 3_600_000 {
		return fmt.Sprintf("%.1fm", ms/60_000)
	}
	return fmt.Sprintf("%.1fh", ms/3_600_000)
}

func Bytes(n float64) string {
	if n <= 0 {
		return "0B"
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	i := 0
	for n >= 1024 && i < len(units)-1 {
		n /= 1024
		i++
	}
	if n >= 10 || i == 0 {
		return fmt.Sprintf("%.0f%s", n, units[i])
	}
	return fmt.Sprintf("%.1f%s", n, units[i])
}

// Table renders a borderless aligned table with palette-coloured headings.
func Table(p themes.Palette, head []string, rows [][]string, indent int) string {
	cols := len(head)
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	widths := make([]int, cols)
	all := rows
	if len(head) > 0 {
		all = append([][]string{head}, rows...)
	}
	for _, r := range all {
		for i, cell := range r {
			if i < cols && Width(cell) > widths[i] {
				widths[i] = Width(cell)
			}
		}
	}
	prefix := strings.Repeat(" ", indent)
	format := func(r []string, isHead bool) string {
		var b strings.Builder
		for i := 0; i < cols; i++ {
			cell := ""
			if i < len(r) {
				cell = r[i]
			}
			s := Pad(cell, widths[i], false)
			if isHead {
				s = RGB(Paint(s, "bold"), p.Muted)
			}
			b.WriteString(s)
			if i < cols-1 {
				b.WriteString("  ")
			}
		}
		return strings.TrimRight(b.String(), " ")
	}
	var lines []string
	if len(head) > 0 {
		lines = append(lines, prefix+format(head, true))
	}
	for _, r := range rows {
		lines = append(lines, prefix+format(r, false))
	}
	return strings.Join(lines, "\n")
}

var sparkBlocks = []rune("▁▂▃▄▅▆▇█")

// Spark renders a resampled sparkline in the accent colour.
func Spark(p themes.Palette, values []float64, width int) string {
	if width < 1 {
		width = 1
	}
	if len(values) == 0 {
		return RGB(strings.Repeat(string(sparkBlocks[0]), width), p.Accent)
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	seen := false
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		seen = true
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	if !seen {
		return RGB(strings.Repeat(string(sparkBlocks[0]), width), p.Accent)
	}
	if hi == lo {
		hi = lo + 1
	}
	var b strings.Builder
	for i := 0; i < width; i++ {
		idx := i * len(values) / width
		if idx >= len(values) {
			idx = len(values) - 1
		}
		v := values[idx]
		f := (v - lo) / (hi - lo)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			f = 0
		}
		if f < 0 {
			f = 0
		}
		if f > 1 {
			f = 1
		}
		b.WriteRune(sparkBlocks[int(f*float64(len(sparkBlocks)-1))])
	}
	return RGB(b.String(), p.Accent)
}

// Bar renders a labelled horizontal bar with a numeric suffix.
func Bar(p themes.Palette, label string, value, max float64, width int) string {
	if width < 1 {
		width = 1
	}
	if max <= 0 {
		max = 1
	}
	f := value / max
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	n := int(f * float64(width))
	filled := RGB(strings.Repeat("█", n), p.Accent)
	empty := RGB(strings.Repeat("░", width-n), p.Border2)
	return fmt.Sprintf("%s %s%s %s", Pad(Truncate(label, 18), 18, false), filled, empty, RGB(Num(value), p.Cyan))
}

func KV(p themes.Palette, pairs [][2]string) string {
	w := 0
	for _, pair := range pairs {
		if Width(pair[0]) > w {
			w = Width(pair[0])
		}
	}
	var b strings.Builder
	for _, pair := range pairs {
		b.WriteString(RGB(Pad(pair[0], w+2, false), p.Muted))
		b.WriteString(RGB(pair[1], p.Text))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func Heading(p themes.Palette, title string, width int) string {
	if width < 1 {
		width = 60
	}
	label := " " + title + " "
	rule := strings.Repeat("─", maxInt(0, width-Width(label)-1))
	return RGB(Paint(label, "bold"), p.Accent) + RGB(rule, p.Border2)
}

// StatusColor colours an HTTP status using the palette's semantic ink.
func StatusColor(p themes.Palette, code int, text string) string {
	switch {
	case code == 0:
		return RGB(text, p.Err)
	case code < 300:
		return RGB(text, p.Ok)
	case code < 400:
		return RGB(text, p.Cyan)
	case code < 500:
		return RGB(text, p.Warn)
	default:
		return RGB(text, p.Err)
	}
}
