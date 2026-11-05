package main

// The "show it off" layer: a live TUI with millivolt's own palettes, a traffic
// generator, a load benchmark, a one-shot chat client, a scripted tour and a
// token-injecting dashboard proxy.

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/hv-V6/mvctl/internal/demo"
	"github.com/hv-V6/mvctl/internal/themes"
	"github.com/hv-V6/mvctl/internal/tui"
)

func ShowcaseCommands() []Command {
	return []Command{
		{Name: "top", Group: "show", Usage: "mvctl top [--demo] [--theme NAME] [--mode NAME]", Summary: "Live full-screen terminal dashboard", Run: cmdTop},
		{Name: "demo", Group: "show", Usage: "mvctl demo [--requests N] [--stream]", Summary: "Generate labeled showcase traffic", Run: cmdDemo},
		{Name: "bench", Group: "show", Usage: "mvctl bench --requests N --concurrency C", Summary: "Load test and latency percentiles", Run: cmdBench},
		{Name: "chat", Group: "show", Usage: "mvctl chat \"prompt\" --upstream URL", Summary: "One-shot request through the proxy", Run: cmdChat},
		{Name: "tour", Group: "show", Usage: "mvctl tour [--no-delay]", Summary: "Scripted walkthrough for a recording", Run: cmdTour},
		{Name: "dashboard", Group: "show", Usage: "mvctl dashboard [--print]", Summary: "Open the full dashboard without a token", Run: cmdDashboard},
	}
}

// topFetch converts millivolt wire data into the TUI's plain data struct.
func topFetch(ctx Context, history *[]float64, demoMode bool, tick *int) func(context.Context) (tui.TopData, error) {
	return func(c context.Context) (tui.TopData, error) {
		*tick++
		if demoMode {
			return demoTopData(*tick, ctx.G.Theme, ctx.G.Mode), nil
		}
		s, err := ctx.C.Bootstrap(c)
		if err != nil {
			return tui.TopData{}, err
		}
		page, err := ctx.C.Requests(c, 20, 0, "")
		if err != nil {
			return tui.TopData{}, err
		}
		ex, _ := ctx.C.Explorer(c, "provider", nil)
		last := 0.0
		if len(*history) > 0 {
			last = (*history)[len(*history)-1]
		}
		delta := s.KPI.Requests - last
		if delta < 0 {
			delta = 0
		}
		*history = append(*history, s.KPI.Requests)
		if len(*history) > 90 {
			*history = (*history)[len(*history)-90:]
		}
		series := make([]float64, 0, len(*history)-1)
		for i := 1; i < len(*history); i++ {
			series = append(series, (*history)[i]-(*history)[i-1])
		}
		if len(series) < 2 {
			// A one-shot snapshot has no live samples yet. Seed the sparkline
			// from the durable history buckets so screenshots still look alive.
			if ch, err := ctx.C.Chart(c, nil); err == nil && len(ch.Buckets) > 1 {
				series = make([]float64, 0, len(ch.Buckets))
				for _, b := range ch.Buckets {
					series = append(series, b.Req)
				}
			} else {
				series = []float64{delta, delta * 0.8, delta * 1.2, delta}
			}
		}
		d := tui.TopData{
			URL: ctx.G.BaseURL, Version: shortVersion(s.DashboardVersion),
			Theme: ctx.G.Theme, Mode: ctx.G.Mode,
			Healthy: true, Paused: s.Pause.Paused,
			Requests: s.KPI.Requests, Errors: s.KPI.Errors, InFlight: s.KPI.InFlight,
			Cost: s.KPI.Cost, InputTokens: s.KPI.InputTokens, OutputTokens: s.KPI.OutputTokens,
			CacheReadTokens: s.KPI.CacheReadTokens,
			AvgTTFTMs:       s.KPI.AvgTTFTMs, AvgTPS: s.KPI.AvgTPS,
			Dropped: s.Storage.Dropped, StorageEnabled: s.Storage.Enabled,
			ThrottleActive: len(s.Throttle.Throttles), DebugEnabled: s.Debug.Enabled,
			History: series,
		}
		for _, g := range ex.Groups {
			d.Providers = append(d.Providers, tui.BarItem{Label: g.Name, Value: g.N})
		}
		for _, r := range page.Rows {
			errText := r.ErrorType
			if errText == "" && r.Status >= 400 {
				errText = fmt.Sprintf("HTTP %d", r.Status)
			}
			d.Rows = append(d.Rows, tui.Row{
				Time: shortTime(r.Start), Status: fmt.Sprintf("%d", r.Status),
				Provider: truncate(r.Provider, 14), Model: truncate(r.Model, 18),
				Client: truncate(r.Client, 14), Duration: dur(r.DurationMs),
				Tokens: num(r.InputTokens + r.OutputTokens), Error: errText,
			})
		}
		return d, nil
	}
}

