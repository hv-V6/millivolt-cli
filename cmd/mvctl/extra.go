package main

// Standalone-only analysis and presentation commands. These are NOT part of the
// small upstream contribution: they add no server surface, but they do make the
// CLI a proper tool for reading and showing off millivolt.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hv-V6/mvctl/internal/config"
	"github.com/hv-V6/mvctl/internal/themes"
	"github.com/hv-V6/mvctl/internal/tui"
)

func ExtraCommands() []Command {
	return []Command{
		{Name: "chart", Group: "view", Usage: "mvctl chart [--metric req|cost|tokens]", Summary: "ASCII time series over history buckets", Run: cmdChart},
		{Name: "explore", Group: "view", Usage: "mvctl explore --by provider|model|client|status|error", Summary: "Ranked breakdown by one dimension", Run: cmdExplore},
		{Name: "cost", Group: "view", Usage: "mvctl cost [--by model|provider]", Summary: "Spend attribution and cache savings", Run: cmdCost},
		{Name: "models", Group: "view", Usage: "mvctl models", Summary: "Model leaderboard", Run: cmdModels},
		{Name: "providers", Group: "view", Usage: "mvctl providers", Summary: "Provider health and active limits", Run: cmdProviders},
		{Name: "trace", Group: "view", Usage: "mvctl trace <request-id>", Summary: "Full detail for one request", Run: cmdTrace},
		{Name: "themes", Group: "show", Usage: "mvctl themes list|preview", Summary: "List or preview millivolt palettes", Run: cmdThemes},
		{Name: "profiles", Group: "manage", Usage: "mvctl profiles list|add|remove|use", Summary: "Manage named deployments", Run: cmdProfiles},
		{Name: "wait", Group: "manage", Usage: "mvctl wait --healthy --timeout 90s", Summary: "Block until the proxy is healthy", Run: cmdWait},
	}
}

func cmdChart(ctx Context) (Result, error) {
	ch, err := ctx.C.Chart(ctx.Ctx, nil)
	if err != nil {
		return Result{Code: 1}, err
	}
	metric := flag(ctx, "metric", "req")
	series := make([]float64, 0, len(ch.Buckets))
	rows := make([][]string, 0, len(ch.Buckets))
	for _, b := range ch.Buckets {
		var v float64
		switch metric {
		case "cost":
			v = b.Cost
		case "tokens":
			v = b.In + b.Out
		case "err", "errors":
			v = b.Err
		default:
			v = b.Req
		}
		series = append(series, v)
		rows = append(rows, []string{
			time.UnixMilli(int64(b.T)).Format("2006-01-02"),
			num(b.Req), num(b.Err), cost(b.Cost), num(b.In + b.Out),
		})
	}
	pretty := heading("chart — "+metric) + "\n" + spark(series, 60) + "\n\n"
	pretty += table([]string{"bucket", "requests", "errors", "cost", "tokens"}, rows)
	return Result{Pretty: pretty, Data: map[string]any{"metric": metric, "series": series, "buckets": ch.Buckets}}, nil
}

func explorerDim(ctx Context, fallback string) string {
	d := flag(ctx, "by", fallback)
	if d == "" {
		d = fallback
	}
	switch d {
	case "provider", "model", "client", "status", "error", "conversation", "tool", "key", "time":
		return d
	default:
		return fallback
	}
}

