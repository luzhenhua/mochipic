package imaging

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"imageflow/internal/caption"
	"imageflow/internal/certification"
	"imageflow/internal/testkit"
)

func TestCertificationPreviewAndFinalCanvasAgree(t *testing.T) {
	ctx := t.Context()
	photo, logo := filepath.Join(t.TempDir(), "photo.png"), filepath.Join(t.TempDir(), "logo.png")
	baseColor := color.NRGBA{R: 30, G: 75, B: 120, A: 255}
	logoColor := color.NRGBA{R: 230, G: 20, B: 40, A: 255}
	testkit.PNG(t, photo, testkit.Solid(600, 375, baseColor))
	mark := testkit.Solid(52, 22, logoColor)
	mark.SetNRGBA(0, 0, color.NRGBA{})
	testkit.PNG(t, logo, mark)
	original, originalLogo := testkit.Hash(t, photo), testkit.Hash(t, logo)

	art := certification.Builtins()
	cache := make(map[*certification.Badge]CertificationPreview, len(art))
	for _, badge := range art {
		p, err := RenderCertificationBadge(ctx, badge)
		if err != nil || p.Pixels == nil || p.Size != image.Pt(1024, 1024) {
			t.Fatalf("%s embedded artwork: %+v %v", badge.Name(), p, err)
		}
		if _, _, _, a := p.Pixels.At(0, 0).RGBA(); a != 0 {
			t.Fatalf("%s has an opaque corner", badge.Name())
		}
		cache[badge] = p
	}
	g := certification.Defaults()
	g.Toggle(art[0])
	g.Toggle(art[3])
	opts := Defaults()
	opts.Certification, opts.Format = g, PNG
	opts.Resize = Resize{Enabled: true, Mode: Exact, Fit: Stretch, Width: 1200, Height: 750}
	opts.Logo = LogoMark{Enabled: true, Path: logo, KeepRatio: true, Width: 10, Placement: Placement{Anchor: 6, MarginX: 16, MarginY: 16, Opacity: 100}}
	canvas := image.Pt(1200, 750)
	check := func(g certification.Group, first, second image.Rectangle) {
		t.Helper()
		layers, bounds, err := CertificationLayers(canvas, g, cache)
		if err != nil || len(layers) != 2 || bounds != first.Union(second) || layers[0].Bounds != first || layers[1].Bounds != second {
			t.Fatalf("preview layout: %+v %v %v", layers, bounds, err)
		}
		opts.Certification = g
		var buf bytes.Buffer
		if _, err := Export(ctx, photo, opts, &buf); err != nil {
			t.Fatal(err)
		}
		out, err := png.Decode(&buf)
		if err != nil || out.Bounds().Size() != canvas {
			t.Fatalf("wrong final size: %v", err)
		}
		for _, r := range []image.Rectangle{first, second} {
			if rgb(out, r.Min.X, r.Min.Y) != baseColor {
				t.Fatalf("PNG corners were filled at %v", r.Min)
			}
			p := r.Min.Add(r.Size().Div(2))
			if rgb(out, p.X, p.Y) == baseColor {
				t.Fatalf("missing mark at preview center %v", p)
			}
		}
		if rgb(out, first.Max.X+5, first.Min.Y+40) != baseColor || rgb(out, 50, 706) != logoColor {
			t.Fatal("gap or independent logo changed")
		}
	}
	check(g, image.Rect(878, 36, 986, 144), image.Rect(1020, 36, 1128, 144))
	g.MoveTo(image.Pt(100, 280), canvas, image.Pt(250, 108))
	check(g, image.Rect(100, 280, 208, 388), image.Rect(242, 280, 350, 388))
	if original != testkit.Hash(t, photo) || originalLogo != testkit.Hash(t, logo) {
		t.Fatal("source image or watermark was changed")
	}
}

