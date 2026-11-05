package tui

import (
	"strings"
	"testing"

	"github.com/hv-V6/mvctl/internal/themes"
)

func sampleData() TopData {
	ttft, tps := 3674.28, 270.10
	return TopData{
		URL: "http://127.0.0.1:8471", Version: "4c94a75604d6",
		Theme: "instrument", Mode: "black", Healthy: true, Demo: true,
		Requests: 39275, Errors: 108, InFlight: 1, Cost: 28.69,
		InputTokens: 9909959702, OutputTokens: 28766213, CacheReadTokens: 9770446257,
		AvgTTFTMs: &ttft, AvgTPS: &tps, StorageEnabled: true,
		History:   []float64{8, 9, 11, 7, 14, 12, 16, 13, 18, 15, 20, 17},
		Providers: []BarItem{{Label: "openrouter", Value: 16842}, {Label: "opencode-zen", Value: 9201}},
		Rows: []Row{{
			Time: "10:17:13", Status: "200", Provider: "opencode-go",
			Model: "hy4", Client: "js 7.19.0", Duration: "1.2s", Tokens: "150",
		}},
	}
}

func TestRenderFrameAtSeveralSizes(t *testing.T) {
	p := themes.Get("midnight", "black")
	d := sampleData()
	for _, size := range [][2]int{{120, 40}, {80, 24}, {40, 10}, {1, 1}, {0, 0}} {
		cols, rows := size[0], size[1]
		frame := RenderFrame(d, p, cols, rows, false)
		if frame == "" {
			t.Fatalf("empty frame at %dx%d", cols, rows)
		}
		if cols >= 20 && !strings.Contains(frame, "mvctl") {
			t.Fatalf("missing title at %dx%d", cols, rows)
		}
		for i, line := range strings.Split(frame, "\n") {
			if w := Width(line); cols > 0 && w > cols {
				t.Fatalf("line %d at %dx%d is %d wide: %q", i, cols, rows, w, line)
			}
		}
	}
}

func TestRenderFrameContent(t *testing.T) {
	p := themes.Get("instrument", "black")
	d := sampleData()
	frame := Strip(RenderFrame(d, p, 120, 40, false))
	for _, want := range []string{
		"Requests", "Errors", "In flight", "Cost", "Cache read", "Avg TTFT", "Avg TPS",
		"requests / sample", "providers", "recent requests", "DEMO", "q quit",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("frame missing %q:\n%s", want, frame)
		}
	}
}

func TestRenderFrameBadges(t *testing.T) {
	p := themes.Get("rose", "clear")
	d := sampleData()
	d.Paused = true
	frame := Strip(RenderFrame(d, p, 100, 30, true))
	if !strings.Contains(frame, "PAUSED") || !strings.Contains(frame, "toggle pause hold") {
		t.Fatalf("paused badge / help missing:\n%s", frame)
	}
}

func TestPalettesProduceDifferentColours(t *testing.T) {
	defer SetColor(ColorEnabled())
	SetColor(true)
	d := sampleData()
	a := RenderFrame(d, themes.Get("instrument", "black"), 100, 30, false)
	b := RenderFrame(d, themes.Get("ember", "clear"), 100, 30, false)
	if a == b {
		t.Fatal("different palettes produced identical escape sequences")
	}
	if !strings.Contains(a, "\x1b[") {
		t.Fatal("colour enabled but no escape sequences")
	}
}

func TestRenderers(t *testing.T) {
	p := themes.Get("nord", "black")
	if Num(1234567) != "1.23M" || Cost(28.6933) != "$28.693" || Dur(3674) != "3.7s" {
		t.Fatal("formatters")
	}
	if Bytes(2*1024*1024) != "2.0MiB" {
		t.Fatal("bytes")
	}
	if Strip(Spark(p, []float64{1, 2, 3}, 10)) == "" {
		t.Fatal("spark empty")
	}
	if !strings.Contains(Strip(Bar(p, "provider", 5, 10, 10)), "provider") {
		t.Fatal("bar missing label")
	}
	if !strings.Contains(Table(p, []string{"a", "b"}, [][]string{{"1", "2"}}, 0), "1") {
		t.Fatal("table")
	}
	if !strings.Contains(KV(p, [][2]string{{"key", "value"}}), "value") {
		t.Fatal("kv")
	}
	if !strings.Contains(Heading(p, "title", 40), "title") {
		t.Fatal("heading")
	}
	if Strip(Pad("ab", 5, true)) != "   ab" {
		t.Fatal("pad right")
	}
	if Width(Clip(RGB("abcdefgh", "#112233"), 3)) != 3 {
		t.Fatal("clip must preserve visible width")
	}
}
