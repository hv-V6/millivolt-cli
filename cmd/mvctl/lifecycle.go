package main

// Deployment lifecycle commands (Docker). Standalone-only: the small upstream
// contribution deliberately manages no containers.

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hv-V6/mvctl/internal/config"
	"github.com/hv-V6/mvctl/internal/docker"
	"github.com/hv-V6/mvctl/internal/provision"
)

func LifecycleCommands() []Command {
	return []Command{
		{Name: "discover", Group: "manage", Usage: "mvctl discover", Summary: "Find an existing millivolt install", Run: cmdDiscover},
		{Name: "setup", Group: "manage", Usage: "mvctl setup [--dir PATH] [--port N]", Summary: "Install a fresh millivolt deployment", Run: cmdSetup},
		{Name: "adopt", Group: "manage", Usage: "mvctl adopt --dir PATH --port N", Summary: "Record an existing deployment", Run: cmdAdopt},
		{Name: "start", Group: "manage", Usage: "mvctl start", Summary: "Start the container", Run: cmdStart},
		{Name: "stop", Group: "manage", Usage: "mvctl stop", Summary: "Stop the container", Run: cmdStop},
		{Name: "up", Group: "manage", Usage: "mvctl up", Summary: "docker compose up -d", Run: cmdUp},
		{Name: "update", Group: "manage", Usage: "mvctl update [--dry-run]", Summary: "Pull and recreate when the image changed", Run: cmdUpdate},
		{Name: "repair", Group: "manage", Usage: "mvctl repair", Summary: "Diagnose and fix a stuck deployment", Run: cmdRepair},
		{Name: "revert", Group: "manage", Usage: "mvctl revert", Summary: "Pin the previous image", Run: cmdRevert},
		{Name: "resume", Group: "manage", Usage: "mvctl resume", Summary: "Track the published image again", Run: cmdResume},
		{Name: "logs", Group: "manage", Usage: "mvctl logs [--tail N]", Summary: "Show deployment logs", Run: cmdLogs},
		{Name: "doctor", Group: "manage", Usage: "mvctl doctor", Summary: "Read-only full diagnostic", Run: cmdDoctor},
		{Name: "images", Group: "manage", Usage: "mvctl images", Summary: "Local image tags and drift", Run: cmdImages},
		{Name: "serve", Group: "manage", Usage: "mvctl serve", Summary: "Foreground health + update supervisor", Run: cmdServe},
	}
}

func deployDir(ctx Context) string {
	if v := flag(ctx, "dir", ""); v != "" {
		return v
	}
	_, s, _ := config.Load()
	return s.DeployDir
}

func composeRun(ctx Context, args ...string) docker.RunResult {
	return docker.Compose(ctx.Ctx, deployDir(ctx), args, 180_000)
}

func cmdDiscover(ctx Context) (Result, error) {
	excluded := []int{8472}
	if p := intFromFlag(ctx, "port"); p > 0 {
		excluded = append(excluded, p)
	}
	d := provision.Discover(ctx.Ctx, provision.DiscoverOptions{ExcludePorts: excluded})
	rows := make([][]string, 0, len(d.Candidates))
	for _, c := range d.Candidates {
		port := "—"
		if c.Port > 0 {
			port = strconv.Itoa(c.Port)
		}
		rows = append(rows, []string{c.Kind, port, c.DeployDir, c.ContainerName, c.Evidence})
	}
	pretty := heading("discover")
	if len(rows) == 0 {
		pretty += "\n" + dim("No existing millivolt found.")
	} else {
		pretty += "\n" + table([]string{"kind", "port", "directory", "container", "evidence"}, rows)
	}
	pretty += fmt.Sprintf("\n\nscanned %d ports, %d directories", len(d.ScannedPorts), d.ScannedDirs)
	if d.Truncated {
		pretty += yellow(" (truncated)")
	}
	// No token values are ever marshalled here: DiscoveryCandidate only carries HasToken.
	return Result{Pretty: pretty, Data: d}, nil
}

