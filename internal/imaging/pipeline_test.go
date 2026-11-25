package imaging

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"imageflow/internal/testkit"

	"github.com/davidbyttow/govips/v2/vips"
)

func TestMain(m *testing.M) {
	if err := Start(); err != nil {
		panic(err)
	}
	code := m.Run()
	Shutdown()
	os.Exit(code)
}

func export(t *testing.T, path string, o Options) ([]byte, Info) {
	t.Helper()
	var b bytes.Buffer
	i, e := Export(t.Context(), path, o, &b)
	if e != nil {
		t.Fatal(e)
	}
	return b.Bytes(), i
}
func pixels(t *testing.T, buf []byte, f Format) image.Image {
	t.Helper()
	s, err := loadBuffer(buf, f, MaxPixels)
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	im, err := displayPixels(s.ImageRef, PreviewEdge)
	if err != nil {
		t.Fatal(err)
	}
	return im
}
func rgb(im image.Image, x, y int) color.NRGBA {
	return color.NRGBAModel.Convert(im.At(x, y)).(color.NRGBA)
}

func TestFormatsRoundTripAndFlatten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "原图.png")
	im := testkit.Solid(47, 31, color.NRGBA{R: 80, G: 30, B: 10, A: 0})
	for y := 0; y < 31; y++ {
		for x := 22; x < 47; x++ {
			im.SetNRGBA(x, y, color.NRGBA{R: 230, G: 50, B: 30, A: 255})
		}
	}
	testkit.PNG(t, path, im)
	sha := testkit.Hash(t, path)
	for _, f := range OutputFormats() {
		t.Run(string(f), func(t *testing.T) {
			o := Defaults()
			o.Format, o.Background, o.Lossless = f, "#18A04A", true
			b, info := export(t, path, o)
			if info.Width != 47 || info.Height != 31 || info.Format != f {
				t.Fatal(info)
			}
			out := pixels(t, b, f)
			p := rgb(out, 4, 8)
			if f == JPEG {
				if p.A != 255 || abs(int(p.R)-24) > 5 || abs(int(p.G)-160) > 5 || abs(int(p.B)-74) > 5 {
					t.Fatalf("JPG flatten: %+v", p)
				}
			} else if p.A != 0 {
				t.Fatalf("alpha lost: %+v", p)
			}
			round := filepath.Join(dir, "round"+f.Extension())
			if err := os.WriteFile(round, b, 0o600); err != nil {
				t.Fatal(err)
			}
			o.Format = PNG
			_, info = export(t, round, o)
			if info.Width != 47 || info.Height != 31 {
				t.Fatal(info)
			}
		})
	}
	o := Defaults()
	o.Quality = 19
	b, _ := export(t, path, o)
	out := pixels(t, b, WebP)
	if rgb(out, 4, 8).A != 0 || rgb(out, 40, 8).A != 255 {
		t.Fatal("lossy WebP alpha")
	}
	if testkit.Hash(t, path) != sha {
		t.Fatal("source changed")
	}
}

