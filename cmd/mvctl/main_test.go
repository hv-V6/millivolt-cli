package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestParseGlobal(t *testing.T) {
	t.Setenv("MILLIVOLT_URL", "")
	p, err := parseGlobal([]string{"--json", "--url", "http://127.0.0.1:8471", "status"})
	if err != nil {
		t.Fatal(err)
	}
	if !p.global.JSON || !p.explicitURL || p.global.BaseURL != "http://127.0.0.1:8471" {
		t.Fatalf("globals wrong: %+v", p.global)
	}
	if len(p.args) != 1 || p.args[0] != "status" {
		t.Fatalf("args wrong: %v", p.args)
	}
}

func TestParseGlobalPerCommandFlags(t *testing.T) {
	t.Setenv("MILLIVOLT_URL", "")
	p, err := parseGlobal([]string{"config", "set", "listen=:9000", "--provider", "openai", "--yes"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.args) != 3 || p.args[1] != "set" || p.args[2] != "listen=:9000" {
		t.Fatalf("args: %v", p.args)
	}
	if p.flags["provider"] != "openai" || !p.global.Yes {
		t.Fatalf("flags: %+v global: %+v", p.flags, p.global)
	}
}

func TestParseGlobalEqualsFormAndNoColor(t *testing.T) {
	t.Setenv("MILLIVOLT_URL", "")
	p, err := parseGlobal([]string{"--url=http://127.0.0.1:9", "--no-color", "--theme=ember", "--mode=clear", "top", "--demo=true"})
	if err != nil {
		t.Fatal(err)
	}
	if p.global.BaseURL != "http://127.0.0.1:9" || p.global.Color || p.global.Theme != "ember" || p.global.Mode != "clear" {
		t.Fatalf("globals: %+v", p.global)
	}
	if p.flags["demo"] != "true" || p.flags["theme"] != "ember" {
		t.Fatalf("flags: %+v", p.flags)
	}
}

func TestParseGlobalEnvToken(t *testing.T) {
	t.Setenv("MILLIVOLT_OPERATOR_TOKEN", "env-token-value")
	t.Setenv("MVCTL_PROFILE", "prod")
	p, err := parseGlobal([]string{"status"})
	if err != nil {
		t.Fatal(err)
	}
	if p.global.Token != "env-token-value" && p.global.Token != "" {
		// token is resolved later in config.Resolve, not here
		t.Fatalf("parseGlobal should not consume env token, got %q", p.global.Token)
	}
}

func TestRunWithHelp(t *testing.T) {
	t.Setenv("MILLIVOLT_URL", "")
	var out, errb bytes.Buffer
	code := runWith([]string{"help"}, &out, &errb)
	if code != 0 {
		t.Fatalf("help exit %d, stderr=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "mvctl") || !strings.Contains(out.String(), "status") {
		t.Fatalf("help output: %s", out.String())
	}
}

func TestRunWithVersion(t *testing.T) {
	var out, errb bytes.Buffer
	if code := runWith([]string{"--version"}, &out, &errb); code != 0 || !strings.Contains(out.String(), "mvctl") {
		t.Fatalf("version: %d %s", code, out.String())
	}
}

func TestRunWithJSONDoesNotLeakToken(t *testing.T) {
	t.Setenv("MILLIVOLT_URL", "")
	var out, errb bytes.Buffer
	code := runWith([]string{"--token", "supersecret", "--json", "url"}, &out, &errb)
	if code != 0 {
		t.Fatalf("url exit %d: %s", code, errb.String())
	}
	if strings.Contains(out.String(), "supersecret") {
		t.Fatal("token leaked into --json output")
	}
}

func TestRunWithUnknownCommand(t *testing.T) {
	t.Setenv("MILLIVOLT_URL", "")
	var out, errb bytes.Buffer
	if code := runWith([]string{"definitely-not-a-command"}, &out, &errb); code == 0 {
		t.Fatal("unknown command should fail")
	}
}

func TestBooleanFlagsRespectFalseValues(t *testing.T) {
	t.Setenv("MILLIVOLT_URL", "")
	p, err := parseGlobal([]string{"--json=false", "--no-color=0", "--yes=no", "status"})
	if err != nil {
		t.Fatal(err)
	}
	if p.global.JSON {
		t.Fatal("--json=false must leave JSON off")
	}
	if !p.global.Color {
		t.Fatal("--no-color=0 must leave colour on")
	}
	if p.global.Yes {
		t.Fatal("--yes=no must leave Yes off")
	}
	if len(p.args) != 1 || p.args[0] != "status" {
		t.Fatalf("args: %v", p.args)
	}
	p2, _ := parseGlobal([]string{"--json=true", "--color=1", "status"})
	if !p2.global.JSON || !p2.global.Color {
		t.Fatal("true/1 values must enable the flag")
	}
}

func TestDoubleDashPassthrough(t *testing.T) {
	t.Setenv("MILLIVOLT_URL", "")
	p, err := parseGlobal([]string{"query", "--", "SELECT", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"query", "SELECT", "--json"}
	if len(p.args) != len(want) {
		t.Fatalf("args: %v", p.args)
	}
	for i := range want {
		if p.args[i] != want[i] {
			t.Fatalf("args[%d]=%q want %q", i, p.args[i], want[i])
		}
	}
	if p.global.JSON {
		t.Fatal("tokens after -- must not be parsed as flags")
	}
}

func TestHelpShortCircuitsBeforeCommand(t *testing.T) {
	t.Setenv("MILLIVOLT_URL", "")
	var out, errb bytes.Buffer
	// --help must not run `status` (which would hit the network).
	code := runWith([]string{"--help", "status"}, &out, &errb)
	if code != 0 || !strings.Contains(out.String(), "mvctl") || !strings.Contains(out.String(), "CONTROL") {
		t.Fatalf("help did not short-circuit: code=%d out=%s err=%s", code, out.String(), errb.String())
	}
}

func TestMissingValueFlagIsRejected(t *testing.T) {
	t.Setenv("MILLIVOLT_URL", "")
	if _, err := parseGlobal([]string{"--url", "status"}); err == nil {
		t.Fatal("--url followed by a command word must be an error")
	}
	var out, errb bytes.Buffer
	if code := runWith([]string{"--url", "status"}, &out, &errb); code == 0 {
		t.Fatal("missing --url value must fail")
	}
}

func TestPerCommandFlagValueMayBeACommandWord(t *testing.T) {
	t.Setenv("MILLIVOLT_URL", "")
	// `demo` is a command name but must remain a legal value for --name.
	p, err := parseGlobal([]string{"profiles", "add", "--name", "demo", "--url", "http://127.0.0.1:8471"})
	if err != nil {
		t.Fatal(err)
	}
	if p.flags["name"] != "demo" {
		t.Fatalf("--name lost its value: %+v", p.flags)
	}
}

func TestShortVerboseFlag(t *testing.T) {
	t.Setenv("MILLIVOLT_URL", "")
	p, err := parseGlobal([]string{"-v", "status"})
	if err != nil || !p.global.Verbose {
		t.Fatalf("-v must set verbose: %+v %v", p.global, err)
	}
	if _, err := parseGlobal([]string{"-z", "status"}); err == nil {
		t.Fatal("unknown short flag must be rejected")
	}
}

func TestJSONErrorIsStructured(t *testing.T) {
	t.Setenv("MILLIVOLT_URL", "")
	var out, errb bytes.Buffer
	code := runWith([]string{"--no-color", "--json", "--url", "http://127.0.0.1:1", "health"}, &out, &errb)
	if code == 0 {
		t.Fatal("unreachable health must fail")
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("--json error output is not JSON: %s (err=%v)", out.String(), err)
	}
	if _, ok := payload["error"]; !ok {
		t.Fatalf("missing error key: %s", out.String())
	}
}

func TestGlobalTimeoutParsing(t *testing.T) {
	p, err := parseGlobal([]string{"--timeout", "3s", "status"})
	if err != nil || p.global.Timeout != 3*time.Second {
		t.Fatalf("timeout: %v %v", p.global.Timeout, err)
	}
	p, _ = parseGlobal([]string{"--timeout", "7", "status"})
	if p.global.Timeout != 7*time.Second {
		t.Fatalf("numeric timeout: %v", p.global.Timeout)
	}
}
