package imaging

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"path/filepath"
	"strings"
	"testing"

	"imageflow/internal/caption"
	"imageflow/internal/testkit"
)

func TestCaptionPreviewAndExportShareGeometry(t *testing.T) {
	tint := color.NRGBA{R: 180, G: 160, B: 140, A: 255}
	canvas := image.Pt(401, 301)
	path := filepath.Join(t.TempDir(), "product.png")
	base := testkit.Solid(canvas.X, canvas.Y, tint)
	testkit.PNG(t, path, base)
	hash := testkit.Hash(t, path)
	style := caption.Defaults()
	c := caption.Caption{Text: "Pump & Valve <Q5> / 水泵", Style: style}
	first, err := RenderPreviewCaption(t.Context(), canvas, c.Typography())
	if err != nil {
		t.Fatal(err)
	}
	wrap := c
	wrap.Text = strings.Repeat("Industrial pump selection guide ", 5)
	auto, err := RenderPreviewCaption(t.Context(), canvas, wrap.Typography())
	if err != nil || auto.layout.ink.Dy() < 2*first.layout.ink.Dy() || auto.layout.band.Dy() <= first.layout.band.Dy() {
		t.Fatalf("automatic wrapping should increase the strip height: %+v %v", auto.layout, err)
	}
	c.Text += "\nProduct specifications"
	preview, err := RenderPreviewCaption(t.Context(), canvas, c.Typography())
	if err != nil || preview.layout.ink.Dy() <= first.layout.ink.Dy() ||
		preview.layout.band.Min.X != 0 || preview.layout.band.Max != canvas || !preview.layout.ink.In(preview.layout.band) {
		t.Fatalf("manual newline or strip fit: %+v %v", preview.layout, err)
	}
	for _, opacity := range []int{0, 40, 100} {
		c.Style.Opacity = opacity
		layers := preview.Layers(canvas, c)
		count := 2
		if opacity == 0 {
			count = 1
		}
		if len(layers) != count || layers[len(layers)-1].Pixels != preview.ink {
			t.Fatal("opacity must affect the strip fill without altering the glyphs")
		}
		want := testkit.Solid(canvas.X, canvas.Y, tint)
		for _, l := range layers {
			im := l.Pixels
			if im == nil {
				im = image.NewUniform(l.Fill)
			}
			draw.Draw(want, l.Bounds, im, image.Point{}, draw.Over)
		}
		opts := Defaults()
		opts.Format, opts.Caption = PNG, c
		var buf bytes.Buffer
		if _, err := Export(t.Context(), path, opts, &buf); err != nil {
			t.Fatal(err)
		}
		got := pixels(t, buf.Bytes(), PNG)
		var white, different int
		for y := range canvas.Y {
			for x := range canvas.X {
				a, b := rgb(want, x, y), rgb(got, x, y)
				if max(delta(a.R, b.R), delta(a.G, b.G), delta(a.B, b.B), delta(a.A, b.A)) > 2 {
					different++
				}
				if b == (color.NRGBA{255, 255, 255, 255}) {
					white++
				}
			}
		}
		if different != 0 || white < 100 || rgb(got, 0, 0) != tint {
			t.Errorf("opacity %d: %d differing pixels, %d white glyph pixels, top %v", opacity, different, white, rgb(got, 0, 0))
		}
	}
	if testkit.Hash(t, path) != hash {
		t.Fatal("original modified")
	}
}

func TestCaptionFixedHeightFitsLandscapePortraitAndTinyCanvas(t *testing.T) {
	for _, sz := range []image.Point{{300, 181}, {199, 407}, {3, 3}} {
		for _, align := range []caption.Alignment{caption.Left, caption.Center, caption.Right} {
			c := caption.Caption{Text: "装配参数 & dimensions\n" + strings.Repeat("X", 40), Style: caption.Defaults()}
			c.Style.Height, c.Style.Size, c.Style.Alignment = 10, 12, align
			out, err := RenderPreviewCaption(t.Context(), sz, c.Typography())
			if err != nil || out.layout.band.Dy() != max(1, (sz.Y+5)/10) || !out.layout.ink.In(out.layout.band) || out.ink == nil {
				t.Fatalf("%v alignment %v: %+v %v", sz, align, out.layout, err)
			}
			if sz.X >= 100 && (align == caption.Left && out.layout.ink.Min.X != sz.X*5/100 ||
				align == caption.Right && out.layout.ink.Max.X != sz.X-sz.X*5/100) {
				t.Fatalf("%v alignment %v: %+v", sz, align, out.layout.ink)
			}
		}
	}
}

func delta(a, b uint8) int { return max(int(a)-int(b), int(b)-int(a)) }
