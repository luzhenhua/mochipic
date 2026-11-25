package imaging

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"testing"

	"imageflow/internal/crop"
	"imageflow/internal/series"
	"imageflow/internal/testkit"
)

func TestCropBeforeResizeAndWatermarkMatchesPreview(t *testing.T) {
	in := filepath.Join(t.TempDir(), "input.png")
	red, blue := color.NRGBA{R: 220, A: 255}, color.NRGBA{B: 210, A: 255}
	photo := testkit.Solid(64, 32, red)
	draw.Draw(photo, image.Rect(32, 0, 64, 32), image.NewUniform(blue), image.Point{}, draw.Src)
	testkit.PNG(t, in, photo)
	hash := testkit.Hash(t, in)
	logo := filepath.Join(t.TempDir(), "logo.png")
	green := color.NRGBA{G: 210, A: 255}
	mark := testkit.Solid(8, 8, green)
	mark.SetNRGBA(0, 0, color.NRGBA{})
	testkit.PNG(t, logo, mark)
	o := Defaults()
	o.Format = PNG
	o.Crop = crop.Region{Box: image.Rect(33, 2, 62, 31), Reference: image.Pt(64, 32), Ratio: crop.Square}
	o.Resize = Resize{Enabled: true, Mode: Exact, Fit: Contain, Width: 72, Height: 72, PadColor: "#FFFFFF"}
	o.Logo = LogoMark{Enabled: true, Path: logo, KeepRatio: true, Width: 25, Placement: Placement{Anchor: 8, MarginX: 4, MarginY: 4, Opacity: 100}}
	buf, info := export(t, in, o)
	got := pixels(t, buf, PNG)
	if info.Width != 72 || info.Height != 72 || rgb(got, 5, 5) != blue || rgb(got, 63, 63) != green {
		t.Fatalf("crop → resize → mark produced %+v; pixels %v, %v", info, rgb(got, 5, 5), rgb(got, 63, 63))
	}
	p, err := RenderPreview(t.Context(), in, o, true, true)
	if err != nil || p.Before.Bounds().Size() != image.Pt(64, 32) || p.After.Bounds() != got.Bounds() {
		t.Fatalf("preview: %+v %v", p, err)
	}
	for y := range 72 {
		for x := range 72 {
			if rgb(p.After, x, y) != rgb(got, x, y) {
				t.Fatalf("preview and export differ at %d,%d", x, y)
			}
		}
	}
	if testkit.Hash(t, in) != hash {
		t.Fatal("source changed")
	}
}

func TestCropUsesSameBoxForSelectedAnimationFrames(t *testing.T) {
	in := filepath.Join("testdata", "timed.gif")
	s := series.Info{Kind: series.Frames, Count: 3}
	o := Defaults()
	o.Crop = crop.Region{Box: image.Rect(3, 4, 21, 22), Reference: image.Pt(48, 32), Ratio: crop.Square}
	var b bytes.Buffer
	if err := ExportSeries(t.Context(), in, o, s, []int{1, 3}, &b); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cropped.webp")
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, part := range []int{1, 2} {
		p, err := RenderPreviewBasePart(t.Context(), path, Resize{}, part)
		if err != nil || p.Input.Width != 18 || p.Input.Height != 18 {
			t.Fatalf("frame %d did not share the crop: %+v %v", part, p.Input, err)
		}
	}
}