func cmdExplore(ctx Context) (Result, error) {
	dim := explorerDim(ctx, "provider")
	limit := 12
	if v := flag(ctx, "limit", ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	ex, err := ctx.C.Explorer(ctx.Ctx, dim, nil)
	if err != nil {
		return Result{Code: 1}, err
	}
	groups := ex.Groups
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].N > groups[j].N })
	if len(groups) > limit {
		groups = groups[:limit]
	}
	max := 0.0
	for _, g := range groups {
		if g.N > max {
			max = g.N
		}
	}
	var lines []string
	rows := make([][]string, 0, len(groups))
	for _, g := range groups {
		lines = append(lines, bar(g.Name, g.N, max, 28))
		rows = append(rows, []string{g.Name, num(g.N), cost(g.Cost), num(g.In + g.Out), num(g.Err)})
	}
	pretty := heading(fmt.Sprintf("explore — by %s (total %s, errors %s)", dim, num(ex.Total), num(ex.ErrorTotal)))
	pretty += "\n" + strings.Join(lines, "\n") + "\n\n"
	pretty += table([]string{dim, "requests", "cost", "tokens", "errors"}, rows)
	return Result{Pretty: pretty, Data: map[string]any{"dim": dim, "total": ex.Total, "error_total": ex.ErrorTotal, "groups": groups}}, nil
}

func cmdCost(ctx Context) (Result, error) {
	by := flag(ctx, "by", "model")
	s, err := ctx.C.Bootstrap(ctx.Ctx)
	if err != nil {
		return Result{Code: 1}, err
	}
	ex, err := ctx.C.Explorer(ctx.Ctx, explorerDim(ctx, by), nil)
	if err != nil {
		return Result{Code: 1}, err
	}
	groups := ex.Groups
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].Cost > groups[j].Cost })
	rows := make([][]string, 0, len(groups))
	for i, g := range groups {
		if i >= 15 {
			break
		}
		share := 0.0
		if s.KPI.Cost > 0 {
			share = g.Cost / s.KPI.Cost * 100
		}
		rows = append(rows, []string{truncate(g.Name, 34), cost(g.Cost), fmt.Sprintf("%.1f%%", share), num(g.N), num(g.Cache)})
	}
	pretty := heading("cost attribution") + "\n" + kv([][2]string{
		{"total cost", cost(s.KPI.Cost)},
		{"cost / request", cost(s.KPI.CostPerReq)},
		{"cost / MTok", cost(s.KPI.CostPerMTok)},
		{"cache-read tokens", num(s.KPI.CacheReadTokens)},
		{"reasoning tokens", num(s.KPI.ReasoningTokens)},
	}) + "\n\n" + table([]string{"by " + by, "cost", "share", "requests", "cache read"}, rows)
	return Result{Pretty: pretty, Data: map[string]any{"total": s.KPI.Cost, "groups": groups}}, nil
}

func cmdModels(ctx Context) (Result, error) {
	ex, err := ctx.C.Explorer(ctx.Ctx, "model", nil)
	if err != nil {
		return Result{Code: 1}, err
	}
	groups := ex.Groups
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].N > groups[j].N })
	rows := make([][]string, 0, len(groups))
	for i, g := range groups {
		if i >= 20 {
			break
		}
		rows = append(rows, []string{truncate(g.Name, 40), num(g.N), cost(g.Cost), num(g.In + g.Out), num(g.Err)})
	}
	return Result{
		Pretty: heading("models") + "\n" + table([]string{"model", "requests", "cost", "tokens", "errors"}, rows),
		Data:   map[string]any{"groups": groups},
	}, nil
}

func cmdProviders(ctx Context) (Result, error) {
	ex, err := ctx.C.Explorer(ctx.Ctx, "provider", nil)
	if err != nil {
		return Result{Code: 1}, err
	}
	pause, _ := ctx.C.Get(ctx.Ctx, "/admin/pause", nil)
	throttle, _ := ctx.C.Get(ctx.Ctx, "/admin/throttle", nil)
	quota, _ := ctx.C.Get(ctx.Ctx, "/admin/quota", nil)
	groups := ex.Groups
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].N > groups[j].N })
	rows := make([][]string, 0, len(groups))
	for _, g := range groups {
		rate := 0.0
		if g.N > 0 {
			rate = g.Err / g.N * 100
		}
		rows = append(rows, []string{truncate(g.Name, 28), num(g.N), fmt.Sprintf("%.1f%%", rate), cost(g.Cost), num(g.In + g.Out)})
	}
	pretty := heading("providers") + "\n" + table([]string{"provider", "requests", "error rate", "cost", "tokens"}, rows)
	pretty += "\n\n" + heading("operator state") + "\n" + kv([][2]string{
		{"paused", fmt.Sprintf("%v", pause["paused"])},
		{"queued", fmt.Sprintf("%v", pause["queued"])},
		{"throttles", fmt.Sprintf("%v", len(asSlice(throttle["throttles"])))},
		{"quota / storm", fmt.Sprintf("%v", quota["ok"])},
	})
	return Result{Pretty: pretty, Data: map[string]any{"groups": groups, "pause": pause, "throttle": throttle, "quota": quota}}, nil
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

