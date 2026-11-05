// Package docker is the millivolt CLI's bounded, argv-only Docker plumbing:
// every call has a deadline and capped output, and never runs through a shell.
package docker

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTimeoutMS, composeTimeoutMS = 60_000, 120_000
	shortTimeoutMS, listTimeoutMS      = 10_000, 15_000
	maxProcessOutput                   = 16 * 1024
	maxRows                            = 10
	maxLastLine                        = 400
	daemonPollEvery                    = 2 * time.Second
)

// RunOptions controls one RunProcess call; TimeoutMS <= 0 uses the 60s default.
type RunOptions struct {
	Cwd       string
	TimeoutMS int
}

// RunResult holds a process outcome; Code is -1 on start failure or timeout.
type RunResult struct {
	Code           int
	Stdout, Stderr string
	TimedOut       bool
}

// cappedWriter keeps only the first maxProcessOutput bytes it receives.
type cappedWriter struct{ buf []byte }

func (w *cappedWriter) Write(p []byte) (int, error) {
	n := len(p)
	if room := maxProcessOutput - len(w.buf); room > 0 {
		if n > room {
			p = p[:room]
		}
		w.buf = append(w.buf, p...)
	}
	return n, nil
}

// RunProcess runs argv (never a shell) bounded by ctx and a per-call deadline,
// capturing stdout/stderr into 16 KiB caps. TimedOut marks a deadline hit.
func RunProcess(ctx context.Context, command string, args []string, opts RunOptions) RunResult {
	ms := opts.TimeoutMS
	if ms <= 0 {
		ms = defaultTimeoutMS
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(ms)*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(runCtx, command, args...)
	cmd.Dir = opts.Cwd
	// Without WaitDelay, a surviving grandchild holding the output pipes can
	// make Wait() hang forever after the direct child is killed on timeout.
	cmd.WaitDelay = time.Second
	var stdout, stderr cappedWriter
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	res := RunResult{Code: -1}
	err := cmd.Run()
	res.Stdout, res.Stderr = string(stdout.buf), string(stderr.buf)
	if err == nil {
		res.Code = 0
		return res
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		res.TimedOut = true
		return res
	}
	if errors.Is(err, exec.ErrWaitDelay) {
		// The child itself finished; only its descendants held the pipes open.
		res.Code = 0
		return res
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.Code = exitErr.ExitCode()
	}
	return res
}

// docker runs the Docker CLI with a fixed deadline.
func docker(ctx context.Context, timeoutMS int, args ...string) RunResult {
	return RunProcess(ctx, "docker", args, RunOptions{TimeoutMS: timeoutMS})
}

// LastLine returns the last non-blank line, capped at 400 characters.
func LastLine(text string) string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(strings.TrimSuffix(lines[i], "\r"))
		if line == "" {
			continue
		}
		runes := []rune(line)
		if len(runes) > maxLastLine {
			runes = runes[:maxLastLine]
		}
		return string(runes)
	}
	return ""
}

// DockerDaemonUp reports whether the daemon answers `docker info`.
func DockerDaemonUp(ctx context.Context) bool {
	r := docker(ctx, shortTimeoutMS, "info", "--format", "{{.ServerVersion}}")
	return r.Code == 0
}

// LaunchDockerDesktop starts the GUI out-of-band: stdio is discarded and the
// process is released, not waited on. Only a start failure is reported.
func LaunchDockerDesktop(path string) error {
	cmd := exec.Command(path)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	return nil
}

// WaitForDaemon polls every 2s until timeoutMS elapses or ctx is done.
func WaitForDaemon(ctx context.Context, timeoutMS int) bool {
	if timeoutMS <= 0 {
		timeoutMS = defaultTimeoutMS
	}
	deadline, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()
	for {
		if deadline.Err() != nil {
			return false
		}
		if DockerDaemonUp(deadline) {
			return true
		}
		timer := time.NewTimer(daemonPollEvery)
		select {
		case <-deadline.Done():
			timer.Stop()
			return false
		case <-timer.C:
		}
	}
}

// Compose runs `docker compose <args...>` inside cwd.
func Compose(ctx context.Context, cwd string, args []string, timeoutMS int) RunResult {
	if timeoutMS <= 0 {
		timeoutMS = composeTimeoutMS
	}
	argv := append([]string{"compose"}, args...)
	return RunProcess(ctx, "docker", argv, RunOptions{Cwd: cwd, TimeoutMS: timeoutMS})
}

// ContainerRef describes one millivolt container on the daemon.
type ContainerRef struct {
	Name, Image, ProjectDir, ProjectName, State string
	HostPort                                    int
}

