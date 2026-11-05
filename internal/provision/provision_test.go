package provision

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hv-V6/mvctl/internal/docker"
)

func TestComposeFor(t *testing.T) {
	c := ComposeFor(8471, "millivolt", "millivolt")
	for _, want := range []string{
		"name: millivolt", "container_name: millivolt",
		"127.0.0.1:8471:8080", "ghcr.io/llm-4-people/millivolt:main",
		"MILLIVOLT_OPERATOR_TOKEN", "restart: unless-stopped", "read_only: true",
	} {
		if !strings.Contains(c, want) {
			t.Fatalf("compose missing %q:\n%s", want, c)
		}
	}
	if strings.Contains(c, "0.0.0.0:8471") {
		t.Fatal("compose must bind loopback only")
	}
}

func TestGenerateToken(t *testing.T) {
	a, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 64 || a == b {
		t.Fatalf("bad token: %q / %q", a, b)
	}
	if _, err := json.Marshal(a); err != nil {
		t.Fatal(err)
	}
}

func TestReadEnv(t *testing.T) {
	dir := t.TempDir()
	content := "# comment\n\nMILLIVOLT_OPERATOR_TOKEN=\"abc123\"\nMILLIVOLT_PORT = '8471'\nPLAIN=x\nBADLINE\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	env := ReadEnv(dir)
	if env["MILLIVOLT_OPERATOR_TOKEN"] != "abc123" || env["MILLIVOLT_PORT"] != "8471" || env["PLAIN"] != "x" {
		t.Fatalf("readEnv: %+v", env)
	}
	if _, ok := env["BADLINE"]; ok {
		t.Fatal("malformed line must be skipped")
	}
}

func TestListenPortHelpers(t *testing.T) {
	text := "# config\nlisten: \"0.0.0.0:8080\"\nhistory_size: 10000\n"
	if got := ListenPortOf(text); got != 8080 {
		t.Fatalf("listen: %d", got)
	}
	next := WithListenPort(text, 8471)
	if next == "" || !strings.Contains(next, `listen: "0.0.0.0:8471"`) {
		t.Fatalf("rewrite: %q", next)
	}
	if !strings.Contains(next, "history_size: 10000") {
		t.Fatal("rewrite must keep the rest of the file")
	}
	if WithListenPort("no listen here", 1) != "" {
		t.Fatal("missing listen must return empty")
	}
}

func TestComposeArgsFor(t *testing.T) {
	dir := t.TempDir()
	if got := ComposeArgsFor(dir); len(got) != 0 {
		t.Fatalf("expected no override, got %v", got)
	}
	if err := WriteRollbackOverride(dir); err != nil {
		t.Fatal(err)
	}
	got := ComposeArgsFor(dir)
	if len(got) != 4 || got[0] != "-f" || got[3] != RollbackOverride {
		t.Fatalf("override args: %v", got)
	}
	if !strings.Contains(RollbackImage, "bb-rollback") {
		t.Fatal("rollback image tag")
	}
	if err := RemoveRollbackOverride(dir); err != nil {
		t.Fatal(err)
	}
	if got := ComposeArgsFor(dir); len(got) != 0 {
		t.Fatal("override not removed")
	}
}

func TestDiscoveryNeverSerializesToken(t *testing.T) {
	oldProbe, oldList, oldVolumes := probeFn, listContainersFn, volumesFn
	probeFn = func(context.Context, int) (bool, bool) { return false, false }
	listContainersFn = func(context.Context) []docker.ContainerRef {
		return []docker.ContainerRef{{
			Name: "millivolt", Image: "ghcr.io/llm-4-people/millivolt:main",
			ProjectDir: t.TempDir(), HostPort: 8471, State: "running",
		}}
	}
	volumesFn = func(context.Context) []string { return nil }
	defer func() {
		probeFn, listContainersFn, volumesFn = oldProbe, oldList, oldVolumes
	}()
	d := Discover(context.Background(), DiscoverOptions{
		Roots: []string{t.TempDir()}, DeadlineMS: 50,
	})
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if strings.Contains(s, "MILLIVOLT_OPERATOR_TOKEN") || strings.Contains(s, "\"token\"") {
		t.Fatal("discovery output leaked a token field")
	}
	if !strings.Contains(s, "has_token") && len(d.Candidates) > 0 {
		t.Fatal("candidates must expose has_token, never token")
	}
}

func TestDiscoverPrefersContainerEvidence(t *testing.T) {
	oldProbe, oldList, oldVolumes := probeFn, listContainersFn, volumesFn
	probeFn = func(_ context.Context, port int) (bool, bool) {
		return port == 8471, true
	}
	listContainersFn = func(context.Context) []docker.ContainerRef { return nil }
	volumesFn = func(context.Context) []string { return nil }
	defer func() {
		probeFn, listContainersFn, volumesFn = oldProbe, oldList, oldVolumes
	}()
	d := Discover(context.Background(), DiscoverOptions{
		ExtraPorts: []int{8471}, ExcludePorts: []int{8472},
		Roots: []string{t.TempDir()}, DeadlineMS: 50,
	})
	found := false
	for _, c := range d.Candidates {
		if c.Kind == "port" && c.Port == 8471 {
			found = true
			if !strings.Contains(c.Evidence, "operator challenge") {
				t.Fatalf("strong probe evidence missing: %s", c.Evidence)
			}
		}
	}
	if !found {
		t.Fatalf("expected a port candidate, got %+v", d.Candidates)
	}
}

func TestDefaultRootsIsPortable(t *testing.T) {
	roots := DefaultRoots()
	if len(roots) < 3 {
		t.Fatal("no roots")
	}
	for _, r := range roots {
		if strings.Contains(r, "\x00") {
			t.Fatal("invalid root")
		}
	}
}

func TestDefaultInstallDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	dir := DefaultInstallDir(8471)
	if !strings.HasPrefix(dir, home) || !strings.Contains(dir, "millivolt") {
		t.Fatalf("install dir: %s", dir)
	}
}
