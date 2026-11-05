package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hv-V6/mvctl/internal/themes"
)

type Row struct {
	Time, Status, Provider, Model, Client, Duration, Tokens, Error string
}

type BarItem struct {
	Label string
	Value float64
}

type TopData struct {
	URL, Version, Theme, Mode                  string
	Healthy, Paused, Demo                      bool
	Requests, Errors, InFlight, Cost           float64
	InputTokens, OutputTokens, CacheReadTokens float64
	AvgTTFTMs, AvgTPS                          *float64
	Dropped                                    float64
	StorageEnabled                             bool
	ThrottleActive                             int
	DebugEnabled                               bool
	Rows                                       []Row
	History                                    []float64
	Providers                                  []BarItem
	Notice                                     string
}

type Options struct {
	Theme, Mode string
	RefreshMS   int
	Demo        bool
	NonTTY      bool
	ShowHelp    bool
}

const footerKeys = "q quit  p pause  d debug  r refresh  t theme  m mode  ? help  arrows scroll"

func ptr(v *float64) string {
	if v == nil {
		return "—"
	}
	return Num(*v)
}

// RenderFrame builds one complete dashboard frame. It never emits a line wider
// than cols and never panics on tiny terminals.
func RenderFrame(d TopData, p themes.Palette, cols, rows int, showHelp bool) string {
	if cols < 1 {
		cols = 80
	}
	if rows < 1 {
		rows = 24
	}
	compact := cols < 80 || rows < 20
	lines := []string{}
	emit := func(s string) { lines = append(lines, Clip(s, cols)) }

	// Header -----------------------------------------------------------------
	dot := RGB("●", p.Ok)
	if !d.Healthy {
		dot = RGB("●", p.Err)
	}
	badge := ""
	if d.Paused {
		badge += " " + BGRGB(RGB(" PAUSED ", p.Text), p.Err)
	}
	if d.Demo {
		badge += " " + BGRGB(RGB(" DEMO ", p.Text), p.Accent)
	}
	themeMode := d.Theme + "/" + d.Mode
	header := " mvctl  " + dot + " " + RGB(d.URL, p.Text) +
		"  " + RGB(d.Version, p.Muted) + "  " + RGB(themeMode, p.Cyan) + badge
	emit(BGRGB(Pad(header, cols, false), p.Rail))
	emit(RGB(strings.Repeat("─", cols), p.Border2))

	// KPI tiles --------------------------------------------------------------
	type tile struct{ label, value string }
	tiles := []tile{
		{"Requests", Num(d.Requests)}, {"Errors", Num(d.Errors)}, {"In flight", Num(d.InFlight)},
		{"Cost", Cost(d.Cost)}, {"In tokens", Num(d.InputTokens)}, {"Out tokens", Num(d.OutputTokens)},
		{"Cache read", Num(d.CacheReadTokens)}, {"Avg TTFT", durPtr(d.AvgTTFTMs)}, {"Avg TPS", ptr(d.AvgTPS)},
	}
	perRow := 3
	if compact {
		perRow = 2
	}
	tileWidth := (cols - 2) / perRow
	for i := 0; i < len(tiles); i += perRow {
		var b strings.Builder
		for j := 0; j < perRow && i+j < len(tiles); j++ {
			t := tiles[i+j]
			cell := RGB(t.label, p.Muted) + " " + RGB(Paint(t.value, "bold"), p.Text)
			b.WriteString(Pad(cell, tileWidth, false))
		}
		emit(" " + b.String())
	}

	// Sparkline --------------------------------------------------------------
	emit(Heading(p, "requests / sample", cols))
	emit(" " + Spark(p, d.History, maxInt(cols-2, 8)))

	// Body: providers + recent requests --------------------------------------
	emit(RGB(strings.Repeat("─", cols), p.Border2))
	left := make([]string, 0, 12)
	left = append(left, Heading(p, "providers", maxInt(cols/2-2, 20)))
	max := 0.0
	for _, item := range d.Providers {
		if item.Value > max {
			max = item.Value
		}
	}
	if len(d.Providers) == 0 {
		left = append(left, RGB(" (none)", p.Muted))
	}
	for i, item := range d.Providers {
		if i >= 8 {
			break
		}
		left = append(left, " "+Bar(p, item.Label, item.Value, max, maxInt(minInt(14, cols/2-28), 4)))
	}

	rightLines := make([]string, 0, 12)
	rightLines = append(rightLines, Heading(p, "recent requests", maxInt(cols/2-2, 20)))
	head := []string{"time", "status", "provider", "model", "client", "dur", "tokens"}
	body := make([][]string, 0, len(d.Rows))
	for _, r := range d.Rows {
		code := 0
		_, _ = fmt.Sscanf(r.Status, "%d", &code)
		body = append(body, []string{
			r.Time, StatusColor(p, code, r.Status), r.Provider, r.Model, r.Client, r.Duration, r.Tokens,
		})
	}
	if len(body) == 0 {
		rightLines = append(rightLines, RGB(" (no requests)", p.Muted))
	} else {
		tableText := Table(p, head, body, 0)
		for _, line := range strings.Split(tableText, "\n") {
			rightLines = append(rightLines, line)
		}
	}
	if compact {
		for _, l := range left {
			emit(l)
		}
		for _, l := range rightLines[:minInt(len(rightLines), 6)] {
			emit(l)
		}
	} else {
		leftWidth := cols/2 - 2
		n := maxInt(len(left), len(rightLines))
		for i := 0; i < n; i++ {
			l, r := "", ""
			if i < len(left) {
				l = Clip(left[i], leftWidth)
			}
			if i < len(rightLines) {
				r = Clip(rightLines[i], cols-leftWidth-3)
			}
			emit(Pad(l, leftWidth, false) + " " + RGB("│", p.Border2) + " " + r)
		}
	}

	// State + footer ---------------------------------------------------------
	emit(RGB(strings.Repeat("─", cols), p.Border2))
	state := fmt.Sprintf("storage %s  throttles %d  debug %s  dropped %s",
		boolWord(d.StorageEnabled), d.ThrottleActive, boolWord(d.DebugEnabled), Num(d.Dropped))
	emit(" " + RGB(state, p.Muted))
	if d.Notice != "" {
		emit(" " + RGB(Paint(d.Notice, "bold"), p.Warn))
	}
	if showHelp {
		for _, h := range []string{
			" q  quit                    p  toggle pause hold",
			" d  toggle debug capture    r  force refresh",
			" t  cycle theme             m  toggle black/clear",
			" ?  close this help         ↑/↓ scroll requests",
		} {
			emit(" " + RGB(h, p.Text))
		}
	}
	emit(BGRGB(Pad(" "+footerKeys, cols, false), p.Panel2))

	if rows > 0 && len(lines) > rows {
		lines = lines[:rows]
	}
	if len(lines) == 0 {
		lines = []string{Clip(RGB("mvctl", p.Accent), cols)}
	}
	return strings.Join(lines, "\n")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func durPtr(v *float64) string {
	if v == nil {
		return "—"
	}
	return Dur(*v)
}

func boolWord(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

// RunTop drives the interactive dashboard. When stdout is not a terminal (or
// NonTTY is set) it prints exactly one snapshot and returns, which is how the
// CLI is tested and how screenshots are produced.
func RunTop(ctx context.Context, fetch func(context.Context) (TopData, error), opts Options) error {
	t, m := themes.Normalize(opts.Theme, opts.Mode)
	p := themes.Get(t, m)
	refresh := opts.RefreshMS
	if refresh < 200 {
		refresh = 1000
	}
	cols, rows := terminalSize()
	if opts.NonTTY || !isTTY(os.Stdout) {
		d, err := fetch(ctx)
		if err != nil {
			return err
		}
		cols, rows = 100, 32
		fmt.Println(RenderFrame(d, p, cols, rows, false))
		return nil
	}

	restore, err := enableRaw()
	if err != nil {
		restore = func() {}
	}
	defer restore()
	fmt.Print(Screen.Enter)
	defer fmt.Print(Screen.Leave)

	themeIdx := 0
	for i, name := range themes.Names() {
		if name == t {
			themeIdx = i
			break
		}
	}
	modeIdx := 0
	for i, name := range themes.Modes() {
		if name == m {
			modeIdx = i
			break
		}
	}
	showHelp := opts.ShowHelp
	notice := ""
	ticker := time.NewTicker(time.Duration(refresh) * time.Millisecond)
	defer ticker.Stop()
	keys := make(chan byte, 16)
	done := make(chan struct{})
	defer close(done)
	go func() {
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				return
			}
			select {
			case keys <- buf[0]:
			case <-done:
				return
			default:
			}
		}
	}()

	draw := func() {
		d, err := fetch(ctx)
		if err != nil {
			notice = "fetch error: " + err.Error()
			d = TopData{URL: "millivolt", Healthy: false, Theme: themes.Names()[themeIdx], Mode: themes.Modes()[modeIdx], Demo: opts.Demo}
		}
		d.Theme, d.Mode, d.Demo = themes.Names()[themeIdx], themes.Modes()[modeIdx], opts.Demo
		d.Notice = notice
		// Clear before every frame: a shrinking frame (help closed, fewer
		// rows) would otherwise leave stale characters on the lower lines.
		fmt.Print(Screen.Clear)
		fmt.Print(RenderFrame(d, themes.Get(d.Theme, d.Mode), cols, rows, showHelp))
	}
	draw()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			draw()
		case k := <-keys:
			switch k {
			case 'q', 'Q', 3:
				return nil
			case 'r', 'R':
				notice, showHelp = "", false
				draw()
			case '?', 'h':
				showHelp = !showHelp
				draw()
			case 't', 'T':
				themeIdx = (themeIdx + 1) % len(themes.Names())
				notice = "theme: " + themes.Names()[themeIdx]
				draw()
			case 'm', 'M':
				modeIdx = (modeIdx + 1) % len(themes.Modes())
				notice = "mode: " + themes.Modes()[modeIdx]
				draw()
			case 'p', 'P':
				notice = "pause toggled from the CLI (mvctl pause on|off)"
				draw()
			case 'd', 'D':
				notice = "debug capture toggled from the CLI (mvctl debug on|off)"
				draw()
			}
		}
	}
}
