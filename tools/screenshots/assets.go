package main

// README asset generator. It composes the real command PNGs into three
// presentation images, entirely in Go (no HTML, no image editor, no shell).

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

var (
	assetBG     = color.RGBA{R: 0x0d, G: 0x10, B: 0x18, A: 255}
	assetBG2    = color.RGBA{R: 0x18, G: 0x11, B: 0x0c, A: 255}
	assetPanel  = color.RGBA{R: 0x1a, G: 0x1e, B: 0x28, A: 255}
	assetBorder = color.RGBA{R: 0x2c, G: 0x34, B: 0x44, A: 255}
	assetText   = color.RGBA{R: 0xec, G: 0xef, B: 0xf4, A: 255}
	assetMuted  = color.RGBA{R: 0x8b, G: 0x96, B: 0xa8, A: 255}
	assetAccent = color.RGBA{R: 0x7a, G: 0xa0, B: 0xff, A: 255}
	assetCyan   = color.RGBA{R: 0x38, G: 0xd5, B: 0xc0, A: 255}
	assetOrange = color.RGBA{R: 0xff, G: 0x66, B: 0x00, A: 255}
	assetGreen  = color.RGBA{R: 0x2f, G: 0xd1, B: 0x86, A: 255}
)

type assetFonts struct {
	title   font.Face
	heading font.Face
	body    font.Face
	small   font.Face
}

func loadAssetFonts(fontPath string) (*assetFonts, error) {
	data, err := os.ReadFile(fontPath)
	if err != nil {
		return nil, err
	}
	parsed, err := opentype.Parse(data)
	if err != nil {
		return nil, err
	}
	make := func(size float64) (font.Face, error) {
		return opentype.NewFace(parsed, &opentype.FaceOptions{
			Size: size, DPI: 96, Hinting: font.HintingNone,
		})
	}
	title, err := make(62)
	if err != nil {
		return nil, err
	}
	heading, err := make(22)
	if err != nil {
		return nil, err
	}
	body, err := make(15)
	if err != nil {
		return nil, err
	}
	small, err := make(12)
	if err != nil {
		return nil, err
	}
	return &assetFonts{title: title, heading: heading, body: body, small: small}, nil
}

func drawText(img *image.RGBA, face font.Face, x, y int, text string, col color.RGBA) {
	d := &font.Drawer{Dst: img, Src: &image.Uniform{C: col}, Face: face}
	d.Dot = fixed.P(x, y)
	d.DrawString(text)
}

func textWidth(face font.Face, s string) int {
	return font.MeasureString(face, s).Ceil()
}

func fill(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	draw.Draw(img, r, &image.Uniform{C: c}, image.Point{}, draw.Src)
}

func gradient(img *image.RGBA, w, h int, from, to color.RGBA) {
	for y := 0; y < h; y++ {
		f := float64(y) / float64(maxInt2(h-1, 1))
		c := color.RGBA{
			R: uint8(float64(from.R) + (float64(to.R)-float64(from.R))*f),
			G: uint8(float64(from.G) + (float64(to.G)-float64(from.G))*f),
			B: uint8(float64(from.B) + (float64(to.B)-float64(from.B))*f),
			A: 255,
		}
		fill(img, image.Rect(0, y, w, y+1), c)
	}
}

