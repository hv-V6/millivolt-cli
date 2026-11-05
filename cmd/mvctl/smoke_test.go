package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeMillivolt serves the exact shapes observed from a live millivolt, so the
// whole CLI (parse -> client -> decode -> render) is exercised end to end.
func fakeMillivolt(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/metrics/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"kpi":{"requests":39275,"errors":108,"in_flight":1,"cost":28.69,
				"cost_per_req":0.0007,"cost_per_mtok":0.007,
				"input_tokens":9909959702,"output_tokens":28766213,
				"cache_read_tokens":9770446257,"reasoning_tokens":15131460,
				"avg_ttft_ms":3674.27,"avg_tps":270.10},
			"storage":{"enabled":true,"dropped":0,"totals_degraded":false},
			"pause":{"paused":false,"queued":0,"holds":[],"known_clients":[],"known_providers":[]},
			"throttle":{"active":false,"throttles":[],"known_providers":[]},
			"debug":{"enabled":false,"sessions":[]},
			"dashboard_version":"4c94a75604d63700",
			"counters":{"in_flight":1,"total_requests":39275,"total_errors":108}}`))
	})
	mux.HandleFunc("/metrics/agg/log", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"records":[
			{"id":"a","start":"2026-11-05T10:17:13Z","status_code":200,
			 "provider":"opencode-go","model":"hy4","client":"js 7.19.0","stream":true,
			 "duration_ms":1234,"ttft_ms":367,"usage":{"input_tokens":120,"output_tokens":30},
			 "retries":0,"error_type":""},
			{"id":"b","start":"2026-11-05T10:17:14Z","status_code":429,
			 "provider":"openrouter","model":"gpt-5","client":"codex",
			 "duration_ms":88,"usage":{"input_tokens":10,"output_tokens":0},
			 "error_type":"rate_limited"}],
			"more":false,"cursor_ms":0,"cursor_id":"b"}`))
	})
	mux.HandleFunc("/metrics/agg/chart", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"now_ms":1,"from_ms":0,"bucket_ms":86400000,
			"buckets":[{"t":1792454400000,"req":3343,"err":9,"cost":4.35,"in":135337045,"out":1210843},
			           {"t":1792540800000,"req":4374,"err":10,"cost":10.4,"in":746034325,"out":3118990}]}`))
	})
	mux.HandleFunc("/metrics/agg/explorer", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"dim":"provider","total":39451,"error_total":109,
			"groups":[{"name":"opendesign","n":16842,"cost":12.4,"in":4179692100,"out":1329,"err":4},
			          {"name":"openrouter","n":9201,"cost":8.1,"in":991004400,"out":8821,"err":2}]}`))
	})
	mux.HandleFunc("/metrics/prometheus", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("# HELP llm_proxy_requests_total Requests\nllm_proxy_requests_total 10000\n"))
	})
	mux.HandleFunc("/metrics/query", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"n":1}]`))
	})
	mux.HandleFunc("/admin/config", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"values":{"listen":"0.0.0.0:8471","history_size":10000},
			"effective":{"listen":"0.0.0.0:8471"},"defaults":{"listen":":8080","history_size":10000},
			"revision":"rev-1","restart_required":["listen"],
			"fields":[{"key":"listen","category":"server","kind":"string"},{"key":"history_size","category":"server","kind":"int"}],
			"categories":[],"overrides":{},"writable":true,"path":"/config/proxy.yaml"}`))
	})
	mux.HandleFunc("/admin/pause", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"paused":false,"queued":0,"holds":[]}`))
	})
	mux.HandleFunc("/admin/throttle", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"active":false,"throttles":[],"known_providers":[]}`))
	})
	mux.HandleFunc("/admin/quota", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/admin/debug", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"enabled":false,"sessions":[]}`))
	})
	mux.HandleFunc("/admin/purge/count", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"count":0}`))
	})
	return httptest.NewServer(mux)
}

func runCLI(t *testing.T, argv ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := runWith(argv, &out, &errb)
	return code, out.String(), errb.String()
}

func TestSmokeEndToEnd(t *testing.T) {
	srv := fakeMillivolt(t)
	defer srv.Close()
	base := []string{"--url", srv.URL, "--token", "test-secret-value", "--no-color"}

	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"status"}, []string{"millivolt status", "39.3k", "$28.690", "durable"}},
		{[]string{"overview"}, []string{"overview", "Cache read", "Avg TTFT"}},
		{[]string{"requests", "10"}, []string{"recent requests", "opencode-go", "rate_limited"}},
		{[]string{"chart"}, []string{"chart", "requests", "cost"}},
		{[]string{"explore", "--by", "provider"}, []string{"explore", "opendesign"}},
		{[]string{"cost"}, []string{"cost attribution", "total cost"}},
		{[]string{"models"}, []string{"models"}},
		{[]string{"providers"}, []string{"providers", "operator state"}},
		{[]string{"config", "get"}, []string{"config", "revision", "restart required"}},
		{[]string{"config", "keys", "listen"}, []string{"listen"}},
		{[]string{"config", "diff"}, []string{"config diff", "listen"}},
		{[]string{"pause"}, []string{"pause"}},
		{[]string{"throttle"}, []string{"throttle"}},
		{[]string{"purge", "--client", "mvctl-demo"}, []string{"purge preview", "Preview only"}},
		{[]string{"prometheus"}, []string{"llm_proxy_requests_total"}},
		{[]string{"query", "SELECT 1"}, []string{"n"}},
		{[]string{"themes", "list"}, []string{"instrument", "contrast", "modes"}},
		{[]string{"themes", "preview", "--theme", "midnight", "--mode", "black"}, []string{"theme preview", "midnight"}},
		{[]string{"top", "--demo", "--no-tty", "--theme", "rose"}, []string{"mvctl", "Requests", "DEMO"}},
		{[]string{"tour", "--no-delay"}, []string{"mvctl tour", "status", "chart"}},
		{[]string{"url"}, []string{srv.URL}},
	}
	for _, tc := range cases {
		argv := append(append([]string{}, base...), tc.args...)
		code, out, errb := runCLI(t, argv...)
		if code != 0 {
			t.Fatalf("%v -> exit %d, stderr=%s", tc.args, code, errb)
		}
		for _, want := range tc.want {
			if !strings.Contains(out, want) {
				t.Fatalf("%v: output missing %q:\n%s", tc.args, want, out)
			}
		}
		if strings.Contains(out, "test-secret-value") {
			t.Fatalf("%v: token leaked", tc.args)
		}
	}
}

func TestSmokeJSONMode(t *testing.T) {
	srv := fakeMillivolt(t)
	defer srv.Close()
	code, out, errb := runCLI(t, "--url", srv.URL, "--token", "test-secret-value", "--json", "status")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb)
	}
	for _, want := range []string{`"healthy"`, `"kpi"`, `"requests"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("json missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "test-secret-value") {
		t.Fatal("token leaked into json")
	}
}

func TestSmokeAllCommandsRegister(t *testing.T) {
	cmds := append(CoreCommands(), ExtraCommands()...)
	cmds = append(cmds, LifecycleCommands()...)
	cmds = append(cmds, ShowcaseCommands()...)
	if len(cmds) < 35 {
		t.Fatalf("expected a full command surface, got %d", len(cmds))
	}
	seen := map[string]bool{}
	for _, c := range cmds {
		if seen[c.Name] {
			t.Fatalf("duplicate command name %q", c.Name)
		}
		seen[c.Name] = true
	}
}
