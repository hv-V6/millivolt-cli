package themes

import (
	"regexp"
	"testing"
)

var hexPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func TestRegistryCompleteness(t *testing.T) {
	want := []string{"instrument", "slate", "midnight", "nord", "forest", "rose", "ember", "contrast"}
	got := Names()
	if len(got) != len(want) {
		t.Fatalf("expected %d themes, got %d: %v", len(want), len(got), got)
	}
	for i, id := range want {
		if got[i] != id {
			t.Fatalf("theme %d: want %s got %s", i, id, got[i])
		}
	}
	if len(Modes()) != 2 || Modes()[0] != "black" || Modes()[1] != "clear" {
		t.Fatalf("modes: %v", Modes())
	}
	if th, mo := Default(); th != "instrument" || mo != "black" {
		t.Fatalf("default: %s/%s", th, mo)
	}
}

func TestEveryPaletteIsComplete(t *testing.T) {
	for _, id := range Names() {
		for _, mode := range Modes() {
			p := Get(id, mode)
			if p.ID != id || p.Mode != mode || p.Label == "" || p.Hint == "" {
				t.Fatalf("incomplete metadata for %s/%s: %+v", id, mode, p)
			}
			for name, value := range map[string]string{
				"bg": p.Bg, "panel": p.Panel, "panel2": p.Panel2, "rail": p.Rail,
				"border": p.Border, "border2": p.Border2, "accent": p.Accent,
				"accentHover": p.AccentHover, "accent2": p.Accent2, "cyan": p.Cyan,
				"text": p.Text, "muted": p.Muted, "ok": p.Ok, "warn": p.Warn,
				"err": p.Err, "rl": p.RL, "neutral": p.Neutral,
			} {
				if !hexPattern.MatchString(value) {
					t.Fatalf("%s/%s %s is not a 6-digit hex: %q", id, mode, name, value)
				}
			}
			if p.ShadowRGB == "" || p.VeilRGB == "" {
				t.Fatalf("%s/%s missing material tokens", id, mode)
			}
		}
	}
}

func TestNormalizeFallsBack(t *testing.T) {
	th, mo := Normalize("NOPE", "NOPE")
	if th != "instrument" || mo != "black" {
		t.Fatalf("normalize: %s/%s", th, mo)
	}
	th, mo = Normalize("  MIDNIGHT ", "CLEAR")
	if th != "midnight" || mo != "clear" {
		t.Fatalf("normalize case/space: %s/%s", th, mo)
	}
}

func TestHexTokens(t *testing.T) {
	p := Get("forest", "black")
	if p.Hex("accent") != p.Accent || p.Hex("--bg") != p.Bg || p.Hex("err") != p.Err {
		t.Fatal("Hex() token lookup")
	}
	if p.Hex("not-a-token") != "#888888" {
		t.Fatal("unknown token must degrade to grey")
	}
}

func TestContrastOverridesInk(t *testing.T) {
	black := Get("contrast", "black")
	clear := Get("contrast", "clear")
	if black.Text != "#ffffff" || black.Muted != "#c9c9c9" {
		t.Fatalf("contrast black ink: %+v", black)
	}
	if clear.Text != "#000000" || clear.Muted != "#444444" {
		t.Fatalf("contrast clear ink: %+v", clear)
	}
	normal := Get("instrument", "black")
	if normal.Text == black.Text {
		t.Fatal("contrast must not leak into other palettes")
	}
}
