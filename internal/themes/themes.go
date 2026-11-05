// Package themes is the millivolt dashboard palette registry, expressed for a
// terminal. Values are copied from millivolt upstream:
//   internal/web/static/css/themes.css  (surface, accent and material tokens)
//   internal/web/static/js/theme.js     (registry, labels, hints, defaults)
//
// Every palette has a "black" and a "clear" mode. Mode blocks own text and
// semantic status ink; palette blocks own surfaces and accents. Contrast is
// the one palette that also overrides text and muted.

package themes

import "strings"

type Theme struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Hint  string `json:"hint"`
}

type Palette struct {
	ID, Label, Hint, Mode                    string
	Bg, Panel, Panel2, Rail, Border, Border2 string
	Accent, AccentHover, Accent2, Cyan       string
	Text, Muted, Ok, Warn, Err, RL, Neutral  string
	ShadowRGB, VeilRGB                       string
}

type block struct {
	Bg, Panel, Panel2, Rail, Border, Border2 string
	Accent, AccentHover, Accent2, Cyan       string
	ShadowRGB, VeilRGB                       string
}

var registry = []Theme{
	{ID: "instrument", Label: "Instrument", Hint: "Warm graphite, blue and violet"},
	{ID: "slate", Label: "Slate", Hint: "Cool stone, lilac and sand"},
	{ID: "midnight", Label: "Midnight", Hint: "Deep navy, sky and periwinkle"},
	{ID: "nord", Label: "Nord", Hint: "Arctic slate, frost and heather"},
	{ID: "forest", Label: "Forest", Hint: "Moss, emerald and lime"},
	{ID: "rose", Label: "Rose", Hint: "Plum, pink and lilac"},
	{ID: "ember", Label: "Ember", Hint: "Charcoal, orange and orchid"},
	{ID: "contrast", Label: "Contrast", Hint: "Maximum separation"},
}

// Order: bg, panel, panel2, rail, border, border2, accent, accent-hover,
// accent2, cyan, shadow-rgb, veil-rgb.
var blocks = map[string]map[string]block{
	"instrument": {
		"black": {"#141210", "#1a1714", "#221e19", "#100e0b", "#2b2620", "#3a342b", "#7aa0ff", "#9ab8ff", "#b18cff", "#38d5c0", "0,0,0", "16,14,11"},
		"clear": {"#f7f2e8", "#fffdf7", "#f0e8d8", "#efe7d6", "#ddd2bf", "#c6b89f", "#1e40af", "#1e3a8a", "#5b21b6", "#0b5f66", "74,60,38", "58,44,24"},
	},
	"slate": {
		"black": {"#17141b", "#1f1b25", "#28222f", "#121016", "#352d3d", "#473b51", "#b8a8c8", "#d0c0e0", "#c8b8a8", "#5eead4", "6,4,10", "10,8,14"},
		"clear": {"#f5f1f7", "#fdfbff", "#eee7f2", "#efe8f3", "#dbd0e2", "#c3b3cd", "#5b4b6b", "#4a3b58", "#5a4a40", "#3f5f66", "58,48,66", "48,38,56"},
	},
	"midnight": {
		"black": {"#0d1420", "#141d2b", "#1b2738", "#09101a", "#26364c", "#344a64", "#38bdf8", "#67d0fb", "#9aa4ff", "#67e8f9", "2,6,12", "6,10,16"},
		"clear": {"#eaf1fb", "#f8fcff", "#e2ebf7", "#e5eef9", "#cad9ea", "#abc1d8", "#075985", "#0c4a6e", "#3730a3", "#155e75", "38,52,72", "28,40,58"},
	},
	"nord": {
		"black": {"#121820", "#1a222b", "#202932", "#0e1319", "#2f3c47", "#40515f", "#88c0d0", "#9fd0dd", "#c4a0b8", "#99f6e4", "4,8,12", "8,12,16"},
		"clear": {"#edf3f1", "#f9fdfa", "#e2ede8", "#e5efeb", "#cbdad3", "#aec5ba", "#1f5561", "#16424c", "#6b3f63", "#2f5d5a", "40,56,60", "32,46,50"},
	},
	"forest": {
		"black": {"#0f1713", "#16211a", "#1d2b22", "#0b100d", "#293c30", "#385240", "#34d399", "#6ee7b7", "#a3e635", "#2dd4bf", "3,8,6", "6,10,8"},
		"clear": {"#eef6ef", "#fbfdfb", "#e3efe5", "#e6f1e8", "#c9ddcd", "#abc9b2", "#065f46", "#064e3b", "#3f6212", "#1f5a4e", "38,58,44", "30,48,36"},
	},
	"rose": {
		"black": {"#181016", "#21171e", "#2a1e27", "#130b10", "#3a2a35", "#4d3947", "#f472b6", "#f9a8d4", "#f0abfc", "#22d3ee", "10,3,7", "14,6,10"},
		"clear": {"#fbf0f5", "#fffafd", "#f6e7ee", "#f5e3eb", "#e2ccd6", "#ceadbc", "#9d174d", "#831843", "#86198f", "#5b4a63", "64,42,52", "54,32,44"},
	},
	"ember": {
		"black": {"#18110c", "#211810", "#281d14", "#130c07", "#3d2d20", "#523c2b", "#ff6600", "#ff8a3d", "#e879f9", "#4fd1c5", "8,5,2", "12,8,4"},
		"clear": {"#fbeee2", "#fff8f2", "#f6e4d6", "#f2dbc7", "#e2c9b8", "#ceab92", "#5b2710", "#471e0b", "#7e1a86", "#6b4a3f", "68,48,32", "58,38,24"},
	},
	"contrast": {
		"black": {"#000000", "#0a0a0a", "#141414", "#000000", "#333333", "#555555", "#ffe14d", "#fff08a", "#7cc7ff", "#4de8e0", "0,0,0", "0,0,0"},
		"clear": {"#ffffff", "#ffffff", "#f2f2f2", "#f2f2f2", "#999999", "#555555", "#000000", "#1a1a1a", "#003f8f", "#004f4f", "0,0,0", "0,0,0"},
	},
}

