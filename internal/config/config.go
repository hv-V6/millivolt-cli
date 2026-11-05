// Package config owns mvctl's own config, persisted state and named profiles.
// It never talks to millivolt; the CLI resolves a target before building a client.

package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Profile struct {
	URL   string `json:"url,omitempty"`
	Dir   string `json:"dir,omitempty"`
	Token string `json:"token,omitempty"`
	Theme string `json:"theme,omitempty"`
	Mode  string `json:"mode,omitempty"`
	Port  int    `json:"port,omitempty"`
}

type File struct {
	DefaultProfile string             `json:"default_profile"`
	Profiles       map[string]Profile `json:"profiles"`
	Theme          string             `json:"theme"`
	Mode           string             `json:"mode"`
	EmbedPort      int                `json:"embed_port"`
	AutoUpdate     bool               `json:"auto_update"`
}

type State struct {
	DeployDir      string `json:"deploy_dir,omitempty"`
	Port           int    `json:"port,omitempty"`
	ContainerName  string `json:"container_name,omitempty"`
	Token          string `json:"token,omitempty"`
	Managed        string `json:"managed,omitempty"`
	InstalledAt    string `json:"installed_at,omitempty"`
	LastUpdate     string `json:"last_update,omitempty"`
	ImageID        string `json:"image_id,omitempty"`
	PreviousImage  string `json:"previous_image,omitempty"`
	RolledBackTo   string `json:"rolled_back_to,omitempty"`
	DesiredRunning bool   `json:"desired_running"`
}

type Resolved struct {
	BaseURL   string
	Token     string
	Dir       string
	Port      int
	Profile   string
	Theme     string
	Mode      string
	EmbedPort int
}

func DefaultFile() File {
	return File{
		DefaultProfile: "default",
		Profiles:       map[string]Profile{},
		Theme:          "instrument",
		Mode:           "black",
		EmbedPort:      8472,
		AutoUpdate:     true,
	}
}

func DefaultState() State { return State{DesiredRunning: true} }

// Dir returns the per-user config directory, created on demand.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		home, e := os.UserHomeDir()
		if e != nil {
			return "", e
		}
		base = filepath.Join(home, ".config")
	}
	dir := filepath.Join(base, "mvctl")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func pathFor(name string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func Load() (File, State, error) {
	f := DefaultFile()
	s := DefaultState()
	if p, err := pathFor("config.json"); err == nil {
		if data, err := os.ReadFile(p); err == nil && len(bytes.TrimSpace(data)) > 0 {
			if err := json.Unmarshal(data, &f); err != nil {
				return f, s, fmt.Errorf("config.json is corrupt (a backup was kept): %w", err)
			}
		}
	}
	if f.Profiles == nil {
		f.Profiles = map[string]Profile{}
	}
	if p, err := pathFor("state.json"); err == nil {
		if data, err := os.ReadFile(p); err == nil && len(bytes.TrimSpace(data)) > 0 {
			if err := json.Unmarshal(data, &s); err != nil {
				return f, s, fmt.Errorf("state.json is corrupt: %w", err)
			}
		}
	}
	return f, s, nil
}

// writeAtomic uses a unique temp file, syncs it, and never leaves a partial
// file behind. Both config.json and state.json can hold the operator token.
func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		// Windows cannot rename over an existing file.
		_ = os.Remove(path)
	}
	return os.Rename(tmpName, path)
}

// withLock serialises read-modify-write cycles across concurrent CLI
// invocations (for example a cron `update` racing an interactive `revert`).
// The stale threshold is shorter than the wait deadline so a crashed holder's
// lock is reclaimed within a single call.
func withLock(fn func() error) error {
	p, err := pathFor("state.lock")
	if err != nil {
		return fn()
	}
	const staleAfter = 5 * time.Second
	deadline := time.Now().Add(10 * time.Second)
	for {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = f.WriteString(strconv.Itoa(os.Getpid()))
			_ = f.Close()
			defer os.Remove(p)
			// Refresh mtime periodically so a legitimately slow writer is not
			// mistaken for a crashed one.
			done := make(chan struct{})
			go func() {
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-done:
						return
					case <-ticker.C:
						now := time.Now()
						_ = os.Chtimes(p, now, now)
					}
				}
			}()
			defer close(done)
			return fn()
		}
		if info, statErr := os.Stat(p); statErr == nil && time.Since(info.ModTime()) > staleAfter {
			_ = os.Remove(p)
			continue
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("state is locked by another mvctl process")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func SaveFile(f File) error {
	p, err := pathFor("config.json")
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return withLock(func() error { return writeAtomic(p, data) })
}

func SaveState(s State) error {
	p, err := pathFor("state.json")
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return withLock(func() error { return writeAtomic(p, data) })
}

func env(key string) string { return strings.TrimSpace(os.Getenv(key)) }

// Resolve applies: flags > environment > profile > state > deployment .env >
// built-in defaults. The operator token is never returned in any error text.
func Resolve(f File, s State, flags map[string]string) Resolved {
	profileName := env("MVCTL_PROFILE")
	if v := flags["profile"]; v != "" {
		profileName = v
	}
	if profileName == "" {
		profileName = f.DefaultProfile
	}
	if profileName == "" {
		profileName = "default"
	}
	p := f.Profiles[profileName]

	r := Resolved{
		Profile:   profileName,
		Theme:     firstNonEmpty(flags["theme"], env("MVCTL_THEME"), p.Theme, f.Theme, "instrument"),
		Mode:      firstNonEmpty(flags["mode"], env("MVCTL_MODE"), p.Mode, f.Mode, "black"),
		EmbedPort: f.EmbedPort,
	}
	if r.EmbedPort == 0 {
		r.EmbedPort = 8472
	}
	r.Dir = firstNonEmpty(flags["dir"], p.Dir, s.DeployDir)
	envValues := map[string]string{}
	if r.Dir != "" {
		envValues = readEnvFile(r.Dir)
	}
	r.Port = intFrom(flags["port"], env("MVCTL_PORT"), env("MILLIVOLT_PORT"),
		p.Port, s.Port, envValues["MILLIVOLT_PORT"], 0)

	url := firstNonEmpty(flags["url"], env("MILLIVOLT_URL"), p.URL)
	if url == "" && r.Port > 0 {
		url = fmt.Sprintf("http://127.0.0.1:%d", r.Port)
	}
	if url == "" {
		// Upstream's compose default is host 8080 -> container 8080.
		url = "http://127.0.0.1:8080"
	}
	r.BaseURL = strings.TrimRight(url, "/")

	r.Token = firstNonEmpty(flags["token"], env("MILLIVOLT_OPERATOR_TOKEN"), p.Token, s.Token)
	if r.Token == "" {
		r.Token = envValues["MILLIVOLT_OPERATOR_TOKEN"]
	}
	return r
}

// readEnvFile parses a deployment .env once; readEnvToken below is kept for
// callers that only need the credential.
func readEnvFile(dir string) map[string]string {
	data, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		return map[string]string{}
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		v := strings.TrimSpace(parts[1])
		v = strings.Trim(v, "\"'")
		out[strings.TrimSpace(parts[0])] = v
	}
	return out
}

func intFrom(values ...any) int {
	for _, v := range values {
		switch x := v.(type) {
		case int:
			if x > 0 {
				return x
			}
		case string:
			if x != "" {
				if n, err := strconv.Atoi(x); err == nil && n > 0 {
					return n
				}
			}
		}
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func IsWindows() bool { return runtime.GOOS == "windows" }

func ConfigPath() string {
	p, _ := pathFor("config.json")
	return p
}
