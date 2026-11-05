// Package demo generates labeled showcase traffic through a millivolt proxy so
// the TUI, counters, cost and request log light up without real provider keys.
// Everything is in-process and standard library only.

package demo

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type Config struct {
	BaseURL string
	// The operator credential deliberately has no place here: showcase traffic
	// targets millivolt's transparent inference plane, which forwards the
	// *upstream* key. Passing the operator token upstream would leak it.
	Client      string
	Upstream    string
	Requests    int
	Concurrency int
	Stream      bool
	Models      []string
	ErrorRate   float64
	TimeoutMS   int
	Realtime    bool
}

type Result struct {
	Sent, Succeeded, Failed int
	P50, P95, P99, Min, Max float64
	TTFTMs                  float64
	Elapsed                 float64
	Bytes                   int64
}

type ChatConfig struct {
	BaseURL, Upstream, Model, Prompt, Client, APIKey string
	Stream                                           bool
	TimeoutMS                                        int
}

func defaults(c *Config) {
	if c.Client == "" {
		c.Client = "mvctl-demo"
	}
	if c.Requests <= 0 {
		c.Requests = 12
	}
	if c.Concurrency <= 0 {
		c.Concurrency = 4
	}
	if len(c.Models) == 0 {
		c.Models = []string{"mvctl-demo-small", "mvctl-demo-large"}
	}
	if c.TimeoutMS <= 0 {
		c.TimeoutMS = 15_000
	}
	if c.ErrorRate < 0 {
		c.ErrorRate = 0
	}
	if c.ErrorRate > 1 {
		c.ErrorRate = 1
	}
}

// StartMockUpstream starts a minimal OpenAI-compatible upstream. When millivolt
// runs in Docker the listener must be reachable from the container via the host
// gateway, so it binds all interfaces in that mode; MVCTL_DEMO_LOOPBACK=1 keeps
// it loopback-only for tests and for a host-native proxy.
func StartMockUpstream(ctx context.Context) (addr string, baseURL string, stop func(), err error) {
	bind := "0.0.0.0:0"
	loopback := os.Getenv("MVCTL_DEMO_LOOPBACK") == "1"
	if loopback {
		bind = "127.0.0.1:0"
	}
	listener, err := net.Listen("tcp", bind)
	if err != nil {
		return "", "", func() {}, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"object": "list",
			"data":   []map[string]any{{"id": "mvctl-demo-small"}, {"id": "mvctl-demo-large"}},
		})
	})
	mux.HandleFunc("/v1/bad", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "mvctl demo upstream failure"})
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body)
		if body.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			for _, chunk := range []string{"mvctl ", "demo ", "response"} {
				data, _ := json.Marshal(map[string]any{
					"choices": []map[string]any{{"delta": map[string]any{"content": chunk}}},
				})
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "chatcmpl-mvctl", "object": "chat.completion", "model": body.Model,
			"choices": []map[string]any{{"index": 0,
				"message":       map[string]any{"role": "assistant", "content": "mvctl demo response"},
				"finish_reason": "stop"}},
			"usage": map[string]any{"prompt_tokens": 24, "completion_tokens": 12, "total_tokens": 36},
		})
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(listener) }()
	port := listener.Addr().(*net.TCPAddr).Port
	addr = fmt.Sprintf("127.0.0.1:%d", port)
	if loopback {
		baseURL = fmt.Sprintf("http://127.0.0.1:%d/v1", port)
	} else {
		baseURL = fmt.Sprintf("http://host.docker.internal:%d/v1", port)
	}
	// Tie the server lifetime to ctx so a cancelled demo never leaves a
	// listening socket behind.
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	stopFn := func() {
		shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}
	return addr, baseURL, stopFn, nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// Run sends Config.Requests labeled requests through millivolt.