// demoTopData produces a believable, entirely synthetic frame. It writes
// nothing to millivolt, so it is safe for screenshots and recordings.
func demoTopData(tick int, theme, mode string) tui.TopData {
	models := []string{"claude-sonnet-4.5", "gpt-5.6-sol", "hy4", "deepseek-v4-pro", "grok-4"}
	providers := []string{"openrouter", "opencode-zen", "opencode-go", "inferx", "pareto"}
	clients := []string{"claude-code", "codex", "dsh", "pi", "curl/8.21.0"}
	base := 39000.0 + float64(tick)*7
	series := make([]float64, 60)
	for i := range series {
		series[i] = 8 + 6*float64((i+tick)%7)/7 + float64((i*13+tick*7)%11)
	}
	d := tui.TopData{
		URL: "http://127.0.0.1:8471", Version: "4c94a75604d6",
		Theme: theme, Mode: mode, Healthy: true, Demo: true,
		Requests: base, Errors: 108 + float64(tick%3), InFlight: float64(tick % 5),
		Cost: 28.69 + float64(tick)*0.004, InputTokens: 9.9e9 + float64(tick)*1e6,
		OutputTokens: 2.87e7 + float64(tick)*1e4, CacheReadTokens: 9.77e9,
		AvgTTFTMs: floatPtr(3674.28), AvgTPS: floatPtr(270.10),
		StorageEnabled: true, ThrottleActive: 0, DebugEnabled: false,
		History: series,
	}
	for i, p := range providers {
		d.Providers = append(d.Providers, tui.BarItem{Label: p, Value: 16000 - float64(i)*2600 + float64(tick%7)*30})
	}
	for i := 0; i < 9; i++ {
		status := 200
		errText := ""
		if (i+tick)%11 == 0 {
			status = 429
			errText = "rate_limited"
		}
		d.Rows = append(d.Rows, tui.Row{
			Time:     fmt.Sprintf("10:%02d:%02d", 17+i, (i*7+tick)%60),
			Status:   fmt.Sprintf("%d", status),
			Provider: providers[(i+tick)%len(providers)], Model: models[(i+tick)%len(models)],
			Client:   clients[(i+tick)%len(clients)],
			Duration: dur(float64(420 + (i*137+tick*29)%2100)),
			Tokens:   num(float64(1200 + (i*911+tick*37)%18000)), Error: errText,
		})
	}
	return d
}

func floatPtr(v float64) *float64 { return &v }

