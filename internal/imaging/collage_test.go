package imaging

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"path/filepath"
	"testing"

	"imageflow/internal/crop"
	"imageflow/internal/testkit"
)

func TestCollageExportsCroppedTilesWithAlphaAndFinalWatermark(t *testing.T) {
	dir := t.TempDir()
	red, blue, gold := color.NRGBA{R: 230, A: 255}, color.NRGBA{B: 210, A: 255}, color.NRGBA{R: 240, G: 180, A: 255}
	green, matte := color.NRGBA{G: 210, A: 255}, color.NRGBA{R: 24, G: 40, B: 56, A: 255}
	files := []string{"color.png", "portrait.png", "alpha.png"}
	wide := testkit.Solid(60, 30, red)
	draw.Draw(wide, image.Rect(30, 0, 60, 30), image.NewUniform(blue), image.Point{}, draw.Src)
	alpha := testkit.Solid(30, 30, color.NRGBA{})
	draw.Draw(alpha, image.Rect(15, 0, 30, 30), image.NewUniform(gold), image.Point{}, draw.Src)
	hashes := make([][32]byte, len(files))
	var inputs []CollageInput
	for i, im := range []image.Image{wide, testkit.Solid(14, 50, red), alpha} {
		path := filepath.Join(dir, files[i])
		testkit.PNG(t, path, im)
		hashes[i] = testkit.Hash(t, path)
		inputs = append(inputs, CollageInput{Path: path})
	}
	inputs[0].Crop = crop.Region{Box: image.Rect(30, 0, 60, 30), Reference: image.Pt(60, 30), Ratio: crop.Square}
	tile, err := LoadCollageTile(t.Context(), inputs[0])
	if err != nil || tile.Size != image.Pt(30, 30) || rgb(tile.Image, 5, 5) != blue {
		t.Fatalf("preview did not use crop: %+v %v", tile.Size, err)
	}
	logo := filepath.Join(dir, "logo.png")
	mark := testkit.Solid(10, 10, green)
	mark.SetNRGBA(0, 0, color.NRGBA{})
	testkit.PNG(t, logo, mark)
	opts := Defaults()
	opts.Format = PNG
	opts.Resize = Resize{Enabled: true, Mode: LongEdge, Limit: 40}
	opts.Logo = LogoMark{Enabled: true, Path: logo, KeepRatio: true, Width: 9, Placement: Placement{Anchor: 8, MarginX: 3, MarginY: 3, Opacity: 100}}
	second := filepath.Join(dir, "dealer.png")
	mark = testkit.Solid(10, 10, gold)
	mark.SetNRGBA(0, 0, color.NRGBA{})
	testkit.PNG(t, second, mark)
	opts.SetLogoMarks(append(opts.LogoMarks(), LogoMark{Enabled: true, Path: second, KeepRatio: true, Width: 9, Placement: Placement{Anchor: 8, MarginX: 12, MarginY: 3, Opacity: 100}}))
	layout := CollageOptions{Width: 199, Height: 127, Gap: 8, Layout: CollageGrid, Fit: Contain, Background: "#182838"}
	for _, fit := range []Fit{Contain, Cover} {
		layout.Fit = fit
		var out bytes.Buffer
		info, err := ExportCollage(t.Context(), inputs, layout, opts, &out)
		if err != nil || info.Width != 199 || info.Height != 127 {
			t.Fatalf("%v collage: %+v, %v", fit, info, err)
		}
		got := pixels(t, out.Bytes(), PNG)
		// First row: cropped blue square, gutter, red portrait. Second
		// row: translucent tile, empty fourth cell, two overlapping watermarks.
		want := map[image.Point]color.NRGBA{
			image.Pt(51, 33): blue, image.Pt(99, 33): matte, image.Pt(147, 33): red,
			image.Pt(32, 94): matte, image.Pt(71, 94): gold, image.Pt(147, 94): matte,
			image.Pt(188, 116): green, image.Pt(182, 116): gold,
		}
		want[image.Pt(14, 33)] = matte
		if fit == Cover {
			want[image.Pt(14, 33)] = blue
		}
		for pt, color := range want {
			if p := rgb(got, pt.X, pt.Y); p != color {
				t.Errorf("%v at %v: got %v, want %v", fit, pt, p, color)
			}
		}
		for i, s := range inputs {
			if testkit.Hash(t, s.Path) != hashes[i] {
				t.Fatalf("original changed: %s", s.Path)
			}
		}
	}
	layout.Width = 10
	var out bytes.Buffer
	if _, err := ExportCollage(t.Context(), inputs, layout, opts, &out); err == nil || out.Len() != 0 {
		t.Fatalf("an impossible gap was encoded: %v", err)
	}
}

func TestCustomCollageOverlapAndCropPixels(t *testing.T) {
	dir := t.TempDir()
	red := color.NRGBA{R: 210, A: 255}
	blue := color.NRGBA{B: 200, A: 255}
	gold := color.NRGBA{R: 220, G: 160, A: 255}
	back := testkit.Solid(80, 60, red)
	front := testkit.Solid(80, 60, blue)
	draw.Draw(front, image.Rect(40, 0, 80, 60), image.NewUniform(gold), image.Point{}, draw.Src)
	draw.Draw(front, image.Rect(20, 20, 30, 40), image.Transparent, image.Point{}, draw.Src)
	inputs := []CollageInput{{Path: filepath.Join(dir, "back.png")}, {Path: filepath.Join(dir, "front.png")}}
	testkit.PNG(t, inputs[0].Path, back)
	testkit.PNG(t, inputs[1].Path, front)
	hashes := [][32]byte{testkit.Hash(t, inputs[0].Path), testkit.Hash(t, inputs[1].Path)}
	o := DefaultCollage()
	o.Width, o.Height, o.Layout = 80, 60, CollageCustom
	box := back.Bounds()
	for _, input := range inputs {
		o.PutPhoto(NewCollagePhoto(input.Path, box.Size(), box.Size(), CollagePlacement{box, box}))
	}
	opts := Defaults()
	opts.Format = PNG
	for _, corner := range []CollageCorner{CornerRound, CornerBevel} {
		p, _ := o.Photo(inputs[1].Path)
		p.Visible = image.Rect(8, 6, 72, 54)
		p.Corner, p.Radius = corner, 10
		o.PutPhoto(p)
		var buf bytes.Buffer
		if _, err := ExportCollage(t.Context(), inputs, o, opts, &buf); err != nil {
			t.Fatal(err)
		}
		im := pixels(t, buf.Bytes(), PNG)
		for pt, want := range map[image.Point]color.NRGBA{{8, 6}: red, {20, 30}: red, {35, 30}: blue, {40, 30}: gold, {73, 30}: red, {0, 0}: red} {
			if got := rgb(im, pt.X, pt.Y); got != want {
				t.Fatalf("corner %d point %v: %v != %v", corner, pt, got, want)
			}
		}
		want := blue
		if corner == CornerBevel {
			want = red
		}
		if got := rgb(im, 11, 9); got != want {
			t.Fatalf("expected different round and bevel contours: %v", got)
		}
	}
	for i, input := range inputs {
		if testkit.Hash(t, input.Path) != hashes[i] {
			t.Fatal("source was modified")
		}
	}
}