func Run(ctx context.Context, c Config) (Result, error) {
	defaults(&c)
	var result Result
	upstream := c.Upstream
	if upstream == "" {
		_, u, stop, err := StartMockUpstream(ctx)
		if err != nil {
			return result, err
		}
		defer stop()
		upstream = u
	}
	client := &http.Client{Timeout: time.Duration(c.TimeoutMS) * time.Millisecond}
	start := time.Now()
	jobs := make(chan int, c.Requests)
	for i := 0; i < c.Requests; i++ {
		jobs <- i
	}
	close(jobs)

	var mu sync.Mutex
	latencies := make([]float64, 0, c.Requests)
	var wg sync.WaitGroup
	for w := 0; w < c.Concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}
				// Deterministic exact count, no overflow from 1/ErrorRate.
				bad := i < int(math.Round(float64(c.Requests)*c.ErrorRate))
				elapsed, ok, n := sendOne(ctx, client, c, upstream, i, bad)
				mu.Lock()
				result.Sent++
				if ok {
					result.Succeeded++
				} else {
					result.Failed++
				}
				result.Bytes += n
				latencies = append(latencies, elapsed)
				mu.Unlock()
				if c.Realtime {
					time.Sleep(120 * time.Millisecond)
				}
			}
		}()
	}
	wg.Wait()
	result.Elapsed = time.Since(start).Seconds()
	if len(latencies) > 0 {
		sort.Float64s(latencies)
		result.Min = latencies[0]
		result.Max = latencies[len(latencies)-1]
		result.P50 = percentile(latencies, 0.50)
		result.P95 = percentile(latencies, 0.95)
		result.P99 = percentile(latencies, 0.99)
	}
	return result, nil
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p*float64(len(sorted)-1) + 0.5)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func sendOne(ctx context.Context, client *http.Client, c Config, upstream string, i int, bad bool) (float64, bool, int64) {
	model := c.Models[i%len(c.Models)]
	payload := map[string]any{
		"model": model, "stream": c.Stream, "max_tokens": 32,
		"messages": []map[string]any{{"role": "user", "content": fmt.Sprintf("mvctl showcase request %d", i)}},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.BaseURL, "/")+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return 0, false, 0
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Proxy-Base-URL", upstream)
	req.Header.Set("X-Proxy-Client", c.Client)
	req.Header.Set("Authorization", "Bearer mvctl-demo-key")
	if bad {
		req.Header.Set("X-Proxy-Path", "/v1/bad")
	}
	t0 := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return time.Since(t0).Seconds() * 1000, false, 0
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return time.Since(t0).Seconds() * 1000, resp.StatusCode >= 200 && resp.StatusCode < 300, int64(len(data))
}

// Chat performs one request through the proxy and returns the assistant text.
func Chat(ctx context.Context, cfg ChatConfig) (string, error) {
	if cfg.Upstream == "" {
		return "", fmt.Errorf("--upstream is required (point it at an OpenAI-compatible base URL)")
	}
	if cfg.Prompt == "" {
		cfg.Prompt = "Say hello from mvctl."
	}
	if cfg.Client == "" {
		cfg.Client = "mvctl-chat"
	}
	if cfg.APIKey == "" {
		cfg.APIKey = "mvctl-demo-key"
	}
	if cfg.TimeoutMS <= 0 {
		cfg.TimeoutMS = 30_000
	}
	payload := map[string]any{
		"model": cfg.Model, "stream": cfg.Stream,
		"messages": []map[string]any{{"role": "user", "content": cfg.Prompt}},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(cfg.BaseURL, "/")+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Proxy-Base-URL", cfg.Upstream)
	req.Header.Set("X-Proxy-Client", cfg.Client)
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	client := &http.Client{Timeout: time.Duration(cfg.TimeoutMS) * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
		return "", fmt.Errorf("proxy returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	if cfg.Stream {
		return readSSE(resp.Body), nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return string(data), nil
	}
	if len(out.Choices) > 0 {
		return out.Choices[0].Message.Content, nil
	}
	return string(data), nil
}

func readSSE(r io.Reader) string {
	var b strings.Builder
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err == nil && len(chunk.Choices) > 0 {
			b.WriteString(chunk.Choices[0].Delta.Content)
		}
	}
	_ = scanner.Err() // a truncated SSE stream simply yields what was read
	return b.String()
}