func TestGeometryAndAlpha(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "odd.png")
	testkit.PNG(t, p, testkit.Solid(97, 59, color.NRGBA{R: 210, G: 12, B: 7, A: 255}))
	for _, tc := range []struct {
		name                       string
		mode                       ResizeMode
		fit                        Fit
		w, h, expectedW, expectedH int
	}{
		{"contain", Exact, Contain, 61, 61, 61, 61}, {"cover", Exact, Cover, 61, 61, 61, 61},
		{"stretch", Exact, Stretch, 22, 53, 22, 53}, {"percent", Percent, Contain, 25, 0, 24, 15},
		{"long", LongEdge, Contain, 31, 0, 31, 19}, {"no upsample", LongEdge, Contain, 120, 0, 97, 59},
		{"one pixel", Exact, Contain, 1, 1, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := Defaults()
			o.Format = PNG
			o.Resize = Resize{Enabled: true, Mode: tc.mode, Fit: tc.fit, Width: tc.w, Height: tc.h, Percent: tc.w, Limit: tc.w, Transparent: true}
			b, info := export(t, p, o)
			if info.Width != tc.expectedW || info.Height != tc.expectedH {
				t.Fatalf("wrong final geometry: %+v", info)
			}
			im := pixels(t, b, PNG)
			wantTransparent := tc.name == "contain"
			if (rgb(im, 0, 0).A == 0) != wantTransparent || rgb(im, info.Width/2, info.Height/2).R < 205 {
				t.Fatalf("geometry or alpha incorrect, corner=%+v center=%+v", rgb(im, 0, 0), rgb(im, info.Width/2, info.Height/2))
			}
		})
	}
	r := Resize{Enabled: true, Mode: Exact, Fit: Cover, Width: 4000, Height: 4000}
	if _, err := r.Geometry(12000, 1); err == nil {
		t.Fatal("unsafe intermediate accepted")
	}
	r.Height = 0
	if _, err := r.Geometry(20, 31); err == nil {
		t.Fatal("zero height accepted")
	}
	r = Resize{Enabled: true, Mode: Exact, Fit: Contain, Width: 64, Height: 64}
	if g, e := r.Geometry(59, 97); e != nil || g.X <= 0 || g.Y != 0 {
		t.Fatalf("portrait: %+v %v", g, e)
	}
}

func TestEXIFOrientation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rotated.jpeg")
	var b bytes.Buffer
	src, _ := vips.NewImageFromGoImage(testkit.Solid(35, 19, color.NRGBA{R: 80, A: 255}))
	if src == nil {
		t.Fatal("failed to generate source")
	}
	if err := src.SaveToWriterJpeg(&b, nil); err != nil {
		t.Fatal(err)
	}
	src.Close()
	// APP1 Exif, big-endian TIFF, one Orientation (short) tag set to 6.
	exif := []byte{0xff, 0xe1, 0, 34, 'E', 'x', 'i', 'f', 0, 0, 'M', 'M', 0, 42, 0, 0, 0, 8, 0, 1, 0x01, 0x12, 0, 3, 0, 0, 0, 1, 0, 6, 0, 0, 0, 0, 0, 0}
	tagged := append(append(append([]byte{}, b.Bytes()[:2]...), exif...), b.Bytes()[2:]...)
	if err := os.WriteFile(path, tagged, 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := RenderPreview(t.Context(), path, Defaults(), true, true)
	if err != nil {
		t.Fatal(err)
	}
	if p.Input.Width != 19 || p.Input.Height != 35 || p.Output.Width != 19 || p.Output.Height != 35 {
		t.Fatalf("orientation was not baked in: %+v", p)
	}
}

func TestConvertEmbeddedICC(t *testing.T) {
	profile := "/System/Library/ColorSync/Profiles/Display P3.icc"
	if _, err := os.Stat(profile); err != nil {
		t.Skipf("macOS Display P3 fixture unavailable: %v", err)
	}
	mark := testkit.Solid(20, 17, color.NRGBA{R: 190, G: 95, B: 40, A: 255})
	profileBytes, err := os.ReadFile(profile)
	if err != nil {
		t.Fatal(err)
	}
	var raw, compressed bytes.Buffer
	if err = png.Encode(&raw, mark); err != nil {
		t.Fatal(err)
	}
	zw := zlib.NewWriter(&compressed)
	if _, err = zw.Write(profileBytes); err != nil {
		t.Fatal(err)
	}
	if err = zw.Close(); err != nil {
		t.Fatal(err)
	}
	content := append([]byte("iCCPDisplay P3\x00\x00"), compressed.Bytes()...)
	b := append([]byte{}, raw.Bytes()[:33]...)
	b = binary.BigEndian.AppendUint32(b, uint32(len(content)-4))
	b = append(b, content...)
	b = binary.BigEndian.AppendUint32(b, crc32.ChecksumIEEE(content))
	b = append(b, raw.Bytes()[33:]...)
	path := filepath.Join(t.TempDir(), "tagged.png")
	if err = os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	tagged, err := loadBuffer(b, PNG, MaxPixels)
	if err != nil {
		t.Fatal(err)
	}
	inICC := tagged.GetICCProfile()
	tagged.close()
	if len(inICC) == 0 {
		t.Fatal("P3 profile not embedded in fixture")
	}
	o := Defaults()
	o.Format = PNG
	b, _ = export(t, path, o)
	s, err := loadBuffer(b, PNG, MaxPixels)
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	if outputICC := s.GetICCProfile(); len(outputICC) == 0 || bytes.Equal(inICC, outputICC) {
		t.Fatal("sRGB profile absent or P3 profile unchanged")
	}
	pt, err := s.GetPoint(5, 5)
	if err != nil || len(pt) < 3 || pt[0] <= 194 || pt[1] >= 95 {
		t.Fatalf("P3 was not transformed: %v %v", pt, err)
	}
}

