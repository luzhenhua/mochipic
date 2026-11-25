package imaging

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"imageflow/internal/l10n"
	"imageflow/internal/series"
	"imageflow/internal/testkit"
)

func TestAdditionalInputFormats(t *testing.T) {
	for _, tc := range []struct {
		name   string
		format Format
		width  int
		height int
	}{
		{"apple.heic", HEIF, 48, 32},
		{"single.tiff", TIFF, 48, 32},
		{"raster.bmp", BMP, 48, 32},
		{"two-sizes.ico", ICO, 32, 32},
		{"still.gif", GIF, 48, 32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join("testdata", tc.name)
			sha := testkit.Hash(t, path)
			if err := InspectInput(t.Context(), path, FormatOf(path)); err != nil {
				t.Fatal(err)
			}
			o := Defaults()
			o.Format = AVIF
			p, err := RenderPreview(t.Context(), path, o, true, true)
			if err != nil || p.Before == nil || p.After == nil || p.Output.Bytes == 0 {
				t.Fatalf("preview: %+v, %v", p, err)
			}
			if p.Input.Format != tc.format || p.Input.Width != tc.width || p.Input.Height != tc.height ||
				p.After.Bounds().Dx() != tc.width || p.After.Bounds().Dy() != tc.height {
				t.Fatalf("incorrect input format or dimensions: %+v", p.Input)
			}
			if tc.format == ICO {
				// Largest representation, including partial alpha and a transparent half.
				a := rgb(p.Before, 5, 5)
				if a.A != 128 || a.B < 210 || rgb(p.After, 26, 5).A != 0 || !p.Input.Alpha {
					t.Fatalf("ICO pixels/alpha incorrect: %v", a)
				}
			} else if rgb(p.Before, 5, 5).R < 160 || rgb(p.Before, 5, 26).B < 160 {
				t.Fatal("colors or row orientation changed")
			}
			if testkit.Hash(t, path) != sha {
				t.Fatal("input was changed")
			}
		})
	}
}

func TestInputRejectsWrongExtension(t *testing.T) {
	if err := InspectInput(t.Context(), filepath.Join("testdata", "apple.heic"), AVIF); err == nil {
		t.Fatal("HEVC image accepted as AVIF")
	}
}

// The larger, independently encoded sample corpus is optional so contributor
// tests never download files or require a camera.
func TestExternalFormatCorpus(t *testing.T) {
	dir := os.Getenv("IMAGEFLOW_FORMAT_FIXTURES")
	if dir == "" {
		t.Skip("set IMAGEFLOW_FORMAT_FIXTURES to the local QA corpus")
	}
	for _, name := range []string{"photo.heic", "photo-alt.heif", "photo-16bit.tif", "photo.avif", "oriented.avif", "alpha.bmp", "rle.bmp", "favicon.ico", "48mp.heic"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name)
			sha := testkit.Hash(t, path)
			if err := InspectInput(t.Context(), path, FormatOf(path)); err != nil {
				t.Fatal(err)
			}
			o := Defaults()
			o.Format = AVIF
			o.Resize = Resize{Enabled: true, Mode: LongEdge, Limit: 720}
			p, err := RenderPreview(t.Context(), path, o, true, true)
			if err != nil || p.Before == nil || p.After == nil || p.Output.Bytes == 0 ||
				max(p.Output.Width, p.Output.Height) > 720 {
				t.Fatalf("preview: %+v, %v", p, err)
			}
			if name == "oriented.avif" && (p.Input.Width != 3024 || p.Input.Height != 4032) {
				t.Fatalf("orientation not preserved: %+v", p.Input)
			}
			if name == "48mp.heic" {
				if p.Input.Width != 8000 || p.Input.Height != 6000 {
					t.Fatalf("48 MP import dimensions: %+v", p.Input)
				}
				_, err := Export(t.Context(), path, Defaults(), io.Discard)
				if err == nil || !strings.Contains(l10n.ErrorText("en", err), "24 million pixels") {
					t.Fatalf("missing large export guidance: %v", err)
				}
			}
			var out bytes.Buffer
			if _, err := Export(t.Context(), path, o, &out); err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(out.Bytes()[:min(24, out.Len())], []byte("ftypavif")) || testkit.Hash(t, path) != sha {
				t.Fatal("incorrect AVIF container or input changed")
			}
			t.Logf("%d×%d %s → %d×%d AVIF (%d bytes)", p.Input.Width, p.Input.Height, p.Input.Format, p.Output.Width, p.Output.Height, out.Len())
		})
	}
	for _, name := range []string{"animated.gif", "animated.avif", "animated.png"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name)
			sha := testkit.Hash(t, path)
			info, err := InspectSource(t.Context(), path, FormatOf(name))
			if err != nil || info.Kind != series.Frames || info.Count < 2 {
				t.Fatalf("animation inspection: %+v, %v", info, err)
			}
			if err := ExportSeries(t.Context(), path, Defaults(), info, []int{1, info.Count}, io.Discard); err != nil {
				t.Fatal(err)
			}
			if testkit.Hash(t, path) != sha {
				t.Fatal("animation input changed")
			}
		})
	}
}

func TestCancelledInspectionDoesNotRead(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := InspectInput(ctx, "missing.gif", GIF); err != context.Canceled {
		t.Fatalf("expected cancellation before opening the file; got %v", err)
	}
}
