// Package provision finds, installs, updates and repairs a millivolt Docker
// deployment. Ported from the bb plugin's pure TypeScript modules; all Docker
// access goes through internal/docker (argv only, bounded, no shell).

package provision

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hv-V6/mvctl/internal/docker"
)

var ProbePorts = []int{8471, 8472, 8080, 8985, 8000, 8001}

const (
	ImageListenPort  = 8080
	ManagedMarker    = ".millivolt-managed"
	RollbackImage    = "ghcr.io/llm-4-people/millivolt:bb-rollback"
	RollbackOverride = "rollback.yaml"
)

type DiscoveryCandidate struct {
	Kind          string `json:"kind"`
	DeployDir     string `json:"deploy_dir,omitempty"`
	Port          int    `json:"port,omitempty"`
	ContainerName string `json:"container_name,omitempty"`
	HasToken      bool   `json:"has_token"`
	Evidence      string `json:"evidence"`
}

type Discovery struct {
	Candidates   []DiscoveryCandidate `json:"candidates"`
	Volumes      []string             `json:"volumes"`
	ScannedPorts []int                `json:"scanned_ports"`
	ScannedDirs  int                  `json:"scanned_dirs"`
	Truncated    bool                 `json:"truncated"`
}

type DiscoverOptions struct {
	ExtraPorts   []int
	ExcludePorts []int
	Roots        []string
	DeadlineMS   int
}

type InstallOptions struct {
	Dir           string
	Port          int
	ContainerName string
	ProjectName   string
	Token         string
	Log           func(string)
}

type InstallResult struct {
	OK            bool   `json:"ok"`
	Detail        string `json:"detail,omitempty"`
	Token         string `json:"-"`
	ContainerName string `json:"container_name"`
	ComposePath   string `json:"compose_path"`
	Port          int    `json:"port"`
}