func TestDualWatermarkPositionsAndPreview(t *testing.T) {
	dir := t.TempDir()
	src, logo := filepath.Join(dir, "source.png"), filepath.Join(dir, "logo.png")
	testkit.PNG(t, src, testkit.Solid(183, 104, color.NRGBA{R: 255, G: 255, B: 255, A: 255}))
	l := testkit.Solid(20, 10, color.NRGBA{G: 255, A: 255})
	for y := 0; y < 10; y++ {
		l.SetNRGBA(0, y, color.NRGBA{})
	}
	testkit.PNG(t, logo, l)
	o := Defaults()
	o.Format = PNG
	o.Resize = Resize{Enabled: true, Mode: Exact, Fit: Stretch, Width: 100, Height: 80}
	o.Logo = LogoMark{Enabled: true, Path: logo, KeepRatio: true, Width: 20, Placement: Placement{MarginX: 4, MarginY: 5, Opacity: 50}}
	o.Text = TextMark{Enabled: true, Text: "检验 <A&>", Font: "PingFang SC", Color: "#D50000", Size: 14, Scale: 100, Placement: Placement{Anchor: 8, MarginX: 6, MarginY: 7, Opacity: 70}}
	data, info := export(t, src, o)
	out := pixels(t, data, PNG)
	if info.Width != 100 || info.Height != 80 {
		t.Fatal(info)
	}
	inside := rgb(out, 9, 8)
	if inside.G < 249 || inside.R < 125 || inside.R > 132 || rgb(out, 3, 8).R != 255 || rgb(out, 24, 8).R != 255 {
		t.Fatalf("logo position or opacity: %+v", inside)
	}
	reds := 0
	for y := 0; y < 80; y++ {
		for x := 0; x < 100; x++ {
			p := rgb(out, x, y)
			if int(p.R)-int(p.G) > 50 {
				reds++
				if y < 49 || y >= 73 || x >= 94 {
					t.Fatalf("text outside expected bottom-right bounds: %d,%d", x, y)
				}
			}
		}
	}
	if reds < 30 {
		t.Fatalf("text not rendered, %d non-white pixels", reds)
	}
	p, err := RenderPreview(t.Context(), src, o, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if p.Input.Width != 183 || p.Output.Width != 100 || p.Output.Bytes != int64(len(data)) {
		t.Fatal(p.Output)
	}
	for y := 0; y < 80; y++ {
		for x := 0; x < 100; x++ {
			if rgb(p.After, x, y) != rgb(out, x, y) {
				t.Fatalf("preview and actual output differ at %d,%d", x, y)
			}
		}
	}
}
func TestNineAnchors(t *testing.T) {
	want := [][2]int{{7, 9}, {45, 9}, {83, 9}, {7, 35}, {45, 35}, {83, 35}, {7, 61}, {45, 61}, {83, 61}}
	for i, v := range want {
		at := (Placement{Anchor: i, MarginX: 7, MarginY: 9, Opacity: 100}).Origin(image.Pt(100, 80), image.Pt(10, 10))
		if at != image.Pt(v[0], v[1]) {
			t.Fatalf("anchor %d: %v; want %v", i, at, v)
		}
	}
}
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
