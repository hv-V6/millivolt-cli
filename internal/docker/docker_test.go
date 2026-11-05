package docker

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestFilterMillivoltRowsIgnoresSidecars(t *testing.T) {
	rows := []string{
		"millivolt\tghcr.io/llm-4-people/millivolt:main\trunning",
		"millivolt-opencode-go\tcaddy:2-alpine\trunning", // name-only match must be dropped
		"other\tsome/other:image\trunning",
		"malformed",
		"millivolt-2\tMILLIVOLT:custom\texited", // case-insensitive image match
	}
	got := FilterMillivoltRows(rows)
	if len(got) != 2 {
		t.Fatalf("expected 2 rows, got %d: %v", len(got), got)
	}
	for _, row := range got {
		if strings.Contains(row, "caddy") || strings.Contains(row, "other:image") {
			t.Fatalf("sidecar leaked through: %v", got)
		}
	}
}

func TestHostPortPatternIPv4IPv6(t *testing.T) {
	cases := []struct {
		line string
		port string
	}{
		{"8080/tcp -> 127.0.0.1:8471", "8471"},
		{"8080/tcp -> 0.0.0.0:8080", "8080"},
		{"8080/tcp -> [::1]:8471", "8471"},
		{"8080/tcp -> :::8471", "8471"},
	}
	for _, tc := range cases {
		m := hostPortPattern.FindStringSubmatch(tc.line)
		if m == nil || m[1] != tc.port {
			t.Fatalf("line %q did not match: %v", tc.line, m)
		}
	}
	if hostPortPattern.MatchString("8080/tcp -> nonsense") {
		t.Fatal("nonsense must not match")
	}
}

func TestLastLine(t *testing.T) {
	if got := LastLine("a\nb\n\nc\n"); got != "c" {
		t.Fatalf("last line: %q", got)
	}
	if got := LastLine(""); got != "" {
		t.Fatalf("empty: %q", got)
	}
	long := strings.Repeat("x", 500)
	if got := LastLine(long); len(got) != 400 {
		t.Fatalf("last line not capped: %d", len(got))
	}
}

func TestRunProcessTimesOut(t *testing.T) {
	// A missing binary fails fast and must never panic or hang.
	res := RunProcess(context.Background(), "mvctl-definitely-not-a-real-binary",
		[]string{"--help"}, RunOptions{TimeoutMS: 1000})
	if res.Code == 0 {
		t.Fatal("missing binary must not report success")
	}
	_ = time.Now()
}