type UpdateResult struct {
	Changed bool   `json:"changed"`
	Before  string `json:"before,omitempty"`
	After   string `json:"after,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// probeFn is injectable so tests never need a real proxy.
var probeFn = ProbePort

const composeTemplate = `# Compose project "%s" owns volumes %s_data and
# %s_config. Keep the project name unique per installation: two
# deployments that share a project share their history and their config.
name: %s

services:
  millivolt:
    image: ghcr.io/llm-4-people/millivolt:main
    container_name: %s
    ports:
      # The image listens on :8080; this publishes it on the chosen host port.
      # If Settings -> listen changes, change this mapping to match.
      - "127.0.0.1:%d:%d"
    environment:
      # Env-only by design: the credential never lands in the config volume.
      MILLIVOLT_OPERATOR_TOKEN: ${MILLIVOLT_OPERATOR_TOKEN:-}
    volumes:
      - data:/data
      - config:/config
    read_only: true
    tmpfs:
      - /tmp:rw,nosuid,nodev,noexec,size=16m
    cap_drop:
      - ALL
    security_opt:
      - no-new-privileges:true
    restart: unless-stopped
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
    stop_grace_period: 30s

volumes:
  data:
  config:
`

func ComposeFor(port int, containerName, projectName string) string {
	if projectName == "" {
		projectName = containerName
	}
	return fmt.Sprintf(composeTemplate, projectName, projectName, projectName,
		projectName, containerName, port, ImageListenPort)
}

// GenerateToken returns a cryptographically strong 64-character hex token.
// A broken system RNG is fatal: a predictable fallback would be a credential,
// not a convenience.
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating operator token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func ReadEnv(dir string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		return out
	}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		value := strings.TrimSpace(line[eq+1:])
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		out[key] = value
	}
	return out
}

func ProbePort(ctx context.Context, port int) (ok, strong bool) {
	client := &http.Client{
		Timeout:       1500 * time.Millisecond,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("http://127.0.0.1:%d/healthz", port), nil)
	if err != nil {
		return false, false
	}
	health, err := client.Do(req)
	if err != nil {
		return false, false
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(health.Body, 1<<20))
	_ = health.Body.Close()
	if health.StatusCode < 200 || health.StatusCode >= 300 {
		return false, false
	}
	rootReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("http://127.0.0.1:%d/", port), nil)
	if err != nil {
		return true, false
	}
	root, err := client.Do(rootReq)
	if err != nil {
		return true, false
	}
	defer root.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(root.Body, 1<<20))
	realm := root.Header.Get("WWW-Authenticate")
	return true, strings.Contains(strings.ToLower(realm), "millivolt")
}

var skipDirs = map[string]bool{
	"node_modules": true, ".git": true, ".cache": true, "dist": true,
	"build": true, "target": true, "AppData": true, "Windows": true,
	"Program Files": true, "Program Files (x86)": true, "ProgramData": true,
	"$Recycle.Bin": true, "System Volume Information": true, "Library": true,
	".venv": true, "venv": true, "__pycache__": true,
}

var composeNames = map[string]bool{
	"compose.yaml": true, "compose.yml": true,
	"docker-compose.yaml": true, "docker-compose.yml": true,
}

func ScanForComposeDirs(ctx context.Context, roots []string, deadlineMS, maxDirs int) ([]string, int, bool) {
	if deadlineMS <= 0 {
		deadlineMS = 2500
	}
	if maxDirs <= 0 {
		maxDirs = 400
	}
	deadline := time.Now().Add(time.Duration(deadlineMS) * time.Millisecond)
	type node struct {
		dir   string
		depth int
	}
	queue := make([]node, 0, len(roots))
	for _, r := range roots {
		queue = append(queue, node{r, 0})
	}
	seen := map[string]bool{}
	var dirs []string
	visited := 0
	truncated := false
	for len(queue) > 0 {
		if time.Now().After(deadline) || visited >= maxDirs || len(dirs) >= 5 {
			truncated = len(queue) > 0
			break
		}
		select {
		case <-ctx.Done():
			return dirs, visited, true
		default:
		}
		next := queue[0]
		queue = queue[1:]
		if seen[next.dir] || next.depth > 3 {
			continue
		}
		seen[next.dir] = true
		entries, err := os.ReadDir(next.dir)
		if err != nil {
			continue
		}
		visited++
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if !composeNames[strings.ToLower(e.Name())] {
				continue
			}
			data, err := os.ReadFile(filepath.Join(next.dir, e.Name()))
			if err != nil {
				continue
			}
			text := string(data)
			if len(text) > 8192 {
				text = text[:8192]
			}
			if strings.Contains(strings.ToLower(text), "millivolt") {
				dirs = append(dirs, next.dir)
			}
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || skipDirs[e.Name()] {
				continue
			}
			queue = append(queue, node{filepath.Join(next.dir, e.Name()), next.depth + 1})
		}
	}
	unique := map[string]bool{}
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		if !unique[d] {
			unique[d] = true
			out = append(out, d)
		}
	}
	return out, visited, truncated
}

func DefaultRoots() []string {
	home, _ := os.UserHomeDir()
	roots := []string{home}
	for _, name := range []string{"dev", "docker", "projects", "code", "src", "repos", "Documents"} {
		roots = append(roots, filepath.Join(home, name))
	}
	for _, drive := range []string{"C:", "D:", "E:"} {
		root := drive + string(filepath.Separator)
		if _, err := os.Stat(root); err == nil {
			roots = append(roots, root)
		}
	}
	return roots
}

func DefaultInstallDir(port int) string {
	home, _ := os.UserHomeDir()
	first := filepath.Join(home, "millivolt")
	if _, err := os.Stat(filepath.Join(first, ManagedMarker)); err == nil {
		return first
	}
	if _, err := os.Stat(first); err == nil {
		return filepath.Join(home, fmt.Sprintf("millivolt-%d", port))
	}
	return first
}

// Seams for tests: Discover must never require a live Docker daemon.
var listContainersFn = docker.ListMillivoltContainers
var volumesFn = docker.MillivoltVolumes

func Discover(ctx context.Context, opts DiscoverOptions) Discovery {
	d := Discovery{Candidates: []DiscoveryCandidate{}, Volumes: []string{}, ScannedPorts: []int{}}
	excluded := map[int]bool{}
	for _, p := range opts.ExcludePorts {
		excluded[p] = true
	}
	containers := listContainersFn(ctx)
	for _, c := range containers {
		if c.HostPort > 0 && excluded[c.HostPort] {
			continue
		}
		env := map[string]string{}
		if c.ProjectDir != "" {
			env = ReadEnv(c.ProjectDir)
		}
		d.Candidates = append(d.Candidates, DiscoveryCandidate{
			Kind: "container", DeployDir: c.ProjectDir, Port: c.HostPort,
			ContainerName: c.Name, HasToken: env["MILLIVOLT_OPERATOR_TOKEN"] != "",
			Evidence: fmt.Sprintf("container %s (%s) from %s", c.Name, c.State, c.Image),
		})
	}
	d.Volumes = volumesFn(ctx)
	known := map[int]bool{}
	for _, c := range d.Candidates {
		if c.Port > 0 {
			known[c.Port] = true
		}
	}
	seenPorts := map[int]bool{}
	ports := make([]int, 0, len(opts.ExtraPorts)+len(ProbePorts))
	for _, p := range append(append([]int{}, opts.ExtraPorts...), ProbePorts...) {
		if seenPorts[p] || known[p] || excluded[p] {
			continue
		}
		seenPorts[p] = true
		ports = append(ports, p)
	}
	for i, p := range ports {
		if i >= 8 {
			break
		}
		d.ScannedPorts = append(d.ScannedPorts, p)
		ok, strong := probeFn(ctx, p)
		if !ok {
			continue
		}
		evidence := fmt.Sprintf(":%d answered /healthz only", p)
		if strong {
			evidence = fmt.Sprintf(":%d answered /healthz and a millivolt operator challenge", p)
		}
		d.Candidates = append(d.Candidates, DiscoveryCandidate{Kind: "port", Port: p, Evidence: evidence})
	}
	if len(d.Candidates) == 0 {
		roots := opts.Roots
		if len(roots) == 0 {
			roots = DefaultRoots()
		}
		found, visited, truncated := ScanForComposeDirs(ctx, roots, opts.DeadlineMS, 400)
		d.ScannedDirs, d.Truncated = visited, truncated
		for _, dir := range found {
			env := ReadEnv(dir)
			port := 0
			if n, err := strconv.Atoi(env["MILLIVOLT_PORT"]); err == nil && n > 0 {
				port = n
			}
			d.Candidates = append(d.Candidates, DiscoveryCandidate{
				Kind: "compose-file", DeployDir: dir, Port: port,
				ContainerName: env["MILLIVOLT_CONTAINER"],
				HasToken:      env["MILLIVOLT_OPERATOR_TOKEN"] != "",
				Evidence:      "compose file in " + dir,
			})
		}
	}
	return d
}

func Install(ctx context.Context, opts InstallOptions) InstallResult {
	res := InstallResult{Token: opts.Token, ContainerName: opts.ContainerName,
		ComposePath: filepath.Join(opts.Dir, "compose.yaml"), Port: opts.Port}
	if opts.ProjectName == "" {
		opts.ProjectName = opts.ContainerName
	}
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		res.Detail = err.Error()
		return res
	}
	existingEnv := ReadEnv(opts.Dir)
	// Never rotate an existing operator credential: state.json, other profiles
	// and open dashboards all hold the previous value.
	if opts.Token == "" {
		opts.Token = existingEnv["MILLIVOLT_OPERATOR_TOKEN"]
	}
	if opts.Token == "" {
		token, err := GenerateToken()
		if err != nil {
			res.Detail = err.Error()
			return res
		}
		opts.Token = token
	}
	res.Token = opts.Token
	markerPath := filepath.Join(opts.Dir, ManagedMarker)
	marker, _ := os.ReadFile(markerPath)
	wroteCompose := false
	// Refuse any foreign compose file, under any of the four recognised names.
	existingCompose := ""
	for _, name := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"} {
		if _, err := os.Stat(filepath.Join(opts.Dir, name)); err == nil {
			existingCompose = name
			break
		}
	}
	if existingCompose != "" {
		if len(marker) == 0 {
			res.Detail = filepath.Join(opts.Dir, existingCompose) +
				" already exists and was not created by mvctl; pick another directory or adopt it"
			return res
		}
	} else {
		if err := os.WriteFile(res.ComposePath, []byte(ComposeFor(opts.Port, opts.ContainerName, opts.ProjectName)), 0o644); err != nil {
			res.Detail = err.Error()
			return res
		}
		wroteCompose = true
	}
	if len(marker) == 0 {
		_ = os.WriteFile(markerPath, []byte(fmt.Sprintf(`{"by":"mvctl","at":%q}`+"\n", time.Now().UTC().Format(time.RFC3339))), 0o644)
	}
	// Preserve every existing key we do not explicitly own.
	managed := map[string]bool{
		"MILLIVOLT_OPERATOR_TOKEN": true, "MILLIVOLT_PORT": true, "MILLIVOLT_CONTAINER": true,
	}
	var envLines = []string{
		"# Written by mvctl. The operator credential gates the dashboard and",
		"# the inference listener's operator plane.",
		"MILLIVOLT_OPERATOR_TOKEN=" + opts.Token,
		fmt.Sprintf("MILLIVOLT_PORT=%d", opts.Port),
		"MILLIVOLT_CONTAINER=" + opts.ContainerName,
	}
	for k, v := range existingEnv {
		if !managed[k] && v != "" {
			envLines = append(envLines, k+"="+v)
		}
	}
	envPath := filepath.Join(opts.Dir, ".env")
	tmpEnv := envPath + ".new"
	if err := os.WriteFile(tmpEnv, []byte(strings.Join(envLines, "\n")+"\n"), 0o600); err != nil {
		res.Detail = err.Error()
		return res
	}
	if err := os.Rename(tmpEnv, envPath); err != nil {
		_ = os.Remove(tmpEnv)
		res.Detail = err.Error()
		return res
	}
	if opts.Log != nil {
		opts.Log(fmt.Sprintf("installing millivolt in %s on port %d", opts.Dir, opts.Port))
	}
	pulled := docker.Compose(ctx, opts.Dir, append(ComposeArgsFor(opts.Dir), "pull", "--quiet"), 300_000)
	if pulled.Code != 0 {
		res.Detail = docker.LastLine(pulled.Stderr + pulled.Stdout)
		if res.Detail == "" {
			res.Detail = "docker compose pull failed"
		}
		return res
	}
	up := docker.Compose(ctx, opts.Dir, append(ComposeArgsFor(opts.Dir), "up", "-d"), 180_000)
	if up.Code != 0 {
		res.Detail = docker.LastLine(up.Stderr + up.Stdout)
		if res.Detail == "" {
			res.Detail = "docker compose up failed"
		}
		return res
	}
	res.OK = true
	if !wroteCompose {
		res.Detail = "reused the existing managed compose file"
	}
	return res
}

func PullDeployment(ctx context.Context, dir, containerName string) UpdateResult {
	before := docker.ContainerImageID(ctx, containerName)
	// Honour an active rollback override so a pinned deployment is not
	// reported as "updated" while the container keeps the old image.
	pulled := docker.Compose(ctx, dir, append(ComposeArgsFor(dir), "pull", "--quiet"), 300_000)
	if pulled.Code != 0 {
		detail := docker.LastLine(pulled.Stderr + pulled.Stdout)
		if detail == "" {
			detail = "docker compose pull failed"
		}
		return UpdateResult{Changed: false, Before: before, After: before, Detail: detail}
	}
	after := docker.TaggedImageID(ctx, "ghcr.io/llm-4-people/millivolt:main")
	return UpdateResult{Changed: before != "" && after != "" && before != after, Before: before, After: after}
}

func FreeContainerName(ctx context.Context, preferred string) string {
	r := docker.RunProcess(ctx, "docker", []string{"ps", "-a", "--format", "{{.Names}}"}, docker.RunOptions{TimeoutMS: 10_000})
	taken := map[string]bool{}
	for _, line := range strings.Split(r.Stdout, "\n") {
		taken[strings.TrimSpace(line)] = true
	}
	if !taken[preferred] {
		return preferred
	}
	for i := 2; i < 50; i++ {
		c := fmt.Sprintf("%s-%d", preferred, i)
		if !taken[c] {
			return c
		}
	}
	return fmt.Sprintf("%s-%d", preferred, time.Now().Unix())
}

func FreeProjectName(ctx context.Context, preferred string) string {
	r := docker.RunProcess(ctx, "docker", []string{"compose", "ls", "-a", "--format", "json"}, docker.RunOptions{TimeoutMS: 15_000})
	taken := map[string]bool{}
	var entries []struct{ Name string }
	if r.Code == 0 {
		_ = json.Unmarshal([]byte(r.Stdout), &entries)
		for _, e := range entries {
			taken[strings.ToLower(e.Name)] = true
		}
	}
	volumes := map[string]bool{}
	for _, v := range docker.MillivoltVolumes(ctx) {
		volumes[strings.ToLower(v)] = true
	}
	free := func(name string) bool {
		n := strings.ToLower(name)
		return !taken[n] && !volumes[n+"_data"] && !volumes[n+"_config"]
	}
	if free(preferred) {
		return preferred
	}
	for i := 2; i < 50; i++ {
		c := fmt.Sprintf("%s-%d", preferred, i)
		if free(c) {
			return c
		}
	}
	return fmt.Sprintf("%s-%d", preferred, time.Now().Unix())
}

func ImageExists(ctx context.Context, ref string) bool {
	r := docker.RunProcess(ctx, "docker", []string{"image", "inspect", "-f", "{{.Id}}", ref}, docker.RunOptions{TimeoutMS: 10_000})
	return r.Code == 0 && strings.TrimSpace(r.Stdout) != ""
}

func TagImage(ctx context.Context, id, ref string) bool {
	r := docker.RunProcess(ctx, "docker", []string{"tag", id, ref}, docker.RunOptions{TimeoutMS: 30_000})
	return r.Code == 0
}

func WriteRollbackOverride(dir string) error {
	return os.WriteFile(filepath.Join(dir, RollbackOverride), []byte(fmt.Sprintf(
		"# Written by mvctl while a rolled-back image is pinned.\n"+
			"# Delete this file (or run 'mvctl resume') to track the published tag again.\n"+
			"services:\n  millivolt:\n    image: %s\n    pull_policy: never\n", RollbackImage)), 0o644)
}

func RemoveRollbackOverride(dir string) error {
	err := os.Remove(filepath.Join(dir, RollbackOverride))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func ComposeArgsFor(dir string) []string {
	if _, err := os.Stat(filepath.Join(dir, RollbackOverride)); err == nil {
		return []string{"-f", "compose.yaml", "-f", RollbackOverride}
	}
	return []string{}
}

var portMappingRe = regexp.MustCompile(`^(\d{2,5})/tcp\s*->\s*(?:[\d.]+|\[[^\]]+\]|::[^:]*):(\d{2,5})$`)

func PortMapping(ctx context.Context, container string) (int, int, bool) {
	r := docker.RunProcess(ctx, "docker", []string{"port", container}, docker.RunOptions{TimeoutMS: 10_000})
	if r.Code != 0 {
		return 0, 0, false
	}
	for _, line := range strings.Split(r.Stdout, "\n") {
		m := portMappingRe.FindStringSubmatch(strings.TrimSpace(line))
		if len(m) == 3 {
			cp, _ := strconv.Atoi(m[1])
			hp, _ := strconv.Atoi(m[2])
			return hp, cp, true
		}
	}
	return 0, 0, false
}

func ReadContainerFile(ctx context.Context, container, containerPath string) string {
	tmp, err := os.CreateTemp("", "mvctl-*-cfg")
	if err != nil {
		return ""
	}
	name := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(name)
	defer os.Remove(name)
	r := docker.RunProcess(ctx, "docker", []string{"cp", container + ":" + containerPath, name}, docker.RunOptions{TimeoutMS: 20_000})
	if r.Code != 0 {
		return ""
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return ""
	}
	return string(data)
}

func WriteContainerFile(ctx context.Context, container, containerPath, content string) bool {
	tmp, err := os.CreateTemp("", "mvctl-*-cfg")
	if err != nil {
		return false
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return false
	}
	_ = tmp.Close()
	r := docker.RunProcess(ctx, "docker", []string{"cp", tmp.Name(), container + ":" + containerPath}, docker.RunOptions{TimeoutMS: 20_000})
	return r.Code == 0
}

func ListenPortOf(configText string) int {
	for _, line := range strings.Split(configText, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "listen:") {
			continue
		}
		colon := strings.LastIndex(trimmed, ":")
		if colon < 0 {
			continue
		}
		v := strings.Trim(trimmed[colon+1:], "\"' \t\r")
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

func WithListenPort(configText string, port int) string {
	lines := strings.Split(configText, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "listen:") {
			trailing := ""
			if strings.HasSuffix(line, "\r") {
				trailing = "\r"
			}
			lines[i] = fmt.Sprintf(`listen: "0.0.0.0:%d"`, port) + trailing
			return strings.Join(lines, "\n")
		}
	}
	return ""
}
