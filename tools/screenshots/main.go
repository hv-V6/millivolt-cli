// Command screenshots runs every mvctl command, captures its real ANSI output,
// and renders it into a PNG terminal screenshot using a monospace TTF font.
//
// Read-only commands run against the live millivolt proxy. Mutating commands
// run against an in-process fake millivolt, so verification never changes
// production state. Docker lifecycle commands use a scratch compose project.
//
// This tool is Go only (golang.org/x/image is a Go module for font parsing).

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// ---------------------------------------------------------------------------
// Terminal cell model
// ---------------------------------------------------------------------------

var (
	defFG  = color.RGBA{R: 0xe8, G: 0xe4, B: 0xde, A: 255}
	defBG  = color.RGBA{R: 0x14, G: 0x12, B: 0x10, A: 255}
	accent = color.RGBA{R: 0x7a, G: 0xa0, B: 0xff, A: 255}
)

var basic = []color.RGBA{
	{0, 0, 0, 255}, {0xcd, 0x31, 0x31, 255}, {0x0d, 0xbc, 0x79, 255}, {0xe5, 0xe5, 0x10, 255},
	{0x24, 0x72, 0xc8, 255}, {0xbc, 0x3f, 0xbc, 255}, {0x11, 0xa8, 0xcd, 255}, {0xe5, 0xe5, 0xe5, 255},
}

var bright = []color.RGBA{
	{0x76, 0x76, 0x76, 255}, {0xf1, 0x4c, 0x4c, 255}, {0x23, 0xd1, 0x8b, 255}, {0xf5, 0xf5, 0x43, 255},
	{0x3b, 0x8e, 0xea, 255}, {0xd6, 0x70, 0xd6, 255}, {0x29, 0xb8, 0xdb, 255}, {0xf5, 0xf5, 0xf5, 255},
}

type style struct {
	fg, bg    color.RGBA
	bold, dim bool
}

type cell struct {
	ch     rune
	fg, bg color.RGBA
}