func maxInt2(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func loadPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func savePNG(img image.Image, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// roundRect fills a rectangle with slightly softened corners.
func roundRect(img *image.RGBA, r image.Rectangle, radius int, c color.RGBA) {
	fill(img, image.Rect(r.Min.X+radius, r.Min.Y, r.Max.X-radius, r.Max.Y), c)
	fill(img, image.Rect(r.Min.X, r.Min.Y+radius, r.Max.X, r.Max.Y-radius), c)
	for dy := 0; dy < radius; dy++ {
		for dx := 0; dx < radius; dx++ {
			if dx*dx+dy*dy > radius*radius {
				continue
			}
			cx, cy := r.Min.X+radius-dx, r.Min.Y+radius-dy
			fill(img, image.Rect(cx, cy, cx+1, cy+1), c)
			cx, cy = r.Max.X-radius+dx-1, r.Min.Y+radius-dy
			fill(img, image.Rect(cx, cy, cx+1, cy+1), c)
			cx, cy = r.Min.X+radius-dx, r.Max.Y-radius+dy-1
			fill(img, image.Rect(cx, cy, cx+1, cy+1), c)
			cx, cy = r.Max.X-radius+dx-1, r.Max.Y-radius+dy-1
			fill(img, image.Rect(cx, cy, cx+1, cy+1), c)
		}
	}
}

// fit scaled src into rect, letterboxed on c, using bilinear interpolation.
func fit(img *image.RGBA, r image.Rectangle, src image.Image, c color.RGBA) {
	fill(img, r, c)
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	tw, th := r.Dx()-14, r.Dy()-14
	if tw < 1 || th < 1 {
		return
	}
	scale := math.Min(float64(tw)/float64(sw), float64(th)/float64(sh))
	dw := int(float64(sw) * scale)
	dh := int(float64(sh) * scale)
	dr := image.Rect(
		r.Min.X+(r.Dx()-dw)/2, r.Min.Y+(r.Dy()-dh)/2,
		r.Min.X+(r.Dx()-dw)/2+dw, r.Min.Y+(r.Dy()-dh)/2+dh,
	)
	xdraw.ApproxBiLinear.Scale(img, dr, src, src.Bounds(), xdraw.Over, nil)
}

// windowFrame wraps a terminal screenshot in a desktop window with a title bar
// and three traffic-light dots.
func windowFrame(inner image.Image, title string, fonts *assetFonts, accent color.RGBA) image.Image {
	const pad, titleH = 20, 40
	w := inner.Bounds().Dx() + 2*pad
	h := inner.Bounds().Dy() + 2*pad + titleH
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	fill(out, out.Bounds(), assetBG)
	roundRect(out, image.Rect(0, 0, w, h), 10, assetBorder)
	roundRect(out, image.Rect(1, 1, w-1, h-1), 9, assetPanel)
	fill(out, image.Rect(1, 1, w-1, titleH), color.RGBA{R: 0x23, G: 0x28, B: 0x34, A: 255})
	for i, c := range []color.RGBA{
		{R: 0xff, G: 0x5f, B: 0x57, A: 255},
		{R: 0xfe, G: 0xbc, B: 0x2e, A: 255},
		{R: 0x28, G: 0xc8, B: 0x40, A: 255},
	} {
		cx := 18 + i*20
		cy := titleH/2 - 5
		roundRect(out, image.Rect(cx, cy, cx+10, cy+10), 5, c)
	}
	tw := textWidth(fonts.small, title)
	drawText(out, fonts.small, (w-tw)/2, titleH/2+5, title, assetMuted)
	draw.Draw(out, image.Rect(pad, pad+titleH, pad+inner.Bounds().Dx(), pad+titleH+inner.Bounds().Dy()),
		inner, image.Point{}, draw.Src)
	return out
}

// ---------------------------------------------------------------------------
// Banner
// ---------------------------------------------------------------------------

func generateBanner(shotDir, outPath, fontPath string, fonts *assetFonts) error {
	tui, err := loadPNG(filepath.Join(shotDir, "58-top-demo.png"))
	if err != nil {
		return err
	}
	frame := windowFrame(tui, "mvctl top --demo --theme midnight --mode black", fonts, assetAccent)
	const W, H = 1600, 900
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	gradient(img, W, H, assetBG, assetBG2)

	// Accent hairline at the top.
	fill(img, image.Rect(0, 0, W, 4), assetAccent)

	drawText(img, fonts.title, 96, 132, "mvctl", assetText)
	// Accent dot after the wordmark.
	cx := 96 + textWidth(fonts.title, "mvctl") + 18
	roundRect(img, image.Rect(cx, 100, cx+14, 114), 7, assetAccent)
	drawText(img, fonts.body, 98, 176, "Control, inspect and show off millivolt from a single Go binary.", assetMuted)
	drawText(img, fonts.small, 98, 206, "zero runtime dependencies  ·  standard library only  ·  one static binary", assetCyan)

	// Feature pills on the right.
	pills := []struct {
		text string
		col  color.RGBA
	}{
		{"74 commands", assetAccent},
		{"8 themes", assetOrange},
		{"live TUI", assetCyan},
		{"--json", assetGreen},
	}
	px := W - 96
	for i := len(pills) - 1; i >= 0; i-- {
		p := pills[i]
		pw := textWidth(fonts.small, p.text) + 28
		px -= pw + 12
		roundRect(img, image.Rect(px, 96, px+pw, 130), 17, color.RGBA{R: 0x24, G: 0x2b, B: 0x38, A: 255})
		drawText(img, fonts.small, px+14, 118, p.text, p.col)
	}

	// Windowed TUI screenshot, centered.
	fw, fh := frame.Bounds().Dx(), frame.Bounds().Dy()
	scale := math.Min(float64(W-160)/float64(fw), float64(520)/float64(fh))
	dw, dh := int(float64(fw)*scale), int(float64(fh)*scale)
	dst := image.Rect((W-dw)/2, 250, (W-dw)/2+dw, 250+dh)
	// Soft shadow.
	roundRect(img, image.Rect(dst.Min.X+8, dst.Min.Y+10, dst.Max.X+8, dst.Max.Y+10), 12,
		color.RGBA{R: 0x04, G: 0x06, B: 0x0a, A: 255})
	xdraw.ApproxBiLinear.Scale(img, dst, frame, frame.Bounds(), xdraw.Over, nil)

	// Theme swatch strip.
	swatches := []color.RGBA{
		{0x7a, 0xa0, 0xff, 255}, {0xb8, 0xa8, 0xc8, 255}, {0x38, 0xbd, 0xf8, 255}, {0x88, 0xc0, 0xd0, 255},
		{0x34, 0xd3, 0x99, 255}, {0xf4, 0x72, 0xb6, 255}, {0xff, 0x66, 0x00, 255}, {0xff, 0xe1, 0x4d, 255},
	}
	labels := []string{"instrument", "slate", "midnight", "nord", "forest", "rose", "ember", "contrast"}
	sw := (W - 2*96) / len(swatches)
	for i, c := range swatches {
		x := 96 + i*sw
		fill(img, image.Rect(x, H-116, x+sw-10, H-84), c)
		drawText(img, fonts.small, x, H-62, labels[i], assetMuted)
	}
	drawText(img, fonts.small, 96, H-30, "millivolt's own palette registry  ·  black / clear modes", assetMuted)
	drawText(img, fonts.small, W-96-textWidth(fonts.small, "github.com/hv-V6/millivolt-cli"), H-30,
		"github.com/hv-V6/millivolt-cli", assetMuted)
	return savePNG(img, outPath)
}

// ---------------------------------------------------------------------------
// Theme matrix
// ---------------------------------------------------------------------------

func generateThemeMatrix(shotDir, outPath string, fonts *assetFonts) error {
	entries, err := os.ReadDir(shotDir)
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.Contains(e.Name(), "-theme-") &&
			strings.HasSuffix(e.Name(), ".png") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	// The first eight are the palette previews; "74-theme-clear" demonstrates
	// clear mode and is intentionally excluded from the matrix.
	if len(names) < 8 {
		return fmt.Errorf("found %d theme screenshots, need 8", len(names))
	}
	names = names[:8]

	const cols, rows = 4, 2
	const tileW, tileH = 420, 400
	const pad, header = 24, 92
	W := pad*2 + cols*tileW + (cols-1)*pad
	H := header + rows*tileH + (rows-1)*pad + pad
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	gradient(img, W, H, assetBG, assetBG2)
	drawText(img, fonts.heading, pad, 46, "Eight palettes, two modes", assetText)
	drawText(img, fonts.small, pad, 72, "Derived from millivolt's themes.css  ·  switch with --theme and --mode", assetMuted)

	for i, name := range names {
		col, row := i%cols, i/cols
		x := pad + col*(tileW+pad)
		y := header + row*(tileH+pad)
		roundRect(img, image.Rect(x, y, x+tileW, y+tileH), 10, assetPanel)
		src, err := loadPNG(filepath.Join(shotDir, name))
		if err != nil {
			return err
		}
		fit(img, image.Rect(x+6, y+6, x+tileW-6, y+tileH-46), src, color.RGBA{R: 0x11, G: 0x14, B: 0x1c, A: 255})
		label := strings.TrimSuffix(strings.TrimPrefix(name, "6"), ".png")
		// Strip the numeric prefix, e.g. "6-theme-midnight" -> "midnight".
		if idx := strings.LastIndex(label, "-theme-"); idx >= 0 {
			label = label[idx+len("-theme-"):]
		} else if idx := strings.Index(label, "-"); idx >= 0 {
			label = label[idx+1:]
		}
		drawText(img, fonts.body, x+14, y+tileH-16, label, assetAccent)
	}
	return savePNG(img, outPath)
}

// ---------------------------------------------------------------------------
// Command gallery
// ---------------------------------------------------------------------------

type galleryItem struct {
	file, caption string
}

var gallery = []galleryItem{
	{"03-status.png", "mvctl status"},
	{"32-chart.png", "mvctl chart"},
	{"33-explore.png", "mvctl explore"},
	{"44-doctor.png", "mvctl doctor"},
	{"34-cost.png", "mvctl cost"},
	{"06-requests.png", "mvctl requests 10"},
}

func generateGallery(shotDir, outPath string, fonts *assetFonts) error {
	const cols = 3
	const tileW, tileH = 520, 380
	const pad, header = 22, 86
	rows := (len(gallery) + cols - 1) / cols
	W := pad*2 + cols*tileW + (cols-1)*pad
	H := header + rows*tileH + (rows-1)*pad + pad
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	gradient(img, W, H, assetBG, assetBG2)
	drawText(img, fonts.heading, pad, 44, "Every command, captured", assetText)
	drawText(img, fonts.small, pad, 70, "Real ANSI output rendered as PNG  ·  74 verification screenshots in screenshots/", assetMuted)

	for i, item := range gallery {
		col, row := i%cols, i/cols
		x := pad + col*(tileW+pad)
		y := header + row*(tileH+pad)
		roundRect(img, image.Rect(x, y, x+tileW, y+tileH), 10, assetPanel)
		src, err := loadPNG(filepath.Join(shotDir, item.file))
		if err != nil {
			return err
		}
		fit(img, image.Rect(x+6, y+6, x+tileW-6, y+tileH-44), src, color.RGBA{R: 0x11, G: 0x14, B: 0x1c, A: 255})
		drawText(img, fonts.body, x+14, y+tileH-14, "$ "+item.caption, assetCyan)
	}
	return savePNG(img, outPath)
}

func generateAssets(shotDir, outDir, fontPath string) error {
	fonts, err := loadAssetFonts(fontPath)
	if err != nil {
		return err
	}
	if err := generateBanner(shotDir, filepath.Join(outDir, "banner.png"), fontPath, fonts); err != nil {
		return err
	}
	if err := generateThemeMatrix(shotDir, filepath.Join(outDir, "themes.png"), fonts); err != nil {
		return err
	}
	if err := generateGallery(shotDir, filepath.Join(outDir, "gallery.png"), fonts); err != nil {
		return err
	}
	return nil
}
