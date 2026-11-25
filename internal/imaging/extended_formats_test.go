package imaging

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"imageflow/internal/l10n"
	"imageflow/internal/testkit"
)

func TestExtendedStaticFormats(t *testing.T) {
	for _, tc := range []struct {
		name   string
		format Format
	}{
		{"still.jxl", JXL}, {"still.jp2", JP2}, {"still.j2k", JP2}, {"merged.psd", PSD},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join("testdata", tc.name)
			sha := testkit.Hash(t, path)
			if err := InspectInput(t.Context(), path, tc.format); err != nil {
				t.Fatal(err)
			}
			o := Defaults()
			o.Format = PNG
			p, err := RenderPreview(t.Context(), path, o, true, true)
			if err != nil || p.Before == nil || p.After == nil || p.Input.Format != tc.format {
				t.Fatalf("preview: %+v %v", p, err)
			}
			out, _ := export(t, path, o)
			im, err := png.Decode(bytes.NewReader(out))
			if err != nil || im.Bounds().Size() != image.Pt(48, 32) {
				t.Fatalf("export did not retain the image dimensions: %v", err)
			}
			if rgb(im, 5, 5).R < 160 || rgb(im, 5, 26).B < 160 || testkit.Hash(t, path) != sha {
				t.Fatal("export colors or row orientation changed, or input was modified")
			}
		})
	}
}

func TestExtendedCompositeAlpha(t *testing.T) {
	for _, name := range []string{"alpha.jxl", "alpha.psd"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("testdata", name)
			o := Defaults()
			o.Format = PNG
			b, info := export(t, path, o)
			im := pixels(t, b, PNG)
			partial := rgb(im, 5, 8)
			if !info.Alpha || partial.A < 126 || partial.A > 130 || partial.B < 210 ||
				rgb(im, 27, 8).A != 0 {
				t.Fatalf("merged pixels lost transparency: %+v %+v", info, partial)
			}
		})
	}
}