// FilterMillivoltRows keeps "name\timage\tstate" rows whose IMAGE is
// millivolt-tagged, plus the container named exactly "millivolt". A mere
// "millivolt" substring in the NAME (e.g. a caddy sidecar) is NOT enough;
// malformed rows (fewer than 3 fields) are dropped and kept rows are unchanged.
func FilterMillivoltRows(rows []string) []string {
	kept := make([]string, 0, len(rows))
	for _, row := range rows {
		fields := strings.Split(strings.TrimSuffix(row, "\r"), "\t")
		if len(fields) < 3 {
			continue
		}
		name, image := strings.TrimSpace(fields[0]), strings.TrimSpace(fields[1])
		if strings.Contains(strings.ToLower(image), "millivolt") || name == "millivolt" {
			kept = append(kept, row)
		}
	}
	return kept
}

// ListMillivoltContainers enumerates up to ten millivolt containers, adding
// compose metadata and the published host port to each.
func ListMillivoltContainers(ctx context.Context) []ContainerRef {
	listed := docker(ctx, listTimeoutMS, "ps", "-a", "--format", "{{.Names}}\t{{.Image}}\t{{.State}}")
	if listed.Code != 0 {
		return []ContainerRef{}
	}
	rows := FilterMillivoltRows(strings.Split(listed.Stdout, "\n"))
	if len(rows) > maxRows {
		rows = rows[:maxRows]
	}
	refs := make([]ContainerRef, 0, len(rows))
	for _, row := range rows {
		fields := strings.Split(strings.TrimSuffix(row, "\r"), "\t")
		if len(fields) < 3 {
			continue
		}
		ref := ContainerRef{Name: strings.TrimSpace(fields[0]), Image: strings.TrimSpace(fields[1]), State: strings.TrimSpace(fields[2])}
		inspected := docker(ctx, shortTimeoutMS, "inspect", "-f",
			`{{index .Config.Labels "com.docker.compose.project.working_dir"}}|{{index .Config.Labels "com.docker.compose.project"}}`,
			ref.Name)
		if inspected.Code == 0 {
			parts := strings.SplitN(strings.TrimSpace(inspected.Stdout), "|", 2)
			ref.ProjectDir = strings.TrimSpace(parts[0])
			if len(parts) > 1 {
				ref.ProjectName = strings.TrimSpace(parts[1])
			}
		}
		ref.HostPort = HostPortOf(ctx, ref.Name)
		refs = append(refs, ref)
	}
	return refs
}

// Accept IPv4, bracketed IPv6, and bare IPv6 host forms, e.g.
// "8080/tcp -> :::8471" or "8080/tcp -> [::1]:8471".
var hostPortPattern = regexp.MustCompile(`->\s*(?:[\d.]+|\[[^\]]+\]|::[^:]*):(\d{2,5})\s*$`)

// HostPortOf returns the first published host port for container, or 0.
func HostPortOf(ctx context.Context, container string) int {
	r := docker(ctx, shortTimeoutMS, "port", container)
	if r.Code != 0 {
		return 0
	}
	for _, line := range strings.Split(r.Stdout, "\n") {
		match := hostPortPattern.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil {
			continue
		}
		port, err := strconv.Atoi(match[1])
		if err == nil && port >= 1 && port <= 65535 {
			return port
		}
	}
	return 0
}

// ContainerImageID is the digest the container is actually running.
func ContainerImageID(ctx context.Context, container string) string {
	r := docker(ctx, shortTimeoutMS, "inspect", "-f", "{{.Image}}", container)
	if r.Code != 0 {
		return ""
	}
	return strings.TrimSpace(r.Stdout)
}

// TaggedImageID is the id the local image tag currently points at.
func TaggedImageID(ctx context.Context, image string) string {
	r := docker(ctx, shortTimeoutMS, "image", "inspect", "-f", "{{.Id}}", image)
	if r.Code != 0 {
		return ""
	}
	return strings.TrimSpace(r.Stdout)
}

// MillivoltVolumes lists at most ten volume names mentioning millivolt.
func MillivoltVolumes(ctx context.Context) []string {
	r := docker(ctx, listTimeoutMS, "volume", "ls", "--format", "{{.Name}}")
	if r.Code != 0 {
		return []string{}
	}
	volumes := []string{}
	for _, line := range strings.Split(r.Stdout, "\n") {
		name := strings.TrimSpace(line)
		if name == "" || !strings.Contains(strings.ToLower(name), "millivolt") {
			continue
		}
		volumes = append(volumes, name)
		if len(volumes) == maxRows {
			break
		}
	}
	return volumes
}
