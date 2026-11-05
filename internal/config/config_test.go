package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func isolatedConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	return dir
}

func TestReadEnvFile(t *testing.T) {
	dir := t.TempDir()
	content := `# comment
MILLIVOLT_OPERATOR_TOKEN="abc123"
MILLIVOLT_PORT = '8471'
MILLIVOLT_IMAGE=ghcr.io/llm-4-people/millivolt:main
BADLINE
`
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got := readEnvFile(dir)
	if got["MILLIVOLT_OPERATOR_TOKEN"] != "abc123" || got["MILLIVOLT_PORT"] != "8471" ||
		got["MILLIVOLT_IMAGE"] != "ghcr.io/llm-4-people/millivolt:main" {
		t.Fatalf("env: %+v", got)
	}
	if _, ok := got["BADLINE"]; ok {
		t.Fatal("malformed line must be skipped")
	}
	if len(readEnvFile(filepath.Join(dir, "missing"))) != 0 {
		t.Fatal("missing .env must return an empty map")
	}
}

func TestResolvePrecedence(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"),
		[]byte("MILLIVOLT_PORT=8471\nMILLIVOLT_OPERATOR_TOKEN=from-dotenv\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MILLIVOLT_OPERATOR_TOKEN", "")
	t.Setenv("MVCTL_PORT", "")
	t.Setenv("MILLIVOLT_PORT", "")
	t.Setenv("MILLIVOLT_URL", "")
	t.Setenv("MVCTL_PROFILE", "")
	f := DefaultFile()
	f.Profiles["prod"] = Profile{URL: "http://profile:9000", Token: "profile-token", Port: 9000}
	f.DefaultProfile = "prod"
	s := DefaultState()
	s.Port = 8000
	s.Token = "state-token"

	r := Resolve(f, s, map[string]string{"dir": dir})
	if r.Port != 9000 || r.BaseURL != "http://profile:9000" || r.Token != "profile-token" {
		t.Fatalf("profile precedence: %+v", r)
	}
	// A flag beats the profile.
	r = Resolve(f, s, map[string]string{"dir": dir, "port": "9999", "url": "http://flag:1", "token": "flag-token"})
	if r.Port != 9999 || r.BaseURL != "http://flag:1" || r.Token != "flag-token" {
		t.Fatalf("flag precedence: %+v", r)
	}
	// With no profile/state/flags, the deployment .env supplies port + token.
	f.Profiles = map[string]Profile{}
	r = Resolve(f, DefaultState(), map[string]string{"dir": dir})
	if r.Port != 8471 || r.BaseURL != "http://127.0.0.1:8471" || r.Token != "from-dotenv" {
		t.Fatalf(".env precedence: %+v", r)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := isolatedConfig(t)
	f := DefaultFile()
	f.Profiles["prod"] = Profile{URL: "http://x", Token: "secret-token"}
	if err := SaveFile(f); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Profiles["prod"].Token != "secret-token" {
		t.Fatalf("round trip: %+v", loaded.Profiles)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "mvctl"))
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestCorruptConfigIsReported(t *testing.T) {
	isolatedConfig(t)
	p, _ := pathFor("config.json")
	if err := os.WriteFile(p, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(); err == nil {
		t.Fatal("corrupt config.json must be reported, not silently reset")
	}
}

func TestConcurrentSaveFileNoLostUpdate(t *testing.T) {
	isolatedConfig(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			f := DefaultFile()
			f.Profiles[string(rune('a'+i))] = Profile{URL: "http://x"}
			if err := SaveFile(f); err != nil {
				t.Errorf("SaveFile: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if _, _, err := Load(); err != nil {
		t.Fatalf("concurrent writes corrupted config: %v", err)
	}
}