func TestCertificationSeriesAndCustomBadge(t *testing.T) {
	ctx := t.Context()
	custom := filepath.Join(t.TempDir(), "Custom badge.png")
	markColor := color.NRGBA{R: 48, G: 205, B: 112, A: 255}
	testkit.PNG(t, custom, testkit.Solid(78, 39, markColor)) // An opaque, non-square PNG.
	hash := testkit.Hash(t, custom)
	data, sprite, err := PrepareCertificationImport(ctx, custom)
	if err != nil || sprite.Size != image.Pt(78, 39) {
		t.Fatalf("custom sprite: %+v %v", sprite, err)
	}
	library := certification.Library(t.TempDir())
	b, err := library.Save(custom, data)
	if err != nil {
		t.Fatal(err)
	}
	reused, err := library.Save("another name.png", data)
	if err != nil || reused.LocalPath() != b.LocalPath() {
		t.Fatalf("duplicate asset got a different file: %+v %v", reused, err)
	}
	saved, err := library.List()
	if err != nil || len(saved) != 1 || saved[0].Name() != "Custom badge" {
		t.Fatalf("persisted library: %v %v", saved, err)
	}

	g := certification.Defaults()
	g.Toggle(saved[0])
	g.Size = 20
	opts := Defaults()
	opts.Certification = g
	opts.Caption = caption.Caption{Text: "Pump\nQ5", Style: caption.Defaults()}
	opts.Caption.Style.Background, opts.Caption.Style.Opacity, opts.Caption.Style.Height = "#C02060", 100, 30
	opts.Resize = Resize{Enabled: true, Mode: Exact, Fit: Stretch, Width: 128, Height: 96}
	art := t.TempDir()
	var marks []LogoMark
	colors := []color.NRGBA{{R: 224, G: 10, A: 255}, {B: 228, A: 255}}
	for i, c := range colors {
		path := filepath.Join(art, []string{"brand.png", "dealer.png"}[i])
		im := testkit.Solid(16, 8, c)
		im.SetNRGBA(0, 0, color.NRGBA{})
		testkit.PNG(t, path, im)
		marks = append(marks, LogoMark{Enabled: true, Path: path, KeepRatio: true, Width: 16,
			Placement: Placement{Anchor: 6 + i*2, MarginX: 6, MarginY: 6, Opacity: 100}})
	}
	opts.SetLogoMarks(marks)
	for _, name := range []string{"timed.gif", "different-pages.tiff"} {
		t.Run(name, func(t *testing.T) {
			source := filepath.Join("testdata", name)
			original := testkit.Hash(t, source)
			format := FormatOf(source)
			series, err := InspectSource(ctx, source, format)
			if err != nil {
				t.Fatal(err)
			}
			ext := ".tiff"
			if format == GIF {
				ext = ".webp"
			}
			var output bytes.Buffer
			opts.Lossless = true
			if err := ExportSeries(ctx, source, opts, series, []int{1, series.Count}, &output); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(t.TempDir(), "result"+ext)
			if err := os.WriteFile(dest, output.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, part := range []int{1, 2} {
				got, err := RenderPreviewBasePart(ctx, dest, Resize{}, part)
				if err != nil || got.Size != image.Pt(128, 96) || rgb(got.Canvas, 90, 13) != markColor {
					t.Fatalf("badge missing or incorrectly sized on part %d: %v", part, err)
				}
				if rgb(got.Canvas, 14, 84) != colors[0] || rgb(got.Canvas, 114, 84) != colors[1] {
					t.Fatalf("one or both Logos are missing on part %d", part)
				}
				if gotColor := rgb(got.Canvas, 0, 95); gotColor != (color.NRGBA{R: 192, G: 32, B: 96, A: 255}) {
					t.Fatalf("title strip is missing on part %d: %v", part, gotColor)
				}
			}
			if testkit.Hash(t, source) != original {
				t.Fatal("series source was changed")
			}
		})
	}
	if hash != testkit.Hash(t, custom) {
		t.Fatal("custom source was changed")
	}
}
