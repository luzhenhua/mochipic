package imaging

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"imageflow/internal/testkit"
)

func BenchmarkOriginalPreview(b *testing.B) {
	for _, tc := range []struct {
		name, ext string
		w, h      int
	}{
		{"720px_PNG", ".png", 540, 720},
		{"12MP_JPEG", ".jpg", 4000, 3000},
	} {
		b.Run(tc.name, func(b *testing.B) {
			path := filepath.Join(b.TempDir(), "photo"+tc.ext)
			im := image.NewNRGBA(image.Rect(0, 0, tc.w, tc.h))
			for y := range tc.h {
				for x := range tc.w {
					im.SetNRGBA(x, y, color.NRGBA{R: uint8(x/3 + y/7), G: uint8(x/9 + y/2), B: uint8((x+y)/5 + x*y/32768), A: 255})
				}
			}
			if tc.ext == ".png" {
				testkit.PNG(b, path, im)
			} else {
				f, err := os.Create(path)
				if err != nil {
					b.Fatal(err)
				}
				encodeErr := jpeg.Encode(f, im, &jpeg.Options{Quality: 92})
				if err := f.Close(); err != nil {
					b.Fatal(err)
				}
				if encodeErr != nil {
					b.Fatal(encodeErr)
				}
			}
			b.ReportAllocs()
			for b.Loop() {
				got, err := RenderPreviewBase(b.Context(), path, Resize{})
				if err != nil || got.Original == nil {
					b.Fatalf("preview failed: %v", err)
				}
			}
		})
	}
}