func cmdSetup(ctx Context) (Result, error) {
	if !wantYes(ctx) {
		return Result{Pretty: yellow("Preview only. Re-run with --yes to write files and start a container."), Data: map[string]any{"applied": false}}, nil
	}
	if !docker.DockerDaemonUp(ctx.Ctx) {
		return Result{Code: 1}, fmt.Errorf("Docker daemon is not running")
	}
	port := intFromFlag(ctx, "port")
	if port == 0 {
		port = 8471
	}
	dir := flag(ctx, "dir", "")
	if dir == "" {
		dir = provision.DefaultInstallDir(port)
	}
	name := provision.FreeContainerName(ctx.Ctx, "millivolt")
	project := provision.FreeProjectName(ctx.Ctx, name)
	// Let Install preserve an existing deployment credential; only a genuinely
	// fresh install needs a new one, and Install will generate it safely.
	token := ""
	res := provision.Install(ctx.Ctx, provision.InstallOptions{
		Dir: dir, Port: port, ContainerName: name, ProjectName: project, Token: token,
		Log: func(msg string) {
			if ctx.G.Verbose {
				fmt.Fprintln(os.Stderr, dim(msg))
			}
		},
	})
	if !res.OK {
		return Result{Code: 1}, fmt.Errorf("install failed: %s", res.Detail)
	}
	_, s, _ := config.Load()
	s.DeployDir, s.Port, s.ContainerName, s.Token, s.Managed = dir, port, name, token, "installed"
	s.InstalledAt = time.Now().UTC().Format(time.RFC3339)
	s.DesiredRunning = true
	if err := config.SaveState(s); err != nil {
		return Result{Code: 1}, err
	}
	return Result{
		Pretty: green("millivolt installed") + "\n" + kv([][2]string{
			{"directory", dir}, {"port", strconv.Itoa(port)},
			{"container", name}, {"project", project},
			{"compose", res.ComposePath},
		}),
		Data: map[string]any{"applied": true, "dir": dir, "port": port, "container": name, "project": project, "compose": res.ComposePath},
	}, nil
}

func cmdAdopt(ctx Context) (Result, error) {
	if r, proceed, err := guarded(ctx, "record this deployment"); !proceed {
		return r, err
	}
	dir := flag(ctx, "dir", "")
	port := intFromFlag(ctx, "port")
	if dir == "" || port == 0 {
		return Result{Code: 2}, fmt.Errorf("usage: mvctl adopt --dir PATH --port N")
	}
	if _, err := os.Stat(dir); err != nil {
		return Result{Code: 1}, fmt.Errorf("directory not found: %s", dir)
	}
	env := provision.ReadEnv(dir)
	name := env["MILLIVOLT_CONTAINER"]
	if name == "" {
		name = "millivolt"
	}
	token := env["MILLIVOLT_OPERATOR_TOKEN"]
	_, s, _ := config.Load()
	s.DeployDir, s.Port, s.ContainerName, s.Token, s.Managed = dir, port, name, token, "adopted"
	s.InstalledAt = time.Now().UTC().Format(time.RFC3339)
	s.DesiredRunning = true
	if err := config.SaveState(s); err != nil {
		return Result{Code: 1}, err
	}
	return Result{
		Pretty: green("deployment adopted") + "\n" + kv([][2]string{
			{"directory", dir}, {"port", strconv.Itoa(port)},
			{"container", name}, {"token", boolText(token != "")},
		}),
		Data: map[string]any{"applied": true, "dir": dir, "port": port, "container": name, "has_token": token != ""},
	}, nil
}

func cmdStart(ctx Context) (Result, error) {
	if r, proceed, err := guarded(ctx, "start the container"); !proceed {
		return r, err
	}
	if !docker.DockerDaemonUp(ctx.Ctx) {
		return Result{Code: 1}, fmt.Errorf("Docker daemon is not running")
	}
	r := composeRun(ctx, "up", "-d")
	if r.Code != 0 {
		return Result{Code: 1}, fmt.Errorf("compose up failed: %s", docker.LastLine(r.Stderr+r.Stdout))
	}
	_, s, _ := config.Load()
	s.DesiredRunning = true
	_ = config.SaveState(s)
	return Result{Pretty: green("started"), Data: map[string]any{"started": true}}, nil
}

func cmdUp(ctx Context) (Result, error) { return cmdStart(ctx) }

func cmdStop(ctx Context) (Result, error) {
	if r, proceed, err := guarded(ctx, "stop the container"); !proceed {
		return r, err
	}
	r := composeRun(ctx, "stop")
	if r.Code != 0 {
		return Result{Code: 1}, fmt.Errorf("compose stop failed: %s", docker.LastLine(r.Stderr+r.Stdout))
	}
	_, s, _ := config.Load()
	s.DesiredRunning = false
	_ = config.SaveState(s)
	return Result{Pretty: yellow("stopped (sticky until 'mvctl start')"), Data: map[string]any{"stopped": true}}, nil
}