func TestSVGUsesExportDimensionsAndKeepsAlpha(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subpixel.svg")
	data := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">
  <rect x="4.125" y="2" width=".25" height="12" fill="#f00"/>
</svg>`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	sha := testkit.Hash(t, path)
	if err := InspectInput(t.Context(), path, SVG); err != nil {
		t.Fatal(err)
	}
	o := Defaults()
	o.Format = PNG
	o.Resize = Resize{Enabled: true, Mode: Percent, Percent: 400}
	p, err := RenderPreview(t.Context(), path, o, true, true)
	if err != nil || p.Before == nil || p.After == nil || p.Input.Width != 16 ||
		p.Before.Bounds().Size() != image.Pt(PreviewEdge, PreviewEdge) ||
		p.Output.Width != 64 || p.Output.Height != 64 || p.After.Bounds().Size() != image.Pt(64, 64) {
		t.Fatalf("incorrect SVG geometry: %+v %v", p, err)
	}
	// A line only 1/4 of a source pixel wide becomes a near-opaque pixel
	// when rasterized at 4×. Scaling a pre-rendered 16 px bitmap cannot.
	at := rgb(p.After, 16, 22)
	if at.A < 110 || at.R < 230 || rgb(p.After, 14, 22).A != 0 {
		t.Fatalf("SVG was not rasterized at the export scale: %+v", at)
	}
	b, _ := export(t, path, o)
	im, err := png.Decode(bytes.NewReader(b))
	if err != nil || rgb(im, 16, 22) != at || testkit.Hash(t, path) != sha {
		t.Fatalf("SVG export did not match the preview or input was modified: %v", err)
	}
	base, err := RenderPreviewBase(t.Context(), path, o.Resize)
	if err != nil || base.Size != image.Pt(64, 64) || rgb(base.Canvas, 16, 22) != at {
		t.Fatalf("SVG layer preview did not use the same raster geometry: %v", err)
	}
}

func TestSVGEmbeddedImageAndLocalReference(t *testing.T) {
	dir := t.TempDir()
	dependency := filepath.Join(dir, "other.png")
	testkit.PNG(t, dependency, testkit.Solid(8, 8, color.NRGBA{B: 240, A: 255}))
	raw, err := os.ReadFile(dependency)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "image.svg")
	// A local image next to the SVG should not be read. Embedded data can
	// render, proving the renderer is actually handling <image> elements.
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="32" height="8">
<image width="8" height="8" href="other.png"/>
<image x="8" width="8" height="8" href="file://` + dependency + `"/>
<image x="16" width="8" height="8" href="https://127.0.0.1:1/never-load.png"/>
<image x="24" width="8" height="8" href="data:image/png;base64,` + base64.StdEncoding.EncodeToString(raw) + `"/>
</svg>`
	if err := os.WriteFile(path, []byte(svg), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := RenderPreview(t.Context(), path, Defaults(), true, false)
	if err != nil || p.Before == nil {
		t.Fatalf("SVG preview: %v", err)
	}
	w, h := p.Before.Bounds().Dx(), p.Before.Bounds().Dy()
	for i := 0; i < 4; i++ {
		pixel := rgb(p.Before, (2*i+1)*w/8, h/2)
		if (i < 3 && pixel.A != 0) || (i == 3 && (pixel.B < 230 || pixel.A != 255)) {
			t.Fatalf("SVG external and embedded references were not isolated at tile %d: %+v", i, pixel)
		}
	}
}

func TestRejectsMissingCompositeAndFakeRaw(t *testing.T) {
	// A PSD with a real pixel payload but marked as having placeholder data.
	merged, err := os.ReadFile(filepath.Join("testdata", "merged.psd"))
	if err != nil {
		t.Fatal(err)
	}
	resourceAt := 30 + int(binary.BigEndian.Uint32(merged[26:30]))
	resourceLen := int(binary.BigEndian.Uint32(merged[resourceAt : resourceAt+4]))
	resource := []byte{'8', 'B', 'I', 'M', 0x04, 0x21, 0, 0, 0, 0, 0, 5, 0, 0, 0, 1, 0, 0}
	data := append(bytes.Clone(merged[:resourceAt+4]), resource...)
	data = append(data, merged[resourceAt+4:]...)
	binary.BigEndian.PutUint32(data[resourceAt:resourceAt+4], uint32(resourceLen+len(resource)))
	path := filepath.Join(t.TempDir(), "placeholder.psd")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := InspectInput(t.Context(), path, PSD); err == nil ||
		l10n.ErrorText("en", err) != l10n.T("en", "error.psdMissingComposite") {
		t.Fatalf("PSD without merged data needs specific feedback: %v", err)
	}
	wide := bytes.Clone(merged[:26])
	binary.BigEndian.PutUint16(wide[12:14], 56)
	binary.BigEndian.PutUint32(wide[14:18], 5000)
	binary.BigEndian.PutUint32(wide[18:22], 10000)
	if err := inspectPSD(bytes.NewReader(wide), int64(len(wide))); err == nil ||
		l10n.ErrorText("en", err) != l10n.Format("en", "error.decodedImageLimit", l10n.Args{"megabytes": 512}) {
		t.Fatalf("large PSD composite did not get a bounded decode error: %v", err)
	}
	tif, err := os.ReadFile(filepath.Join("testdata", "single.tiff"))
	if err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(t.TempDir(), "fake.nef")
	if err := os.WriteFile(fake, tif, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := InspectInput(t.Context(), fake, RAW); err == nil {
		t.Fatal("a normal TIFF disguised as a RAW was accepted")
	}
}

func TestCameraRawCorpus(t *testing.T) {
	dir := os.Getenv("IMAGEFLOW_RAW_FIXTURES")
	if dir == "" {
		t.Skip("see scripts/raw-corpus.sh for the local camera samples")
	}
	// LibRaw retains different sensor margins from Core Image. Check the
	// developed dimensions and orientation for the decoder each build uses.
	for _, tc := range []struct {
		name          string
		coreImageSize image.Point
		libRawSize    image.Point
	}{
		{"canon-5d3.cr2", image.Pt(5760, 3840), image.Pt(5796, 3870)},
		{"canon-r6.cr3", image.Pt(5472, 3648), image.Pt(5496, 3670)},
		{"nikon-d750.nef", image.Pt(6016, 4016), image.Pt(6032, 4032)},
		{"sony-a7m3.arw", image.Pt(6000, 4000), image.Pt(6024, 4024)},
		{"fuji-xt3.raf", image.Pt(6240, 4160), image.Pt(6246, 4170)},
		{"panasonic-g9.rw2", image.Pt(5202, 3900), image.Pt(5220, 3912)},
		{"olympus-em1ii.orf", image.Pt(5184, 3888), image.Pt(5240, 3912)},
		{"pentax-k5.pef", image.Pt(4928, 3264), image.Pt(4950, 3284)},
		{"iphone12pro.dng", image.Pt(3024, 4032), image.Pt(3024, 4032)}, // EXIF orientation 6.
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name)
			sha := testkit.Hash(t, path)
			if err := InspectInput(t.Context(), path, RAW); err != nil {
				t.Fatal(err)
			}
			o := Defaults()
			o.Resize = Resize{Enabled: true, Mode: LongEdge, Limit: 720}
			p, err := RenderPreview(t.Context(), path, o, true, true)
			inputPixels := image.Pt(p.Input.Width, p.Input.Height)
			// Core Image can crop the G9 sensor margins differently across
			// decoder builds. Both sizes represent the full developed image.
			sizeOK := inputPixels == tc.coreImageSize || tc.name == "panasonic-g9.rw2" && inputPixels == image.Pt(5184, 3888)
			if runtime.GOOS == "windows" {
				sizeOK = inputPixels == tc.libRawSize
			}
			if err != nil || p.Before == nil || p.After == nil || p.Input.Format != RAW || !sizeOK ||
				max(p.Output.Width, p.Output.Height) != 720 || p.Output.Bytes == 0 {
				t.Fatalf("RAW preview or camera orientation: %+v %v", p, err)
			}
			// Reject a blank render even if a native decoder returned a buffer.
			bounds := p.After.Bounds()
			first := rgb(p.After, bounds.Dx()/5, bounds.Dy()/5)
			changed := false
			for y := 1; y < 5; y++ {
				for x := 1; x < 5; x++ {
					c := rgb(p.After, bounds.Dx()*x/5, bounds.Dy()*y/5)
					changed = changed || c != first
				}
			}
			out, info := export(t, path, o)
			if !changed || pixels(t, out, WebP).Bounds() != p.After.Bounds() ||
				info.Width != p.Output.Width || info.Height != p.Output.Height || testkit.Hash(t, path) != sha {
				t.Fatal("RAW exported blank pixels, incorrect dimensions, or modified the source")
			}
			t.Logf("RAW %s → %d×%d WebP; SHA-256 unchanged", inputPixels, info.Width, info.Height)
		})
	}
}

