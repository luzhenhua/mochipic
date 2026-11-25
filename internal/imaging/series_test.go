package imaging

import (
	"bytes"
	"encoding/binary"
	"image"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"imageflow/internal/l10n"
	"imageflow/internal/series"
	"imageflow/internal/testkit"
)

func TestTimedFormatsExportSelectedFrames(t *testing.T) {
	for _, tc := range []struct {
		name          string
		width, height int
		delays        []int
		loop          int
		partial       bool
	}{
		{"timed.gif", 48, 32, []int{70, 130, 220}, 3, true},
		{"timed.apng", 48, 32, []int{70, 130, 220}, 2, true},
		{"poster.apng", 48, 32, []int{70, 130, 220}, 2, true},
		{"timed.jxl", 48, 32, []int{70, 130, 220}, 3, true},
		{"timed.avis", 96, 96, []int{250, 250, 250}, 0, false},
		{"timed.heics", 64, 48, []int{130, 260, 260}, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join("testdata", tc.name)
			sha := testkit.Hash(t, path)
			info, err := InspectSource(t.Context(), path, FormatOf(path))
			if info != (series.Info{Kind: series.Frames, Count: 3}) || err != nil {
				t.Fatalf("inspect: %+v, %v", info, err)
			}
			for i, delay := range tc.delays {
				src, err := openImagePart(t.Context(), path, MaxInputBytes, MaxInputPixels, i+1)
				if err != nil {
					t.Fatal(err)
				}
				meta := src.frame
				src.close()
				if meta.series != info || meta.delay != delay || meta.loop != tc.loop {
					t.Fatalf("frame %d: timing %#v; expected %d ms, loop %d", i+1, meta, delay, tc.loop)
				}
				p, err := RenderPreviewBasePart(t.Context(), path, Resize{}, i+1)
				if err != nil || p.Size != image.Pt(tc.width, tc.height) {
					t.Fatalf("frame %d: %+v %v", i+1, p, err)
				}
				if tc.partial {
					left, right := rgb(p.Original, 7, 7), rgb(p.Original, 39, 7)
					if (i == 1 && left.G < 180 || i != 1 && left.R < 210) || (i == 2 && right.B < 210 || i != 2 && right.R < 210) {
						t.Fatalf("frame %d ignored disposal/composition or included the poster: %v %v", i+1, left, right)
					}
				}
			}
			if _, err := Export(t.Context(), path, Defaults(), io.Discard); !strings.Contains(l10n.ErrorText("en", err), l10n.T("en", "error.seriesScope")) {
				t.Fatalf("implicit single-frame export: %v", err)
			}
			opts := Defaults()
			opts.Format, opts.Lossless = PNG, true // Combined output must still be WebP.
			opts.Resize = Resize{Enabled: true, Mode: Exact, Fit: Stretch, Width: 32, Height: 24}
			var data bytes.Buffer
			if err := ExportSeries(t.Context(), path, opts, info, []int{1, 3}, &data); err != nil {
				t.Fatal(err)
			}
			loop, delays := readWebPTiming(t, data.Bytes())
			if loop != tc.loop || !slices.Equal(delays, []int{tc.delays[0], tc.delays[2]}) {
				t.Fatalf("output timeline: %v ms, loop %d", delays, loop)
			}
			output := filepath.Join(t.TempDir(), "combined.webp")
			if err := os.WriteFile(output, data.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			if got, err := InspectSource(t.Context(), output, WebP); err != nil || got != (series.Info{Kind: series.Frames, Count: 2}) {
				t.Fatalf("WebP reimport: %+v %v", got, err)
			}
			last, err := RenderPreviewBasePart(t.Context(), output, Resize{}, 2)
			if err != nil || last.Size != image.Pt(32, 24) || tc.partial && (rgb(last.Original, 4, 7).R < 210 || rgb(last.Original, 28, 7).B < 210) {
				t.Fatalf("last output frame: %+v %v", last, err)
			}
			if testkit.Hash(t, path) != sha {
				t.Fatal("source changed")
			}
		})
	}
}

func readWebPTiming(t *testing.T, data []byte) (int, []int) {
	t.Helper()
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		t.Fatal("not WebP")
	}
	loop := -1
	var delays []int
	for rest := data[12:]; len(rest) >= 8; {
		size := int(binary.LittleEndian.Uint32(rest[4:8]))
		if 8+size+size%2 > len(rest) {
			t.Fatal("truncated WebP chunk")
		}
		if string(rest[:4]) == "ANIM" && size >= 6 {
			loop = int(binary.LittleEndian.Uint16(rest[12:14]))
		}
		if string(rest[:4]) == "ANMF" && size >= 16 {
			delays = append(delays, int(rest[20])|int(rest[21])<<8|int(rest[22])<<16)
		}
		rest = rest[8+size+size%2:]
	}
	return loop, delays
}