func cmdUpdate(ctx Context) (Result, error) {
	dir := deployDir(ctx)
	if dir == "" {
		return Result{Code: 1}, fmt.Errorf("no deployment recorded; run 'mvctl setup' or 'mvctl adopt' first")
	}
	_, s, _ := config.Load()
	upd := provision.PullDeployment(ctx.Ctx, dir, s.ContainerName)
	dry := flag(ctx, "dry-run", "") == "true" || !wantYes(ctx)
	if upd.Detail != "" {
		return Result{Code: 1}, fmt.Errorf("pull failed: %s", upd.Detail)
	}
	if !upd.Changed {
		return Result{Pretty: green("already up to date"), Data: map[string]any{"changed": false, "applied": false}}, nil
	}
	if dry {
		return Result{Pretty: yellow("A newer image is available. Re-run with --yes to recreate the container."),
			Data: map[string]any{"changed": true, "applied": false, "before": upd.Before, "after": upd.After}}, nil
	}
	r := docker.Compose(ctx.Ctx, dir, append(provision.ComposeArgsFor(dir), "up", "-d"), 180_000)
	if r.Code != 0 {
		return Result{Code: 1}, fmt.Errorf("compose up failed: %s", docker.LastLine(r.Stderr+r.Stdout))
	}
	s.PreviousImage, s.ImageID = upd.Before, upd.After
	s.LastUpdate = time.Now().UTC().Format(time.RFC3339)
	_ = config.SaveState(s)
	return Result{Pretty: green("updated to the newest image"), Data: map[string]any{"changed": true, "applied": true, "before": upd.Before, "after": upd.After}}, nil
}

func cmdRevert(ctx Context) (Result, error) {
	if !wantYes(ctx) {
		return Result{Pretty: yellow("Preview only. Re-run with --yes to pin the previous image."), Data: map[string]any{"applied": false}}, nil
	}
	_, s, _ := config.Load()
	if s.PreviousImage == "" || !provision.ImageExists(ctx.Ctx, s.PreviousImage) {
		return Result{Code: 1}, fmt.Errorf("no usable previous image is recorded")
	}
	if !provision.TagImage(ctx.Ctx, s.PreviousImage, provision.RollbackImage) {
		return Result{Code: 1}, fmt.Errorf("could not tag the previous image")
	}
	dir := deployDir(ctx)
	if err := provision.WriteRollbackOverride(dir); err != nil {
		return Result{Code: 1}, err
	}
	r := docker.Compose(ctx.Ctx, dir, append(provision.ComposeArgsFor(dir), "up", "-d"), 180_000)
	if r.Code != 0 {
		return Result{Code: 1}, fmt.Errorf("compose up failed: %s", docker.LastLine(r.Stderr+r.Stdout))
	}
	s.RolledBackTo, s.ImageID = s.PreviousImage, s.PreviousImage
	_ = config.SaveState(s)
	return Result{Pretty: green("pinned previous image ") + s.RolledBackTo[:min(12, len(s.RolledBackTo))], Data: map[string]any{"applied": true, "pinned": s.RolledBackTo}}, nil
}

func cmdResume(ctx Context) (Result, error) {
	dir := deployDir(ctx)
	if !wantYes(ctx) {
		return Result{Pretty: yellow("Preview only. Re-run with --yes to track the published tag again."), Data: map[string]any{"applied": false}}, nil
	}
	if err := provision.RemoveRollbackOverride(dir); err != nil {
		return Result{Code: 1}, err
	}
	r := docker.Compose(ctx.Ctx, dir, append(provision.ComposeArgsFor(dir), "pull", "--quiet"), 300_000)
	if r.Code != 0 {
		return Result{Code: 1}, fmt.Errorf("pull failed: %s", docker.LastLine(r.Stderr+r.Stdout))
	}
	r = docker.Compose(ctx.Ctx, dir, append(provision.ComposeArgsFor(dir), "up", "-d"), 180_000)
	if r.Code != 0 {
		return Result{Code: 1}, fmt.Errorf("compose up failed: %s", docker.LastLine(r.Stderr+r.Stdout))
	}
	_, s, _ := config.Load()
	s.RolledBackTo = ""
	_ = config.SaveState(s)
	return Result{Pretty: green("tracking the published image again"), Data: map[string]any{"applied": true}}, nil
}

