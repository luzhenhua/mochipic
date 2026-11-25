package imaging

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"path/filepath"
	"testing"

	"imageflow/internal/testkit"
)

func previewFixture(t *testing.T) (string, Options) {
	t.Helper()
	dir := t.TempDir()
	src, logo := filepath.Join(dir, "source.png"), filepath.Join(dir, "logo.png")
	testkit.PNG(t, src, testkit.Solid(480, 300, color.NRGBA{R: 215, G: 225, B: 240, A: 255}))
	mark := testkit.Solid(64, 32, color.NRGBA{G: 155, B: 80, A: 255})
	mark.SetNRGBA(0, 0, color.NRGBA{})
	testkit.PNG(t, logo, mark)
	o := Defaults()
	o.Format = PNG
	o.Resize = Resize{Enabled: true, Mode: Exact, Fit: Contain, Width: 320, Height: 240, PadColor: "#F1F1F1"}
	o.Text = TextMark{Enabled: true, Text: "ImageFlow", Font: "Arial", Color: "#BB2525", Size: 32, Scale: 100,
		Placement: Placement{Anchor: 0, MarginX: 2, MarginY: 6, Opacity: 65}, Stroke: true, Shadow: true}
	o.Logo = LogoMark{Enabled: true, Path: logo, KeepRatio: true, Width: 20,
		Placement: Placement{Anchor: 0, MarginX: 4, MarginY: 5, Opacity: 56}}
	return src, o
}

func TestIncompleteWatermarksPreviewOtherEditsButPreventExport(t *testing.T) {
	src, opts := previewFixture(t)
	sha := testkit.Hash(t, src)
	for _, tc := range []struct {
		name string
		edit func(*Options)
		omit func(*Options)
		err  error
	}{
		{"blank text keeps Logo", func(o *Options) { o.Text.Text = "" }, func(o *Options) { o.Text.Enabled = false }, ErrTextWatermarkMissing},
		{"whitespace text keeps Logo", func(o *Options) { o.Text.Text = " 　" }, func(o *Options) { o.Text.Enabled = false }, ErrTextWatermarkMissing},
		{"missing Logo keeps text", func(o *Options) { o.Logo.Path = "" }, func(o *Options) { o.Logo.Enabled = false }, ErrLogoWatermarkMissing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			draft, want := opts, opts
			tc.edit(&draft)
			tc.omit(&want)
			p, err := RenderPreview(t.Context(), src, draft, true, true)
			if err != nil || p.Before == nil || p.After == nil || p.After.Bounds().Size() != image.Pt(320, 240) {
				t.Fatalf("draft preview missing original or resized output: %v, %v", p.Output, err)
			}
			data, _ := export(t, src, want)
			exact := pixels(t, data, PNG)
			for y := range 240 {
				for x := range 320 {
					if rgb(p.After, x, y) != rgb(exact, x, y) {
						t.Fatalf("draft changed another edit at %d,%d", x, y)
					}
				}
			}
			var output bytes.Buffer
			if _, err := Export(t.Context(), src, draft, &output); !errors.Is(err, tc.err) || output.Len() != 0 {
				t.Fatalf("export of unfinished watermark: %v, wrote %d bytes", err, output.Len())
			}
		})
	}
	if testkit.Hash(t, src) != sha {
		t.Fatal("source was modified")
	}
}

func TestLiveWatermarkLayersMatchPNGGeometryAndOrder(t *testing.T) {
	src, o := previewFixture(t)
	base, err := RenderPreviewBase(t.Context(), src, o.Resize)
	if err != nil {
		t.Fatal(err)
	}
	if base.Canvas.Bounds().Size() != image.Pt(320, 240) || base.Original.Bounds().Size() != image.Pt(480, 300) {
		t.Fatalf("wrong original or canvas dimensions: %v", base.Size)
	}
	text, err := RenderPreviewText(t.Context(), base.Size, o.Text)
	if err != nil {
		t.Fatal(err)
	}
	logo, err := RenderPreviewLogo(t.Context(), o.Logo.Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ logoOnTop, dragged bool }{{true, false}, {false, false}, {true, true}, {false, true}} {
		opts := o
		opts.LogoOnTop = tc.logoOnTop
		textAt, logoAt := image.Pt(72, 85), image.Pt(130, 99)
		if tc.dragged {
			opts.Text.MoveTo(textAt, base.Size, text.size)
			opts.Logo.MoveTo(logoAt, base.Size, logoSize(base.Size, logo.size, opts.Logo))
		}
		data, _ := export(t, src, opts)
		exact := pixels(t, data, PNG)
		layers := PreviewLayers(base.Size, text, map[string]LogoPreview{opts.Logo.Path: logo}, opts)
		if len(layers) != 7 {
			t.Fatalf("missing ink, stroke, shadow or Logo: %d layers", len(layers))
		}
		live := image.NewNRGBA(base.Canvas.Bounds())
		draw.Draw(live, live.Bounds(), base.Canvas, image.Point{}, draw.Src)
		for _, l := range layers {
			if tc.dragged && (l.Mark == TextWatermark && l.Bounds.Min != textAt || l.Mark == LogoWatermark && l.Bounds.Min != logoAt) {
				t.Fatalf("wrong dragged origin for mark %v: %v", l.Mark, l.Bounds)
			}
			mask := image.NewUniform(color.Alpha{A: uint8(l.Opacity * 255 / 100)})
			draw.DrawMask(live, l.Bounds, l.Pixels, l.Pixels.Bounds().Min, mask, image.Point{}, draw.Over)
		}
		for y := range 240 {
			for x := range 320 {
				a, b := rgb(live, x, y), rgb(exact, x, y)
				// Go and libvips round the 8-bit over operations differently.
				if abs(int(a.R)-int(b.R)) > 4 || abs(int(a.G)-int(b.G)) > 4 || abs(int(a.B)-int(b.B)) > 4 || a.A != b.A {
					t.Fatalf("%+v: live/export differ at %d,%d: %v vs %v", tc, x, y, a, b)
				}
			}
		}
	}
}

func TestFreeWatermarkPositionScalesAndClampsToCanvas(t *testing.T) {
	var p Placement
	p.MoveTo(image.Pt(73, 53), image.Pt(130, 90), image.Pt(30, 20))
	if at := p.Origin(image.Pt(230, 160), image.Pt(30, 20)); at != image.Pt(146, 106) {
		t.Fatalf("larger output lost its relative placement: %v", at)
	}
	p.MoveTo(image.Pt(-20, 200), image.Pt(130, 90), image.Pt(30, 20))
	if at := p.Origin(image.Pt(130, 90), image.Pt(30, 20)); at != image.Pt(0, 70) {
		t.Fatalf("mark moved past the image edge: %v", at)
	}
}