func TestWebPMuxKeepsIdenticalFastFrames(t *testing.T) {
	path := filepath.Join("testdata", "still.gif")
	opts := Defaults()
	opts.Format = WebP
	var frame, combined bytes.Buffer
	if _, err := Export(t.Context(), path, opts, &frame); err != nil {
		t.Fatal(err)
	}
	a, err := newWebPAnimation(2)
	if err != nil {
		t.Fatal(err)
	}
	defer a.close()
	for _, ms := range []int{0, 8, 10} {
		if err := a.add(frame.Bytes(), ms); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.write(&combined); err != nil {
		t.Fatal(err)
	}
	if loop, delays := readWebPTiming(t, combined.Bytes()); loop != 2 || !slices.Equal(delays, []int{0, 8, 10}) {
		t.Fatalf("identical frames or their short delays were changed: %d %v", loop, delays)
	}
}

func TestAPNGAlphaBlendAndBackgroundDisposal(t *testing.T) {
	path := filepath.Join("testdata", "alpha-timed.apng")
	p, err := RenderPreviewBasePart(t.Context(), path, Resize{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	a, b := rgb(p.Original, 3, 3), rgb(p.Original, 12, 3)
	if a.A != 255 || a.R < 97 || a.R > 103 || a.B < 97 || a.B > 103 || b.A != 0 {
		t.Fatalf("second frame should blend over the first frame, excluding the opaque poster: %v %v", a, b)
	}
	info := series.Info{Kind: series.Frames, Count: 3}
	o := Defaults()
	o.Lossless = true
	var data bytes.Buffer
	if err := ExportSeries(t.Context(), path, o, info, []int{1, 2, 3}, &data); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "alpha.webp")
	if err := os.WriteFile(output, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err = RenderPreviewBasePart(t.Context(), output, Resize{}, 3)
	if err != nil {
		t.Fatal(err)
	}
	a, b = rgb(p.Original, 3, 3), rgb(p.Original, 12, 3)
	if a.A != 0 || b.A != 128 || b.G < 195 {
		t.Fatalf("third frame should clear the previous rectangle and keep the new alpha: %v %v", a, b)
	}
}

func TestTIFFBookPreservesSelectedPageSizesAndAlpha(t *testing.T) {
	path := filepath.Join("testdata", "different-pages.tiff")
	sha := testkit.Hash(t, path)
	info, err := InspectSource(t.Context(), path, TIFF)
	if err != nil || info != (series.Info{Kind: series.Pages, Count: 3}) {
		t.Fatalf("inspect: %+v %v", info, err)
	}
	if _, err := Export(t.Context(), path, Defaults(), io.Discard); !strings.Contains(l10n.ErrorText("en", err), l10n.T("en", "error.seriesScope")) {
		t.Fatalf("implicit first page export: %v", err)
	}
	out := filepath.Join(t.TempDir(), "selected.tiff")
	var data bytes.Buffer
	if err := ExportSeries(t.Context(), path, Defaults(), info, []int{2, 3}, &data); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := InspectSource(t.Context(), out, TIFF); err != nil || got != (series.Info{Kind: series.Pages, Count: 2}) {
		t.Fatalf("combined TIFF: %+v %v", got, err)
	}
	for i, want := range []image.Point{image.Pt(30, 54), image.Pt(64, 40)} {
		p, err := RenderPreviewBasePart(t.Context(), out, Resize{}, i+1)
		if err != nil || p.Size != want {
			t.Fatalf("page %d: %+v %v", i+1, p, err)
		}
		pixel := rgb(p.Original, 5, 8)
		if i == 0 && (pixel.A < 126 || pixel.A > 130 || pixel.G < 185 || rgb(p.Original, 24, 8).A != 0) || i == 1 && pixel.B < 215 {
			t.Fatalf("page %d lost color or alpha: %v", i+1, pixel)
		}
	}
	if testkit.Hash(t, path) != sha {
		t.Fatal("source changed")
	}
	if got, err := InspectSource(t.Context(), filepath.Join("testdata", "animated.jxl"), JXL); err != nil || got != (series.Info{Kind: series.Pages, Count: 2}) {
		t.Fatalf("untimed JXL collection: %+v %v", got, err)
	}
}