func cmdRepair(ctx Context) (Result, error) {
	if r, proceed, err := guarded(ctx, "diagnose and rewrite the deployment config"); !proceed {
		return r, err
	}
	dir := deployDir(ctx)
	if dir == "" {
		return Result{Code: 1}, fmt.Errorf("no deployment recorded; run setup or adopt first")
	}
	_, s, _ := config.Load()
	var steps []string
	if !docker.DockerDaemonUp(ctx.Ctx) {
		steps = append(steps, "Docker daemon is down; start Docker Desktop first")
		return Result{Code: 1, Pretty: strings.Join(steps, "; ")}, fmt.Errorf("Docker daemon is not running")
	}
	if s.RolledBackTo != "" && !provision.ImageExists(ctx.Ctx, provision.RollbackImage) {
		_ = provision.RemoveRollbackOverride(dir)
		s.RolledBackTo = ""
		_ = config.SaveState(s)
		steps = append(steps, "removed a rollback pin whose image was deleted")
	}
	hostPort, containerPort, ok := provision.PortMapping(ctx.Ctx, s.ContainerName)
	if ok {
		text := provision.ReadContainerFile(ctx.Ctx, s.ContainerName, "/config/proxy.yaml")
		listen := provision.ListenPortOf(text)
		if listen > 0 && listen != containerPort {
			next := provision.WithListenPort(text, containerPort)
			if next != "" && provision.WriteContainerFile(ctx.Ctx, s.ContainerName, "/config/proxy.yaml", next) {
				steps = append(steps, fmt.Sprintf("rewrote listen %d -> %d", listen, containerPort))
				_ = docker.Compose(ctx.Ctx, dir, append(provision.ComposeArgsFor(dir), "restart"), 120_000)
			}
		}
	}
	r := docker.Compose(ctx.Ctx, dir, append(provision.ComposeArgsFor(dir), "up", "-d"), 180_000)
	if r.Code != 0 {
		return Result{Code: 1, Pretty: strings.Join(steps, "; ")}, fmt.Errorf("compose up failed: %s", docker.LastLine(r.Stderr+r.Stdout))
	}
	healthy := waitHealthy(ctx, 30_000)
	if !healthy {
		steps = append(steps, "still not answering; check 'mvctl logs'")
	} else {
		steps = append(steps, fmt.Sprintf("healthy on %s (host %d -> container %d)", ctx.G.BaseURL, hostPort, containerPort))
	}
	return Result{
		Pretty: heading("repair") + "\n" + strings.Join(steps, "\n"),
		Data:   map[string]any{"steps": steps, "healthy": healthy},
		Code:   boolToCode(healthy),
	}, nil
}

func boolToCode(ok bool) int {
	if ok {
		return 0
	}
	return 1
}

func waitHealthy(ctx Context, timeoutMS int) bool {
	deadline := time.Now().Add(time.Duration(timeoutMS) * time.Millisecond)
	for time.Now().Before(deadline) {
		if ctx.C.Health(ctx.Ctx) {
			return true
		}
		select {
		case <-ctx.Ctx.Done():
			return false
		case <-time.After(time.Second):
		}
	}
	return false
}

func cmdLogs(ctx Context) (Result, error) {
	tail := flag(ctx, "tail", "80")
	r := docker.Compose(ctx.Ctx, deployDir(ctx), []string{"logs", "--tail", tail}, 60_000)
	if r.Code != 0 {
		return Result{Code: 1}, fmt.Errorf("compose logs failed: %s", docker.LastLine(r.Stderr+r.Stdout))
	}
	return Result{Pretty: strings.TrimRight(r.Stdout+r.Stderr, "\n"), Data: map[string]any{"logs": r.Stdout + r.Stderr}}, nil
}