func cmdTrace(ctx Context) (Result, error) {
	if len(ctx.Args) == 0 {
		return Result{Code: 2}, fmt.Errorf("usage: mvctl trace <request-id>")
	}
	id := strings.ReplaceAll(ctx.Args[0], "'", "''")
	sql := fmt.Sprintf("SELECT * FROM requests WHERE id = '%s' LIMIT 1", id)
	rows, err := ctx.C.Query(ctx.Ctx, sql)
	if err != nil {
		return Result{Code: 1}, err
	}
	return Result{Pretty: heading("trace "+ctx.Args[0]) + "\n" + toJSON(rows), Data: rows}, nil
}

func cmdThemes(ctx Context) (Result, error) {
	sub := "list"
	if len(ctx.Args) > 0 {
		sub = ctx.Args[0]
	}
	switch sub {
	case "list":
		rows := make([][]string, 0, len(themes.Themes()))
		for _, t := range themes.Themes() {
			rows = append(rows, []string{t.ID, t.Label, t.Hint})
		}
		pretty := heading("millivolt themes") + "\n" + table([]string{"id", "name", "hint"}, rows)
		pretty += "\n\n" + kv([][2]string{
			{"modes", strings.Join(themes.Modes(), ", ")},
			{"default", "instrument / black"},
			{"usage", "mvctl --theme midnight --mode black top"},
		})
		return Result{Pretty: pretty, Data: map[string]any{"themes": themes.Themes(), "modes": themes.Modes()}}, nil
	case "preview":
		theme := flag(ctx, "theme", ctx.G.Theme)
		mode := flag(ctx, "mode", ctx.G.Mode)
		t, m := themes.Normalize(theme, mode)
		p := themes.Get(t, m)
		var b strings.Builder
		b.WriteString(heading(fmt.Sprintf("theme preview — %s / %s", t, m)) + "\n")
		for _, pair := range [][2]string{
			{"bg", p.Bg}, {"panel", p.Panel}, {"panel2", p.Panel2},
			{"rail", p.Rail}, {"border", p.Border}, {"accent", p.Accent},
			{"accent2", p.Accent2}, {"cyan", p.Cyan}, {"text", p.Text},
			{"ok", p.Ok}, {"warn", p.Warn}, {"err", p.Err},
		} {
			b.WriteString(fmt.Sprintf("  %s%s  %s\n",
				pad(pair[0], 10, false),
				tui.BGRGB("        ", pair[1]),
				tui.RGB(pair[1], p.Accent)))
		}
		b.WriteString("\n" + tui.RGB("  mvctl  ", p.Accent) + tui.BGRGB("  top / chart / requests  ", p.Panel2) + "\n")
		b.WriteString(tui.RGB("  ● healthy   requests "+num(39275)+"   cost "+cost(28.69), p.Text) + "\n")
		return Result{Pretty: b.String(), Data: map[string]any{"theme": t, "mode": m, "palette": p}}, nil
	default:
		return Result{Code: 2}, fmt.Errorf("usage: mvctl themes list|preview")
	}
}

