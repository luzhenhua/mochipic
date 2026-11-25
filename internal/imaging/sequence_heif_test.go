//go:build windows || heifsequenceqa

package imaging

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHEIFSequenceCompatibleDecoder(t *testing.T) {
	for _, test := range []struct {
		name                string
		format              Format
		width, height, loop int
		delays              []int
	}{
		{"timed.avis", AVIF, 96, 96, 0, []int{250, 250, 250}},
		{"timed.heics", HEIF, 64, 48, 1, []int{130, 260, 260}},
	} {
		t.Run(test.name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", test.name))
			if err != nil {
				t.Fatal(err)
			}
			for i, delay := range test.delays {
				meta, info, image, err := heifSequence(data, test.format, i+1, MaxInputPixels, true)
				if err != nil {
					t.Fatal(err)
				}
				image.Close()
				if info.Width != test.width || info.Height != test.height || meta.series.Count != len(test.delays) || meta.delay != delay || meta.loop != test.loop {
					t.Fatalf("frame %d decoded with %+v, %+v; expected %dx%d and %dms", i+1, info, meta, test.width, test.height, delay)
				}
			}
		})
	}
}