func cmdTop(ctx Context) (Result, error) {
	demoMode := flag(ctx, "demo", "") == "true"
	theme, mode := themes.Normalize(ctx.G.Theme, ctx.G.Mode)
	refresh := 1000
	if v := flag(ctx, "refresh", ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 200 {
			refresh = n
		}
	}
	nonTTY := flag(ctx, "no-tty", "") == "true" || !isTerminal(os.Stdout) || ctx.G.JSON
	history := []float64{}
	tick := 0
	opts := tui.Options{
		Theme: theme, Mode: mode, RefreshMS: refresh,
		Demo: demoMode, NonTTY: nonTTY, ShowHelp: false,
	}
	if ctx.G.JSON {
		d, err := topFetch(ctx, &history, demoMode, &tick)(ctx.Ctx)
		if err != nil {
			return Result{Code: 1}, err
		}
		return Result{Pretty: toJSON(d), Data: d}, nil
	}
	if nonTTY {
		d, err := topFetch(ctx, &history, demoMode, &tick)(ctx.Ctx)
		if err != nil {
			return Result{Code: 1}, err
		}
		frame := tui.RenderFrame(d, themes.Get(theme, mode), 100, 32, false)
		return Result{Pretty: frame, Data: map[string]any{"top": "snapshot", "theme": theme, "mode": mode}}, nil
	}
	if err := tui.RunTop(ctx.Ctx, topFetch(ctx, &history, demoMode, &tick), opts); err != nil {
		return Result{Code: 1}, err
	}
	return Result{Pretty: "", Data: map[string]any{"top": "closed", "theme": theme, "mode": mode}}, nil
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func demoConfig(ctx Context, requests int) demo.Config {
	return demo.Config{
		BaseURL:  ctx.G.BaseURL,
		Client:   flag(ctx, "client", "mvctl-demo"),
		Upstream: flag(ctx, "upstream", ""),
		Requests: requests, Concurrency: intFromFlagDefault(ctx, "concurrency", 4),
		Stream:    flag(ctx, "stream", "") == "true",
		ErrorRate: floatFlag(ctx, "error-rate", 0),
		TimeoutMS: 15_000,
	}
}

func intFromFlagDefault(ctx Context, name string, fallback int) int {
	if v := flag(ctx, name, ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func floatFlag(ctx Context, name string, fallback float64) float64 {
	if v := flag(ctx, name, ""); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

func cmdDemo(ctx Context) (Result, error) {
	if !wantYes(ctx) {
		return Result{Pretty: yellow("Preview only. Re-run with --yes to generate live showcase traffic through millivolt (labeled 'mvctl-demo')."),
			Data: map[string]any{"applied": false}}, nil
	}
	cfg := demoConfig(ctx, intFromFlagDefault(ctx, "requests", 12))
	r, err := demo.Run(ctx.Ctx, cfg)
	if err != nil {
		return Result{Code: 1}, err
	}
	pretty := heading("demo traffic") + "\n" + kv([][2]string{
		{"sent", num(float64(r.Sent))}, {"succeeded", num(float64(r.Succeeded))},
		{"failed", num(float64(r.Failed))}, {"p50 / p95 / p99", fmt.Sprintf("%s / %s / %s", dur(r.P50), dur(r.P95), dur(r.P99))},
		{"elapsed", dur(r.Elapsed * 1000)}, {"client label", cfg.Client},
	}) + "\n\n" + dim("Clean up with: mvctl purge --client "+cfg.Client+" --yes")
	return Result{Pretty: pretty, Data: r}, nil
}

func cmdBench(ctx Context) (Result, error) {
	if !wantYes(ctx) {
		return Result{Pretty: yellow("Preview only. Re-run with --yes to run the load benchmark."), Data: map[string]any{"applied": false}}, nil
	}
	cfg := demoConfig(ctx, intFromFlagDefault(ctx, "requests", 50))
	cfg.Client = flag(ctx, "client", "mvctl-bench")
	r, err := demo.Run(ctx.Ctx, cfg)
	if err != nil {
		return Result{Code: 1}, err
	}
	rows := [][]string{
		{"requests", num(float64(r.Sent)), "concurrency", strconv.Itoa(cfg.Concurrency)},
		{"succeeded", num(float64(r.Succeeded)), "failed", num(float64(r.Failed))},
		{"p50", dur(r.P50), "p95", dur(r.P95)},
		{"p99", dur(r.P99), "min / max", dur(r.Min) + " / " + dur(r.Max)},
		{"ttft", dur(r.TTFTMs), "elapsed", dur(r.Elapsed * 1000)},
	}
	return Result{Pretty: heading("bench") + "\n" + table([]string{"metric", "value", "metric", "value"}, rows), Data: r}, nil
}

func cmdChat(ctx Context) (Result, error) {
	prompt := strings.Join(ctx.Args, " ")
	if prompt == "" {
		prompt = "Say hello from mvctl."
	}
	upstream := flag(ctx, "upstream", "")
	if upstream == "" {
		return Result{Code: 2}, fmt.Errorf("usage: mvctl chat \"prompt\" --upstream URL  (for a local mock, run 'mvctl demo' first)")
	}
	if r, proceed, err := guarded(ctx, "send an inference request (may incur upstream cost)"); !proceed {
		return r, err
	}
	text, err := demo.Chat(ctx.Ctx, demo.ChatConfig{
		BaseURL: ctx.G.BaseURL, Upstream: upstream,
		Model: flag(ctx, "model", "mvctl-demo-small"), Prompt: prompt,
		Client: flag(ctx, "client", "mvctl-chat"),
		APIKey: flag(ctx, "key", "mvctl-demo-key"),
		Stream: flag(ctx, "stream", "") == "true", TimeoutMS: 30_000,
	})
	if err != nil {
		return Result{Code: 1}, err
	}
	return Result{Pretty: bold("assistant") + "\n" + text, Data: map[string]any{"response": text}}, nil
}

func cmdTour(ctx Context) (Result, error) {
	delay := 700 * time.Millisecond
	if flag(ctx, "no-delay", "") == "true" {
		delay = 0
	}
	steps := []struct {
		label string
		run   func(Context) (Result, error)
	}{
		{"status", cmdStatus}, {"overview", cmdOverview},
		{"requests", func(c Context) (Result, error) { c.Args = []string{"5"}; return cmdRequests(c) }},
		{"chart", cmdChart},
		{"explore", func(c Context) (Result, error) { c.Flags["by"] = "provider"; return cmdExplore(c) }},
	}
	var b strings.Builder
	b.WriteString(heading("mvctl tour — millivolt from the terminal") + "\n\n")
	for i, step := range steps {
		b.WriteString(bold(fmt.Sprintf("%d/%d  %s", i+1, len(steps), step.label)) + "\n")
		r, err := step.run(ctx)
		if err != nil {
			b.WriteString(dim("(unavailable: "+err.Error()+")") + "\n")
		} else {
			b.WriteString(r.Pretty + "\n")
		}
		b.WriteString("\n")
		if delay > 0 {
			time.Sleep(delay)
		}
	}
	b.WriteString(cyan("Next: mvctl top --theme midnight --mode black") + "\n")
	return Result{Pretty: strings.TrimRight(b.String(), "\n"), Data: map[string]any{"steps": len(steps)}}, nil
}

func cmdDashboard(ctx Context) (Result, error) {
	port := 8472
	if v := flag(ctx, "port", ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			port = n
		}
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	// The Host header is NOT an auth boundary: any local process can set it.
	// Generate a one-time secret, exchange it for an HttpOnly cookie on the
	// browser's first navigation, and require that cookie on every later
	// request (including same-origin dashboard assets and API calls).
	secret, err := dashboardNonce()
	if err != nil {
		return Result{Code: 1}, err
	}
	url := "http://" + addr + "/?mvctl=" + secret
	if flag(ctx, "print", "") == "true" || ctx.G.JSON {
		return Result{Pretty: url, Data: map[string]any{"url": url, "port": port, "one_time_secret": true}}, nil
	}
	if ctx.G.Token == "" {
		return Result{Code: 1}, fmt.Errorf("operator token is not configured; set MILLIVOLT_OPERATOR_TOKEN or --token")
	}
	handler, err := dashboardHandler(addr, port, secret, ctx.G.Token, ctx.G.BaseURL)
	if err != nil {
		return Result{Code: 1}, err
	}
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	listenErr := make(chan error, 1)
	go func() { listenErr <- srv.ListenAndServe() }()
	// Wait briefly for the listener so a busy port is reported, not ignored.
	select {
	case err := <-listenErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return Result{Code: 1}, fmt.Errorf("dashboard proxy failed to listen on %s: %w", addr, err)
		}
	case <-time.After(250 * time.Millisecond):
	}
	openBrowser(url)
	fmt.Fprintln(os.Stderr, dim("dashboard proxy on "+url+" (Ctrl-C to stop)"))
	<-ctx.Ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
	return Result{Pretty: dim("dashboard closed"), Data: map[string]any{"closed": true}}, nil
}

// dashboardHandler builds the guarded loopback proxy. It is a separate
// function so the auth gate can be exercised without launching a browser.
func dashboardHandler(addr string, port int, secret, token, baseURL string) (http.Handler, error) {
	target, err := http.NewRequest(http.MethodGet, baseURL, nil)
	if err != nil {
		return nil, err
	}
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.URL.Scheme
			req.URL.Host = target.URL.Host
			req.Host = target.URL.Host
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Del("Origin")
			req.Header.Del("Referer")
		},
		ModifyResponse: func(resp *http.Response) error {
			for _, h := range []string{"Content-Security-Policy", "Content-Security-Policy-Report-Only", "X-Frame-Options", "Referrer-Policy"} {
				resp.Header.Del(h)
			}
			return nil
		},
	}
	const cookieName = "mvctl_dashboard"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != addr && r.Host != "localhost:"+strconv.Itoa(port) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("mvctl")), []byte(secret)) == 1 {
			http.SetCookie(w, &http.Cookie{
				Name: cookieName, Value: secret, Path: "/",
				HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 12 * 3600,
			})
			// Drop the secret from the address bar before serving the page.
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		cookie, cookieErr := r.Cookie(cookieName)
		if cookieErr != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(secret)) != 1 {
			http.Error(w, "forbidden: open the URL printed by mvctl dashboard", http.StatusForbidden)
			return
		}
		proxy.ServeHTTP(w, r)
	}), nil
}

func dashboardNonce() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating dashboard secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	// Wait reaps the launcher process instead of leaving a zombie.
	_ = cmd.Run()
}