func cmdProfiles(ctx Context) (Result, error) {
	f, _, err := config.Load()
	if err != nil {
		return Result{Code: 1}, err
	}
	sub := "list"
	if len(ctx.Args) > 0 {
		sub = ctx.Args[0]
	}
	switch sub {
	case "list":
		names := make([]string, 0, len(f.Profiles))
		for n := range f.Profiles {
			names = append(names, n)
		}
		sort.Strings(names)
		rows := make([][]string, 0, len(names)+1)
		for _, n := range names {
			p := f.Profiles[n]
			mark := ""
			if n == f.DefaultProfile {
				mark = "*"
			}
			rows = append(rows, []string{mark, n, p.URL, p.Dir})
		}
		return Result{Pretty: heading("profiles") + "\n" + table([]string{"", "name", "url", "dir"}, rows), Data: map[string]any{"default": f.DefaultProfile, "profiles": f.Profiles}}, nil
	case "add":
		if r, proceed, err := guarded(ctx, "add a profile"); !proceed {
			return r, err
		}
		name := flag(ctx, "name", "")
		if name == "" && len(ctx.Args) > 1 {
			name = ctx.Args[1]
		}
		if name == "" {
			return Result{Code: 2}, fmt.Errorf("--name is required")
		}
		f.Profiles[name] = config.Profile{
			URL: flag(ctx, "url", ""), Dir: flag(ctx, "dir", ""),
			Token: flag(ctx, "token", ""), Theme: flag(ctx, "theme", ""),
			Mode: flag(ctx, "mode", ""), Port: intFromFlag(ctx, "port"),
		}
		if err := config.SaveFile(f); err != nil {
			return Result{Code: 1}, err
		}
		return Result{Pretty: green("profile added ") + name, Data: map[string]any{"name": name}}, nil
	case "remove":
		if r, proceed, err := guarded(ctx, "remove a profile"); !proceed {
			return r, err
		}
		name := flag(ctx, "name", "")
		if name == "" && len(ctx.Args) > 1 {
			name = ctx.Args[1]
		}
		delete(f.Profiles, name)
		if f.DefaultProfile == name {
			f.DefaultProfile = "default"
		}
		if err := config.SaveFile(f); err != nil {
			return Result{Code: 1}, err
		}
		return Result{Pretty: green("profile removed ") + name, Data: map[string]any{"removed": name}}, nil
	case "use":
		if r, proceed, err := guarded(ctx, "change the default profile"); !proceed {
			return r, err
		}
		name := flag(ctx, "name", "")
		if name == "" && len(ctx.Args) > 1 {
			name = ctx.Args[1]
		}
		if name == "" {
			return Result{Code: 2}, fmt.Errorf("--name is required")
		}
		f.DefaultProfile = name
		if err := config.SaveFile(f); err != nil {
			return Result{Code: 1}, err
		}
		return Result{Pretty: green("default profile: ") + name, Data: map[string]any{"default": name}}, nil
	default:
		return Result{Code: 2}, fmt.Errorf("usage: mvctl profiles list|add|remove|use")
	}
}

func intFromFlag(ctx Context, name string) int {
	v := flag(ctx, name, "")
	if v == "" {
		return 0
	}
	n, _ := strconv.Atoi(v)
	return n
}

func cmdWait(ctx Context) (Result, error) {
	timeout := 90 * time.Second
	if v := flag(ctx, "timeout", ""); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			timeout = d
		} else if n, err := strconv.Atoi(v); err == nil {
			timeout = time.Duration(n) * time.Second
		}
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.C.Health(ctx.Ctx) {
			return Result{Pretty: green("healthy") + " " + ctx.G.BaseURL, Data: map[string]any{"healthy": true}}, nil
		}
		select {
		case <-ctx.Ctx.Done():
			return Result{Code: 1}, ctx.Ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return Result{Code: 1}, fmt.Errorf("millivolt did not become healthy within %s", timeout)
}