func TestUpstreamAdvancedSamples(t *testing.T) {
	dir := os.Getenv("IMAGEFLOW_ADVANCED_FIXTURES")
	if dir == "" {
		t.Skip("see scripts/format-corpus.sh for the pinned decoder samples")
	}
	for _, name := range []string{"jxl-isobmff.jxl", "jxl-8bit-grey-icc-dot-gain.jxl", "jp2k-orientation-6.jp2", "psd.example.psd"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name)
			sha := testkit.Hash(t, path)
			if err := InspectInput(t.Context(), path, FormatOf(path)); err != nil {
				t.Fatal(err)
			}
			o := Defaults()
			o.Resize = Resize{Enabled: true, Mode: LongEdge, Limit: 720}
			p, err := RenderPreview(t.Context(), path, o, true, true)
			if err != nil || p.Before == nil || p.After == nil || p.Output.Bytes == 0 {
				t.Fatalf("actual sample preview: %+v %v", p, err)
			}
			if name == "jp2k-orientation-6.jp2" && (p.Input.Width != 3024 || p.Input.Height != 4032) {
				t.Fatalf("incorrect JPEG 2000 orientation: %+v", p.Input)
			}
			b, info := export(t, path, o)
			if pixels(t, b, WebP).Bounds().Size() != image.Pt(info.Width, info.Height) || testkit.Hash(t, path) != sha {
				t.Fatal("bad export dimensions or source changed")
			}
			t.Logf("%d×%d %s → %d×%d WebP", p.Input.Width, p.Input.Height, p.Input.Format, info.Width, info.Height)
		})
	}
}