type ink struct{ Text, Muted, Ok, Warn, Err, RL, Neutral string }

var inks = map[string]ink{
	"black": {"#e8e4de", "#97907e", "#2fd186", "#f4c14d", "#ff7b7b", "#ff9346", "#a8a096"},
	"clear": {"#2a241a", "#5f574a", "#0c5d33", "#6b4200", "#8f1418", "#8f3300", "#6f6659"},
}

var contrastInk = map[string]ink{
	"black": {"#ffffff", "#c9c9c9", "#2fd186", "#f4c14d", "#ff7b7b", "#ff9346", "#a8a096"},
	"clear": {"#000000", "#444444", "#0c5d33", "#6b4200", "#8f1418", "#8f3300", "#6f6659"},
}

func Themes() []Theme { return append([]Theme{}, registry...) }

func Names() []string {
	out := make([]string, 0, len(registry))
	for _, t := range registry {
		out = append(out, t.ID)
	}
	return out
}

func Modes() []string { return []string{"black", "clear"} }

func Default() (string, string) { return "instrument", "black" }

func Normalize(theme, mode string) (string, string) {
	theme = strings.ToLower(strings.TrimSpace(theme))
	mode = strings.ToLower(strings.TrimSpace(mode))
	validTheme := false
	for _, t := range registry {
		if t.ID == theme {
			validTheme = true
			break
		}
	}
	if !validTheme {
		theme = "instrument"
	}
	if mode != "black" && mode != "clear" {
		mode = "black"
	}
	return theme, mode
}

func Get(theme, mode string) Palette {
	t, m := Normalize(theme, mode)
	var meta Theme
	for _, entry := range registry {
		if entry.ID == t {
			meta = entry
			break
		}
	}
	b := blocks[t][m]
	i := inks[m]
	if t == "contrast" {
		i = contrastInk[m]
	}
	return Palette{
		ID: t, Label: meta.Label, Hint: meta.Hint, Mode: m,
		Bg: b.Bg, Panel: b.Panel, Panel2: b.Panel2, Rail: b.Rail,
		Border: b.Border, Border2: b.Border2,
		Accent: b.Accent, AccentHover: b.AccentHover, Accent2: b.Accent2, Cyan: b.Cyan,
		Text: i.Text, Muted: i.Muted, Ok: i.Ok, Warn: i.Warn, Err: i.Err, RL: i.RL, Neutral: i.Neutral,
		ShadowRGB: b.ShadowRGB, VeilRGB: b.VeilRGB,
	}
}

// Hex resolves a token name to its colour. Unknown tokens return a neutral grey
// so a typo degrades to readable text instead of an invalid escape sequence.
func (p Palette) Hex(token string) string {
	switch strings.ToLower(strings.TrimPrefix(token, "--")) {
	case "bg":
		return p.Bg
	case "panel":
		return p.Panel
	case "panel2":
		return p.Panel2
	case "rail":
		return p.Rail
	case "border":
		return p.Border
	case "border2":
		return p.Border2
	case "accent":
		return p.Accent
	case "accent-hover", "accent_hover":
		return p.AccentHover
	case "accent2":
		return p.Accent2
	case "cyan":
		return p.Cyan
	case "text":
		return p.Text
	case "muted":
		return p.Muted
	case "ok":
		return p.Ok
	case "warn":
		return p.Warn
	case "err":
		return p.Err
	case "rl":
		return p.RL
	case "neutral":
		return p.Neutral
	default:
		return "#888888"
	}
}