func cmdImages(ctx Context) (Result, error) {
	_, s, _ := config.Load()
	container := s.ContainerName
	if container == "" {
		container = "millivolt"
	}
	running := docker.ContainerImageID(ctx.Ctx, container)
	tagged := docker.TaggedImageID(ctx.Ctx, "ghcr.io/llm-4-people/millivolt:main")
	drift := running != "" && tagged != "" && running != tagged
	pretty := heading("images") + "\n" + kv([][2]string{
		{"running", shortImage(running)},
		{"published tag", shortImage(tagged)},
		{"drift", boolText(drift)},
		{"rollback pin", shortImage(s.RolledBackTo)},
	})
	return Result{Pretty: pretty, Data: map[string]any{"running": running, "tagged": tagged, "drift": drift, "rollback": s.RolledBackTo}}, nil
}

func shortImage(s string) string {
	if s == "" {
		return dim("none")
	}
	if len(s) > 20 {
		return s[:20]
	}
	return s
}

func cmdDoctor(ctx Context) (Result, error) {
	_, s, _ := config.Load()
	// Exported keys so --json doctor emits real values instead of [{}].
	type check struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Detail string `json:"detail"`
	}
	checks := []check{}
	add := func(name string, ok bool, detail string) {
		status := "FAIL"
		if ok {
			status = "OK"
		}
		checks = append(checks, check{Name: name, Status: status, Detail: detail})
	}
	add("docker daemon", docker.DockerDaemonUp(ctx.Ctx), "docker info")
	deploymentDetail := s.DeployDir
	if deploymentDetail == "" {
		deploymentDetail = ctx.G.BaseURL
	}
	deploymentOK := s.DeployDir != "" || ctx.G.BaseURL != "http://127.0.0.1:8080"
	add("deployment recorded", deploymentOK, deploymentDetail)
	add("health endpoint", ctx.C.Health(ctx.Ctx), ctx.G.BaseURL+"/healthz")
	add("operator credential", ctx.G.Token != "", "token present, value not shown")
	_, err := ctx.C.Config(ctx.Ctx)
	add("operator API", err == nil, "GET /admin/config")
	snap, berr := ctx.C.Bootstrap(ctx.Ctx)
	add("metrics API", berr == nil, "GET /metrics/bootstrap")
	if berr == nil {
		add("durable history", snap.Storage.Enabled && snap.Storage.Dropped == 0,
			fmt.Sprintf("dropped %s", num(snap.Storage.Dropped)))
	}
	container := s.ContainerName
	if container == "" {
		container = "millivolt"
	}
	hostPort, containerPort, ok := provision.PortMapping(ctx.Ctx, container)
	if ok {
		text := provision.ReadContainerFile(ctx.Ctx, container, "/config/proxy.yaml")
		listen := provision.ListenPortOf(text)
		add("listen matches published port", listen == 0 || listen == containerPort,
			fmt.Sprintf("host %d -> container %d, listen %d", hostPort, containerPort, listen))
	}
	rows := make([][]string, 0, len(checks))
	for _, c := range checks {
		status := red(c.Status)
		if c.Status == "OK" {
			status = green(c.Status)
		}
		rows = append(rows, []string{c.Name, status, c.Detail})
	}
	failed := 0
	for _, c := range checks {
		if c.Status == "FAIL" {
			failed++
		}
	}
	pretty := heading("doctor") + "\n" + table([]string{"check", "result", "detail"}, rows)
	pretty += fmt.Sprintf("\n\n%s  (run 'mvctl repair' to fix what can be fixed)", bold(fmt.Sprintf("%d/%d checks passed", len(checks)-failed, len(checks))))
	return Result{Pretty: pretty, Data: map[string]any{"checks": checks, "failed": failed}, Code: boolToCode(failed == 0)}, nil
}

func cmdServe(ctx Context) (Result, error) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	lastUpdate := time.Now()
	fmt.Fprintln(os.Stderr, dim("supervisor running; Ctrl-C to stop"))
	for {
		select {
		case <-ctx.Ctx.Done():
			return Result{Pretty: dim("supervisor stopped"), Data: map[string]any{"stopped": true}}, nil
		case <-ticker.C:
			healthy := ctx.C.Health(ctx.Ctx)
			if !healthy {
				_, s, _ := config.Load()
				if s.DesiredRunning {
					_ = docker.Compose(ctx.Ctx, s.DeployDir, append(provision.ComposeArgsFor(s.DeployDir), "up", "-d"), 180_000)
				}
			}
			if time.Since(lastUpdate) > 6*time.Hour {
				lastUpdate = time.Now()
				_ = provision.PullDeployment(ctx.Ctx, deployDir(ctx), "millivolt")
			}
		}
	}
}
