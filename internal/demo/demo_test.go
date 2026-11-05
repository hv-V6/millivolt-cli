package demo

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStartMockUpstream(t *testing.T) {
	t.Setenv("MVCTL_DEMO_LOOPBACK", "1")
	addr, baseURL, stop, err := StartMockUpstream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if addr == "" || !strings.Contains(baseURL, addr[len("127.0.0.1:"):]) {
		t.Fatalf("addr=%s baseURL=%s", addr, baseURL)
	}
	client := &http.Client{Timeout: 3 * time.Second}

	models, err := client.Get(baseURL + "/models")
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(models.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	models.Body.Close()
	if len(list.Data) != 2 {
		t.Fatalf("models: %+v", list.Data)
	}

	resp, err := client.Post(baseURL+"/chat/completions", "application/json",
		strings.NewReader(`{"model":"mvctl-demo-small","messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&completion); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if completion.Usage.TotalTokens != 36 || len(completion.Choices) == 0 ||
		completion.Choices[0].Message.Content != "mvctl demo response" {
		t.Fatalf("completion: %+v", completion)
	}

	stream, err := client.Post(baseURL+"/chat/completions", "application/json",
		strings.NewReader(`{"model":"mvctl-demo-large","stream":true}`))
	if err != nil {
		t.Fatal(err)
	}
	sse, _ := io.ReadAll(stream.Body)
	stream.Body.Close()
	if !strings.Contains(string(sse), "data: [DONE]") || !strings.Contains(string(sse), `"delta"`) {
		t.Fatalf("stream: %s", sse)
	}

	bad, err := client.Post(baseURL+"/bad", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	bad.Body.Close()
	if bad.StatusCode != http.StatusInternalServerError {
		t.Fatalf("bad endpoint status %d", bad.StatusCode)
	}
}

// fakeMillivolt records the routing headers mvctl must send and answers with a
// small completion body.
func fakeMillivolt(t *testing.T, seen chan http.Header) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen != nil {
			select {
			case seen <- r.Header.Clone():
			default:
			}
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 1<<16))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}],"usage":{"total_tokens":36}}`))
	}))
}

func TestRunSendsRoutingHeaders(t *testing.T) {
	t.Setenv("MVCTL_DEMO_LOOPBACK", "1")
	_, upstream, stop, err := StartMockUpstream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	seen := make(chan http.Header, 32)
	proxy := fakeMillivolt(t, seen)
	defer proxy.Close()

	res, err := Run(context.Background(), Config{
		BaseURL: proxy.URL, Upstream: upstream, Client: "mvctl-demo",
		Requests: 8, Concurrency: 3, TimeoutMS: 5000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Sent != 8 || res.Succeeded != 8 || res.Failed != 0 {
		t.Fatalf("result: %+v", res)
	}
	if !(res.P50 <= res.P95 && res.P95 <= res.P99) || res.Min > res.P50 {
		t.Fatalf("percentiles: %+v", res)
	}
	close(seen)
	found := false
	for h := range seen {
		if h.Get("X-Proxy-Client") != "mvctl-demo" {
			t.Fatalf("client label missing: %+v", h)
		}
		if h.Get("X-Proxy-Base-URL") != upstream {
			t.Fatalf("upstream header missing: %+v", h)
		}
		found = true
	}
	if !found {
		t.Fatal("no requests recorded")
	}
}

func TestRunErrorRateUsesBadPath(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Proxy-Path") == "/v1/bad" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer proxy.Close()
	res, err := Run(context.Background(), Config{
		BaseURL: proxy.URL, Upstream: "http://127.0.0.1:1/v1",
		Requests: 4, Concurrency: 1, ErrorRate: 1, TimeoutMS: 3000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Succeeded != 0 || res.Failed != 4 {
		t.Fatalf("expected all failures: %+v", res)
	}
}

func TestChat(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello from mvctl"}}]}`))
	}))
	defer proxy.Close()
	text, err := Chat(context.Background(), ChatConfig{
		BaseURL: proxy.URL, Upstream: "http://127.0.0.1:1/v1", Prompt: "hi",
	})
	if err != nil || text != "hello from mvctl" {
		t.Fatalf("chat: %q %v", text, err)
	}
	if _, err := Chat(context.Background(), ChatConfig{BaseURL: proxy.URL}); err == nil {
		t.Fatal("missing upstream must error")
	}
}

func TestChatStreaming(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"a \"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"b\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer proxy.Close()
	text, err := Chat(context.Background(), ChatConfig{
		BaseURL: proxy.URL, Upstream: "http://127.0.0.1:1/v1", Stream: true,
	})
	if err != nil || text != "a b" {
		t.Fatalf("stream chat: %q %v", text, err)
	}
}

func TestRunHonoursCancellationAndDefaults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	proxy := fakeMillivolt(t, nil)
	defer proxy.Close()
	if _, err := Run(ctx, Config{BaseURL: proxy.URL, Upstream: "http://127.0.0.1:1/v1"}); err != nil {
		t.Fatal(err)
	}
	c := Config{}
	defaults(&c)
	if c.Requests != 12 || c.Concurrency != 4 || len(c.Models) != 2 || c.Client != "mvctl-demo" {
		t.Fatalf("defaults: %+v", c)
	}
}