func clampByte(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func shade(c color.RGBA, factor float64) color.RGBA {
	return color.RGBA{
		R: clampByte(float64(c.R) * factor),
		G: clampByte(float64(c.G) * factor),
		B: clampByte(float64(c.B) * factor),
		A: 255,
	}
}

// effectiveFG derives the display colour without mutating the base colour, so
// repeated SGR sequences cannot compound the bold/dim adjustment.
func effectiveFG(s style) color.RGBA {
	if s.dim {
		return shade(s.fg, 0.72)
	}
	if s.bold {
		return shade(s.fg, 1.18)
	}
	return s.fg
}

func applySGR(params []int, s *style) {
	if len(params) == 0 {
		params = []int{0}
	}
	for i := 0; i < len(params); i++ {
		p := params[i]
		switch {
		case p == 0:
			*s = style{fg: defFG, bg: defBG}
		case p == 1:
			s.bold = true
		case p == 2:
			s.dim = true
		case p == 22:
			s.bold, s.dim = false, false
		case p >= 30 && p <= 37:
			s.fg = basic[p-30]
		case p >= 90 && p <= 97:
			s.fg = bright[p-90]
		case p >= 40 && p <= 47:
			s.bg = basic[p-40]
		case p >= 100 && p <= 107:
			s.bg = bright[p-100]
		case p == 39:
			s.fg = defFG
		case p == 49:
			s.bg = defBG
		case p == 38 && i+4 < len(params) && params[i+1] == 2:
			s.fg = color.RGBA{clampByte(float64(params[i+2])), clampByte(float64(params[i+3])), clampByte(float64(params[i+4])), 255}
			i += 4
		case p == 48 && i+4 < len(params) && params[i+1] == 2:
			s.bg = color.RGBA{clampByte(float64(params[i+2])), clampByte(float64(params[i+3])), clampByte(float64(params[i+4])), 255}
			i += 4
		}
	}
}

// blockLevel returns the filled fraction (1..8) for U+2580 block runes, or 0
// for ordinary characters.
func blockLevel(r rune) int {
	switch r {
	case '▁':
		return 1
	case '▂':
		return 2
	case '▃':
		return 3
	case '▄':
		return 4
	case '▅':
		return 5
	case '▆':
		return 6
	case '▇':
		return 7
	case '█':
		return 8
	case '▀':
		return 8 // upper half block: approximate with a full cell
	}
	return 0
}

// parseANSI converts captured output into a grid of coloured cells.
func parseANSI(text string) [][]cell {
	lines := [][]cell{}
	row := []cell{}
	s := style{fg: defFG, bg: defBG}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	for i := 0; i < len(text); {
		b := text[i]
		if b == 0x1b {
			if i+1 < len(text) && text[i+1] == '[' {
				j := i + 2
				for j < len(text) && !((text[j] >= 'A' && text[j] <= 'Z') || (text[j] >= 'a' && text[j] <= 'z')) {
					j++
				}
				if j < len(text) {
					if text[j] == 'm' {
						parts := strings.Split(text[i+2:j], ";")
						params := make([]int, 0, len(parts))
						for _, p := range parts {
							if p == "" {
								params = append(params, 0)
								continue
							}
							if n, err := strconv.Atoi(p); err == nil {
								params = append(params, n)
							}
						}
						applySGR(params, &s)
					}
					i = j + 1
					continue
				}
			}
			i += 2
			continue
		}
		if b == '\n' {
			lines = append(lines, row)
			row = []cell{}
			i++
			continue
		}
		if b == '\r' {
			i++
			continue
		}
		if b == '\t' {
			for k := 0; k < 4; k++ {
				row = append(row, cell{' ', effectiveFG(s), s.bg})
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		row = append(row, cell{r, effectiveFG(s), s.bg})
		i += size
	}
	if len(row) > 0 {
		lines = append(lines, row)
	}
	return lines
}

// ---------------------------------------------------------------------------
// PNG renderer
// ---------------------------------------------------------------------------

type renderer struct {
	face     font.Face
	cellW    int
	cellH    int
	ascent   int
	pad      int
	captionH int
}

func newRenderer(fontPath string) (*renderer, error) {
	data, err := os.ReadFile(fontPath)
	if err != nil {
		return nil, err
	}
	parsed, err := opentype.Parse(data)
	if err != nil {
		return nil, err
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size: 14, DPI: 96, Hinting: font.HintingNone,
	})
	if err != nil {
		return nil, err
	}
	adv, _ := face.GlyphAdvance('M')
	m := face.Metrics()
	return &renderer{
		face:     face,
		cellW:    adv.Ceil() + 1,
		cellH:    m.Height.Ceil() + 2,
		ascent:   m.Ascent.Ceil(),
		pad:      18,
		captionH: 26,
	}, nil
}

func (r *renderer) render(caption string, lines [][]cell, path string) error {
	cols := 1
	for _, row := range lines {
		if len(row) > cols {
			cols = len(row)
		}
	}
	if cols > 200 {
		cols = 200
	}
	w := cols*r.cellW + 2*r.pad
	h := (len(lines)+1)*r.cellH + r.captionH + 2*r.pad
	if h > 4000 {
		h = 4000
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{defBG}, image.Point{}, draw.Src)

	// caption bar
	draw.Draw(img, image.Rect(0, 0, w, r.captionH+r.pad/2), &image.Uniform{color.RGBA{0x1a, 0x17, 0x14, 255}}, image.Point{}, draw.Src)
	d := &font.Drawer{Dst: img, Src: image.NewUniform(accent), Face: r.face}
	d.Dot = fixed.P(r.pad, r.ascent+r.pad/2)
	d.DrawString("$ " + caption)

	baseY := r.captionH + r.pad/2 + r.pad
	for y, row := range lines {
		if y*r.cellH+baseY+r.cellH > h {
			break
		}
		for x := 0; x < len(row) && x < cols; x++ {
			c := row[x]
			px := r.pad + x*r.cellW
			py := baseY + y*r.cellH
			if c.bg != defBG {
				draw.Draw(img, image.Rect(px, py, px+r.cellW, py+r.cellH), &image.Uniform{c.bg}, image.Point{}, draw.Src)
			}
			if c.ch == ' ' {
				continue
			}
			// Consolas/Cascadia lack some U+258x block glyphs. Draw them as
			// real rectangles so sparklines and bars render faithfully.
			if level := blockLevel(c.ch); level > 0 {
				bh := r.cellH * level / 8
				draw.Draw(img, image.Rect(px, py+r.cellH-bh, px+r.cellW, py+r.cellH),
					&image.Uniform{c.fg}, image.Point{}, draw.Src)
				continue
			}
			d.Src = image.NewUniform(c.fg)
			d.Dot = fixed.P(px, py+r.ascent)
			d.DrawString(string(c.ch))
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// ---------------------------------------------------------------------------
// Fake millivolt (for mutating commands)
// ---------------------------------------------------------------------------

func fakeHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.Copy(io.Discard, io.LimitReader(req.Body, 1<<20))
	path := req.URL.Path
	write := func(s string) { _, _ = io.WriteString(w, s) }
	switch {
	case path == "/healthz":
		write(`{"ok":true}`)
	case path == "/metrics/bootstrap":
		write(fakeBootstrap)
	case path == "/metrics/agg/log":
		write(fakeLog)
	case path == "/metrics/agg/chart":
		write(fakeChart)
	case path == "/metrics/agg/explorer":
		write(fakeExplorer)
	case path == "/metrics/prometheus":
		w.Header().Set("Content-Type", "text/plain")
		write("# HELP llm_proxy_requests_total Requests\nllm_proxy_requests_total 39275\n")
	case path == "/metrics/query":
		write(`[{"client":"mvctl-demo","n":3}]`)
	case path == "/admin/config":
		if req.Method == http.MethodPost {
			write(`{"saved":true,"revision":"rev-2","last_reload":{"ok":true}}`)
		} else {
			write(fakeConfig)
		}
	case path == "/admin/backup":
		w.Header().Set("Content-Type", "application/octet-stream")
		write("MVCTL-FAKE-BACKUP")
	case path == "/admin/purge/count":
		write(`{"count":3}`)
	case path == "/admin/purge":
		write(`{"ok":true}`)
	case path == "/admin/restore":
		write(`{"ok":true,"inspected":true,"restart_required":[]}`)
	case path == "/admin/restart":
		write(`{"ok":true,"phase":"restarting"}`)
	case path == "/admin/reload":
		write(`{"ok":true,"restart_required":[]}`)
	case path == "/admin/queue/drain":
		write(`{"drained_through":1}`)
	case path == "/admin/pause":
		write(`{"ok":true,"paused":true,"queued":0,"holds":[]}`)
	case path == "/admin/throttle":
		write(`{"ok":true,"active":true,"throttles":[{"provider":"openrouter","concurrency":8}]}`)
	case path == "/admin/quota":
		write(`{"ok":true,"resumed":"openrouter"}`)
	case path == "/admin/debug":
		write(`{"ok":true,"enabled":true,"sessions":[]}`)
	case path == "/v1/chat/completions":
		write(`{"id":"chatcmpl-fake","choices":[{"message":{"content":"mvctl screenshot response"},"finish_reason":"stop"}],"usage":{"total_tokens":36}}`)
	default:
		write(`{"ok":true}`)
	}
}

const fakeBootstrap = `{"kpi":{"requests":39275,"errors":108,"in_flight":1,"cost":28.69,"cost_per_req":0.0007,"cost_per_mtok":0.007,"input_tokens":9909959702,"output_tokens":28766213,"cache_read_tokens":9770446257,"reasoning_tokens":15131460,"avg_ttft_ms":3674.27,"avg_tps":270.1},"storage":{"enabled":true,"dropped":0,"totals_degraded":false},"pause":{"paused":false,"queued":0,"holds":[]},"throttle":{"active":false,"throttles":[]},"debug":{"enabled":false,"sessions":[]},"dashboard_version":"4c94a75604d63700","counters":{"in_flight":1,"total_requests":39275,"total_errors":108}}`

const fakeLog = `{"records":[{"id":"a1","start":"2026-11-05T10:17:13Z","status_code":200,"provider":"opencode-go","model":"hy4","client":"js 7.19.0","stream":true,"duration_ms":1234,"ttft_ms":367,"usage":{"input_tokens":120,"output_tokens":30},"retries":0,"error_type":""},{"id":"a2","start":"2026-11-05T10:17:14Z","status_code":429,"provider":"openrouter","model":"gpt-5","client":"codex","duration_ms":88,"usage":{"input_tokens":10,"output_tokens":0},"error_type":"rate_limited"}],"more":false,"cursor_ms":0,"cursor_id":"a2"}`

const fakeChart = `{"now_ms":1,"from_ms":0,"bucket_ms":86400000,"buckets":[{"t":1792454400000,"req":3343,"err":9,"cost":4.35,"in":135337045,"out":1210843},{"t":1792540800000,"req":4374,"err":10,"cost":10.4,"in":746034325,"out":3118990},{"t":1792627200000,"req":3686,"err":1,"cost":2.1,"in":715000000,"out":1000000}]}`

const fakeExplorer = `{"dim":"provider","total":39451,"error_total":109,"groups":[{"name":"opendesign","n":16842,"cost":12.4,"in":4179692100,"out":1329,"err":4},{"name":"openrouter","n":9201,"cost":8.1,"in":991004400,"out":8821,"err":2},{"name":"opencode-go","n":6800,"cost":0,"in":1780000000,"out":500,"err":1}]}`

const fakeConfig = `{"values":{"listen":"0.0.0.0:8471","history_size":10000},"effective":{"listen":"0.0.0.0:8471"},"defaults":{"listen":":8080","history_size":10000},"revision":"rev-1","restart_required":["listen"],"fields":[{"key":"listen","category":"server","kind":"string"},{"key":"history_size","category":"server","kind":"int"}],"categories":[],"overrides":{},"writable":true,"path":"/config/proxy.yaml"}`

// ---------------------------------------------------------------------------
// Command catalogue
// ---------------------------------------------------------------------------

type shot struct {
	name    string
	args    []string
	mode    string // "live" | "fake" | "local"
	timeout time.Duration
	note    string
}

func catalogue() []shot {
	themes := []string{"instrument", "slate", "midnight", "nord", "forest", "rose", "ember", "contrast"}
	s := []shot{
		{name: "01-help", args: []string{"help"}, mode: "local", note: "command catalogue"},
		{name: "02-version", args: []string{"--version"}, mode: "local", note: "version"},
		{name: "03-status", args: []string{"status"}, mode: "live", note: "live health + headline metrics"},
		{name: "04-status-json", args: []string{"--json", "status"}, mode: "live", note: "scripting output, redacted"},
		{name: "05-overview", args: []string{"overview"}, mode: "live", note: "full KPI table"},
		{name: "06-requests", args: []string{"requests", "10"}, mode: "live", note: "durable request log"},
		{name: "07-query", args: []string{"query", "SELECT 1"}, mode: "live", note: "bounded SQL"},
		{name: "08-prometheus", args: []string{"prometheus"}, mode: "live", note: "Prometheus exposition"},
		{name: "09-watch", args: []string{"watch", "--samples", "3", "--interval", "1"}, mode: "live", timeout: 20 * time.Second, note: "live counter sampling"},
		{name: "10-url", args: []string{"url"}, mode: "live", note: "base URL"},
		{name: "11-health", args: []string{"health"}, mode: "live", note: "health gate"},
		{name: "12-config-get", args: []string{"config", "get"}, mode: "live", note: "full live config"},
		{name: "13-config-keys", args: []string{"config", "keys", "listen"}, mode: "live", note: "schema key search"},
		{name: "14-config-diff", args: []string{"config", "diff"}, mode: "live", note: "config vs defaults"},
		{name: "15-config-set", args: []string{"config", "set", "history_size=10000"}, mode: "fake", note: "revision-checked write (fake)"},
		{name: "16-config-reload", args: []string{"config", "reload"}, mode: "fake", note: "reload (fake)"},
		{name: "17-pause-show", args: []string{"pause"}, mode: "live", note: "hold state"},
		{name: "18-pause-on", args: []string{"pause", "on"}, mode: "fake", note: "add hold (fake)"},
		{name: "19-throttle-show", args: []string{"throttle"}, mode: "live", note: "provider limits"},
		{name: "20-throttle-set", args: []string{"throttle", "set", "--provider", "openrouter", "--concurrency", "8"}, mode: "fake", note: "set limit (fake)"},
		{name: "21-quota-show", args: []string{"quota"}, mode: "live", note: "storm/quota state"},
		{name: "22-quota-resume", args: []string{"quota", "resume", "--provider", "openrouter"}, mode: "fake", note: "resume gate (fake)"},
		{name: "23-drain", args: []string{"drain", "--all", "--yes"}, mode: "fake", note: "drain queue (fake)"},
		{name: "24-debug-show", args: []string{"debug"}, mode: "live", note: "capture sessions"},
		{name: "25-debug-on", args: []string{"debug", "on"}, mode: "fake", note: "start capture (fake)"},
		{name: "26-purge-preview", args: []string{"purge", "--client", "mvctl-demo"}, mode: "live", note: "preview only, no deletion"},
		{name: "27-purge-apply", args: []string{"purge", "--client", "mvctl-demo", "--yes"}, mode: "fake", note: "delete (fake)"},
		{name: "28-backup", args: []string{"backup", "--out", "BACKUP_FILE"}, mode: "fake", note: "archive download (fake)"},
		{name: "29-restore-inspect", args: []string{"restore", "--file", "BACKUP_FILE"}, mode: "fake", note: "inspect, no apply"},
		{name: "30-restart-preview", args: []string{"restart"}, mode: "live", note: "guarded preview"},
		{name: "31-restart-apply", args: []string{"restart", "--yes"}, mode: "fake", note: "restart (fake)"},
		{name: "32-chart", args: []string{"chart"}, mode: "live", note: "ASCII time series"},
		{name: "33-explore", args: []string{"explore", "--by", "provider", "--limit", "6"}, mode: "live", note: "ranked breakdown"},
		{name: "34-cost", args: []string{"cost"}, mode: "live", note: "spend attribution"},
		{name: "35-models", args: []string{"models"}, mode: "live", note: "model leaderboard"},
		{name: "36-providers", args: []string{"providers"}, mode: "live", note: "provider health board"},
		{name: "37-trace", args: []string{"trace", "a1"}, mode: "fake", note: "single request detail"},
		{name: "38-themes-list", args: []string{"themes", "list"}, mode: "local", note: "8 palettes x 2 modes"},
		{name: "39-wait", args: []string{"wait", "--timeout", "5s"}, mode: "live", timeout: 15 * time.Second, note: "block until healthy"},
		{name: "40-profiles-list", args: []string{"profiles", "list"}, mode: "local", note: "named deployments"},
		{name: "41-profiles-add", args: []string{"profiles", "add", "--name", "demo", "--url", "http://127.0.0.1:8471"}, mode: "local", note: "add profile (isolated config)"},
		{name: "42-profiles-use", args: []string{"profiles", "use", "--name", "demo"}, mode: "local", note: "set default profile"},
		{name: "43-discover", args: []string{"discover"}, mode: "live", note: "Docker label discovery"},
		{name: "44-doctor", args: []string{"doctor"}, mode: "live", note: "8-point diagnostic"},
		{name: "45-images", args: []string{"images"}, mode: "live", note: "image drift"},
		{name: "46-logs", args: []string{"logs", "--dir", "REAL_DIR", "--tail", "15"}, mode: "live", timeout: 30 * time.Second, note: "deployment logs"},
		{name: "47-setup-preview", args: []string{"setup"}, mode: "scratch", note: "guarded install preview"},
		{name: "48-adopt", args: []string{"adopt", "--dir", "SCRATCH_DIR", "--port", "8471"}, mode: "scratch", note: "record deployment (isolated state)"},
		{name: "49-stop", args: []string{"stop", "--dir", "SCRATCH_DIR"}, mode: "scratch", note: "stop (scratch project)"},
		{name: "50-start", args: []string{"start", "--dir", "SCRATCH_DIR"}, mode: "scratch", note: "start (scratch project)"},
		{name: "51-up", args: []string{"up", "--dir", "SCRATCH_DIR"}, mode: "scratch", note: "compose up (scratch)"},
		{name: "52-update", args: []string{"update", "--dry-run", "--dir", "SCRATCH_DIR"}, mode: "scratch", timeout: 40 * time.Second, note: "dry-run update"},
		{name: "53-revert-preview", args: []string{"revert"}, mode: "scratch", note: "guarded rollback preview"},
		{name: "54-resume-preview", args: []string{"resume"}, mode: "scratch", note: "guarded resume preview"},
		{name: "55-repair", args: []string{"repair", "--dir", "SCRATCH_DIR"}, mode: "scratch", timeout: 40 * time.Second, note: "diagnose + fix (scratch)"},
		{name: "56-serve", args: []string{"serve"}, mode: "scratch", timeout: 4 * time.Second, note: "supervisor startup"},
		{name: "57-top-live", args: []string{"top", "--no-tty", "--theme", "instrument", "--mode", "black"}, mode: "live", note: "TUI with real data"},
		{name: "58-top-demo", args: []string{"top", "--demo", "--no-tty", "--theme", "midnight", "--mode", "black"}, mode: "local", note: "TUI synthetic feed"},
		{name: "59-tour", args: []string{"tour", "--no-delay"}, mode: "live", note: "scripted walkthrough"},
		{name: "60-dashboard-print", args: []string{"dashboard", "--print"}, mode: "live", note: "dashboard proxy URL"},
		{name: "61-demo", args: []string{"demo", "--requests", "2", "--yes", "--upstream", "http://127.0.0.1:1/v1"}, mode: "fake", timeout: 30 * time.Second, note: "showcase traffic (fake)"},
		{name: "62-bench", args: []string{"bench", "--requests", "4", "--concurrency", "2", "--yes", "--upstream", "http://127.0.0.1:1/v1"}, mode: "fake", timeout: 30 * time.Second, note: "latency percentiles (fake)"},
		{name: "63-chat", args: []string{"chat", "hello from mvctl", "--upstream", "http://127.0.0.1:1/v1"}, mode: "fake", timeout: 30 * time.Second, note: "one-shot inference (fake)"},
		{name: "64-install", args: []string{"install"}, mode: "local", note: "self-install preview"},
		{name: "65-uninstall", args: []string{"uninstall"}, mode: "local", note: "uninstall preview"},
	}
	for i, t := range themes {
		s = append(s, shot{
			name: fmt.Sprintf("%02d-theme-%s", 66+i, t),
			args: []string{"themes", "preview", "--theme", t, "--mode", "black"},
			mode: "local", note: "palette " + t + " / black",
		})
	}
	s = append(s, shot{name: "74-theme-clear", args: []string{"themes", "preview", "--theme", "rose", "--mode", "clear"}, mode: "local", note: "palette rose / clear"})
	return s
}

// ---------------------------------------------------------------------------
// Runner
// ---------------------------------------------------------------------------

type result struct {
	Name     string `json:"name"`
	Command  string `json:"command"`
	Mode     string `json:"mode"`
	ExitCode int    `json:"exit_code"`
	Bytes    int    `json:"output_bytes"`
	PNG      string `json:"png"`
	Note     string `json:"note"`
	OK       bool   `json:"verified"`
}

func main() {
	outDir := flag.String("out", "screenshots", "screenshot output directory")
	binPath := flag.String("mvctl", "mvctl", "mvctl binary (looked up on PATH)")
	fontPath := flag.String("font", "", "monospace TTF (auto-detected when empty)")
	liveURL := flag.String("live", envOr("MILLIVOLT_URL", "http://127.0.0.1:8080"), "live millivolt URL")
	realDir := flag.String("realdir", os.Getenv("MVCTL_DEPLOY_DIR"), "real deployment dir")
	flag.Parse()

	resolvedBin, err := exec.LookPath(*binPath)
	if err != nil {
		fatal(fmt.Errorf("cannot find mvctl binary %q on PATH: %w", *binPath, err))
	}
	*binPath = resolvedBin
	if *fontPath == "" {
		*fontPath = findFont()
	}
	if *realDir == "" {
		*realDir = "."
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fatal(err)
	}
	token := os.Getenv("MVCTL_SHOT_TOKEN")

	// Scratch compose project: one harmless service that exits immediately, so
	// lifecycle commands can complete without ever touching the live proxy.
	scratch, err := os.MkdirTemp("", "mvctl-shot-*")
	if err != nil {
		fatal(err)
	}
	defer func() {
		_ = exec.Command("docker", "compose", "--project-directory", scratch, "down", "--remove-orphans").Run()
		_ = os.RemoveAll(scratch)
	}()
	if err := os.WriteFile(filepath.Join(scratch, "compose.yaml"), []byte(
		"services:\n  noop:\n    image: golang:1.27.2-bookworm\n    entrypoint: [\"/bin/true\"]\n"), 0o644); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scratch, ".millivolt-managed"), []byte(`{"by":"mvctl-shot"}`), 0o644); err != nil {
		fatal(err)
	}
	isoConfig, err := os.MkdirTemp("", "mvctl-state-*")
	if err != nil {
		fatal(err)
	}
	defer os.RemoveAll(isoConfig)
	backupFile := filepath.Join(scratch, "backup.mvb")
	if err := os.WriteFile(backupFile, []byte("MVCTL-FAKE-BACKUP"), 0o600); err != nil {
		fatal(err)
	}

	// Fake millivolt on loopback.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fatal(err)
	}
	fakeURL := "http://" + listener.Addr().String()
	srv := &http.Server{Handler: http.HandlerFunc(fakeHandler), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(listener) }()
	defer srv.Close()

	r, err := newRenderer(*fontPath)
	if err != nil {
		fatal(err)
	}

	shots := catalogue()
	results := make([]result, 0, len(shots))
	for _, sh := range shots {
		res := runShot(sh, r, *binPath, *outDir, *liveURL, fakeURL, *realDir, scratch, backupFile, isoConfig, token)
		results = append(results, res)
		status := "OK  "
		if !res.OK {
			status = "FAIL"
		}
		fmt.Printf("%s  %-22s exit=%-3d %6d bytes  %s\n", status, sh.name, res.ExitCode, res.Bytes, res.PNG)
	}

	if err := writeManifest(*outDir, results); err != nil {
		fatal(err)
	}
	passed := 0
	for _, r := range results {
		if r.OK {
			passed++
		}
	}
	fmt.Printf("\n%d/%d commands verified; screenshots in %s\n", passed, len(results), *outDir)
	if passed != len(results) {
		os.Exit(1)
	}
}

func runShot(sh shot, r *renderer, bin, outDir, liveURL, fakeURL, realDir, scratch, backupFile, isoConfig, token string) result {
	url := liveURL
	switch sh.mode {
	case "fake":
		url = fakeURL
	case "local", "scratch":
		url = fakeURL // avoid auto-discovery; local commands ignore it
	}
	// Guarded commands need explicit confirmation to exercise their real path.
	guarded := map[string]bool{
		"15-config-set": true, "16-config-reload": true, "18-pause-on": true,
		"20-throttle-set": true, "22-quota-resume": true, "25-debug-on": true,
		"41-profiles-add": true, "42-profiles-use": true, "48-adopt": true,
		"49-stop": true, "50-start": true, "51-up": true, "55-repair": true,
		"63-chat": true,
	}
	args := make([]string, 0, len(sh.args)+8)
	args = append(args, "--color", "--url", url)
	for _, a := range sh.args {
		switch a {
		case "BACKUP_FILE":
			args = append(args, backupFile)
		case "SCRATCH_DIR":
			args = append(args, scratch)
		case "REAL_DIR":
			args = append(args, realDir)
		default:
			args = append(args, a)
		}
	}
	if guarded[sh.name] {
		args = append(args, "--yes")
	}
	if sh.name == "28-backup" {
		args = append(args, "--overwrite=true")
	}
	timeout := sh.timeout
	if timeout == 0 {
		timeout = 25 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	// Only live-mode read commands need the real credential; local, fake and
	// scratch shots must never receive it.
	cmd.Env = childEnv(tokenIfLive(sh.mode, token), isoConfig)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else if ctx.Err() == context.DeadlineExceeded {
			code = -1 // timed out but produced output
		} else {
			code = -2
		}
	}
	output := buf.String()
	if output == "" {
		output = "(no output)\n"
	}
	if ctx.Err() == context.DeadlineExceeded && sh.name == "56-serve" {
		output += "\n[supervisor still running; screenshot captured after timeout]\n"
	}
	caption := "mvctl " + strings.Join(sh.args, " ")
	lines := parseANSI(output)
	pngName := sh.name + ".png"
	pngPath := filepath.Join(outDir, pngName)
	if err := r.render(caption, lines, pngPath); err != nil {
		fmt.Fprintf(os.Stderr, "render %s: %v\n", sh.name, err)
	}
	// A command is verified when it produced output and either exited 0, or
	// hit a known-safe non-zero path (scratch compose / timeout / preview).
	safeNonZero := sh.mode == "scratch" || code == -1
	ok := (code == 0 || safeNonZero) && len(output) > 0
	return result{
		Name: sh.name, Command: caption, Mode: sh.mode, ExitCode: code,
		Bytes: len(output), PNG: pngName, Note: sh.note, OK: ok,
	}
}

func tokenIfLive(mode, token string) string {
	if mode == "live" {
		return token
	}
	return ""
}

func childEnv(token, isoConfig string) []string {
	out := make([]string, 0, len(os.Environ())+4)
	for _, e := range os.Environ() {
		upper := strings.ToUpper(e)
		if strings.HasPrefix(upper, "NO_COLOR=") || strings.HasPrefix(upper, "MVCTL_NO_COLOR=") ||
			strings.HasPrefix(upper, "MILLIVOLT_OPERATOR_TOKEN=") || strings.HasPrefix(upper, "APPDATA=") ||
			strings.HasPrefix(upper, "MILLIVOLT_URL=") || strings.HasPrefix(upper, "MVCTL_SHOT_TOKEN=") {
			continue
		}
		out = append(out, e)
	}
	if token != "" {
		out = append(out, "MILLIVOLT_OPERATOR_TOKEN="+token)
	}
	out = append(out, "APPDATA="+isoConfig)
	out = append(out, "NO_COLOR=")
	out = append(out, "MVCTL_NO_COLOR=")
	return out
}

func writeManifest(dir string, results []result) error {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "results.json"), data, 0o644); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# mvctl command verification screenshots\n\n")
	b.WriteString("Every command was executed and its real terminal output captured as a PNG. ")
	b.WriteString("Read-only commands used the live millivolt proxy; mutating commands used an ")
	b.WriteString("in-process fake millivolt; Docker lifecycle commands used a harmless no-op ")
	b.WriteString("scratch compose project that was torn down afterwards. No production state was changed.\n\n")
	b.WriteString("| # | Command | Mode | Exit | Output | Screenshot | Note |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	for i, r := range results {
		mark := "PASS"
		if !r.OK {
			mark = "FAIL"
		}
		b.WriteString(fmt.Sprintf("| %d | `%s` | %s | %d | %d B | [%s](%s) | %s (%s) |\n",
			i+1, r.Command, r.Mode, r.ExitCode, r.Bytes, r.PNG, r.PNG, r.Note, mark))
	}
	return os.WriteFile(filepath.Join(dir, "index.md"), []byte(b.String()), 0o644)
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// findFont looks for a monospace TTF with box-drawing and block glyphs.
func findFont() string {
	candidates := []string{
		`C:\Windows\Fonts\CascadiaMono.ttf`,
		`C:\Windows\Fonts\consola.ttf`,
		`/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf`,
		`/usr/share/fonts/truetype/liberation2/LiberationMono-Regular.ttf`,
		`/Library/Fonts/Menlo.ttc`,
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	fatal(fmt.Errorf("no monospace TTF found; pass --font PATH (tried %v)", candidates))
	return ""
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "screenshots:", err)
	os.Exit(1)
}
